package processing

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Brxck2203/LenguajesProject1/go_service/internal/domain"
	"github.com/Brxck2203/LenguajesProject1/go_service/internal/match"
	"github.com/Brxck2203/LenguajesProject1/go_service/internal/scala"
)

type analyzerFunc func(context.Context, domain.AnalysisRequest) (domain.AnalysisResult, error)

func (f analyzerFunc) Analyze(ctx context.Context, r domain.AnalysisRequest) (domain.AnalysisResult, error) {
	return f(ctx, r)
}

func testService(t *testing.T, analyzer Analyzer, capacity int) *Service {
	t.Helper()
	s, err := NewService(match.NewStore(), analyzer, Config{Workers: 2, Capacity: capacity, MaxAttempts: 3, RetryBase: time.Millisecond, RetryMax: 2 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		s.Shutdown(ctx)
	})
	return s
}
func lifecycle(id string, finish bool) domain.Event {
	action := domain.ActionMatchStarted
	suffix := "start"
	second := 0
	if finish {
		action = domain.ActionMatchFinished
		suffix = "finish"
		second = 1
	}
	return domain.Event{EventID: id + suffix, MatchID: id, GameID: domain.GameFreeFire, Timestamp: time.Date(2026, 1, 1, 0, 0, second, 0, time.UTC), PlayerID: domain.SystemPlayerID, Type: domain.TypeMatch, Action: action, Data: json.RawMessage(`{}`)}
}
func closeMatch(t *testing.T, s *Service, id string) *domain.AnalysisRequest {
	t.Helper()
	if _, err := s.Add(lifecycle(id, false)); err != nil {
		t.Fatal(err)
	}
	r, err := s.Add(lifecycle(id, true))
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func awaitStatus(t *testing.T, s *Service, id string, status domain.AnalysisStatus) domain.AnalysisSnapshot {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if snapshot, ok := s.Analysis(id); ok && snapshot.Status == status {
			return snapshot
		}
		time.Sleep(time.Millisecond)
	}
	snapshot, _ := s.Analysis(id)
	t.Fatalf("analysis %s: got %s, want %s; %s", id, snapshot.Status, status, snapshot.LastError)
	return snapshot
}
func resultFor(r domain.AnalysisRequest) domain.AnalysisResult {
	return domain.AnalysisResult{MatchID: r.MatchID, GameID: r.GameID, PlayerStats: json.RawMessage(`{}`), MatchStats: json.RawMessage(`{}`), Patterns: json.RawMessage(`[]`)}
}

func TestRetryPreservesBatchAndDefensiveCopies(t *testing.T) {
	var calls atomic.Int32
	s := testService(t, analyzerFunc(func(ctx context.Context, r domain.AnalysisRequest) (domain.AnalysisResult, error) {
		if string(r.Events[0].Data) != `{}` {
			t.Error("batch corrupted across retries")
		}
		r.Events[0].Data[0] = '['
		if calls.Add(1) < 3 {
			return domain.AnalysisResult{}, errors.New("temporary outage")
		}
		return resultFor(r), nil
	}), 4)
	r := closeMatch(t, s, "m1")
	r.Events[0].Data[0] = '!'
	snapshot := awaitStatus(t, s, "m1", domain.AnalysisCompleted)
	if snapshot.Attempts != 3 || snapshot.LastError != "" {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	snapshot.Result.MatchStats[0] = '!'
	other, _ := s.Analysis("m1")
	if string(other.Result.MatchStats) != `{}` {
		t.Fatal("result snapshot aliases internal memory")
	}
	stored, _ := s.Get("m1")
	if string(stored.Events[0].Data) != `{}` {
		t.Fatal("stored history corrupted")
	}
}

func TestFailedAnalysisCanBeManuallyRetriedWithoutReopeningMatch(t *testing.T) {
	var ready atomic.Bool
	s := testService(t, analyzerFunc(func(ctx context.Context, r domain.AnalysisRequest) (domain.AnalysisResult, error) {
		if !ready.Load() {
			return domain.AnalysisResult{}, &scala.Error{Message: "incompatible endpoint"}
		}
		return resultFor(r), nil
	}), 4)
	closeMatch(t, s, "m1")
	failed := awaitStatus(t, s, "m1", domain.AnalysisFailed)
	if failed.Attempts != 1 {
		t.Fatal("permanent contract errors must not automatically retry")
	}
	ready.Store(true)
	if err := s.Retry("m1"); err != nil {
		t.Fatal(err)
	}
	completed := awaitStatus(t, s, "m1", domain.AnalysisCompleted)
	if completed.Attempts != 2 || completed.EventCount != 2 {
		t.Fatalf("unexpected retry: %+v", completed)
	}
	if err := s.Retry("m1"); !errors.Is(err, ErrRetryConflict) {
		t.Fatalf("completed analysis retried: %v", err)
	}
}

func TestTransientFailureStopsAtAttemptLimitAndRetainsBatch(t *testing.T) {
	s := testService(t, analyzerFunc(func(ctx context.Context, r domain.AnalysisRequest) (domain.AnalysisResult, error) {
		return domain.AnalysisResult{}, errors.New("offline")
	}), 2)
	closeMatch(t, s, "offline")
	snapshot := awaitStatus(t, s, "offline", domain.AnalysisFailed)
	if snapshot.Attempts != 3 || snapshot.LastError != "offline" || snapshot.Result != nil {
		t.Fatalf("unexpected failure state: %+v", snapshot)
	}
	if len(s.jobs["offline"].request.Events) != 2 {
		t.Fatal("failed batch was discarded")
	}
}

func TestFullCapacityDoesNotConsumeFinishEvent(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	s := testService(t, analyzerFunc(func(ctx context.Context, r domain.AnalysisRequest) (domain.AnalysisResult, error) {
		select {
		case <-release:
			return resultFor(r), nil
		case <-ctx.Done():
			return domain.AnalysisResult{}, ctx.Err()
		}
	}), 1)
	closeMatch(t, s, "first")
	if _, err := s.Add(lifecycle("second", false)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Add(lifecycle("second", true)); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("error = %v", err)
	}
	snapshot, _ := s.Get("second")
	if snapshot.Status != match.StatusActive || len(snapshot.Events) != 1 {
		t.Fatal("rejected finish changed match")
	}
	release <- struct{}{}
	awaitStatus(t, s, "first", domain.AnalysisCompleted)
	if _, err := s.Add(lifecycle("second", true)); err != nil {
		t.Fatalf("same finish event cannot be retried: %v", err)
	}
}

func TestShutdownDrainsAcceptedJobsAndRejectsNewEvents(t *testing.T) {
	s := testService(t, analyzerFunc(func(ctx context.Context, r domain.AnalysisRequest) (domain.AnalysisResult, error) {
		return resultFor(r), nil
	}), 10)
	for _, id := range []string{"a", "b", "c", "d"} {
		closeMatch(t, s, id)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b", "c", "d"} {
		snapshot, _ := s.Analysis(id)
		if snapshot.Status != domain.AnalysisCompleted {
			t.Fatalf("undrained: %+v", snapshot)
		}
	}
	if _, err := s.Add(lifecycle("late", false)); !errors.Is(err, ErrStopping) {
		t.Fatalf("ingestion accepted after shutdown: %v", err)
	}
}

func TestShutdownDeadlineCancelsRequestsAndRetainsFailedJobs(t *testing.T) {
	entered := make(chan struct{}, 2)
	s := testService(t, analyzerFunc(func(ctx context.Context, r domain.AnalysisRequest) (domain.AnalysisResult, error) {
		entered <- struct{}{}
		<-ctx.Done()
		return domain.AnalysisResult{}, ctx.Err()
	}), 3)
	for _, id := range []string{"a", "b", "c"} {
		closeMatch(t, s, id)
	}
	<-entered
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := s.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown = %v", err)
	}
	for _, id := range []string{"a", "b", "c"} {
		snapshot := awaitStatus(t, s, id, domain.AnalysisFailed)
		if snapshot.EventCount != 2 {
			t.Fatal("lost pending batch")
		}
	}
}
