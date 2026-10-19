package validation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/Brxck2203/LenguajesProject1/go_service/internal/domain"
)

// Validator checks events against a Catalog. It is immutable after creation and
// therefore safe for concurrent use.
type Validator struct {
	catalog Catalog
}

func NewValidator(catalog Catalog) (*Validator, error) {
	if err := catalog.check(); err != nil {
		return nil, err
	}
	return &Validator{catalog: catalog}, nil
}

var defaultValidator = mustDefaultValidator()

func mustDefaultValidator() *Validator {
	catalog, err := ParseCatalog(defaultCatalogJSON)
	if err != nil {
		panic(err) // the embedded catalog is covered by tests
	}
	return &Validator{catalog: catalog}
}

// Default returns the validator built from the embedded catalog.
func Default() *Validator { return defaultValidator }

// Catalog returns the catalog used by the validator.
func (validator *Validator) Catalog() Catalog { return validator.catalog }

// IsLifecycleAction reports whether action controls a match's lifecycle.
func IsLifecycleAction(action string) bool {
	return action == domain.ActionMatchStarted || action == domain.ActionMatchFinished
}

// ValidateEvent checks an event with the embedded default catalog.
func ValidateEvent(event domain.Event) error { return defaultValidator.Validate(event) }

// Validate checks the common event envelope and the data fields declared for the action.
func (validator *Validator) Validate(event domain.Event) error {
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

	game, exists := validator.catalog.Games[event.GameID]
	if !exists {
		return fmt.Errorf("unsupported gameId: %s (supported: %s)", event.GameID, strings.Join(validator.catalog.GameIDs(), ", "))
	}
	var spec ActionSpec
	if IsLifecycleAction(event.Action) {
		spec = validator.catalog.Lifecycle[event.Action]
	} else if spec, exists = game.Actions[event.Action]; !exists {
		return fmt.Errorf("action %s is not supported for gameId %s (supported: %s)", event.Action, event.GameID, strings.Join(actionNames(game), ", "))
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
	if err := checkPresent(data, validator.catalog.CommonOptional); err != nil {
		return err
	}
	for _, field := range sortedKeys(spec.Required) {
		if err := checkField(data, field, spec.Required[field]); err != nil {
			return err
		}
	}
	return checkPresent(data, spec.Optional)
}

func actionNames(game GameSpec) []string {
	names := make([]string, 0, len(game.Actions))
	for name := range game.Actions {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedKeys(fields map[string]FieldKind) []string {
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// checkPresent validates optional fields only when the producer includes them.
func checkPresent(data map[string]any, fields map[string]FieldKind) error {
	for _, field := range sortedKeys(fields) {
		if _, exists := data[field]; exists {
			if err := checkField(data, field, fields[field]); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkField(data map[string]any, field string, kind FieldKind) error {
	switch kind {
	case KindString:
		return requireNonEmptyString(data, field)
	case KindBoolean:
		if _, ok := data[field].(bool); !ok {
			return fmt.Errorf("data.%s must be a boolean", field)
		}
		return nil
	case KindPositiveNumber:
		return requirePositiveNumber(data, field)
	case KindPositiveInteger:
		return requirePositiveInteger(data, field)
	case KindNonNegativeInteger:
		value, ok := data[field].(float64)
		if !ok || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || math.Trunc(value) != value {
			return fmt.Errorf("data.%s must be a non-negative integer", field)
		}
		return nil
	default:
		return fmt.Errorf("data.%s has unsupported kind %s", field, kind)
	}
}

func validateEventType(event domain.Event) error {
	if IsLifecycleAction(event.Action) {
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
	if IsLifecycleAction(event.Action) {
		if event.PlayerID != domain.SystemPlayerID {
			return fmt.Errorf("action %s requires playerId SYSTEM", event.Action)
		}
	}
	return nil
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
		return fmt.Errorf("data.%s must be a non-empty string", field)
	}
	return nil
}

func requirePositiveNumber(data map[string]any, field string) error {
	value, ok := data[field].(float64)
	if !ok || math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return fmt.Errorf("data.%s must be a positive number", field)
	}
	return nil
}

func requirePositiveInteger(data map[string]any, field string) error {
	if err := requirePositiveNumber(data, field); err != nil {
		return err
	}
	value := data[field].(float64)
	if math.Trunc(value) != value {
		return fmt.Errorf("data.%s must be an integer", field)
	}
	return nil
}
