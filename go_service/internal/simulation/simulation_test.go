package simulation

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Brxck2203/LenguajesProject1/go_service/internal/domain"
	"github.com/Brxck2203/LenguajesProject1/go_service/internal/inbox"
	"github.com/Brxck2203/LenguajesProject1/go_service/internal/match"
)

// Every generated match must be accepted by the real store and close cleanly.
func assertAccepted(t *testing.T, matches []Match) {
	t.Helper()
	store := match.NewStore()
	for _, m := range matches {
		closed := false
		for _, event := range m.Events {
			request, err := store.Add(event)
			if err != nil {
				t.Fatalf("%s: event %s (%s) rejected: %v", m.ID, event.EventID, event.Action, err)
			}
			closed = closed || request != nil
		}
		if !closed {
			t.Fatalf("%s never closed", m.ID)
		}
	}
}

func TestFixturesAreValid(t *testing.T) {
	matches := Fixtures("fx")
	if len(matches) != 8 {
		t.Fatalf("expected 8 fixtures, got %d", len(matches))
	}
	assertAccepted(t, matches)
}

func TestRandomMatchesAreValidAndReproducible(t *testing.T) {
	start := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	first := Random(RandomConfig{Prefix: "rnd", Seed: 42, MatchesPerGame: 5, Start: start})
	second := Random(RandomConfig{Prefix: "rnd", Seed: 42, MatchesPerGame: 5, Start: start})
	if len(first) != 20 {
		t.Fatalf("expected 20 matches, got %d", len(first))
	}
	assertAccepted(t, first)
	games := map[string]int{}
	for i := range first {
		games[first[i].Events[0].GameID]++
		if len(first[i].Events) != len(second[i].Events) || first[i].Events[len(first[i].Events)-1].Timestamp != second[i].Events[len(second[i].Events)-1].Timestamp {
			t.Fatalf("same seed produced different match %s", first[i].ID)
		}
		if len(first[i].Events) < 10 {
			t.Fatalf("%s is too small to be meaningful: %d events", first[i].ID, len(first[i].Events))
		}
	}
	for _, game := range []string{domain.GameMMA, domain.GameFootball, domain.GameMotorcycleRacing, domain.GameFreeFire} {
		if games[game] != 5 {
			t.Fatalf("expected 5 %s matches, got %d", game, games[game])
		}
	}
}

func TestFileRunnerFeedsTheInbox(t *testing.T) {
	dir := t.TempDir()
	store := match.NewStore()
	watcher, err := inbox.New(inbox.Config{Dir: dir, PollInterval: 10 * time.Millisecond, Workers: 4}, store)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { watcher.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()

	matches := append(Fixtures("files"), Random(RandomConfig{Prefix: "files", Seed: 7, MatchesPerGame: 1})...)
	if err := (FileRunner{Dir: dir, Interval: time.Millisecond, ChunkSize: 7}).Run(context.Background(), matches); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		finished := 0
		for _, m := range matches {
			if snapshot, ok := store.Get(m.ID); ok && snapshot.Status == match.StatusFinished {
				finished++
			}
		}
		if finished == len(matches) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("only %d/%d matches finished; stats=%+v", finished, len(matches), watcher.Stats())
		}
		time.Sleep(20 * time.Millisecond)
	}
	if rejected, _ := os.ReadDir(filepath.Join(dir, inbox.RejectedDir)); len(rejected) != 0 {
		t.Fatalf("unexpected rejected files: %d", len(rejected))
	}
}
