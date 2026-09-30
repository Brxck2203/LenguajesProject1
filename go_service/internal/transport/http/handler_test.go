package httptransport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Brxck2203/LenguajesProject1/go_service/internal/match"
)

func newTestRequest(method, path, body string, contentType string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	store := match.NewStore()
	recorder := httptest.NewRecorder()
	NewRouter(store).ServeHTTP(recorder, request)
	return recorder
}

func eventJSON(eventID, action, eventType, playerID, data string) string {
	return fmt.Sprintf(
		`{"eventId":%q,"timestamp":"2026-09-27T10:00:00Z","gameId":"FREE_FIRE","matchId":"match_001","playerId":%q,"type":%q,"action":%q,"data":%s}`,
		eventID,
		playerID,
		eventType,
		action,
		data,
	)
}

func decodeResponse(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("response body is not valid JSON: %v", err)
	}
	return response
}

func TestHealth(t *testing.T) {
	recorder := newTestRequest(http.MethodGet, "/health", "", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := decodeResponse(t, recorder)["status"]; got != "ok" {
		t.Fatalf("status response = %v, want ok", got)
	}
}

func TestPostStartedEvent(t *testing.T) {
	body := eventJSON("match_001_start", "MATCH_STARTED", "MATCH", "SYSTEM", `{}`)
	recorder := newTestRequest(http.MethodPost, "/events", body, "application/json")
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusAccepted, recorder.Body)
	}
	response := decodeResponse(t, recorder)
	if response["matchStatus"] != "ACTIVE" {
		t.Fatalf("matchStatus = %v, want ACTIVE", response["matchStatus"])
	}
}

func TestPostInvalidJSON(t *testing.T) {
	recorder := newTestRequest(http.MethodPost, "/events", "{", "application/json")
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestPostUnknownField(t *testing.T) {
	body := strings.TrimSuffix(eventJSON("match_001_start", "MATCH_STARTED", "MATCH", "SYSTEM", `{}`), "}")
	body += `,"unexpected":true}`
	recorder := newTestRequest(http.MethodPost, "/events", body, "application/json")
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestPostMissingJSONContentType(t *testing.T) {
	recorder := newTestRequest(http.MethodPost, "/events", `{}`, "text/plain")
	if recorder.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnsupportedMediaType)
	}
}

func TestPostBodyTooLarge(t *testing.T) {
	padding := strings.Repeat("x", 1<<20)
	body := fmt.Sprintf(
		`{"eventId":"evt","timestamp":"2026-09-27T10:00:00Z","gameId":"FREE_FIRE","matchId":"match_001","playerId":"SYSTEM","type":"MATCH","action":"MATCH_STARTED","data":{"padding":%q}}`,
		padding,
	)
	recorder := newTestRequest(http.MethodPost, "/events", body, "application/json")
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestPostEventBeforeMatchStarted(t *testing.T) {
	body := eventJSON("evt_action", "PLAYER_ELIMINATED", "ACTION", "player_123", `{"victimId":"player_456"}`)
	recorder := newTestRequest(http.MethodPost, "/events", body, "application/json")
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusConflict, recorder.Body)
	}
}

func TestPostFinishedMatchEvent(t *testing.T) {
	store := match.NewStore()
	router := NewRouter(store)
	for _, body := range []string{
		eventJSON("match_001_start", "MATCH_STARTED", "MATCH", "SYSTEM", `{}`),
		eventJSON("match_001_end", "MATCH_FINISHED", "MATCH", "SYSTEM", `{}`),
	} {
		request := httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusAccepted {
			t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusAccepted, recorder.Body)
		}
		if strings.Contains(body, "MATCH_FINISHED") && decodeResponse(t, recorder)["status"] != "match_ready_for_analysis" {
			t.Fatalf("status response = %v, want match_ready_for_analysis", decodeResponse(t, recorder)["status"])
		}
	}
}

func TestBatchProcessesMatchLifecycle(t *testing.T) {
	body := `{"events":[` +
		eventJSON("match_001_start", "MATCH_STARTED", "MATCH", "SYSTEM", `{}`) + `,` +
		eventJSON("evt_elimination", "PLAYER_ELIMINATED", "ACTION", "player_123", `{"victimId":"player_456"}`) + `,` +
		eventJSON("match_001_end", "MATCH_FINISHED", "MATCH", "SYSTEM", `{}`) +
		`]}`
	recorder := newTestRequest(http.MethodPost, "/events/batch", body, "application/json")
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusAccepted, recorder.Body)
	}
	response := decodeResponse(t, recorder)
	if response["accepted"] != float64(3) {
		t.Fatalf("accepted = %v, want 3", response["accepted"])
	}
	ready, ok := response["readyForAnalysis"].([]any)
	if !ok || len(ready) != 1 {
		t.Fatalf("readyForAnalysis = %v, want one entry", response["readyForAnalysis"])
	}
}

func TestBatchPartiallyAcceptsValidEvents(t *testing.T) {
	body := `{"events":[` +
		eventJSON("match_001_start", "MATCH_STARTED", "MATCH", "SYSTEM", `{}`) + `,` +
		eventJSON("evt_invalid", "PLAYER_ELIMINATED", "ACTION", "player_123", `{}`) +
		`]}`
	store := match.NewStore()
	router := NewRouter(store)
	request := httptest.NewRequest(http.MethodPost, "/events/batch", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusBadRequest, recorder.Body)
	}
	response := decodeResponse(t, recorder)
	if response["accepted"] != float64(1) || response["failedAtIndex"] != float64(1) {
		t.Fatalf("partial response = %v, want accepted 1 and failedAtIndex 1", response)
	}
	if snapshot, exists := store.Get("match_001"); !exists || len(snapshot.Events) != 1 {
		t.Fatalf("valid first event was not retained: exists=%v events=%d", exists, len(snapshot.Events))
	}
}

func TestGetMatchSummaryOmitsEvents(t *testing.T) {
	store := match.NewStore()
	router := NewRouter(store)
	body := eventJSON("match_001_start", "MATCH_STARTED", "MATCH", "SYSTEM", `{}`)
	request := httptest.NewRequest(http.MethodPost, "/events", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(httptest.NewRecorder(), request)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/matches/match_001", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body)
	}
	response := decodeResponse(t, recorder)
	if _, exists := response["events"]; exists {
		t.Fatal("match summary unexpectedly includes events")
	}
	if response["eventCount"] != float64(1) || response["status"] != "ACTIVE" {
		t.Fatalf("unexpected match summary: %v", response)
	}
}

func TestGetMissingMatch(t *testing.T) {
	recorder := newTestRequest(http.MethodGet, "/matches/inexistente", "", "")
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestInvalidMethodsReturnMethodNotAllowed(t *testing.T) {
	for _, testCase := range []struct {
		method string
		path   string
	}{
		{method: http.MethodPost, path: "/health"},
		{method: http.MethodGet, path: "/events"},
	} {
		recorder := newTestRequest(testCase.method, testCase.path, "", "")
		if recorder.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s status = %d, want %d", testCase.method, testCase.path, recorder.Code, http.StatusMethodNotAllowed)
		}
	}
}
