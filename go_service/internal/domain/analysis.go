package domain

import (
	"encoding/json"
	"time"
)

// AnalysisResult contains Scala's output. Go never computes these metrics.
// All three JSON sections are required; empty objects/arrays are valid.
type AnalysisResult struct {
	MatchID     string          `json:"matchId"`
	GameID      string          `json:"gameId"`
	PlayerStats json.RawMessage `json:"playerStats"`
	MatchStats  json.RawMessage `json:"matchStats"`
	Patterns    json.RawMessage `json:"patterns"`
}

type AnalysisStatus string

const (
	AnalysisQueued    AnalysisStatus = "QUEUED"
	AnalysisRunning   AnalysisStatus = "PROCESSING"
	AnalysisRetrying  AnalysisStatus = "RETRYING"
	AnalysisCompleted AnalysisStatus = "COMPLETED"
	AnalysisFailed    AnalysisStatus = "FAILED"
)

type AnalysisSnapshot struct {
	MatchID    string          `json:"matchId"`
	GameID     string          `json:"gameId"`
	Status     AnalysisStatus  `json:"status"`
	EventCount int             `json:"eventCount"`
	Attempts   int             `json:"attempts"`
	CreatedAt  time.Time       `json:"createdAt"`
	UpdatedAt  time.Time       `json:"updatedAt"`
	LastError  string          `json:"lastError,omitempty"`
	Result     *AnalysisResult `json:"result,omitempty"`
}

func (request AnalysisRequest) Clone() AnalysisRequest {
	copyOfRequest := request
	copyOfRequest.Events = make([]Event, len(request.Events))
	for i, event := range request.Events {
		event.Data = append(json.RawMessage(nil), event.Data...)
		copyOfRequest.Events[i] = event
	}
	return copyOfRequest
}

func (result AnalysisResult) Clone() AnalysisResult {
	result.PlayerStats = append(json.RawMessage(nil), result.PlayerStats...)
	result.MatchStats = append(json.RawMessage(nil), result.MatchStats...)
	result.Patterns = append(json.RawMessage(nil), result.Patterns...)
	return result
}
