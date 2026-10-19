package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Brxck2203/LenguajesProject1/go_service/internal/match"
)

func TestPanelAndReadOnlyProxy(t *testing.T) {
	var paths []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.RequestURI())
		if r.Method != "GET" {
			t.Errorf("unexpected method %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = fmt.Fprint(w, `{"ok":true}`)
	}))
	defer upstream.Close()
	panel, err := New(upstream.URL+"/prefix/analyze", match.NewStore())
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/dashboard", "/dashboard/"} {
		rec := httptest.NewRecorder()
		panel.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Partidas y patrones") || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
			t.Fatalf("page: %d", rec.Code)
		}
	}
	for _, path := range []string{"/dashboard/api/health", "/dashboard/api/results", "/dashboard/api/games", "/dashboard/api/results/m%2B1", "/dashboard/api/games/FREE_FIRE/players"} {
		rec := httptest.NewRecorder()
		panel.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != 200 {
			t.Fatalf("proxy %s: %d %s", path, rec.Code, rec.Body)
		}
	}
	expected := []string{"/prefix/health", "/prefix/results", "/prefix/stats/games", "/prefix/results/m%2B1", "/prefix/stats/games/FREE_FIRE/players"}
	for i, path := range expected {
		if paths[i] != path {
			t.Fatalf("upstream path %q, want %q", paths[i], path)
		}
	}
	for _, test := range []struct {
		method, path string
		status       int
	}{{"POST", "/dashboard/api/results", 405}, {"GET", "/dashboard/api/unknown", 404}, {"GET", "/dashboard/api/proxy?url=http://example.com", 404}} {
		rec := httptest.NewRecorder()
		panel.ServeHTTP(rec, httptest.NewRequest(test.method, test.path, nil))
		if rec.Code != test.status {
			t.Fatalf("%s %s: %d", test.method, test.path, rec.Code)
		}
	}
}

func TestProxyRejectsRedirectHTMLAndOversizedResponses(t *testing.T) {
	for _, kind := range []string{"redirect", "html", "oversized"} {
		t.Run(kind, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch kind {
				case "redirect":
					http.Redirect(w, r, "http://example.com", http.StatusFound)
				case "html":
					w.Header().Set("Content-Type", "text/html")
					_, _ = fmt.Fprint(w, "<html>error</html>")
				case "oversized":
					w.Header().Set("Content-Type", "application/json")
					_, _ = fmt.Fprint(w, `{"text":"`+strings.Repeat("a", 8<<20)+`"}`)
				}
			}))
			defer upstream.Close()
			p, _ := New(upstream.URL+"/analyze", match.NewStore())
			rec := httptest.NewRecorder()
			p.ServeHTTP(rec, httptest.NewRequest("GET", "/dashboard/api/health", nil))
			if rec.Code != 502 {
				t.Fatalf("got %d", rec.Code)
			}
		})
	}
}

func TestBrowserDemoFeedsEightMatchesThroughRealStore(t *testing.T) {
	store := match.NewStore()
	p, _ := New("http://localhost:8080/analyze", store)
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, httptest.NewRequest("POST", "/dashboard/demo", nil))
	if rec.Code != 403 {
		t.Fatalf("missing header: %d", rec.Code)
	}
	for round := 0; round < 2; round++ {
		req := httptest.NewRequest("POST", "/dashboard/demo", nil)
		req.Header.Set("X-GameStats-Demo", "1")
		rec = httptest.NewRecorder()
		p.ServeHTTP(rec, req)
		if rec.Code != 202 {
			t.Fatalf("demo: %d %s", rec.Code, rec.Body)
		}
		var body struct {
			Accepted int `json:"acceptedMatches"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Accepted != 8 {
			t.Fatalf("response: %s", rec.Body)
		}
	}
	if len(store.MatchIDs()) != 16 {
		t.Fatal("demo reused identifiers")
	}
	total := 0
	games := map[string]int{}
	for _, id := range store.MatchIDs() {
		s, _ := store.Get(id)
		if s.Status != match.StatusFinished {
			t.Fatalf("not closed: %s", id)
		}
		total += len(s.Events)
		games[s.GameID]++
	}
	if total != 146 || len(games) != 4 {
		t.Fatalf("events %d games %v", total, games)
	}
}
