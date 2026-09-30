package match

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Brxck2203/LenguajesProject1/go_service/internal/domain"
)

func TestStoreAddCompletesMatchAndBuildsAnalysisRequest(t *testing.T) {
	store := NewStore()

	start := matchEvent("match_001_start", "match_001", domain.TypeMatch, domain.ActionMatchStarted, domain.SystemPlayerID, domain.GameFreeFire, map[string]any{})
	if request, err := store.Add(start); err != nil || request != nil {
		t.Fatalf("Add(MATCH_STARTED) = (%v, %v), want (nil, nil)", request, err)
	}

	damage := matchEvent("match_001_damage", "match_001", domain.TypeAction, domain.ActionDamageDealt, "player_1", domain.GameFreeFire, map[string]any{
		"targetPlayerId": "player_2",
		"damage":         50,
	})
	if request, err := store.Add(damage); err != nil || request != nil {
		t.Fatalf("Add(DAMAGE_DEALT) = (%v, %v), want (nil, nil)", request, err)
	}

	finish := matchEvent("match_001_finish", "match_001", domain.TypeMatch, domain.ActionMatchFinished, domain.SystemPlayerID, domain.GameFreeFire, map[string]any{})
	request, err := store.Add(finish)
	if err != nil {
		t.Fatalf("Add(MATCH_FINISHED) returned error: %v", err)
	}
	if request == nil || request.MatchID != "match_001" || request.GameID != domain.GameFreeFire || len(request.Events) != 3 {
		t.Fatalf("unexpected analysis request: %#v", request)
	}

	snapshot, ok := store.Get("match_001")
	if !ok {
		t.Fatal("expected finished match snapshot")
	}
	if snapshot.Status != StatusFinished {
		t.Fatalf("snapshot status = %q, want %q", snapshot.Status, StatusFinished)
	}
	if snapshot.FinishedAt == nil {
		t.Fatal("expected FinishedAt to be set")
	}
	if store.ActiveCount() != 0 {
		t.Fatalf("ActiveCount() = %d, want 0", store.ActiveCount())
	}
}

func TestStoreRejectsActionBeforeMatchStarted(t *testing.T) {
	store := NewStore()
	event := matchEvent("pass_1", "match_001", domain.TypeAction, domain.ActionPass, "player_1", domain.GameFootball, map[string]any{
		"receiverPlayerId": "player_2",
		"completed":        true,
	})
	if _, err := store.Add(event); !errors.Is(err, ErrMatchNotStarted) {
		t.Fatalf("Add() error = %v, want ErrMatchNotStarted", err)
	}
}

func TestStoreRejectsDuplicateEventID(t *testing.T) {
	store := NewStore()
	start := matchEvent("shared_id", "match_001", domain.TypeMatch, domain.ActionMatchStarted, domain.SystemPlayerID, domain.GameFreeFire, map[string]any{})
	if _, err := store.Add(start); err != nil {
		t.Fatalf("Add(MATCH_STARTED) returned error: %v", err)
	}
	duplicate := matchEvent("shared_id", "match_001", domain.TypeAction, domain.ActionDamageDealt, "player_1", domain.GameFreeFire, map[string]any{
		"targetPlayerId": "player_2",
		"damage":         10,
	})
	if _, err := store.Add(duplicate); !errors.Is(err, ErrDuplicateEvent) {
		t.Fatalf("Add() error = %v, want ErrDuplicateEvent", err)
	}
}

func TestStoreRejectsActionAfterMatchFinished(t *testing.T) {
	store := NewStore()
	addMatchStartAndFinish(t, store, "match_001")
	event := matchEvent("late_action", "match_001", domain.TypeAction, domain.ActionDamageDealt, "player_1", domain.GameFreeFire, map[string]any{
		"targetPlayerId": "player_2",
		"damage":         10,
	})
	if _, err := store.Add(event); !errors.Is(err, ErrMatchFinished) {
		t.Fatalf("Add() error = %v, want ErrMatchFinished", err)
	}
}

func TestStoreProcessesConcurrentMatches(t *testing.T) {
	const matchCount = 12
	store := NewStore()
	var waitGroup sync.WaitGroup
	errorsByMatch := make(chan error, matchCount)

	for i := 0; i < matchCount; i++ {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()
			matchID := fmt.Sprintf("match_%02d", index)
			events := []domain.Event{
				matchEvent(matchID+"_start", matchID, domain.TypeMatch, domain.ActionMatchStarted, domain.SystemPlayerID, domain.GameFootball, map[string]any{}),
				matchEvent(matchID+"_pass", matchID, domain.TypeAction, domain.ActionPass, "player_1", domain.GameFootball, map[string]any{
					"receiverPlayerId": "player_2",
					"completed":        true,
				}),
				matchEvent(matchID+"_finish", matchID, domain.TypeMatch, domain.ActionMatchFinished, domain.SystemPlayerID, domain.GameFootball, map[string]any{}),
			}
			for _, event := range events {
				if _, err := store.Add(event); err != nil {
					errorsByMatch <- fmt.Errorf("%s: %w", matchID, err)
					return
				}
			}
		}(i)
	}
	waitGroup.Wait()
	close(errorsByMatch)
	for err := range errorsByMatch {
		t.Error(err)
	}

	if count := store.ActiveCount(); count != 0 {
		t.Fatalf("ActiveCount() = %d, want 0", count)
	}
}

func TestStoreSnapshotsAreDefensiveCopies(t *testing.T) {
	store := NewStore()
	start := matchEvent("start", "match_001", domain.TypeMatch, domain.ActionMatchStarted, domain.SystemPlayerID, domain.GameFreeFire, map[string]any{})
	if _, err := store.Add(start); err != nil {
		t.Fatalf("Add(MATCH_STARTED) returned error: %v", err)
	}
	action := matchEvent("action", "match_001", domain.TypeAction, domain.ActionDamageDealt, "player_1", domain.GameFreeFire, map[string]any{
		"targetPlayerId": "player_2",
		"damage":         10,
	})
	if _, err := store.Add(action); err != nil {
		t.Fatalf("Add(DAMAGE_DEALT) returned error: %v", err)
	}

	snapshot, _ := store.Get("match_001")
	snapshot.Events[0].Data[0] = '['
	snapshot.PlayerIDs["injected"] = struct{}{}

	request, err := store.Add(matchEvent("finish", "match_001", domain.TypeMatch, domain.ActionMatchFinished, domain.SystemPlayerID, domain.GameFreeFire, map[string]any{}))
	if err != nil {
		t.Fatalf("Add(MATCH_FINISHED) returned error: %v", err)
	}
	if !json.Valid(request.Events[0].Data) {
		t.Fatal("mutating a snapshot changed the stored event data")
	}
	finalSnapshot, _ := store.Get("match_001")
	if _, exists := finalSnapshot.PlayerIDs["injected"]; exists {
		t.Fatal("mutating a snapshot changed the stored player IDs")
	}
	expectedFinishedAt := *finalSnapshot.FinishedAt
	*finalSnapshot.FinishedAt = time.Time{}
	reloadedSnapshot, _ := store.Get("match_001")
	if !reloadedSnapshot.FinishedAt.Equal(expectedFinishedAt) {
		t.Fatal("mutating a snapshot changed the stored FinishedAt")
	}
}

func addMatchStartAndFinish(t *testing.T, store *Store, matchID string) {
	t.Helper()
	start := matchEvent(matchID+"_start", matchID, domain.TypeMatch, domain.ActionMatchStarted, domain.SystemPlayerID, domain.GameFreeFire, map[string]any{})
	if _, err := store.Add(start); err != nil {
		t.Fatalf("Add(MATCH_STARTED) returned error: %v", err)
	}
	finish := matchEvent(matchID+"_finish", matchID, domain.TypeMatch, domain.ActionMatchFinished, domain.SystemPlayerID, domain.GameFreeFire, map[string]any{})
	if _, err := store.Add(finish); err != nil {
		t.Fatalf("Add(MATCH_FINISHED) returned error: %v", err)
	}
}

func matchEvent(eventID, matchID, eventType, action, playerID, gameID string, data map[string]any) domain.Event {
	rawData, err := json.Marshal(data)
	if err != nil {
		panic(err)
	}
	return domain.Event{
		EventID:   eventID,
		Timestamp: time.Date(2026, time.September, 29, 20, 0, 0, 0, time.UTC),
		GameID:    gameID,
		MatchID:   matchID,
		PlayerID:  playerID,
		Type:      eventType,
		Action:    action,
		Data:      rawData,
	}
}
