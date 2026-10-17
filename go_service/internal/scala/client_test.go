package scala

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Brxck2203/LenguajesProject1/go_service/internal/domain"
)

const validResult = `{"matchId":"m1","gameId":"FREE_FIRE","playerStats":{},"matchStats":{},"patterns":[]}`

func TestClientSendsContractWithStableIdempotencyKey(t *testing.T) {
	var mu sync.Mutex
	var keys, bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/analyze" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected request: %s %s %v", r.Method, r.URL.Path, r.Header)
		}
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		bodies = append(bodies, string(body))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, validResult)
	}))
	defer server.Close()
	client, _ := NewClient(server.URL+"/analyze", time.Second)
	batch := domain.AnalysisRequest{MatchID: "m1", GameID: domain.GameFreeFire, Events: []domain.Event{}}
	for i := 0; i < 2; i++ {
		result, err := client.Analyze(context.Background(), batch)
		if err != nil || result.MatchID != "m1" {
			t.Fatalf("Analyze: %v, %v", result, err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if keys[0] == "" || keys[0] != keys[1] || bodies[0] != bodies[1] {
		t.Fatalf("retries changed identity: %v", keys)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(bodies[0]), &payload); err != nil || len(payload) != 3 || payload["events"] == nil {
		t.Fatalf("invalid envelope: %s", bodies[0])
	}
}

func TestClientRejectsIncompatibleAndTransientResponses(t *testing.T) {
	for _, tc := range []struct {
		name              string
		status            int
		body, contentType string
		retry             bool
	}{
		{"unavailable", 503, "down", "text/plain", true},
		{"rate_limited", 429, "", "application/json", true},
		{"not_implemented", 404, "", "application/json", false},
		{"accepted_without_result", 202, validResult, "application/json", false},
		{"wrong_match", 200, strings.Replace(validResult, "m1", "other", 1), "application/json", false},
		{"sample_analytics", 200, `{"playerId":"p1","mma":{}}`, "application/json", false},
		{"missing_sections", 200, `{"matchId":"m1","gameId":"FREE_FIRE"}`, "application/json", false},
		{"wrong_shape", 200, strings.Replace(validResult, `"patterns":[]`, `"patterns":null`, 1), "application/json", false},
		{"extra_json", 200, validResult + `{}`, "application/json", false},
		{"html", 200, validResult, "text/html", false},
		{"oversized", 200, strings.Repeat(" ", maxResponseSize+1), "application/json", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			}))
			defer server.Close()
			client, _ := NewClient(server.URL, time.Second)
			_, err := client.Analyze(context.Background(), domain.AnalysisRequest{MatchID: "m1", GameID: domain.GameFreeFire})
			var failure *Error
			if !errors.As(err, &failure) || failure.Retryable != tc.retry {
				t.Fatalf("error = %v; want retryable %v", err, tc.retry)
			}
		})
	}
}

func TestClientTimeoutAndRedirect(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-time.After(time.Second):
			}
		}))
		defer server.Close()
		client, _ := NewClient(server.URL, 20*time.Millisecond)
		_, err := client.Analyze(context.Background(), domain.AnalysisRequest{})
		var failure *Error
		if !errors.As(err, &failure) || !failure.Retryable {
			t.Fatalf("timeout error = %v", err)
		}
	})
	t.Run("redirect", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/elsewhere" {
				t.Error("client followed redirect")
			}
			http.Redirect(w, r, "/elsewhere", http.StatusFound)
		}))
		defer server.Close()
		client, _ := NewClient(server.URL, time.Second)
		if _, err := client.Analyze(context.Background(), domain.AnalysisRequest{}); err == nil {
			t.Fatal("redirect accepted")
		}
	})
}
