package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Brxck2203/LenguajesProject1/go_service/internal/config"
	"github.com/Brxck2203/LenguajesProject1/go_service/internal/inbox"
	"github.com/Brxck2203/LenguajesProject1/go_service/internal/match"
	"github.com/Brxck2203/LenguajesProject1/go_service/internal/processing"
	"github.com/Brxck2203/LenguajesProject1/go_service/internal/scala"
	httptransport "github.com/Brxck2203/LenguajesProject1/go_service/internal/transport/http"
	"github.com/Brxck2203/LenguajesProject1/go_service/internal/validation"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}
func run() error {
	settings, err := config.Load()
	if err != nil {
		return err
	}
	client, err := scala.NewClient(settings.ScalaURL, settings.ScalaTimeout)
	if err != nil {
		return err
	}
	validator := validation.Default()
	if settings.EventCatalogPath != "" {
		catalog, err := validation.LoadCatalogFile(settings.EventCatalogPath)
		if err != nil {
			return err
		}
		if validator, err = validation.NewValidator(catalog); err != nil {
			return err
		}
		log.Printf("event catalog loaded from %s", settings.EventCatalogPath)
	}
	service, err := processing.NewService(match.NewStoreWithValidator(validator), client, settings.Processing)
	if err != nil {
		return err
	}
	options := []httptransport.Option{httptransport.WithCatalog(validator.Catalog())}

	// Optional file ingestion: the watcher feeds the same service as the HTTP API.
	inboxCtx, stopInbox := context.WithCancel(context.Background())
	defer stopInbox()
	inboxDone := make(chan struct{})
	if settings.InboxDir != "" {
		watcher, err := inbox.New(inbox.Config{
			Dir: settings.InboxDir, PollInterval: settings.InboxPollInterval,
			Workers: settings.InboxWorkers, StableFor: 300 * time.Millisecond,
		}, service)
		if err != nil {
			// The HTTP API keeps working; the problem is reported in the log and at GET /inbox.
			log.Printf("inbox disabled: %v (check that %s exists and is writable)", err, settings.InboxDir)
			options = append(options, httptransport.WithInboxError(err))
			close(inboxDone)
		} else {
			options = append(options, httptransport.WithInbox(watcher))
			go func() { watcher.Run(inboxCtx); close(inboxDone) }()
		}
	} else {
		close(inboxDone)
	}

	server := &http.Server{
		Addr: ":" + settings.Port, Handler: httptransport.NewRouter(service, service, options...),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.ListenAndServe() }()
	log.Printf("GameStats Go listening on :%s; Scala analysis URL: %s", settings.Port, settings.ScalaURL)
	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-serverErrors:
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), settings.ShutdownTimeout)
	defer cancel()
	// Stop reading new files first; files already dispatched are finished.
	stopInbox()
	select {
	case <-inboxDone:
	case <-shutdownCtx.Done():
	}
	// Stop HTTP first, allowing in-flight handlers to register their jobs.
	httpErr := server.Shutdown(shutdownCtx)
	if httpErr != nil {
		_ = server.Close()
	}
	analysisErr := service.Shutdown(shutdownCtx)
	if errors.Is(serveErr, http.ErrServerClosed) {
		serveErr = nil
	}
	if analysisErr != nil {
		log.Print("analysis drain deadline reached; memory-only state will be lost on process exit")
	}
	if err := errors.Join(serveErr, httpErr, analysisErr); err != nil {
		return fmt.Errorf("server stopped: %w", err)
	}
	log.Print("server stopped; accepted analysis jobs drained")
	return nil
}
