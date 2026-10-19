// Package inbox ingests event files that external producers (the future game console)
// drop into a shared folder. It polls the folder, decodes each file and feeds the
// events to the same pipeline used by the HTTP API.
//
// Concurrency model: one dispatcher goroutine scans the folder in filename order and
// splits each file by matchId. Every match is routed to a fixed worker (hash of the
// matchId), so the events of one match are always applied in order while different
// matches are processed in parallel by different goroutines.
package inbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Brxck2203/LenguajesProject1/go_service/internal/domain"
	"github.com/Brxck2203/LenguajesProject1/go_service/internal/processing"
)

const (
	ProcessedDir = "processed"
	RejectedDir  = "rejected"
	maxFileSize  = 16 << 20
)

type Ingestor interface {
	Add(domain.Event) (*domain.AnalysisRequest, error)
}

type Config struct {
	Dir          string
	PollInterval time.Duration
	Workers      int
	// StableFor skips files modified more recently, so a producer that writes in place
	// is not read halfway. Writing to *.tmp and renaming is still the recommended way.
	StableFor time.Duration
	// CapacityRetries bounds how long an event waits when the analysis queue is full.
	CapacityRetries int
	RetryDelay      time.Duration
}

// Stats is a snapshot of the watcher counters.
type Stats struct {
	Dir             string    `json:"dir"`
	FilesProcessed  int64     `json:"filesProcessed"`
	FilesRejected   int64     `json:"filesRejected"`
	EventsAccepted  int64     `json:"eventsAccepted"`
	FilesInProgress int       `json:"filesInProgress"`
	LastError       string    `json:"lastError,omitempty"`
	LastErrorFile   string    `json:"lastErrorFile,omitempty"`
	LastActivity    time.Time `json:"lastActivity,omitempty"`
}

// Failure describes the first error found for one match inside a file.
type Failure struct {
	MatchID       string `json:"matchId"`
	FailedAtIndex int    `json:"failedAtIndex"` // index of the event inside the file
	EventID       string `json:"eventId,omitempty"`
	Error         string `json:"error"`
}

type Report struct {
	File     string    `json:"file"`
	Accepted int       `json:"accepted"`
	Total    int       `json:"total"`
	Failures []Failure `json:"failures"`
}

type indexedEvent struct {
	index int
	event domain.Event
}

type fileState struct {
	name      string
	path      string
	total     int
	remaining atomic.Int32
	mu        sync.Mutex
	accepted  int
	failures  []Failure
}

type matchJob struct {
	file    *fileState
	matchID string
	events  []indexedEvent
}

type Watcher struct {
	config   Config
	ingest   Ingestor
	mu       sync.Mutex
	inFlight map[string]struct{}
	stats    Stats
	queues   []chan matchJob
}

func New(config Config, ingest Ingestor) (*Watcher, error) {
	if strings.TrimSpace(config.Dir) == "" || ingest == nil || config.Workers <= 0 || config.PollInterval <= 0 {
		return nil, fmt.Errorf("inbox requires a directory, an ingestor, workers and a poll interval")
	}
	if config.CapacityRetries <= 0 {
		config.CapacityRetries = 60
	}
	if config.RetryDelay <= 0 {
		config.RetryDelay = 500 * time.Millisecond
	}
	for _, dir := range []string{config.Dir, filepath.Join(config.Dir, ProcessedDir), filepath.Join(config.Dir, RejectedDir)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create inbox folder: %w", err)
		}
	}
	return &Watcher{config: config, ingest: ingest, inFlight: make(map[string]struct{}), stats: Stats{Dir: config.Dir}}, nil
}

// Run polls the folder until ctx is cancelled, then waits for the workers to finish.
func (watcher *Watcher) Run(ctx context.Context) {
	var workers sync.WaitGroup
	watcher.queues = make([]chan matchJob, watcher.config.Workers)
	for i := range watcher.queues {
		watcher.queues[i] = make(chan matchJob, 64)
		workers.Add(1)
		go func(queue <-chan matchJob) {
			defer workers.Done()
			for job := range queue {
				watcher.process(ctx, job)
			}
		}(watcher.queues[i])
	}
	log.Printf("inbox: watching %s with %d workers", watcher.config.Dir, watcher.config.Workers)
	ticker := time.NewTicker(watcher.config.PollInterval)
	defer ticker.Stop()
	for {
		watcher.scan(ctx)
		select {
		case <-ctx.Done():
			for _, queue := range watcher.queues {
				close(queue)
			}
			workers.Wait()
			log.Print("inbox: stopped")
			return
		case <-ticker.C:
		}
	}
}

func (watcher *Watcher) Stats() Stats {
	watcher.mu.Lock()
	defer watcher.mu.Unlock()
	stats := watcher.stats
	stats.FilesInProgress = len(watcher.inFlight)
	return stats
}

// scan dispatches every ready file in name order.
func (watcher *Watcher) scan(ctx context.Context) {
	entries, err := os.ReadDir(watcher.config.Dir)
	if err != nil {
		watcher.recordError("", fmt.Sprintf("read inbox: %v", err))
		return
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if ctx.Err() != nil {
			return
		}
		name := entry.Name()
		if entry.IsDir() || !isEventFile(name) || watcher.isInFlight(name) {
			continue
		}
		info, err := entry.Info()
		if err != nil || time.Since(info.ModTime()) < watcher.config.StableFor {
			continue
		}
		path := filepath.Join(watcher.config.Dir, name)
		events, err := ReadEventsFile(path, info.Size())
		if err != nil {
			watcher.reject(path, name, Report{File: name, Failures: []Failure{{FailedAtIndex: 0, Error: err.Error()}}})
			continue
		}
		watcher.dispatch(path, name, events)
	}
}

func isEventFile(name string) bool {
	if strings.HasPrefix(name, ".") {
		return false
	}
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".json") || strings.HasSuffix(lower, ".jsonl") || strings.HasSuffix(lower, ".ndjson")
}

func (watcher *Watcher) isInFlight(name string) bool {
	watcher.mu.Lock()
	defer watcher.mu.Unlock()
	_, exists := watcher.inFlight[name]
	return exists
}

// dispatch splits a file by matchId, keeping file order inside each match.
func (watcher *Watcher) dispatch(path, name string, events []domain.Event) {
	groups := make(map[string][]indexedEvent)
	var order []string
	for index, event := range events {
		if _, seen := groups[event.MatchID]; !seen {
			order = append(order, event.MatchID)
		}
		groups[event.MatchID] = append(groups[event.MatchID], indexedEvent{index: index, event: event})
	}
	state := &fileState{name: name, path: path, total: len(events)}
	state.remaining.Store(int32(len(order)))
	watcher.mu.Lock()
	watcher.inFlight[name] = struct{}{}
	watcher.mu.Unlock()
	for _, matchID := range order {
		watcher.queues[partition(matchID, len(watcher.queues))] <- matchJob{file: state, matchID: matchID, events: groups[matchID]}
	}
}

func partition(matchID string, workers int) int {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(matchID))
	return int(hash.Sum32() % uint32(workers))
}

func (watcher *Watcher) process(ctx context.Context, job matchJob) {
	accepted := 0
	var failure *Failure
	for _, item := range job.events {
		if err := watcher.add(ctx, item.event); err != nil {
			failure = &Failure{MatchID: job.matchID, FailedAtIndex: item.index, EventID: item.event.EventID, Error: err.Error()}
			break
		}
		accepted++
	}
	file := job.file
	file.mu.Lock()
	file.accepted += accepted
	if failure != nil {
		file.failures = append(file.failures, *failure)
	}
	file.mu.Unlock()
	watcher.mu.Lock()
	watcher.stats.EventsAccepted += int64(accepted)
	watcher.mu.Unlock()
	if file.remaining.Add(-1) == 0 {
		watcher.finish(file)
	}
}

// add retries only when the analysis capacity is temporarily exhausted.
func (watcher *Watcher) add(ctx context.Context, event domain.Event) error {
	for attempt := 0; ; attempt++ {
		_, err := watcher.ingest.Add(event)
		if !errors.Is(err, processing.ErrQueueFull) || attempt >= watcher.config.CapacityRetries {
			return err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("stopped while waiting for analysis capacity: %w", err)
		case <-time.After(watcher.config.RetryDelay):
		}
	}
}

func (watcher *Watcher) finish(file *fileState) {
	file.mu.Lock()
	report := Report{File: file.name, Accepted: file.accepted, Total: file.total, Failures: file.failures}
	file.mu.Unlock()
	sort.Slice(report.Failures, func(i, j int) bool { return report.Failures[i].FailedAtIndex < report.Failures[j].FailedAtIndex })
	if len(report.Failures) > 0 {
		watcher.reject(file.path, file.name, report)
	} else {
		if err := moveTo(file.path, filepath.Join(watcher.config.Dir, ProcessedDir)); err != nil {
			watcher.recordError(file.name, err.Error())
		}
		watcher.mu.Lock()
		watcher.stats.FilesProcessed++
		watcher.stats.LastActivity = time.Now().UTC()
		watcher.mu.Unlock()
		log.Printf("inbox: processed %s (%d events)", file.name, report.Total)
	}
	watcher.mu.Lock()
	delete(watcher.inFlight, file.name)
	watcher.mu.Unlock()
}

func (watcher *Watcher) reject(path, name string, report Report) {
	rejected := filepath.Join(watcher.config.Dir, RejectedDir)
	if err := moveTo(path, rejected); err != nil {
		watcher.recordError(name, err.Error())
	}
	if data, err := json.MarshalIndent(report, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(rejected, name+".error.json"), data, 0o644)
	}
	message := "rejected"
	if len(report.Failures) > 0 {
		message = report.Failures[0].Error
	}
	watcher.mu.Lock()
	watcher.stats.FilesRejected++
	watcher.stats.LastActivity = time.Now().UTC()
	watcher.mu.Unlock()
	watcher.recordError(name, message)
	log.Printf("inbox: rejected %s (%d/%d events accepted): %s", name, report.Accepted, report.Total, message)
}

func (watcher *Watcher) recordError(file, message string) {
	watcher.mu.Lock()
	defer watcher.mu.Unlock()
	watcher.stats.LastError = message
	watcher.stats.LastErrorFile = file
}

// moveTo renames a file into dir, adding a numeric suffix if the name is taken.
func moveTo(path, dir string) error {
	base := filepath.Base(path)
	target := filepath.Join(dir, base)
	ext := filepath.Ext(base)
	for i := 1; ; i++ {
		if _, err := os.Stat(target); errors.Is(err, os.ErrNotExist) {
			break
		}
		target = filepath.Join(dir, fmt.Sprintf("%s.%d%s", strings.TrimSuffix(base, ext), i, ext))
	}
	if err := os.Rename(path, target); err != nil {
		return fmt.Errorf("move %s: %w", base, err)
	}
	return nil
}

// ReadEventsFile accepts three layouts: one event object, {"events":[...]}, a JSON array
// (all in *.json), or one event per line (*.jsonl / *.ndjson). Unknown envelope fields
// are rejected, exactly as in the HTTP API.
func ReadEventsFile(path string, size int64) ([]domain.Event, error) {
	if size > maxFileSize {
		return nil, fmt.Errorf("file exceeds %d bytes", maxFileSize)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".jsonl") || strings.HasSuffix(lower, ".ndjson") {
		return decodeLines(raw)
	}
	return DecodeEvents(raw)
}

func DecodeEvents(raw []byte) ([]domain.Event, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("file is empty")
	}
	switch trimmed[0] {
	case '[':
		var events []domain.Event
		if err := strictDecode(trimmed, &events); err != nil {
			return nil, err
		}
		return nonEmpty(events)
	case '{':
		var probe map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &probe); err != nil {
			return nil, err
		}
		if _, isBatch := probe["events"]; isBatch {
			var batch struct {
				Events []domain.Event `json:"events"`
			}
			if err := strictDecode(trimmed, &batch); err != nil {
				return nil, err
			}
			return nonEmpty(batch.Events)
		}
		var event domain.Event
		if err := strictDecode(trimmed, &event); err != nil {
			return nil, err
		}
		return []domain.Event{event}, nil
	default:
		return nil, fmt.Errorf("file must contain a JSON object or array")
	}
}

func decodeLines(raw []byte) ([]domain.Event, error) {
	var events []domain.Event
	for number, line := range bytes.Split(raw, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var event domain.Event
		if err := strictDecode(line, &event); err != nil {
			return nil, fmt.Errorf("line %d: %w", number+1, err)
		}
		events = append(events, event)
	}
	return nonEmpty(events)
}

func strictDecode(raw []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("file must contain exactly one JSON value")
	}
	return nil
}

func nonEmpty(events []domain.Event) ([]domain.Event, error) {
	if len(events) == 0 {
		return nil, fmt.Errorf("file contains no events")
	}
	return events, nil
}
