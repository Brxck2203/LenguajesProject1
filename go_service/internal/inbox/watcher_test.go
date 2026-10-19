package inbox

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Brxck2203/LenguajesProject1/go_service/internal/domain"
	"github.com/Brxck2203/LenguajesProject1/go_service/internal/match"
)

// recordingStore wraps the real match store and records the arrival order per match.
type recordingStore struct {
	mu    sync.Mutex
	store *match.Store
	order map[string][]string
}

func (r *recordingStore) Add(event domain.Event) (*domain.AnalysisRequest, error) {
	request, err := r.store.Add(event)
	if err == nil {
		r.mu.Lock()
		r.order[event.MatchID] = append(r.order[event.MatchID], event.EventID)
		r.mu.Unlock()
	}
	return request, err
}

func event(matchID string, index int, second int, action, player string, data map[string]any) domain.Event {
	raw, _ := json.Marshal(data)
	if data == nil {
		raw = []byte("{}")
	}
	typ := domain.TypeAction
	if action == domain.ActionMatchStarted || action == domain.ActionMatchFinished {
		typ = domain.TypeMatch
	}
	return domain.Event{
		EventID: fmt.Sprintf("%s_%02d", matchID, index), Timestamp: time.Date(2026, 10, 1, 12, 0, second, 0, time.UTC),
		GameID: domain.GameFreeFire, MatchID: matchID, PlayerID: player, Type: typ, Action: action, Data: raw,
	}
}

func matchEvents(matchID string) []domain.Event {
	return []domain.Event{
		event(matchID, 0, 0, domain.ActionMatchStarted, domain.SystemPlayerID, nil),
		event(matchID, 1, 1, domain.ActionDamageDealt, "p1", map[string]any{"targetPlayerId": "p2", "damage": 10}),
		event(matchID, 2, 2, domain.ActionPlayerEliminated, "p1", map[string]any{"victimId": "p2"}),
		event(matchID, 3, 3, domain.ActionMatchFinished, domain.SystemPlayerID, nil),
	}
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func startWatcher(t *testing.T, dir string, store Ingestor) (context.CancelFunc, *Watcher, chan struct{}) {
	t.Helper()
	watcher, err := New(Config{Dir: dir, PollInterval: 10 * time.Millisecond, Workers: 3}, store)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { watcher.Run(ctx); close(done) }()
	return cancel, watcher, done
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestWatcherProcessesSplitMatchesInOrderAndInParallel(t *testing.T) {
	dir := t.TempDir()
	store := &recordingStore{store: match.NewStore(), order: map[string][]string{}}
	// Each match is split into two files; file names define the order.
	for i := 0; i < 6; i++ {
		id := fmt.Sprintf("m%d", i)
		events := matchEvents(id)
		writeJSON(t, filepath.Join(dir, fmt.Sprintf("%s_a.json", id)), map[string]any{"events": events[:2]})
		lines := ""
		for _, e := range events[2:] {
			data, _ := json.Marshal(e)
			lines += string(data) + "\n"
		}
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%s_b.jsonl", id)), []byte(lines), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cancel, watcher, done := startWatcher(t, dir, store)
	defer func() { cancel(); <-done }()
	waitFor(t, func() bool { return watcher.Stats().FilesProcessed == 12 })

	for i := 0; i < 6; i++ {
		id := fmt.Sprintf("m%d", i)
		got := strings.Join(store.order[id], ",")
		want := fmt.Sprintf("%[1]s_00,%[1]s_01,%[1]s_02,%[1]s_03", id)
		if got != want {
			t.Fatalf("match %s applied out of order: %s", id, got)
		}
		snapshot, _ := store.store.Get(id)
		if snapshot.Status != match.StatusFinished {
			t.Fatalf("match %s not finished", id)
		}
	}
	processed, _ := os.ReadDir(filepath.Join(dir, ProcessedDir))
	if len(processed) != 12 || watcher.Stats().EventsAccepted != 24 {
		t.Fatalf("processed=%d stats=%+v", len(processed), watcher.Stats())
	}
}

func TestWatcherRejectsInvalidFilesWithAReport(t *testing.T) {
	dir := t.TempDir()
	store := &recordingStore{store: match.NewStore(), order: map[string][]string{}}
	events := matchEvents("bad")
	events[2].Data = json.RawMessage(`{}`) // PLAYER_ELIMINATED without victimId
	writeJSON(t, filepath.Join(dir, "bad.json"), events)
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ignored.tmp"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	cancel, watcher, done := startWatcher(t, dir, store)
	defer func() { cancel(); <-done }()
	waitFor(t, func() bool { return watcher.Stats().FilesRejected == 2 })

	var report Report
	data, err := os.ReadFile(filepath.Join(dir, RejectedDir, "bad.json.error.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if report.Accepted != 2 || len(report.Failures) != 1 || report.Failures[0].FailedAtIndex != 2 {
		t.Fatalf("unexpected report: %+v", report)
	}
	if _, err := os.Stat(filepath.Join(dir, "ignored.tmp")); err != nil {
		t.Fatal("temporary files must be left alone")
	}
}

func TestDecodeEventsAcceptsTheThreeLayouts(t *testing.T) {
	one, _ := json.Marshal(matchEvents("x")[0])
	array, _ := json.Marshal(matchEvents("x"))
	batch, _ := json.Marshal(map[string]any{"events": matchEvents("x")})
	for name, raw := range map[string][]byte{"object": one, "array": array, "batch": batch} {
		if _, err := DecodeEvents(raw); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := DecodeEvents([]byte(`{"eventId":"a","extra":1}`)); err == nil {
		t.Error("unknown envelope fields must be rejected")
	}
	if _, err := DecodeEvents([]byte(`[]`)); err == nil {
		t.Error("empty arrays must be rejected")
	}
}
