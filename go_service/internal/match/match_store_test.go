package match

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
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

func TestStoreRejectsGameMismatch(t *testing.T) {
	store := NewStore()
	start := matchEvent("start", "match_001", domain.TypeMatch, domain.ActionMatchStarted, domain.SystemPlayerID, domain.GameFootball, map[string]any{})
	if _, err := store.Add(start); err != nil {
		t.Fatalf("Add(MATCH_STARTED) returned error: %v", err)
	}
	event := matchEvent("mismatch", "match_001", domain.TypeAction, domain.ActionDamageDealt, "player_1", domain.GameFreeFire, map[string]any{
		"targetPlayerId": "player_2",
		"damage":         10,
	})
	if _, err := store.Add(event); !errors.Is(err, ErrGameMismatch) {
		t.Fatalf("Add() error = %v, want ErrGameMismatch", err)
	}
}

func TestStoreSnapshotPlayerIDsAreSorted(t *testing.T) {
	store := NewStore()
	start := matchEvent("start", "match_001", domain.TypeMatch, domain.ActionMatchStarted, domain.SystemPlayerID, domain.GameFootball, map[string]any{})
	if _, err := store.Add(start); err != nil {
		t.Fatalf("Add(MATCH_STARTED) returned error: %v", err)
	}

	for _, playerID := range []string{"player_z", "player_a", "player_m"} {
		event := matchEvent("pass_"+playerID, "match_001", domain.TypeAction, domain.ActionPass, playerID, domain.GameFootball, map[string]any{
			"receiverPlayerId": "receiver",
			"completed":        true,
		})
		if _, err := store.Add(event); err != nil {
			t.Fatalf("Add(PASS) for %s returned error: %v", playerID, err)
		}
	}

	snapshot, ok := store.Get("match_001")
	if !ok {
		t.Fatal("expected match snapshot")
	}
	expected := []string{"player_a", "player_m", "player_z"}
	if !reflect.DeepEqual(snapshot.PlayerIDs, expected) {
		t.Fatalf("snapshot.PlayerIDs = %v, want %v", snapshot.PlayerIDs, expected)
	}
}

func TestStoreAnalysisRequestOrdersEventsStablyByTimestamp(t *testing.T) {
	store := NewStore()
	start := matchEvent("start", "match_001", domain.TypeMatch, domain.ActionMatchStarted, domain.SystemPlayerID, domain.GameFootball, map[string]any{})
	start.Timestamp = timestampAt(22, 0, 0)
	if _, err := store.Add(start); err != nil {
		t.Fatalf("Add(MATCH_STARTED) returned error: %v", err)
	}

	late := matchEvent("evt_late", "match_001", domain.TypeAction, domain.ActionPass, "player_late", domain.GameFootball, map[string]any{
		"receiverPlayerId": "receiver",
		"completed":        true,
	})
	late.Timestamp = timestampAt(22, 0, 10)
	if _, err := store.Add(late); err != nil {
		t.Fatalf("Add(evt_late) returned error: %v", err)
	}

	sameTimeFirst := matchEvent("evt_same_1", "match_001", domain.TypeAction, domain.ActionPass, "player_first", domain.GameFootball, map[string]any{
		"receiverPlayerId": "receiver",
		"completed":        true,
	})
	sameTimeFirst.Timestamp = timestampAt(22, 0, 5)
	if _, err := store.Add(sameTimeFirst); err != nil {
		t.Fatalf("Add(evt_same_1) returned error: %v", err)
	}

	sameTimeSecond := matchEvent("evt_same_2", "match_001", domain.TypeAction, domain.ActionPass, "player_second", domain.GameFootball, map[string]any{
		"receiverPlayerId": "receiver",
		"completed":        true,
	})
	sameTimeSecond.Timestamp = timestampAt(22, 0, 5)
	if _, err := store.Add(sameTimeSecond); err != nil {
		t.Fatalf("Add(evt_same_2) returned error: %v", err)
	}

	finish := matchEvent("finish", "match_001", domain.TypeMatch, domain.ActionMatchFinished, domain.SystemPlayerID, domain.GameFootball, map[string]any{})
	finish.Timestamp = timestampAt(22, 0, 15)
	request, err := store.Add(finish)
	if err != nil {
		t.Fatalf("Add(MATCH_FINISHED) returned error: %v", err)
	}

	expectedRequestOrder := []string{"start", "evt_same_1", "evt_same_2", "evt_late", "finish"}
	if got := eventIDs(request.Events); !reflect.DeepEqual(got, expectedRequestOrder) {
		t.Fatalf("request event order = %v, want %v", got, expectedRequestOrder)
	}

	snapshot, ok := store.Get("match_001")
	if !ok {
		t.Fatal("expected match snapshot")
	}
	expectedArrivalOrder := []string{"start", "evt_late", "evt_same_1", "evt_same_2", "finish"}
	if got := eventIDs(snapshot.Events); !reflect.DeepEqual(got, expectedArrivalOrder) {
		t.Fatalf("snapshot event order = %v, want arrival order %v", got, expectedArrivalOrder)
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
	snapshot.PlayerIDs[0] = "injected"

	request, err := store.Add(matchEvent("finish", "match_001", domain.TypeMatch, domain.ActionMatchFinished, domain.SystemPlayerID, domain.GameFreeFire, map[string]any{}))
	if err != nil {
		t.Fatalf("Add(MATCH_FINISHED) returned error: %v", err)
	}
	if !json.Valid(request.Events[0].Data) {
		t.Fatal("mutating a snapshot changed the stored event data")
	}
	finalSnapshot, _ := store.Get("match_001")
	if reflect.DeepEqual(finalSnapshot.PlayerIDs, snapshot.PlayerIDs) {
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

func eventIDs(events []domain.Event) []string {
	ids := make([]string, len(events))
	for i, event := range events {
		ids[i] = event.EventID
	}
	return ids
}

func timestampAt(hour, minute, second int) time.Time {
	return time.Date(2026, time.September, 29, hour, minute, second, 0, time.UTC)
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
