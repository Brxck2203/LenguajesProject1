package simulation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Brxck2203/LenguajesProject1/go_service/internal/domain"
)

type Runner struct {
	BaseURL  string
	Client   *http.Client
	Interval time.Duration
}

func (runner Runner) Run(ctx context.Context, matches []Match) error {
	parsed, err := url.Parse(runner.BaseURL)
	if err != nil || parsed == nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("invalid Go server URL")
	}
	if runner.Client == nil || runner.Interval < 0 {
		return fmt.Errorf("HTTP client and non-negative interval are required")
	}
	errorsByMatch := make(chan error, len(matches))
	var workers sync.WaitGroup
	for _, m := range matches {
		workers.Add(1)
		go func(m Match) {
			defer workers.Done()
			for i, event := range m.Events {
				if err := runner.send(ctx, event); err != nil {
					errorsByMatch <- fmt.Errorf("%s event %s: %w", m.ID, event.EventID, err)
					return
				}
				if i+1 < len(m.Events) {
					if err := wait(ctx, runner.Interval); err != nil {
						errorsByMatch <- err
						return
					}
				}
			}
		}(m)
	}
	workers.Wait()
	close(errorsByMatch)
	var failures []error
	for err := range errorsByMatch {
		failures = append(failures, err)
	}
	return errors.Join(failures...)
}

func (runner Runner) send(ctx context.Context, event domain.Event) error {
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	for attempt := 0; attempt < 30; attempt++ {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(runner.BaseURL, "/")+"/events", bytes.NewReader(body))
		if err != nil {
			return err
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := runner.Client.Do(request)
		if err != nil {
			return err
		}
		data, readErr := io.ReadAll(io.LimitReader(response.Body, 8192))
		response.Body.Close()
		if readErr != nil {
			return readErr
		}
		if response.StatusCode == http.StatusAccepted {
			return nil
		}
		if response.StatusCode != http.StatusServiceUnavailable {
			return fmt.Errorf("HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(data)))
		}
		if err := wait(ctx, time.Second); err != nil {
			return err
		}
	}
	return fmt.Errorf("Go capacity unavailable after 30 attempts; event was not accepted")
}

func (runner Runner) WaitForAnalyses(ctx context.Context, matches []Match) error {
	for _, m := range matches {
		for {
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(runner.BaseURL, "/")+"/matches/"+url.PathEscape(m.ID)+"/analysis", nil)
			if err != nil {
				return err
			}
			response, err := runner.Client.Do(request)
			if err != nil {
				return err
			}
			var snapshot domain.AnalysisSnapshot
			err = json.NewDecoder(io.LimitReader(response.Body, 5<<20)).Decode(&snapshot)
			response.Body.Close()
			if response.StatusCode != http.StatusOK {
				return fmt.Errorf("analysis %s: HTTP %d", m.ID, response.StatusCode)
			}
			if err != nil {
				return err
			}
			if snapshot.Status == domain.AnalysisCompleted {
				break
			}
			if snapshot.Status == domain.AnalysisFailed {
				return fmt.Errorf("analysis %s failed: %s", m.ID, snapshot.LastError)
			}
			if err := wait(ctx, 200*time.Millisecond); err != nil {
				return err
			}
		}
	}
	return nil
}

func wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
