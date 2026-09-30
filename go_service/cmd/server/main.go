package main

import (
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Brxck2203/LenguajesProject1/go_service/internal/match"
	httptransport "github.com/Brxck2203/LenguajesProject1/go_service/internal/transport/http"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	store := match.NewStore()
	router := httptransport.NewRouter(store)
	server := &http.Server{
		Addr:              ":" + port,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("starting GameStats HTTP server on port %s", port)
	log.Printf("health endpoint: http://localhost:%s/health", port)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("HTTP server failed: %v", err)
	}
}
