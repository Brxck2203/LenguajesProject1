package httptransport_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Brxck2203/LenguajesProject1/go_service/internal/domain"
	"github.com/Brxck2203/LenguajesProject1/go_service/internal/match"
	"github.com/Brxck2203/LenguajesProject1/go_service/internal/processing"
	"github.com/Brxck2203/LenguajesProject1/go_service/internal/scala"
	"github.com/Brxck2203/LenguajesProject1/go_service/internal/simulation"
	httptransport "github.com/Brxck2203/LenguajesProject1/go_service/internal/transport/http"
)

func TestConcurrentSimulationThroughHTTPWithScalaRecovery(t *testing.T) {
	var mu sync.Mutex
	batches := map[string]domain.AnalysisRequest{}
	attempts := map[string]int{}
	keys := map[string]string{}
	arrivals := 0
	barrier := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var batch domain.AnalysisRequest
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		mu.Lock()
		attempts[batch.MatchID]++
		first := attempts[batch.MatchID] == 1
		if first {
			keys[batch.MatchID] = r.Header.Get("Idempotency-Key")
		} else if keys[batch.MatchID] != r.Header.Get("Idempotency-Key") {
			t.Error("changed idempotency key")
		}
		batches[batch.MatchID] = batch
		arrivals++
		if arrivals == 2 {
			close(barrier)
		}
		mu.Unlock()
		// Require at least two concurrent outbound requests before allowing progress.
		select {
		case <-barrier:
		case <-r.Context().Done():
			return
		}
		if first {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(domain.AnalysisResult{MatchID: batch.MatchID, GameID: batch.GameID, PlayerStats: json.RawMessage(`{}`), MatchStats: json.RawMessage(`{}`), Patterns: json.RawMessage(`[]`)})
	}))
	defer upstream.Close()
	client, err := scala.NewClient(upstream.URL+"/analyze", 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	service, err := processing.NewService(match.NewStore(), client, processing.Config{Workers: 4, Capacity: 16, MaxAttempts: 3, RetryBase: time.Millisecond, RetryMax: 2 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		service.Shutdown(ctx)
	}()
	server := httptest.NewServer(httptransport.NewRouter(service, service))
	defer server.Close()
	matches := simulation.Fixtures("e2e")
	runner := simulation.Runner{BaseURL: server.URL, Client: server.Client(), Interval: time.Millisecond}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := runner.Run(ctx, matches); err != nil {
		t.Fatal(err)
	}
	if err := runner.WaitForAnalyses(ctx, matches); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(batches) != 8 {
		t.Fatalf("received %d matches", len(batches))
	}
	for _, m := range matches {
		batch := batches[m.ID]
		if !reflect.DeepEqual(batch.Events, m.Events) {
			t.Errorf("events changed or reordered for %s", m.ID)
		}
		if attempts[m.ID] != 2 {
			t.Errorf("attempts %s = %d", m.ID, attempts[m.ID])
		}
		snapshot, _ := service.Get(m.ID)
		if snapshot.Status != match.StatusFinished || len(snapshot.Events) != len(m.Events) {
			t.Errorf("invalid match summary: %+v", snapshot)
		}
		if len(snapshot.PlayerIDs) < 2 {
			t.Errorf("participants missing for %s", m.ID)
		}
	}
}

type immediateAnalyzer struct{}

func (immediateAnalyzer) Analyze(ctx context.Context, r domain.AnalysisRequest) (domain.AnalysisResult, error) {
	return domain.AnalysisResult{MatchID: r.MatchID, GameID: r.GameID, PlayerStats: json.RawMessage(`{}`), MatchStats: json.RawMessage(`{}`), Patterns: json.RawMessage(`[]`)}, nil
}

func TestPartialBatchStillDispatchesClosedMatch(t *testing.T) {
	service, _ := processing.NewService(match.NewStore(), immediateAnalyzer{}, processing.Config{Workers: 1, Capacity: 4, MaxAttempts: 1, RetryBase: time.Millisecond, RetryMax: time.Millisecond})
	defer service.Shutdown(context.Background())
	router := httptransport.NewRouter(service, service)
	fixture := simulation.Fixtures("partial")[0]
	bad := fixture.Events[1]
	bad.MatchID = "not_started"
	bad.EventID = "bad"
	events := append(append([]domain.Event{}, fixture.Events...), bad)
	body, _ := json.Marshal(map[string]any{"events": events})
	request := httptest.NewRequest(http.MethodPost, "/events/batch", strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	var response struct {
		Accepted int
		Ready    []struct {
			MatchID    string `json:"matchId"`
			EventCount int    `json:"eventCount"`
		} `json:"readyForAnalysis"`
	}
	json.Unmarshal(recorder.Body.Bytes(), &response)
	if recorder.Code != 409 || response.Accepted != len(fixture.Events) || len(response.Ready) != 1 || response.Ready[0].EventCount != len(fixture.Events) {
		t.Fatalf("partial batch response: %d %s", recorder.Code, recorder.Body)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := service.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	snapshot, exists := service.Analysis(fixture.ID)
	if !exists || snapshot.Status != domain.AnalysisCompleted {
		t.Fatalf("closed batch not dispatched: %+v", snapshot)
	}
}

func TestAnalysisRoutesRetryFailedJobAndProtectLifecycle(t *testing.T) {
	var ready atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ready.Load() {
			w.WriteHeader(404)
			return
		}
		var batch domain.AnalysisRequest
		json.NewDecoder(r.Body).Decode(&batch)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(domain.AnalysisResult{MatchID: batch.MatchID, GameID: batch.GameID, PlayerStats: json.RawMessage(`{}`), MatchStats: json.RawMessage(`{}`), Patterns: json.RawMessage(`[]`)})
	}))
	defer upstream.Close()
	client, _ := scala.NewClient(upstream.URL, time.Second)
	service, _ := processing.NewService(match.NewStore(), client, processing.Config{Workers: 1, Capacity: 4, MaxAttempts: 3, RetryBase: time.Millisecond, RetryMax: time.Millisecond})
	defer service.Shutdown(context.Background())
	router := httptransport.NewRouter(service, service)
	server := httptest.NewServer(router)
	defer server.Close()
	fixture := simulation.Fixtures("retry_api")[0]
	runner := simulation.Runner{BaseURL: server.URL, Client: server.Client()}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := runner.Run(ctx, []simulation.Match{fixture}); err != nil {
		t.Fatal(err)
	}
	if err := runner.WaitForAnalyses(ctx, []simulation.Match{fixture}); err == nil {
		t.Fatal("failed upstream reported as success")
	}
	ready.Store(true)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest("POST", "/matches/"+fixture.ID+"/analysis/retry", nil))
	if recorder.Code != 202 {
		t.Fatalf("retry: %d %s", recorder.Code, recorder.Body)
	}
	if err := runner.WaitForAnalyses(ctx, []simulation.Match{fixture}); err != nil {
		t.Fatal(err)
	}
	active := simulation.Fixtures("still_active")[0]
	service.Add(active.Events[0])
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{"POST", "/matches/" + fixture.ID + "/analysis/retry", 409},
		{"GET", "/matches/" + fixture.ID + "/analysis/retry", 405},
		{"GET", "/matches/unknown/analysis", 404},
		{"GET", "/matches/" + active.ID + "/analysis", 409},
		{"POST", "/matches/" + fixture.ID + "/analysis", 405},
	} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(tc.method, tc.path, nil))
		if recorder.Code != tc.status {
			t.Errorf("%s %s = %d, want %d", tc.method, tc.path, recorder.Code, tc.status)
		}
	}
}
