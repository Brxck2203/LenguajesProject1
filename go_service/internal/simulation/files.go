package simulation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileRunner writes matches into the inbox folder as the future game console would:
// each match produces numbered chunk files over time. Files are written to a
// temporary name and renamed, so the watcher never reads a half-written file.
type FileRunner struct {
	Dir       string
	Interval  time.Duration
	ChunkSize int
}

func (runner FileRunner) Run(ctx context.Context, matches []Match) error {
	if runner.Dir == "" || runner.ChunkSize <= 0 || runner.Interval < 0 {
		return fmt.Errorf("file mode needs an output folder, a positive chunk size and a non-negative interval")
	}
	if err := os.MkdirAll(runner.Dir, 0o755); err != nil {
		return err
	}
	failures := make(chan error, len(matches))
	var producers sync.WaitGroup
	for _, m := range matches {
		producers.Add(1)
		go func(m Match) {
			defer producers.Done()
			for part, start := 0, 0; start < len(m.Events); part, start = part+1, start+runner.ChunkSize {
				end := min(start+runner.ChunkSize, len(m.Events))
				name := fmt.Sprintf("%s_%04d.json", m.ID, part)
				if err := writeAtomically(runner.Dir, name, map[string]any{"events": m.Events[start:end]}); err != nil {
					failures <- fmt.Errorf("%s: %w", m.ID, err)
					return
				}
				if end < len(m.Events) {
					if err := wait(ctx, runner.Interval); err != nil {
						failures <- err
						return
					}
				}
			}
		}(m)
	}
	producers.Wait()
	close(failures)
	var all []error
	for err := range failures {
		all = append(all, err)
	}
	return errors.Join(all...)
}

func writeAtomically(dir, name string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	temporary := filepath.Join(dir, "."+name+".tmp")
	if err := os.WriteFile(temporary, data, 0o644); err != nil {
		return err
	}
	return os.Rename(temporary, filepath.Join(dir, name))
}
