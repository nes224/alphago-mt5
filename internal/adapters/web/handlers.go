package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strconv"
	"time"

	"github.com/nes224/alphago-mt5/internal/adapters/candlefile"
	"github.com/nes224/alphago-mt5/internal/core/domain"
)

//go:embed static
var static embed.FS

// Server serves the dashboard page and its API.
type Server struct {
	Hub       *Hub
	DataDir   string   // candle CSV root (CANDLE_DATA_DIR)
	Symbols   []string // SYMBOLS
	Heartbeat time.Duration
}

// Register adds the dashboard routes to mux:
//
//	GET /                 the dashboard page
//	GET /api/symbols      configured symbols
//	GET /api/candles      stored candles: ?symbol=XAUUSDm&tf=H1&limit=500
//	GET /api/stream       live ticks as SSE: ?symbol=XAUUSDm
func (s *Server) Register(mux *http.ServeMux) {
	page, _ := fs.Sub(static, "static")
	mux.Handle("GET /", http.FileServerFS(page))
	mux.HandleFunc("GET /api/symbols", s.symbols)
	mux.HandleFunc("GET /api/candles", s.candles)
	mux.HandleFunc("GET /api/stream", s.stream)
}

func (s *Server) symbols(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.Symbols)
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
