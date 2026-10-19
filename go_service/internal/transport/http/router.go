package httptransport

import (
	"net/http"

	"github.com/Brxck2203/LenguajesProject1/go_service/internal/domain"
	"github.com/Brxck2203/LenguajesProject1/go_service/internal/inbox"
	"github.com/Brxck2203/LenguajesProject1/go_service/internal/match"
	"github.com/Brxck2203/LenguajesProject1/go_service/internal/validation"
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

// InboxStatus reports the file-ingestion counters.
type InboxStatus interface {
	Stats() inbox.Stats
}

// Option configures optional routes.
type Option func(*Handler)

// WithCatalog publishes the event catalog at GET /catalog so producers can discover the format.
func WithCatalog(catalog validation.Catalog) Option {
	return func(handler *Handler) { handler.catalog = &catalog }
}

// WithInboxError reports why file ingestion could not start.
func WithInboxError(err error) Option {
	return func(handler *Handler) { handler.inboxError = err.Error() }
}

// WithInbox publishes the file-ingestion status at GET /inbox.
func WithInbox(status InboxStatus) Option {
	return func(handler *Handler) { handler.inbox = status }
}

func NewRouter(store EventStore, analysis AnalysisService, options ...Option) http.Handler {
	handler := &Handler{store: store, analysis: analysis}
	for _, option := range options {
		option(handler)
	}
	if handler.catalog == nil {
		catalog := validation.Default().Catalog()
		handler.catalog = &catalog
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/catalog", handler.handleCatalog)
	mux.HandleFunc("/inbox", handler.handleInbox)
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
