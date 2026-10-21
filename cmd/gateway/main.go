package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nes224/alphago-mt5/internal/adapters/config"
	"github.com/nes224/alphago-mt5/internal/adapters/logging"
	"github.com/nes224/alphago-mt5/internal/adapters/mt5"
	"github.com/nes224/alphago-mt5/internal/core/services"
)

func main() {
	if err := run(); err != nil {
		slog.Error("gateway exited", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadGateway()
	if err != nil {
		return err
	}

	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		return fmt.Errorf("LOG_LEVEL: %w", err)
	}

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	pub := logging.NewPublisher(log)
	gw := services.NewGatewayService(pub, log)
	stream := mt5.NewStreamClient(cfg.StreamAddr(), gw, log)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		connected := stream.Connected()
		w.Header().Set("Content-Type", "application/json")
		if !connected {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"status":     map[bool]string{true: "ok", false: "degraded"}[connected],
			"mt5_stream": connected,
			"gateway":    gw.Stats(),
		})
	})

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errc := make(chan error, 2)
	go func() {
		log.Info("http listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- fmt.Errorf("http: %w", err)
		}
	}()

	go func() {
		log.Info("connecting to EA", "stream", cfg.StreamAddr())
		if err := stream.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			errc <- fmt.Errorf("mt5 stream: %w", err)
		}
	}()

	select {
	case <-ctx.Done():
		log.Info("shutting down")
	case err = <-errc:
		cancel()
	}

	shutdownCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
	defer c()
	if e := srv.Shutdown(shutdownCtx); e != nil {
		log.Warn("http shutdown", "err", e)
	}

	return err
}
