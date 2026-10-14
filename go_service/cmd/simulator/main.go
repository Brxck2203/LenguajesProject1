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
	baseURL := flag.String("url", "http://localhost:8081", "Go ingestion URL")
	prefix := flag.String("prefix", fmt.Sprintf("sim_%d", time.Now().UnixNano()), "unique prefix for matches, players and events")
	interval := flag.Duration("interval", 25*time.Millisecond, "delay between events in each match")
	waitForResults := flag.Bool("wait", false, "wait for completed analyses; requires compatible Scala")
	timeout := flag.Duration("timeout", 2*time.Minute, "total simulation timeout")
	flag.Parse()
	if *prefix == "" || *timeout <= 0 {
		return fmt.Errorf("prefix and positive timeout are required")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	matches := simulation.Fixtures(*prefix)
	runner := simulation.Runner{BaseURL: *baseURL, Client: &http.Client{Timeout: 10 * time.Second}, Interval: *interval}
	if err := runner.Run(ctx, matches); err != nil {
		return err
	}
	for _, m := range matches {
		log.Printf("accepted matchId=%s events=%d scenario=%s", m.ID, len(m.Events), m.Scenario)
	}
	log.Printf("%d matches accepted; query /matches/{matchId}/analysis for actual analysis status", len(matches))
	if *waitForResults {
		if err := runner.WaitForAnalyses(ctx, matches); err != nil {
			return err
		}
		log.Print("all analyses completed")
	}
	return nil
}
