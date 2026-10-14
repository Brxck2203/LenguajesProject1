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
	"github.com/Brxck2203/LenguajesProject1/go_service/internal/match"
	"github.com/Brxck2203/LenguajesProject1/go_service/internal/processing"
	"github.com/Brxck2203/LenguajesProject1/go_service/internal/scala"
	httptransport "github.com/Brxck2203/LenguajesProject1/go_service/internal/transport/http"
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
	service, err := processing.NewService(match.NewStore(), client, settings.Processing)
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr: ":" + settings.Port, Handler: httptransport.NewRouter(service, service),
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
