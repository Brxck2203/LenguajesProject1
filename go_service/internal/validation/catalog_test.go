package validation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Brxck2203/LenguajesProject1/go_service/internal/domain"
)

func TestEmbeddedCatalogDeclaresTheFourGames(t *testing.T) {
	catalog, err := ParseCatalog(DefaultCatalogJSON())
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(catalog.GameIDs(), ",")
	if got != "FOOTBALL,FREE_FIRE,MMA,MOTORCYCLE_RACING" {
		t.Fatalf("unexpected games: %s", got)
	}
}

func TestNewGameCanBeAddedOnlyWithACatalog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.json")
	content := `{"version":1,"games":{"CHESS":{"actions":{"MOVE":{"required":{"from":"string","to":"string"},"optional":{"capture":"boolean"}}}}}}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog, err := LoadCatalogFile(path)
	if err != nil {
		t.Fatal(err)
	}
	validator, err := NewValidator(catalog)
	if err != nil {
		t.Fatal(err)
	}
	move := validEvent("CHESS", domain.TypeAction, "MOVE", map[string]any{"from": "e2", "to": "e4", "capture": false})
	if err := validator.Validate(move); err != nil {
		t.Fatalf("expected chess move to be valid: %v", err)
	}
	start := validEvent("CHESS", domain.TypeMatch, domain.ActionMatchStarted, map[string]any{})
	start.PlayerID = domain.SystemPlayerID
	if err := validator.Validate(start); err != nil {
		t.Fatalf("lifecycle must work for every catalog game: %v", err)
	}
	bad := validEvent("CHESS", domain.TypeAction, "MOVE", map[string]any{"from": "e2", "to": "e4", "capture": "yes"})
	if err := validator.Validate(bad); err == nil {
		t.Fatal("expected optional field with wrong type to fail")
	}
	if err := validator.Validate(validEvent(domain.GameMMA, domain.TypeAction, domain.ActionStrikeLanded, map[string]any{"targetPlayerId": "x"})); err == nil {
		t.Fatal("a custom catalog replaces the default games")
	}
}

func TestInvalidCatalogsAreRejected(t *testing.T) {
	for name, content := range map[string]string{
		"unknown kind":        `{"version":1,"games":{"X":{"actions":{"A":{"required":{"f":"date"}}}}}}`,
		"lifecycle redefined": `{"version":1,"games":{"X":{"actions":{"MATCH_STARTED":{}}}}}`,
		"no games":            `{"version":1,"games":{}}`,
		"lower case game":     `{"version":1,"games":{"chess":{"actions":{"MOVE":{}}}}}`,
		"unknown property":    `{"version":1,"gamez":{}}`,
		"required lifecycle":  `{"version":1,"lifecycle":{"MATCH_FINISHED":{"required":{"winnerPlayerId":"string"}}},"games":{"X":{"actions":{"A":{}}}}}`,
	} {
		if _, err := ParseCatalog([]byte(content)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestMatchFinishedMayDeclareTheWinner(t *testing.T) {
	finish := validEvent(domain.GameFreeFire, domain.TypeMatch, domain.ActionMatchFinished, map[string]any{"winnerPlayerId": "p1"})
	finish.PlayerID = domain.SystemPlayerID
	if err := ValidateEvent(finish); err != nil {
		t.Fatalf("expected declared winner to be valid: %v", err)
	}
	finish = validEvent(domain.GameFreeFire, domain.TypeMatch, domain.ActionMatchFinished, map[string]any{"winnerPlayerId": ""})
	finish.PlayerID = domain.SystemPlayerID
	if err := ValidateEvent(finish); err == nil {
		t.Fatal("expected empty winner to fail")
	}
}
