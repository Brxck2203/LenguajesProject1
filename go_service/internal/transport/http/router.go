package httptransport

import (
	"net/http"

	"github.com/Brxck2203/LenguajesProject1/go_service/internal/domain"
	"github.com/Brxck2203/LenguajesProject1/go_service/internal/match"
)

type EventStore interface {
	Add(domain.Event) (*domain.AnalysisRequest, error)
	Get(string) (match.MatchSnapshot, bool)
	MatchIDs() []string
}
type AnalysisService interface {
	Analysis(string) (domain.AnalysisSnapshot, bool)
	Retry(string) error
}

func NewRouter(store EventStore, analysis AnalysisService) http.Handler {
	handler := &Handler{store: store, analysis: analysis}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", handler.handleHealth)
	mux.HandleFunc("/events", handler.handleEvents)
	mux.HandleFunc("/events/batch", handler.handleEventsBatch)
	mux.HandleFunc("/matches/{matchId}", handler.handleMatchByID)
	mux.HandleFunc("/matches/{matchId}/analysis", handler.handleAnalysis)
	mux.HandleFunc("/matches/{matchId}/analysis/retry", handler.handleRetry)
	mux.HandleFunc("/matches", handler.handleMatches)
	mux.HandleFunc("/matches/{$}", handler.handleMatches)
	mux.HandleFunc("/{$}", handler.handleIndex)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { writeError(w, http.StatusNotFound, "endpoint not found") })
	return mux
}
