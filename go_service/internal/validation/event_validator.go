package validation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/Brxck2203/LenguajesProject1/go_service/internal/domain"
)

var allowedActionsByGame = map[string]map[string]struct{}{
	domain.GameMMA: {
		domain.ActionMatchStarted: {}, domain.ActionMatchFinished: {},
		domain.ActionStrikeAttempt: {}, domain.ActionStrikeLanded: {},
		domain.ActionTakedown: {}, domain.ActionKnockdown: {}, domain.ActionRefereeStoppage: {},
	},
	domain.GameFootball: {
		domain.ActionMatchStarted: {}, domain.ActionMatchFinished: {},
		domain.ActionPass: {}, domain.ActionGoal: {}, domain.ActionInterception: {}, domain.ActionFoul: {},
	},
	domain.GameMotorcycleRacing: {
		domain.ActionMatchStarted: {}, domain.ActionMatchFinished: {},
		domain.ActionLapCompleted: {}, domain.ActionCrash: {}, domain.ActionPitStop: {}, domain.ActionRejoinRace: {},
	},
	domain.GameFreeFire: {
		domain.ActionMatchStarted: {}, domain.ActionMatchFinished: {},
		domain.ActionDamageDealt: {}, domain.ActionPlayerEliminated: {}, domain.ActionZoneShrink: {},
	},
}

// ValidateEvent checks the common event envelope and the minimum data required by each action.
func ValidateEvent(event domain.Event) error {
	if strings.TrimSpace(event.EventID) == "" {
		return fmt.Errorf("eventId is required")
	}
	if event.Timestamp.IsZero() {
		return fmt.Errorf("timestamp is required and must use RFC 3339")
	}
	if strings.TrimSpace(event.GameID) == "" {
		return fmt.Errorf("gameId is required")
	}
	if strings.TrimSpace(event.MatchID) == "" {
		return fmt.Errorf("matchId is required")
	}
	if strings.TrimSpace(event.PlayerID) == "" {
		return fmt.Errorf("playerId is required")
	}
	if strings.TrimSpace(event.Type) == "" {
		return fmt.Errorf("type is required")
	}
	if strings.TrimSpace(event.Action) == "" {
		return fmt.Errorf("action is required")
	}
	if !isJSONObject(event.Data) {
		return fmt.Errorf("data must be a JSON object")
	}

	actions, exists := allowedActionsByGame[event.GameID]
	if !exists {
		return fmt.Errorf("unsupported gameId: %s", event.GameID)
	}
	if _, exists := actions[event.Action]; !exists {
		return fmt.Errorf("action %s is not supported for gameId %s", event.Action, event.GameID)
	}
	if err := validateEventType(event); err != nil {
		return err
	}
	if err := validateLifecycleActor(event); err != nil {
		return err
	}

	data, err := decodeData(event.Data)
	if err != nil {
		return err
	}
	return validateActionData(event.Action, data)
}

func validateEventType(event domain.Event) error {
	if event.Action == domain.ActionMatchStarted || event.Action == domain.ActionMatchFinished {
		if event.Type != domain.TypeMatch {
			return fmt.Errorf("action %s requires type MATCH", event.Action)
		}
		return nil
	}
	if event.Type != domain.TypeAction {
		return fmt.Errorf("action %s requires type ACTION", event.Action)
	}
	return nil
}

func validateLifecycleActor(event domain.Event) error {
	if event.Action == domain.ActionMatchStarted || event.Action == domain.ActionMatchFinished {
		if event.PlayerID != domain.SystemPlayerID {
			return fmt.Errorf("action %s requires playerId SYSTEM", event.Action)
		}
	}
	return nil
}

func validateActionData(action string, data map[string]any) error {
	switch action {
	case domain.ActionStrikeAttempt, domain.ActionStrikeLanded, domain.ActionTakedown, domain.ActionKnockdown, domain.ActionInterception, domain.ActionFoul:
		return requireNonEmptyString(data, "targetPlayerId")
	case domain.ActionRefereeStoppage, domain.ActionPlayerEliminated:
		return requireNonEmptyString(data, "victimId")
	case domain.ActionPass:
		if err := requireNonEmptyString(data, "receiverPlayerId"); err != nil {
			return err
		}
		if _, ok := data["completed"].(bool); !ok {
			return fmt.Errorf("data.completed must be a boolean")
		}
		return nil
	case domain.ActionLapCompleted:
		if err := requirePositiveNumber(data, "lapNumber"); err != nil {
			return err
		}
		if err := requirePositiveNumber(data, "lapTimeMs"); err != nil {
			return err
		}
		return requirePositiveNumber(data, "position")
	case domain.ActionDamageDealt:
		if err := requireNonEmptyString(data, "targetPlayerId"); err != nil {
			return err
		}
		return requirePositiveNumber(data, "damage")
	default:
		return nil
	}
}

func isJSONObject(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) >= 2 && trimmed[0] == '{' && trimmed[len(trimmed)-1] == '}'
}

func decodeData(raw json.RawMessage) (map[string]any, error) {
	data := make(map[string]any)
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("data must be a valid JSON object: %w", err)
	}
	return data, nil
}

func requireNonEmptyString(data map[string]any, field string) error {
	value, ok := data[field].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return fmt.Errorf("data.%s is required and must be a non-empty string", field)
	}
	return nil
}

func requirePositiveNumber(data map[string]any, field string) error {
	value, ok := data[field].(float64)
	if !ok || math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return fmt.Errorf("data.%s is required and must be a positive number", field)
	}
	return nil
}
