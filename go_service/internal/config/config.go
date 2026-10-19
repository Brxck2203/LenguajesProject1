package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/Brxck2203/LenguajesProject1/go_service/internal/processing"
)

type Config struct {
	Port            string
	ScalaURL        string
	ScalaTimeout    time.Duration
	ShutdownTimeout time.Duration
	Processing      processing.Config
	// EventCatalogPath replaces the embedded event catalog when set.
	EventCatalogPath string
	// InboxDir enables file ingestion: producers drop *.json / *.jsonl files there.
	InboxDir          string
	InboxPollInterval time.Duration
	InboxWorkers      int
}

func Load() (Config, error) {
	var result Config
	result.Port = value("PORT", "8081")
	port, err := strconv.Atoi(result.Port)
	if err != nil || port < 1 || port > 65535 {
		return result, fmt.Errorf("PORT must be between 1 and 65535")
	}
	result.ScalaURL = value("SCALA_ANALYSIS_URL", "http://localhost:8080/analyze")
	result.EventCatalogPath = value("EVENT_CATALOG_PATH", "")
	result.InboxDir = value("INBOX_DIR", "")
	for _, setting := range []struct {
		name     string
		fallback time.Duration
		target   *time.Duration
	}{
		{"SCALA_TIMEOUT", 10 * time.Second, &result.ScalaTimeout},
		{"SHUTDOWN_TIMEOUT", 30 * time.Second, &result.ShutdownTimeout},
		{"ANALYSIS_RETRY_BASE", time.Second, &result.Processing.RetryBase},
		{"ANALYSIS_RETRY_MAX", 10 * time.Second, &result.Processing.RetryMax},
		{"INBOX_POLL_INTERVAL", 500 * time.Millisecond, &result.InboxPollInterval},
	} {
		parsed, err := time.ParseDuration(value(setting.name, setting.fallback.String()))
		if err != nil || parsed <= 0 {
			return result, fmt.Errorf("%s must be a positive duration, e.g. 5s", setting.name)
		}
		*setting.target = parsed
	}
	for _, setting := range []struct {
		name     string
		fallback string
		target   *int
	}{
		{"ANALYSIS_WORKERS", "4", &result.Processing.Workers},
		{"ANALYSIS_CAPACITY", "128", &result.Processing.Capacity},
		{"ANALYSIS_MAX_ATTEMPTS", "3", &result.Processing.MaxAttempts},
		{"INBOX_WORKERS", "4", &result.InboxWorkers},
	} {
		parsed, err := strconv.Atoi(value(setting.name, setting.fallback))
		if err != nil || parsed <= 0 {
			return result, fmt.Errorf("%s must be a positive integer", setting.name)
		}
		*setting.target = parsed
	}
	if result.Processing.RetryMax < result.Processing.RetryBase {
		return result, fmt.Errorf("ANALYSIS_RETRY_MAX must be >= ANALYSIS_RETRY_BASE")
	}
	return result, nil
}

func value(name, fallback string) string {
	if v, exists := os.LookupEnv(name); exists {
		return v
	}
	return fallback
}
