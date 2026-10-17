package match

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/Brxck2203/LenguajesProject1/go_service/internal/domain"
)

type Status string

const (
	StatusActive   Status = "ACTIVE"
	StatusFinished Status = "FINISHED"
)

type MatchState struct {
	latestTimestamp time.Time
	MatchID         string
	GameID          string
	Status          Status
	Events          []domain.Event
	PlayerIDs       map[string]struct{}
	StartedAt       time.Time
	FinishedAt      *time.Time
}

type MatchSnapshot struct {
	MatchID    string
	GameID     string
	Status     Status
	Events     []domain.Event
	PlayerIDs  []string
	StartedAt  time.Time
	FinishedAt *time.Time
}

func newMatchState(event domain.Event) *MatchState {
	state := &MatchState{
		MatchID:   event.MatchID,
		GameID:    event.GameID,
		Status:    StatusActive,
		PlayerIDs: make(map[string]struct{}),
		StartedAt: event.Timestamp,
	}
	state.addEvent(event)
	return state
}

func (state *MatchState) addEvent(event domain.Event) {
	state.Events = append(state.Events, cloneEvent(event))
	if event.Timestamp.After(state.latestTimestamp) {
		state.latestTimestamp = event.Timestamp
	}
	var participants map[string]json.RawMessage
	if json.Unmarshal(event.Data, &participants) == nil {
		for _, key := range []string{"targetPlayerId", "victimId", "receiverPlayerId"} {
			var playerID string
			if json.Unmarshal(participants[key], &playerID) == nil && playerID != "" && playerID != domain.SystemPlayerID {
				state.PlayerIDs[playerID] = struct{}{}
			}
		}
	}
	if event.PlayerID != domain.SystemPlayerID {
		state.PlayerIDs[event.PlayerID] = struct{}{}
	}
}

func (state *MatchState) Snapshot() MatchSnapshot {
	snapshot := MatchSnapshot{
		MatchID:   state.MatchID,
		GameID:    state.GameID,
		Status:    state.Status,
		Events:    cloneEvents(state.Events),
		StartedAt: state.StartedAt,
	}
	for playerID := range state.PlayerIDs {
		snapshot.PlayerIDs = append(snapshot.PlayerIDs, playerID)
	}
	sort.Strings(snapshot.PlayerIDs)
	if state.FinishedAt != nil {
		finishedAt := *state.FinishedAt
		snapshot.FinishedAt = &finishedAt
	}
	return snapshot
}

func cloneEvents(events []domain.Event) []domain.Event {
	cloned := make([]domain.Event, len(events))
	for i, event := range events {
		cloned[i] = cloneEvent(event)
	}
	return cloned
}

func cloneEvent(event domain.Event) domain.Event {
	event.Data = append(json.RawMessage(nil), event.Data...)
	return event
}
