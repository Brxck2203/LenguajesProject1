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

func TestValidateEventRejectsNonObjectData(t *testing.T) {
	event := validEvent(domain.GameMMA, domain.TypeAction, domain.ActionStrikeLanded, map[string]any{
		"targetPlayerId": "fighter_b",
	})
	event.Data = json.RawMessage(`[]`)

	if err := ValidateEvent(event); err == nil {
		t.Fatal("expected array data to fail validation")
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
