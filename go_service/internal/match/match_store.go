package match

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/Brxck2203/LenguajesProject1/go_service/internal/domain"
	"github.com/Brxck2203/LenguajesProject1/go_service/internal/validation"
)

var (
	ErrDuplicateEvent  = errors.New("eventId was already processed")
	ErrMatchNotStarted = errors.New("match has not started")
	ErrMatchFinished   = errors.New("match is already finished")
	ErrGameMismatch    = errors.New("gameId does not match the active match")
	ErrInvalidEvent    = errors.New("invalid event")
	ErrMatchStarted    = errors.New("match has already started")
)

type Store struct {
	mu       sync.RWMutex
	matches  map[string]*MatchState
	eventIDs map[string]struct{}
}

func NewStore() *Store {
	return &Store{
		matches:  make(map[string]*MatchState),
		eventIDs: make(map[string]struct{}),
	}
}

func (store *Store) Add(event domain.Event) (*domain.AnalysisRequest, error) {
	if err := validation.ValidateEvent(event); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidEvent, err)
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	if _, exists := store.eventIDs[event.EventID]; exists {
		return nil, fmt.Errorf("%w: %s", ErrDuplicateEvent, event.EventID)
	}

	state, exists := store.matches[event.MatchID]
	if !exists {
		if event.Action != domain.ActionMatchStarted {
			return nil, fmt.Errorf("%w: matchId %s", ErrMatchNotStarted, event.MatchID)
		}
		state = newMatchState(event)
		store.matches[event.MatchID] = state
		store.eventIDs[event.EventID] = struct{}{}
		return nil, nil
	}

	if state.GameID != event.GameID {
		return nil, fmt.Errorf("%w: matchId %s", ErrGameMismatch, event.MatchID)
	}
	if state.Status == StatusFinished {
		return nil, fmt.Errorf("%w: matchId %s", ErrMatchFinished, event.MatchID)
	}
	if event.Action == domain.ActionMatchStarted {
		return nil, fmt.Errorf("%w: matchId %s", ErrMatchStarted, event.MatchID)
	}

	state.addEvent(event)
	store.eventIDs[event.EventID] = struct{}{}
	if event.Action != domain.ActionMatchFinished {
		return nil, nil
	}

	finishedAt := event.Timestamp
	state.Status = StatusFinished
	state.FinishedAt = &finishedAt
	events := cloneEvents(state.Events)
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})
	return &domain.AnalysisRequest{
		MatchID: state.MatchID,
		GameID:  state.GameID,
		Events:  events,
	}, nil
}

func (store *Store) Get(matchID string) (MatchSnapshot, bool) {
	store.mu.RLock()
	defer store.mu.RUnlock()

	state, exists := store.matches[matchID]
	if !exists {
		return MatchSnapshot{}, false
	}
	return state.Snapshot(), true
}

func (store *Store) ActiveCount() int {
	store.mu.RLock()
	defer store.mu.RUnlock()

	active := 0
	for _, state := range store.matches {
		if state.Status == StatusActive {
			active++
		}
	}
	return active
}
