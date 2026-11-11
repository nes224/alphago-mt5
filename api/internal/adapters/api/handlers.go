package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/nes224/alphago-mt5/api/internal/adapters/candlefile"
	"github.com/nes224/alphago-mt5/api/internal/core/domain"
	"github.com/nes224/alphago-mt5/api/internal/core/services"
)

// SymbolStore is the list of tracked symbols (services.SymbolSet).
type SymbolStore interface {
	List() []string
	Add(symbol string) error
	Remove(symbol string) error
}

// BrokerInfo is the trading account and the symbols the broker offers
// (the symbols in MT5's Market Watch).
type BrokerInfo struct {
	Broker      string   `json:"broker"`
	Login       int64    `json:"login"`
	AccountType string   `json:"account_type"`
	Currency    string   `json:"currency"`
	Balance     float64  `json:"balance"`
	Equity      float64  `json:"equity"`
	Leverage    int64    `json:"leverage"`
	Symbols     []string `json:"symbols"`
}

// Server serves the dashboard API.
type Server struct {
	Hub     *Hub
	DataDir string // candle CSV root (CANDLE_DATA_DIR)
	Symbols SymbolStore
	// Broker asks MT5 for the account and its symbols.
	Broker func(ctx context.Context) (BrokerInfo, error)
	// OnSymbolAdded runs after a symbol is added (starts its backfill).
	OnSymbolAdded func()
	Heartbeat     time.Duration
}

// Register adds the API routes to mux:
//
//	GET    /api/broker            account info and the symbols the broker offers
//	GET    /api/symbols           tracked symbols
//	POST   /api/symbols           track a symbol: {"symbol":"BTCUSDm"}
//	DELETE /api/symbols/{symbol}  stop tracking a symbol (its candle files are kept)
//	GET    /api/candles           stored candles: ?symbol=XAUUSDm&tf=H1&limit=500
//	GET    /api/stream            live ticks as SSE: ?symbol=XAUUSDm
func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/broker", s.broker)
	mux.HandleFunc("GET /api/symbols", s.symbols)
	mux.HandleFunc("POST /api/symbols", s.addSymbol)
	mux.HandleFunc("DELETE /api/symbols/{symbol}", s.removeSymbol)
	mux.HandleFunc("GET /api/candles", s.candles)
	mux.HandleFunc("GET /api/stream", s.stream)
}

func (s *Server) symbols(w http.ResponseWriter, _ *http.Request) {
	list := s.Symbols.List()
	if list == nil {
		list = []string{}
	}
	writeJSON(w, http.StatusOK, list)
}

const brokerTimeout = 30 * time.Second

func (s *Server) brokerInfo(ctx context.Context) (BrokerInfo, error) {
	if s.Broker == nil {
		return BrokerInfo{}, errors.New("broker lookup not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, brokerTimeout)
	defer cancel()
	return s.Broker(ctx)
}

func (s *Server) broker(w http.ResponseWriter, r *http.Request) {
	info, err := s.brokerInfo(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "MT5 is not reachable: "+err.Error())
		return
	}
	if info.Symbols == nil {
		info.Symbols = []string{}
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) addSymbol(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Symbol string `json:"symbol"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Symbol == "" {
		writeError(w, http.StatusBadRequest, `body must be {"symbol":"..."}`)
		return
	}
	// only symbols the broker actually offers can be tracked
	info, err := s.brokerInfo(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "MT5 is not reachable: "+err.Error())
		return
	}
	if !slices.Contains(info.Symbols, body.Symbol) {
		writeError(w, http.StatusUnprocessableEntity, body.Symbol+" is not in MT5 Market Watch")
		return
	}
	switch err := s.Symbols.Add(body.Symbol); {
	case errors.Is(err, services.ErrSymbolExists):
		writeError(w, http.StatusConflict, body.Symbol+" is already tracked")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s.OnSymbolAdded != nil {
		s.OnSymbolAdded()
	}
	writeJSON(w, http.StatusCreated, s.Symbols.List())
}

func (s *Server) removeSymbol(w http.ResponseWriter, r *http.Request) {
	symbol := r.PathValue("symbol")
	switch err := s.Symbols.Remove(symbol); {
	case errors.Is(err, services.ErrSymbolNotFound):
		writeError(w, http.StatusNotFound, symbol+" is not tracked")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	list := s.Symbols.List()
	if list == nil {
		list = []string{}
	}
	writeJSON(w, http.StatusOK, list)
}

// CandleJSON is one candle as sent to the chart (time in unix seconds, broker server time).
type CandleJSON struct {
	T int64   `json:"t"`
	O float64 `json:"o"`
	H float64 `json:"h"`
	L float64 `json:"l"`
	C float64 `json:"c"`
}

const maxCandles = 5000

func (s *Server) candles(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	symbol := q.Get("symbol")
	tf := domain.Timeframe(q.Get("tf"))
	if symbol == "" || tf.Minutes() == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "symbol and tf (M1 M5 M15 M30 H1 H4 Daily) are required"})
		return
	}
	limit := 500
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be a positive number"})
			return
		}
		limit = min(n, maxCandles)
	}

	all, err := candlefile.ReadAll(s.DataDir, symbol, tf)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if len(all) > limit {
		all = all[len(all)-limit:]
	}
	out := make([]CandleJSON, len(all))
	for i, c := range all {
		out[i] = CandleJSON{T: c.OpenTime.Unix(), O: c.Open, H: c.High, L: c.Low, C: c.Close}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	symbol := r.URL.Query().Get("symbol")

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	ticks, unsubscribe := s.Hub.Subscribe()
	defer unsubscribe()

	beat := s.Heartbeat
	if beat <= 0 {
		beat = 15 * time.Second
	}
	heartbeat := time.NewTicker(beat)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			fmt.Fprint(w, ": ping\n\n") // keeps proxies from closing an idle stream
			flusher.Flush()
		case t := <-ticks:
			if symbol != "" && t.Symbol != symbol {
				continue
			}
			b, _ := json.Marshal(t)
			fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		}
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// CORS lets the web app at origin (e.g. http://localhost:3000) call the API from the browser.
func CORS(origin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
