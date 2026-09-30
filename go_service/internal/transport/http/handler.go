package httptransport

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/Brxck2203/LenguajesProject1/go_service/internal/domain"
	"github.com/Brxck2203/LenguajesProject1/go_service/internal/match"
)

const maxRequestBodySize = 1 << 20

type Handler struct {
	store *match.Store
}

type events_batch_request struct {
	Events []domain.Event `json:"events"`
}

type ready_for_analysis_response struct {
	MatchID    string `json:"matchId"`
	GameID     string `json:"gameId"`
	EventCount int    `json:"eventCount"`
}

type accepted_event_response struct {
	Status      string `json:"status"`
	EventID     string `json:"eventId"`
	MatchID     string `json:"matchId"`
	MatchStatus string `json:"matchStatus"`
}

type accepted_batch_response struct {
	Status           string                        `json:"status"`
	Accepted         int                           `json:"accepted"`
	ReadyForAnalysis []ready_for_analysis_response `json:"readyForAnalysis"`
}

type partial_batch_response struct {
	Status        string `json:"status"`
	Accepted      int    `json:"accepted"`
	FailedAtIndex int    `json:"failedAtIndex"`
	Error         string `json:"error"`
}

type match_response struct {
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
		write_error(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	write_json(writer, http.StatusOK, struct {
		Status string `json:"status"`
	}{Status: "ok"})
}

func (handler *Handler) handleEvents(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		write_error(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !hasJSONContentType(request) {
		write_error(writer, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return
	}

	var event domain.Event
	if err := decodeSingleJSON(writer, request, &event); err != nil {
		write_error(writer, decodeErrorStatus(err), decodeErrorMessage(err))
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
		write_json(writer, http.StatusAccepted, accepted_event_response{
			Status:      "match_ready_for_analysis",
			EventID:     event.EventID,
			MatchID:     event.MatchID,
			MatchStatus: string(match.StatusFinished),
		})
		return
	}

	write_json(writer, http.StatusAccepted, accepted_event_response{
		Status:      "accepted",
		EventID:     event.EventID,
		MatchID:     event.MatchID,
		MatchStatus: string(match.StatusActive),
	})
}

func (handler *Handler) handleEventsBatch(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		write_error(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !hasJSONContentType(request) {
		write_error(writer, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return
	}

	var batch events_batch_request
	if err := decodeSingleJSON(writer, request, &batch); err != nil {
		write_json(writer, decodeErrorStatus(err), partial_batch_response{
			Status:        "partially_accepted",
			Accepted:      0,
			FailedAtIndex: 0,
			Error:         decodeErrorMessage(err),
		})
		return
	}
	if batch.Events == nil {
		write_json(writer, http.StatusBadRequest, partial_batch_response{
			Status:        "partially_accepted",
			Accepted:      0,
			FailedAtIndex: 0,
			Error:         "events is required and must be a non-empty array",
		})
		return
	}
	if len(batch.Events) == 0 {
		write_json(writer, http.StatusBadRequest, partial_batch_response{
			Status:        "partially_accepted",
			Accepted:      0,
			FailedAtIndex: 0,
			Error:         "events must not be empty",
		})
		return
	}

	readyMatches := make([]ready_for_analysis_response, 0)
	accepted := 0
	for index, event := range batch.Events {
		analysisRequest, err := handler.store.Add(event)
		if err != nil {
			status, message := addErrorResponse(err)
			if status == http.StatusInternalServerError {
				log.Printf("failed to add batch event at index %d: %v", index, err)
			}
			write_json(writer, status, partial_batch_response{
				Status:        "partially_accepted",
				Accepted:      accepted,
				FailedAtIndex: index,
				Error:         message,
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
			readyMatches = append(readyMatches, ready_for_analysis_response{
				MatchID:    analysisRequest.MatchID,
				GameID:     analysisRequest.GameID,
				EventCount: eventCount,
			})
		}
	}

	write_json(writer, http.StatusAccepted, accepted_batch_response{
		Status:           "accepted",
		Accepted:         accepted,
		ReadyForAnalysis: readyMatches,
	})
}

func (handler *Handler) handleMatchByID(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		write_error(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	matchID := strings.TrimPrefix(request.URL.Path, "/matches/")
	snapshot, exists := handler.store.Get(matchID)
	if !exists {
		write_error(writer, http.StatusNotFound, "match not found")
		return
	}

	playerIDs := snapshot.PlayerIDs
	if playerIDs == nil {
		playerIDs = []string{}
	}
	write_json(writer, http.StatusOK, match_response{
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
	if status == http.StatusInternalServerError {
		log.Printf("failed to add event: %v", err)
	}
	write_error(writer, status, message)
}

func addErrorResponse(err error) (int, string) {
	switch {
	case errors.Is(err, match.ErrDuplicateEvent),
		errors.Is(err, match.ErrMatchNotStarted),
		errors.Is(err, match.ErrMatchFinished),
		errors.Is(err, match.ErrGameMismatch),
		errors.Is(err, match.ErrMatchStarted):
		return http.StatusConflict, err.Error()
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
