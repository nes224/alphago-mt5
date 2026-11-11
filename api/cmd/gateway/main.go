package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/nes224/alphago-mt5/api/internal/adapters/api"
	"github.com/nes224/alphago-mt5/api/internal/adapters/candlefile"
	"github.com/nes224/alphago-mt5/api/internal/adapters/config"
	"github.com/nes224/alphago-mt5/api/internal/adapters/fanout"
	"github.com/nes224/alphago-mt5/api/internal/adapters/logging"
	"github.com/nes224/alphago-mt5/api/internal/adapters/mt5"
	"github.com/nes224/alphago-mt5/api/internal/adapters/symbolfile"
	"github.com/nes224/alphago-mt5/api/internal/core/services"
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

	// live events go to the log and to every open dashboard
	hub := api.NewHub()
	pub := fanout.Publisher{logging.NewPublisher(log), hub}
	gw := services.NewGatewayService(pub, log)
	stream := mt5.NewStreamClient(cfg.StreamAddr(), gw, log)

	// backfill: defaults from services, overridden by BACKFILL_LOOKBACK
	lookback := maps.Clone(services.DefaultLookback)
	maps.Copy(lookback, cfg.BackfillLookback)

	cmd := mt5.NewCommandClient(cfg.CmdAddr(), cfg.MT5Timeout, log)
	defer cmd.Close()
	store := candlefile.Store{Root: cfg.CandleDataDir}

	// tracked symbols: saved next to the candle folder; SYMBOLS only seeds the first run
	symbolsPath := filepath.Join(filepath.Dir(cfg.CandleDataDir), "symbols.json")
	initial, found, err := symbolfile.Load(symbolsPath)
	if err != nil {
		return err
	}
	if !found {
		initial = cfg.Symbols
	}
	symbols := services.NewSymbolSet(initial, func(l []string) error { return symbolfile.Save(symbolsPath, l) })
	if !found {
		if err := symbolfile.Save(symbolsPath, symbols.List()); err != nil {
			return fmt.Errorf("save %s: %w", symbolsPath, err)
		}
	}
	if len(symbols.List()) == 0 {
		log.Warn("no symbols tracked; add one from the web app or set SYMBOLS")
	}
	log.Info("tracking symbols", "symbols", symbols.List(), "file", symbolsPath)

	runner := services.NewBackfillRunner(services.NewBackfiller(cmd, store, lookback, log), symbols)

	// every (re)connect to the EA may follow a gap, so fill it
	stream.OnConnect = runner.Trigger
	// each closed live candle: fetch the broker's closed candles for every TF and store them
	gw.OnCandleClose = runner.Trigger
	go runner.Run(ctx)

	mux := http.NewServeMux()
	apiServer := &api.Server{
		Hub:           hub,
		DataDir:       cfg.CandleDataDir,
		Symbols:       symbols,
		Broker:        brokerInfo(cmd),
		OnSymbolAdded: runner.Trigger,
	}
	apiServer.Register(mux)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		connected := stream.Connected()
		w.Header().Set("Content-Type", "application/json")
		if !connected {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"status":     map[bool]string{true: "ok", false: "degraded"}[connected],
			"mt5_stream": connected,
			"mt5_cmd":    cmd.Connected(),
			"gateway":    gw.Stats(),
			"backfill":   runner.Status(),
		})
	})

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:           api.CORS(cfg.WebOrigin, mux),
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

// brokerInfo adapts the EA's ACCOUNT_INFO answer for the API.
func brokerInfo(cmd *mt5.CommandClient) func(context.Context) (api.BrokerInfo, error) {
	return func(ctx context.Context) (api.BrokerInfo, error) {
		a, err := cmd.AccountInfo(ctx)
		if err != nil {
			return api.BrokerInfo{}, err
		}
		return api.BrokerInfo{
			Broker: a.Broker, Login: a.Login, AccountType: a.AccountType, Currency: a.Currency,
			Balance: a.Balance, Equity: a.Equity, Leverage: a.Leverage, Symbols: a.Symbols,
		}, nil
	}
}
