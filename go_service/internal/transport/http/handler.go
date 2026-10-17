package httptransport

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Brxck2203/LenguajesProject1/go_service/internal/domain"
	"github.com/Brxck2203/LenguajesProject1/go_service/internal/match"
	"github.com/Brxck2203/LenguajesProject1/go_service/internal/processing"
)

const maxRequestBodySize = 1 << 20

type Handler struct {
	store    EventStore
	analysis AnalysisService
}

type eventsBatchRequest struct {
	Events []domain.Event `json:"events"`
}

type readyForAnalysisResponse struct {
	MatchID    string `json:"matchId"`
	GameID     string `json:"gameId"`
	EventCount int    `json:"eventCount"`
}

type acceptedEventResponse struct {
	EventCount  int    `json:"eventCount,omitempty"`
	Status      string `json:"status"`
	EventID     string `json:"eventId"`
	MatchID     string `json:"matchId"`
	MatchStatus string `json:"matchStatus"`
}

type acceptedBatchResponse struct {
	Status           string                     `json:"status"`
	Accepted         int                        `json:"accepted"`
	ReadyForAnalysis []readyForAnalysisResponse `json:"readyForAnalysis"`
}

type partialBatchResponse struct {
	ReadyForAnalysis []readyForAnalysisResponse `json:"readyForAnalysis"`
	Status           string                     `json:"status"`
	Accepted         int                        `json:"accepted"`
	FailedAtIndex    int                        `json:"failedAtIndex"`
	Error            string                     `json:"error"`
}

type matchResponse struct {
	MatchID    string     `json:"matchId"`
	GameID     string     `json:"gameId"`
	Status     string     `json:"status"`
	EventCount int        `json:"eventCount"`
	PlayerIDs  []string   `json:"playerIds"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt"`
}

func (handler *Handler) handleHealth(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(writer, http.StatusOK, struct {
		Status string `json:"status"`
	}{Status: "ok"})
}

func (handler *Handler) handleEvents(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !hasJSONContentType(request) {
		writeError(writer, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return
	}

	var event domain.Event
	if err := decodeSingleJSON(writer, request, &event); err != nil {
		writeError(writer, decodeErrorStatus(err), decodeErrorMessage(err))
		return
	}

	analysisRequest, err := handler.store.Add(event)
	if err != nil {
		handler.writeAddError(writer, err)
		return
	}

	if analysisRequest != nil {
		log.Printf(
			"match ready for analysis: matchId=%s gameId=%s eventCount=%d",
			analysisRequest.MatchID,
			analysisRequest.GameID,
			len(analysisRequest.Events),
		)
		writeJSON(writer, http.StatusAccepted, acceptedEventResponse{
			Status:      "match_ready_for_analysis",
			EventCount:  len(analysisRequest.Events),
			EventID:     event.EventID,
			MatchID:     event.MatchID,
			MatchStatus: string(match.StatusFinished),
		})
		return
	}

	writeJSON(writer, http.StatusAccepted, acceptedEventResponse{
		Status:      "accepted",
		EventID:     event.EventID,
		MatchID:     event.MatchID,
		MatchStatus: string(match.StatusActive),
	})
}

func (handler *Handler) handleEventsBatch(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !hasJSONContentType(request) {
		writeError(writer, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return
	}

	var batch eventsBatchRequest
	if err := decodeSingleJSON(writer, request, &batch); err != nil {
		writeJSON(writer, decodeErrorStatus(err), partialBatchResponse{
			Status:           "partially_accepted",
			Accepted:         0,
			ReadyForAnalysis: []readyForAnalysisResponse{},
			FailedAtIndex:    0,
			Error:            decodeErrorMessage(err),
		})
		return
	}
	if batch.Events == nil {
		writeJSON(writer, http.StatusBadRequest, partialBatchResponse{
			Status:           "partially_accepted",
			Accepted:         0,
			ReadyForAnalysis: []readyForAnalysisResponse{},
			FailedAtIndex:    0,
			Error:            "events is required and must be a non-empty array",
		})
		return
	}
	if len(batch.Events) == 0 {
		writeJSON(writer, http.StatusBadRequest, partialBatchResponse{
			Status:           "partially_accepted",
			Accepted:         0,
			ReadyForAnalysis: []readyForAnalysisResponse{},
			FailedAtIndex:    0,
			Error:            "events must not be empty",
		})
		return
	}

	readyMatches := make([]readyForAnalysisResponse, 0)
	accepted := 0
	for index, event := range batch.Events {
		analysisRequest, err := handler.store.Add(event)
		if err != nil {
			status, message := addErrorResponse(err)
			if status == http.StatusServiceUnavailable {
				writer.Header().Set("Retry-After", "1")
			}
			if status == http.StatusInternalServerError {
				log.Printf("failed to add batch event at index %d: %v", index, err)
			}
			writeJSON(writer, status, partialBatchResponse{
				Status:           "partially_accepted",
				Accepted:         accepted,
				ReadyForAnalysis: readyMatches,
				FailedAtIndex:    index,
				Error:            message,
			})
			return
		}
		accepted++
		if analysisRequest != nil {
			eventCount := len(analysisRequest.Events)
			log.Printf(
				"match ready for analysis: matchId=%s gameId=%s eventCount=%d",
				analysisRequest.MatchID,
				analysisRequest.GameID,
				eventCount,
			)
			readyMatches = append(readyMatches, readyForAnalysisResponse{
				MatchID:    analysisRequest.MatchID,
				GameID:     analysisRequest.GameID,
				EventCount: eventCount,
			})
		}
	}

	writeJSON(writer, http.StatusAccepted, acceptedBatchResponse{
		Status:           "accepted",
		Accepted:         accepted,
		ReadyForAnalysis: readyMatches,
	})
}

func (handler *Handler) handleMatchByID(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	matchID := request.PathValue("matchId")
	snapshot, exists := handler.store.Get(matchID)
	if !exists {
		writeError(writer, http.StatusNotFound, "match not found")
		return
	}

	playerIDs := snapshot.PlayerIDs
	if playerIDs == nil {
		playerIDs = []string{}
	}
	writeJSON(writer, http.StatusOK, matchResponse{
		MatchID:    snapshot.MatchID,
		GameID:     snapshot.GameID,
		Status:     string(snapshot.Status),
		EventCount: len(snapshot.Events),
		PlayerIDs:  playerIDs,
		StartedAt:  snapshot.StartedAt,
		FinishedAt: snapshot.FinishedAt,
	})
}

func (handler *Handler) writeAddError(writer http.ResponseWriter, err error) {
	status, message := addErrorResponse(err)
	if status == http.StatusServiceUnavailable {
		writer.Header().Set("Retry-After", "1")
	}
	if status == http.StatusInternalServerError {
		log.Printf("failed to add event: %v", err)
	}
	writeError(writer, status, message)
}

func addErrorResponse(err error) (int, string) {
	switch {
	case errors.Is(err, match.ErrDuplicateEvent),
		errors.Is(err, match.ErrMatchNotStarted),
		errors.Is(err, match.ErrMatchFinished),
		errors.Is(err, match.ErrGameMismatch),
		errors.Is(err, match.ErrMatchStarted),
		errors.Is(err, match.ErrInvalidTimeline),
		errors.Is(err, processing.ErrRetryConflict):
		return http.StatusConflict, err.Error()
	case errors.Is(err, processing.ErrQueueFull), errors.Is(err, processing.ErrStopping):
		return http.StatusServiceUnavailable, err.Error()
	case errors.Is(err, processing.ErrAnalysisNotFound):
		return http.StatusNotFound, err.Error()
	case errors.Is(err, match.ErrInvalidEvent):
		return http.StatusBadRequest, err.Error()
	default:
		return http.StatusInternalServerError, "internal server error"
	}
}

func hasJSONContentType(request *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	return err == nil && strings.EqualFold(mediaType, "application/json")
}

func decodeSingleJSON(writer http.ResponseWriter, request *http.Request, destination any) error {
	request.Body = http.MaxBytesReader(writer, request.Body, maxRequestBodySize)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("request body must contain exactly one JSON value")
		}
		return err
	}
	return nil
}

func decodeErrorStatus(err error) int {
	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}

func decodeErrorMessage(err error) string {
	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) {
		return "request body must not exceed 1 MB"
	}
	return err.Error()
}

func (handler *Handler) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"service":   "GameStats Go",
		"health":    "/health",
		"matches":   "/matches",
		"ingestion": "POST /events or POST /events/batch",
		"note":      "Run the simulator to create matches. Health does not verify Scala; query each analysis URL for its status.",
	})
}

func (handler *Handler) handleMatches(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	type item struct {
		MatchID     string `json:"matchId"`
		SummaryURL  string `json:"summaryUrl"`
		AnalysisURL string `json:"analysisUrl"`
	}
	items := make([]item, 0)
	for _, id := range handler.store.MatchIDs() {
		path := "/matches/" + url.PathEscape(id)
		items = append(items, item{MatchID: id, SummaryURL: path, AnalysisURL: path + "/analysis"})
	}
	writeJSON(w, http.StatusOK, map[string]any{"matches": items, "count": len(items)})
}
