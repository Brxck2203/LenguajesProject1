package validation

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Brxck2203/LenguajesProject1/go_service/internal/domain"
)

func TestValidateEventAcceptsBattleRoyaleElimination(t *testing.T) {
	event := validEvent(domain.GameFreeFire, domain.TypeAction, domain.ActionPlayerEliminated, map[string]any{
		"victimId": "player_456",
		"weapon":   "SNIPER",
		"damage":   150,
	})

	if err := ValidateEvent(event); err != nil {
		t.Fatalf("expected event to be valid, got error: %v", err)
	}
}

func TestValidateEventRejectsMissingRequiredField(t *testing.T) {
	event := validEvent(domain.GameFreeFire, domain.TypeAction, domain.ActionDamageDealt, map[string]any{
		"damage": 40,
	})

	if err := ValidateEvent(event); err == nil {
		t.Fatal("expected missing targetPlayerId to fail validation")
	}
}

func TestValidateEventRejectsWrongTypeForLifecycleEvent(t *testing.T) {
	event := validEvent(domain.GameMMA, domain.TypeAction, domain.ActionMatchStarted, map[string]any{})
	event.PlayerID = domain.SystemPlayerID

	if err := ValidateEvent(event); err == nil {
		t.Fatal("expected MATCH_STARTED with ACTION type to fail validation")
	}
}

func TestValidateEventAcceptsFootballPass(t *testing.T) {
	event := validEvent(domain.GameFootball, domain.TypeAction, domain.ActionPass, map[string]any{
		"receiverPlayerId": "player_8",
		"completed":        true,
	})

	if err := ValidateEvent(event); err != nil {
		t.Fatalf("expected pass to be valid, got error: %v", err)
	}
}

func TestValidateEventAcceptsLifecycleForEveryGame(t *testing.T) {
	games := []string{
		domain.GameMMA,
		domain.GameFootball,
		domain.GameMotorcycleRacing,
		domain.GameFreeFire,
	}
	for _, gameID := range games {
		t.Run(gameID, func(t *testing.T) {
			event := validEvent(gameID, domain.TypeMatch, domain.ActionMatchStarted, map[string]any{})
			event.PlayerID = domain.SystemPlayerID
			if err := ValidateEvent(event); err != nil {
				t.Fatalf("expected lifecycle event to be valid, got error: %v", err)
			}
		})
	}
}

func TestValidateEventRejectsLifecycleEventFromNonSystemPlayer(t *testing.T) {
	event := validEvent(domain.GameFreeFire, domain.TypeMatch, domain.ActionMatchFinished, map[string]any{})

	if err := ValidateEvent(event); err == nil {
		t.Fatal("expected MATCH_FINISHED from a non-SYSTEM player to fail validation")
	}
}

func TestValidateEventAcceptsCompletedLap(t *testing.T) {
	event := validEvent(domain.GameMotorcycleRacing, domain.TypeAction, domain.ActionLapCompleted, map[string]any{
		"lapNumber": 2,
		"lapTimeMs": 90500,
		"position":  1,
	})
	if err := ValidateEvent(event); err != nil {
		t.Fatalf("expected lap completion to be valid, got error: %v", err)
	}
}

func TestValidateEventRejectsActionForWrongGame(t *testing.T) {
	event := validEvent(domain.GameMMA, domain.TypeAction, domain.ActionPass, map[string]any{
		"receiverPlayerId": "player_8",
		"completed":        true,
	})
	if err := ValidateEvent(event); err == nil {
		t.Fatal("expected PASS for MMA to fail validation")
	}
}

func TestValidateEventRejectsNonObjectData(t *testing.T) {
	event := validEvent(domain.GameMMA, domain.TypeAction, domain.ActionStrikeLanded, map[string]any{
		"targetPlayerId": "fighter_b",
	})
	event.Data = json.RawMessage(`[]`)

	if err := ValidateEvent(event); err == nil {
		t.Fatal("expected array data to fail validation")
	}
}

func TestIsLifecycleAction(t *testing.T) {
	if !IsLifecycleAction(domain.ActionMatchStarted) || !IsLifecycleAction(domain.ActionMatchFinished) {
		t.Fatal("expected both match lifecycle actions to be recognized")
	}
	if IsLifecycleAction(domain.ActionPass) {
		t.Fatal("expected PASS not to be recognized as a lifecycle action")
	}
}

func validEvent(gameID, eventType, action string, data map[string]any) domain.Event {
	rawData, _ := json.Marshal(data)
	return domain.Event{
		EventID:   "evt_001",
		Timestamp: time.Date(2026, time.September, 29, 20, 0, 0, 0, time.UTC),
		GameID:    gameID,
		MatchID:   "match_001",
		PlayerID:  "player_123",
		Type:      eventType,
		Action:    action,
		Data:      rawData,
	}
}
