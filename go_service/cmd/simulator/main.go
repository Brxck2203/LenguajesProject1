package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Brxck2203/LenguajesProject1/go_service/internal/simulation"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	baseURL := flag.String("url", envOr("SIMULATOR_URL", "http://localhost:8081"), "Go ingestion URL (HTTP mode and -wait); env SIMULATOR_URL")
	mode := flag.String("mode", "http", "delivery: http (POST /events) or files (JSON files in -out)")
	outDir := flag.String("out", envOr("SIMULATOR_OUT", "data/inbox"), "inbox folder for -mode files; env SIMULATOR_OUT")
	scenario := flag.String("scenario", "fixtures", "fixtures (8 scripted matches), random, or all")
	perGame := flag.Int("matches", 3, "random matches per game")
	seed := flag.Uint64("seed", uint64(time.Now().UnixNano()), "random seed; the same seed repeats the same matches")
	chunk := flag.Int("chunk", 25, "events per file in -mode files")
	prefix := flag.String("prefix", fmt.Sprintf("sim_%d", time.Now().UnixNano()), "unique prefix for matches and events")
	interval := flag.Duration("interval", 25*time.Millisecond, "delay between events (http) or files (files) of each match")
	continuous := flag.Bool("continuous", false, "keep generating new rounds until stopped")
	every := flag.Duration("every", 15*time.Second, "pause between rounds with -continuous")
	waitForResults := flag.Bool("wait", false, "wait until Scala completes every analysis")
	timeout := flag.Duration("timeout", 5*time.Minute, "total time limit; 0 disables it")
	flag.Parse()
	if *prefix == "" || *timeout < 0 || *perGame <= 0 {
		return fmt.Errorf("prefix, -matches > 0 and a non-negative timeout are required")
	}
	if *mode != "http" && *mode != "files" {
		return fmt.Errorf("-mode must be http or files")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *timeout)
		defer cancel()
	}
	client := &http.Client{Timeout: 10 * time.Second}
	httpRunner := simulation.Runner{BaseURL: *baseURL, Client: client, Interval: *interval}

	for round := 1; ; round++ {
		roundPrefix := *prefix
		if *continuous {
			roundPrefix = fmt.Sprintf("%s_%03d", *prefix, round)
		}
		var matches []simulation.Match
		if *scenario == "fixtures" || *scenario == "all" {
			matches = append(matches, simulation.Fixtures(roundPrefix)...)
		}
		if *scenario == "random" || *scenario == "all" {
			matches = append(matches, simulation.Random(simulation.RandomConfig{
				Prefix: roundPrefix, Seed: *seed + uint64(round), MatchesPerGame: *perGame,
			})...)
		}
		if len(matches) == 0 {
			return fmt.Errorf("-scenario must be fixtures, random or all")
		}

		var err error
		if *mode == "files" {
			err = simulation.FileRunner{Dir: *outDir, Interval: *interval, ChunkSize: *chunk}.Run(ctx, matches)
		} else {
			err = httpRunner.Run(ctx, matches)
		}
		if err != nil {
			return err
		}
		events := 0
		for _, m := range matches {
			events += len(m.Events)
			log.Printf("sent matchId=%s events=%d scenario=%s", m.ID, len(m.Events), m.Scenario)
		}
		log.Printf("round %d: %d matches, %d events delivered via %s", round, len(matches), events, *mode)
		if *waitForResults {
			if err := httpRunner.WaitForAnalyses(ctx, matches); err != nil {
				return err
			}
			log.Print("all analyses completed; see /matches/{matchId}/analysis or Scala /results")
		}
		if !*continuous {
			return nil
		}
		if err := waitOrStop(ctx, *every); err != nil {
			log.Print("simulation stopped")
			return nil
		}
	}
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func waitOrStop(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
