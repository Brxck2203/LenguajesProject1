package httptransport

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Brxck2203/LenguajesProject1/go_service/internal/inbox"
	"github.com/Brxck2203/LenguajesProject1/go_service/internal/match"
)

type fakeInbox struct{}

func (fakeInbox) Stats() inbox.Stats { return inbox.Stats{Dir: "/data/inbox", FilesProcessed: 3} }

func serve(router http.Handler, path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder
}

func TestCatalogIsPublished(t *testing.T) {
	recorder := newTestRequest(http.MethodGet, "/catalog", "", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	games, ok := decodeResponse(t, recorder)["games"].(map[string]any)
	if !ok || games["FREE_FIRE"] == nil || games["MMA"] == nil {
		t.Fatalf("catalog does not list the games: %s", recorder.Body)
	}
}

func TestInboxStatus(t *testing.T) {
	disabled := decodeResponse(t, newTestRequest(http.MethodGet, "/inbox", "", ""))
	if disabled["enabled"] != false {
		t.Fatalf("inbox should be disabled by default: %v", disabled)
	}
	enabled := decodeResponse(t, serve(NewRouter(match.NewStore(), nil, WithInbox(fakeInbox{})), "/inbox"))
	stats, _ := enabled["stats"].(map[string]any)
	if enabled["enabled"] != true || stats["filesProcessed"] != float64(3) {
		t.Fatalf("unexpected inbox status: %v", enabled)
	}
	failed := decodeResponse(t, serve(NewRouter(match.NewStore(), nil, WithInboxError(errors.New("permission denied"))), "/inbox"))
	if failed["error"] != "permission denied" {
		t.Fatalf("inbox error not reported: %v", failed)
	}
}
