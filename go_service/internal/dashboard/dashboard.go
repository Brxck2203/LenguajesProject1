// Package dashboard presents the existing ingestion and analytics APIs for review.
// Statistics are always calculated by Scala; the dashboard only renders them.
package dashboard

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Brxck2203/LenguajesProject1/go_service/internal/domain"
	"github.com/Brxck2203/LenguajesProject1/go_service/internal/simulation"
)

//go:embed index.html
var assets embed.FS

type Ingestor interface {
	Add(domain.Event) (*domain.AnalysisRequest, error)
}

type panel struct {
	base   string
	client *http.Client
	ingest Ingestor
	demo   sync.Mutex
}

// New uses the same configured Scala address as the analysis pipeline. Only a
// fixed set of read-only Scala routes is exposed; callers cannot choose a host.
func New(analysisURL string, ingest Ingestor) (http.Handler, error) {
	u, err := url.Parse(analysisURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || !strings.HasSuffix(u.Path, "/analyze") || ingest == nil {
		return nil, fmt.Errorf("dashboard requires an absolute HTTP(S) Scala URL ending in /analyze and an ingestor")
	}
	u.Path = strings.TrimSuffix(u.Path, "/analyze")
	u.RawPath, u.RawQuery, u.Fragment = "", "", ""
	p := &panel{base: strings.TrimSuffix(u.String(), "/"), ingest: ingest, client: &http.Client{
		Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
	mux := http.NewServeMux()
	for _, path := range []string{"/dashboard", "/dashboard/{$}"} {
		mux.HandleFunc("GET "+path, func(w http.ResponseWriter, r *http.Request) {
			body, _ := assets.ReadFile("index.html")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			_, _ = w.Write(body)
		})
	}
	mux.HandleFunc("GET /dashboard/api/health", p.proxy("/health"))
	mux.HandleFunc("GET /dashboard/api/results", p.proxy("/results"))
	mux.HandleFunc("GET /dashboard/api/results/{matchId}", func(w http.ResponseWriter, r *http.Request) {
		p.proxy("/results/"+escapeSegment(r.PathValue("matchId")))(w, r)
	})
	mux.HandleFunc("GET /dashboard/api/games", p.proxy("/stats/games"))
	mux.HandleFunc("GET /dashboard/api/games/{gameId}/players", func(w http.ResponseWriter, r *http.Request) {
		p.proxy("/stats/games/"+escapeSegment(r.PathValue("gameId"))+"/players")(w, r)
	})
	mux.HandleFunc("POST /dashboard/demo", p.generate)
	return mux, nil
}

func reply(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (p *panel) proxy(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, p.base+path, nil)
		if err != nil {
			reply(w, 502, map[string]string{"error": "No se pudo preparar la consulta a Scala"})
			return
		}
		req.Header.Set("Accept", "application/json")
		response, err := p.client.Do(req)
		if err != nil {
			reply(w, 502, map[string]string{"error": "Scala no responde. Revisa el estado de su contenedor."})
			return
		}
		defer response.Body.Close()
		body, err := io.ReadAll(io.LimitReader(response.Body, (8<<20)+1))
		media, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
		if err != nil || len(body) > 8<<20 || media != "application/json" || !json.Valid(body) {
			reply(w, 502, map[string]string{"error": "Scala devolvió una respuesta incompatible o demasiado grande"})
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(response.StatusCode)
		_, _ = w.Write(body)
	}
}

// generate runs the same scripted producer used by the CLI, with one goroutine
// per match. It returns ingestion status; analysis completion is queried later.
func (p *panel) generate(w http.ResponseWriter, r *http.Request) {
	// A browser from another origin must not be able to start simulations.
	if r.Header.Get("X-GameStats-Demo") != "1" {
		reply(w, 403, map[string]string{"error": "Usa el botón de demostración del panel"})
		return
	}
	if !p.demo.TryLock() {
		reply(w, 409, map[string]string{"error": "Ya se está generando una demostración"})
		return
	}
	defer p.demo.Unlock()
	matches := simulation.Fixtures(fmt.Sprintf("panel_%d", time.Now().UnixNano()))
	type outcome struct {
		MatchID string `json:"matchId"`
		Error   string `json:"error,omitempty"`
	}
	outcomes := make([]outcome, len(matches))
	var workers sync.WaitGroup
	for i, m := range matches {
		workers.Add(1)
		go func(i int, m simulation.Match) {
			defer workers.Done()
			outcomes[i].MatchID = m.ID
			for _, event := range m.Events {
				if r.Context().Err() != nil {
					outcomes[i].Error = "La solicitud fue cancelada"
					return
				}
				if _, err := p.ingest.Add(event); err != nil {
					outcomes[i].Error = err.Error()
					return
				}
			}
		}(i, m)
	}
	workers.Wait()
	failed := 0
	for _, o := range outcomes {
		if o.Error != "" {
			failed++
		}
	}
	status := http.StatusAccepted
	if failed > 0 {
		status = http.StatusServiceUnavailable
	}
	reply(w, status, map[string]any{"acceptedMatches": len(matches) - failed, "failedMatches": failed, "matches": outcomes})
}

// Scala decodes path segments using URLDecoder, where a literal plus is a space.
func escapeSegment(value string) string {
	return strings.ReplaceAll(url.PathEscape(value), "+", "%2B")
}
