// Package processing owns the in-memory handoff between match ingestion and Scala.
package processing

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Brxck2203/LenguajesProject1/go_service/internal/domain"
	"github.com/Brxck2203/LenguajesProject1/go_service/internal/match"
)

var (
	ErrQueueFull        = errors.New("analysis capacity exhausted; retry the unaccepted event later")
	ErrStopping         = errors.New("service is shutting down")
	ErrAnalysisNotFound = errors.New("analysis not found; the match must be closed first")
	ErrRetryConflict    = errors.New("only a failed analysis can be retried")
)

type Analyzer interface {
	Analyze(context.Context, domain.AnalysisRequest) (domain.AnalysisResult, error)
}

type Config struct {
	Workers     int
	Capacity    int // queued + running + waiting for automatic retry
	MaxAttempts int // per initial submission or manual retry cycle
	RetryBase   time.Duration
	RetryMax    time.Duration
}

type job struct {
	request  domain.AnalysisRequest
	snapshot domain.AnalysisSnapshot
}

type Service struct {
	mu       sync.RWMutex
	store    *match.Store
	analyzer Analyzer
	config   Config
	jobs     map[string]*job
	queue    chan string
	active   int
	stopping bool
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}
}

func NewService(store *match.Store, analyzer Analyzer, config Config) (*Service, error) {
	if store == nil || analyzer == nil || config.Workers <= 0 || config.Capacity <= 0 || config.MaxAttempts <= 0 || config.RetryBase <= 0 || config.RetryMax < config.RetryBase {
		return nil, fmt.Errorf("invalid analysis service configuration")
	}
	ctx, cancel := context.WithCancel(context.Background())
	service := &Service{store: store, analyzer: analyzer, config: config, jobs: make(map[string]*job), queue: make(chan string, config.Capacity), ctx: ctx, cancel: cancel, done: make(chan struct{})}
	var workers sync.WaitGroup
	for i := 0; i < config.Workers; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for matchID := range service.queue {
				service.process(matchID)
			}
		}()
	}
	go func() { workers.Wait(); close(service.done) }()
	return service, nil
}

// Add makes closing a match and registering its analysis one atomic operation
// relative to ingestion and shutdown. No network I/O occurs while holding a lock.
func (service *Service) Add(event domain.Event) (*domain.AnalysisRequest, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.stopping {
		return nil, ErrStopping
	}
	if event.Action == domain.ActionMatchFinished && service.active >= service.config.Capacity {
		// In particular, do not consume the eventId or close the match.
		return nil, ErrQueueFull
	}
	request, err := service.store.Add(event)
	if err != nil || request == nil {
		return request, err
	}
	now := time.Now().UTC()
	service.jobs[request.MatchID] = &job{request: request.Clone(), snapshot: domain.AnalysisSnapshot{
		MatchID: request.MatchID, GameID: request.GameID, Status: domain.AnalysisQueued,
		EventCount: len(request.Events), CreatedAt: now, UpdatedAt: now,
	}}
	service.active++
	// queue has Capacity slots; active includes every queued/running job, so
	// the reservation above guarantees this send cannot block.
	service.queue <- request.MatchID
	return request, nil
}

func (service *Service) Get(matchID string) (match.MatchSnapshot, bool) {
	service.mu.RLock()
	defer service.mu.RUnlock()
	return service.store.Get(matchID)
}

func (service *Service) Analysis(matchID string) (domain.AnalysisSnapshot, bool) {
	service.mu.RLock()
	defer service.mu.RUnlock()
	job, ok := service.jobs[matchID]
	if !ok {
		return domain.AnalysisSnapshot{}, false
	}
	snapshot := job.snapshot
	if snapshot.Result != nil {
		result := snapshot.Result.Clone()
		snapshot.Result = &result
	}
	return snapshot, true
}

func (service *Service) Retry(matchID string) error {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.stopping {
		return ErrStopping
	}
	job, ok := service.jobs[matchID]
	if !ok {
		return ErrAnalysisNotFound
	}
	if job.snapshot.Status != domain.AnalysisFailed {
		return ErrRetryConflict
	}
	if service.active >= service.config.Capacity {
		return ErrQueueFull
	}
	job.snapshot.Status = domain.AnalysisQueued
	job.snapshot.LastError = ""
	job.snapshot.UpdatedAt = time.Now().UTC()
	service.active++
	service.queue <- matchID
	return nil
}

// Shutdown stops accepting events and drains all accepted jobs, including retries.
// At the deadline it cancels requests. State is memory-only and is lost on exit.
func (service *Service) Shutdown(ctx context.Context) error {
	service.mu.Lock()
	if !service.stopping {
		service.stopping = true
		close(service.queue)
	}
	service.mu.Unlock()
	select {
	case <-service.done:
		service.cancel()
		return nil
	case <-ctx.Done():
		service.cancel()
		return ctx.Err()
	}
}

func (service *Service) process(matchID string) {
	delay := service.config.RetryBase
	for attempt := 1; attempt <= service.config.MaxAttempts; attempt++ {
		if err := service.ctx.Err(); err != nil {
			service.finish(matchID, nil, err)
			return
		}
		service.mu.Lock()
		job := service.jobs[matchID]
		job.snapshot.Status = domain.AnalysisRunning
		job.snapshot.Attempts++
		job.snapshot.UpdatedAt = time.Now().UTC()
		request := job.request.Clone()
		service.mu.Unlock()
		result, err := service.analyzer.Analyze(service.ctx, request)
		if err == nil {
			service.finish(matchID, &result, nil)
			return
		}
		var classified interface{ Temporary() bool }
		retryable := !errors.As(err, &classified) || classified.Temporary()
		if !retryable || attempt == service.config.MaxAttempts || service.ctx.Err() != nil {
			service.finish(matchID, nil, err)
			return
		}
		service.mu.Lock()
		job.snapshot.Status = domain.AnalysisRetrying
		job.snapshot.LastError = err.Error()
		job.snapshot.UpdatedAt = time.Now().UTC()
		service.mu.Unlock()
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-service.ctx.Done():
			timer.Stop()
			service.finish(matchID, nil, service.ctx.Err())
			return
		}
		if delay > service.config.RetryMax/2 {
			delay = service.config.RetryMax
		} else {
			delay *= 2
		}
	}
}

func (service *Service) finish(matchID string, result *domain.AnalysisResult, err error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	job := service.jobs[matchID]
	job.snapshot.UpdatedAt = time.Now().UTC()
	if err != nil {
		job.snapshot.Status = domain.AnalysisFailed
		job.snapshot.LastError = err.Error()
	} else {
		copyOfResult := result.Clone()
		job.snapshot.Result = &copyOfResult
		job.snapshot.Status = domain.AnalysisCompleted
		job.snapshot.LastError = ""
	}
	service.active--
}
