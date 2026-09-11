// Command mockserver runs a standalone mock PRTG APIv2 server (see
// pkg/mockprtg) for local development against this repo's Grafana plugin --
// there is no official PRTG container, since PRTG itself is Windows-only,
// closed-source, license-bound software.
//
// It is a plain net/http binary, independent of grafana-plugin-sdk-go and of
// this repo's actual plugin backend (pkg/main.go): it lives under cmd/, not
// pkg/, specifically so Magefile.go's build.BuildAll (which builds the
// plugin backend from pkg/main.go) never picks it up.
//
// Configuration is via environment variables, all optional:
//
//	PORT          listen port (default "8080")
//	MOCK_API_KEY  bearer token accepted for the 'apiKey' auth mode (default "mock-api-key")
//	MOCK_USERNAME username accepted by POST /session (default "mock-user")
//	MOCK_PASSWORD password accepted by POST /session (default "mock-password")
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/srgssr/prtg-datasource/pkg/mockprtg"
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	port := getenv("PORT", "8080")

	dataset := mockprtg.DefaultDataset()
	srv := mockprtg.NewServer(dataset, mockprtg.Options{
		APIKey:   os.Getenv("MOCK_API_KEY"),
		Username: os.Getenv("MOCK_USERNAME"),
		Password: os.Getenv("MOCK_PASSWORD"),
	})

	httpServer := &http.Server{
		Addr:              ":" + port,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("mock-prtg: listening on :%s", port)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("mock-prtg: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("mock-prtg: shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("mock-prtg: shutdown error: %v", err)
	}
}
