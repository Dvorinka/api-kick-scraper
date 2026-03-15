package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"apiservices/kick-scraper/internal/kick/api"
	"apiservices/kick-scraper/internal/kick/auth"
	"apiservices/kick-scraper/internal/kick/scrape"
)

func main() {
	logger := log.New(os.Stdout, "[kick] ", log.LstdFlags)

	port := envString("PORT", "30009")
	apiKey := envString("KICK_API_KEY", "dev-kick-key")
	if apiKey == "dev-kick-key" {
		logger.Println("KICK_API_KEY not set, using default development key")
	}
	baseURL := envString("KICK_BASE_URL", "https://kick.com")

	service := scrape.NewService(baseURL)
	handler := api.NewHandler(service)

	mux := http.NewServeMux()
	mux.Handle("/v1/kick/", auth.Middleware(apiKey)(handler))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadTimeout:       12 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Printf("service listening on :%s", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("server failed: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Printf("shutdown error: %v", err)
	}
}

func envString(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
