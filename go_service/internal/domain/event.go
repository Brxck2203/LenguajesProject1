package domain

import (
	"encoding/json"
	"time"
)

const (
	GameMMA              = "MMA"
	GameFootball         = "FOOTBALL"
	GameMotorcycleRacing = "MOTORCYCLE_RACING"
	GameFreeFire         = "FREE_FIRE"

	TypeAction = "ACTION"
	TypeMatch  = "MATCH"

	ActionMatchStarted  = "MATCH_STARTED"
	ActionMatchFinished = "MATCH_FINISHED"

	ActionStrikeAttempt   = "STRIKE_ATTEMPT"
	ActionStrikeLanded    = "STRIKE_LANDED"
	ActionTakedown        = "TAKEDOWN"
	ActionKnockdown       = "KNOCKDOWN"
	ActionRefereeStoppage = "REFEREE_STOPPAGE"

	ActionPass         = "PASS"
	ActionGoal         = "GOAL"
	ActionInterception = "INTERCEPTION"
	ActionFoul         = "FOUL"

	ActionLapCompleted = "LAP_COMPLETED"
	ActionCrash        = "CRASH"
	ActionPitStop      = "PIT_STOP"
	ActionRejoinRace   = "REJOIN_RACE"

	ActionDamageDealt      = "DAMAGE_DEALT"
	ActionPlayerEliminated = "PLAYER_ELIMINATED"
	ActionZoneShrink       = "ZONE_SHRINK"

	SystemPlayerID = "SYSTEM"
)

// Event is the common JSON envelope exchanged by the event producer, Go, and Scala.
type Event struct {
	EventID   string          `json:"eventId"`
	Timestamp time.Time       `json:"timestamp"`
	GameID    string          `json:"gameId"`
	MatchID   string          `json:"matchId"`
	PlayerID  string          `json:"playerId"`
	Type      string          `json:"type"`
	Action    string          `json:"action"`
	Data      json.RawMessage `json:"data"`
}

// AnalysisRequest is the payload Go will send to Scala after a match is finished.
type AnalysisRequest struct {
	MatchID string  `json:"matchId"`
	GameID  string  `json:"gameId"`
	Events  []Event `json:"events"`
}
