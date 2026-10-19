package validation

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/Brxck2203/LenguajesProject1/go_service/internal/domain"
)

// FieldKind names the accepted JSON type of a field inside event.data.
type FieldKind string

const (
	KindString             FieldKind = "string"
	KindBoolean            FieldKind = "boolean"
	KindPositiveNumber     FieldKind = "positiveNumber"
	KindPositiveInteger    FieldKind = "positiveInteger"
	KindNonNegativeInteger FieldKind = "nonNegativeInteger"
)

// ActionSpec lists the data fields an action must or may carry. Extra fields are allowed.
type ActionSpec struct {
	Description string               `json:"description,omitempty"`
	Required    map[string]FieldKind `json:"required,omitempty"`
	Optional    map[string]FieldKind `json:"optional,omitempty"`
}

type GameSpec struct {
	Description string                `json:"description,omitempty"`
	Actions     map[string]ActionSpec `json:"actions"`
}

// Catalog is the data-driven event contract. A new game or action is added by editing
// the JSON catalog, without changing Go code.
type Catalog struct {
	Version        int                   `json:"version"`
	Description    string                `json:"description,omitempty"`
	CommonOptional map[string]FieldKind  `json:"commonOptional,omitempty"`
	Lifecycle      map[string]ActionSpec `json:"lifecycle,omitempty"`
	Games          map[string]GameSpec   `json:"games"`
}

//go:embed event_catalog.json
var defaultCatalogJSON []byte

// DefaultCatalogJSON returns the embedded catalog document.
func DefaultCatalogJSON() []byte { return append([]byte(nil), defaultCatalogJSON...) }

// ParseCatalog decodes and checks a catalog document.
func ParseCatalog(raw []byte) (Catalog, error) {
	var catalog Catalog
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&catalog); err != nil {
		return Catalog{}, fmt.Errorf("invalid event catalog: %w", err)
	}
	if err := catalog.check(); err != nil {
		return Catalog{}, fmt.Errorf("invalid event catalog: %w", err)
	}
	return catalog, nil
}

// LoadCatalogFile reads a catalog from disk.
func LoadCatalogFile(path string) (Catalog, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Catalog{}, fmt.Errorf("read event catalog: %w", err)
	}
	return ParseCatalog(raw)
}

// GameIDs returns the supported games in alphabetical order.
func (catalog Catalog) GameIDs() []string {
	ids := make([]string, 0, len(catalog.Games))
	for id := range catalog.Games {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (catalog Catalog) check() error {
	if len(catalog.Games) == 0 {
		return fmt.Errorf("at least one game is required")
	}
	if err := checkFields("commonOptional", catalog.CommonOptional); err != nil {
		return err
	}
	for action, spec := range catalog.Lifecycle {
		if !IsLifecycleAction(action) {
			return fmt.Errorf("lifecycle only accepts %s and %s, got %s", domain.ActionMatchStarted, domain.ActionMatchFinished, action)
		}
		if len(spec.Required) > 0 {
			return fmt.Errorf("lifecycle action %s cannot declare required fields", action)
		}
		if err := checkFields("lifecycle."+action, spec.Optional); err != nil {
			return err
		}
	}
	for gameID, game := range catalog.Games {
		if strings.TrimSpace(gameID) == "" || gameID != strings.ToUpper(gameID) {
			return fmt.Errorf("game id %q must be a non-empty UPPER_CASE name", gameID)
		}
		if len(game.Actions) == 0 {
			return fmt.Errorf("game %s needs at least one action", gameID)
		}
		for action, spec := range game.Actions {
			if IsLifecycleAction(action) {
				return fmt.Errorf("game %s must not redefine lifecycle action %s", gameID, action)
			}
			if strings.TrimSpace(action) == "" {
				return fmt.Errorf("game %s has an empty action name", gameID)
			}
			if err := checkFields(gameID+"."+action+".required", spec.Required); err != nil {
				return err
			}
			if err := checkFields(gameID+"."+action+".optional", spec.Optional); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkFields(where string, fields map[string]FieldKind) error {
	for name, kind := range fields {
		switch kind {
		case KindString, KindBoolean, KindPositiveNumber, KindPositiveInteger, KindNonNegativeInteger:
		default:
			return fmt.Errorf("%s.%s has unknown kind %q", where, name, kind)
		}
	}
	return nil
}
