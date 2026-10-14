package httptransport

import (
	"net/http"

	"github.com/Brxck2203/LenguajesProject1/go_service/internal/match"
)

func (handler *Handler) analysisMatch(writer http.ResponseWriter, request *http.Request) (string, bool) {
	matchID := request.PathValue("matchId")
	snapshot, exists := handler.store.Get(matchID)
	if !exists {
		writeError(writer, http.StatusNotFound, "match not found")
		return "", false
	}
	if snapshot.Status != match.StatusFinished {
		writeError(writer, http.StatusConflict, "match is still active")
		return "", false
	}
	if handler.analysis == nil {
		writeError(writer, http.StatusServiceUnavailable, "analysis service is not configured")
		return "", false
	}
	return matchID, true
}

func (handler *Handler) handleAnalysis(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	matchID, ok := handler.analysisMatch(writer, request)
	if !ok {
		return
	}
	snapshot, exists := handler.analysis.Analysis(matchID)
	if !exists {
		writeError(writer, http.StatusNotFound, "analysis not found")
		return
	}
	writeJSON(writer, http.StatusOK, snapshot)
}

func (handler *Handler) handleRetry(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	matchID, ok := handler.analysisMatch(writer, request)
	if !ok {
		return
	}
	if err := handler.analysis.Retry(matchID); err != nil {
		handler.writeAddError(writer, err)
		return
	}
	snapshot, _ := handler.analysis.Analysis(matchID)
	writeJSON(writer, http.StatusAccepted, snapshot)
}
