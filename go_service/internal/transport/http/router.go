package httptransport

import (
	"net/http"

	"github.com/Brxck2203/LenguajesProject1/go_service/internal/match"
)

func NewRouter(store *match.Store) http.Handler {
	handler := &Handler{store: store}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", handler.handleHealth)
	mux.HandleFunc("/events", handler.handleEvents)
	mux.HandleFunc("/events/batch", handler.handleEventsBatch)
	mux.HandleFunc("/matches/", handler.handleMatchByID)
	return mux
}
