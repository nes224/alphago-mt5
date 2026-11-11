package api

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nes224/alphago-mt5/api/internal/adapters/candlefile"
	"github.com/nes224/alphago-mt5/api/internal/core/domain"
)

func tick(symbol string, sec int) domain.Tick {
	return domain.Tick{Symbol: symbol, Bid: 4100, Ask: 4100.25, Time: time.Date(2026, 11, 5, 4, 0, sec, 0, time.UTC)}
}

func TestHub_FanOutAndUnsubscribe(t *testing.T) {
	h := NewHub()
	a, stopA := h.Subscribe()
	b, stopB := h.Subscribe()
	defer stopB()

	h.PublishTick(context.Background(), tick("XAUUSDm", 1))
	if (<-a).Symbol != "XAUUSDm" || (<-b).Symbol != "XAUUSDm" {
		t.Fatal("both subscribers should get the tick")
	}
	stopA()
	if h.Subscribers() != 1 {
		t.Errorf("subscribers = %d, want 1", h.Subscribers())
	}
}

// a subscriber that never reads must not block the publisher
func TestHub_SlowSubscriberDoesNotBlock(t *testing.T) {
	h := NewHub()
	_, stop := h.Subscribe()
	defer stop()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			h.PublishTick(context.Background(), tick("XAUUSDm", i%60))
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("PublishTick blocked on a slow subscriber")
	}
}

func newTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	root := t.TempDir()
	var candles []domain.Candle
	base := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 10; i++ {
		candles = append(candles, domain.Candle{
			Symbol: "XAUUSDm", Timeframe: domain.TimeframeH1, OpenTime: base.Add(time.Duration(i) * time.Hour),
			Open: 4100, High: 4110, Low: 4090, Close: 4100 + float64(i), Closed: true,
		})
	}
	if err := candlefile.WriteCandles(root, "XAUUSDm", domain.TimeframeH1, candles); err != nil {
		t.Fatal(err)
	}
	s := &Server{Hub: NewHub(), DataDir: root, Symbols: []string{"XAUUSDm"}, Heartbeat: time.Hour}
	mux := http.NewServeMux()
	s.Register(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return s, ts
}

func TestCandles_ReturnsNewestLimit(t *testing.T) {
	_, ts := newTestServer(t)
	resp, err := http.Get(ts.URL + "/api/candles?symbol=XAUUSDm&tf=H1&limit=3")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got []CandleJSON
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].C != 4107 || got[2].C != 4109 {
		t.Errorf("got %+v, want the last 3 candles (close 4107..4109)", got)
	}
}

func TestCandles_BadRequest(t *testing.T) {
	_, ts := newTestServer(t)
	for _, q := range []string{"tf=H1", "symbol=XAUUSDm&tf=H2", "symbol=XAUUSDm&tf=H1&limit=-1"} {
		resp, err := http.Get(ts.URL + "/api/candles?" + q)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", q, resp.StatusCode)
		}
	}
}

func TestCORS_AllowsWebOrigin(t *testing.T) {
	h := CORS("http://localhost:3000", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	ts := httptest.NewServer(h)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodOptions, ts.URL+"/api/candles", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent || resp.Header.Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Errorf("preflight: status %d, allow-origin %q", resp.StatusCode, resp.Header.Get("Access-Control-Allow-Origin"))
	}

	resp, err = http.Get(ts.URL + "/api/candles")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Errorf("get: status %d, allow-origin %q", resp.StatusCode, resp.Header.Get("Access-Control-Allow-Origin"))
	}
}

// the stream sends only ticks of the requested symbol
func TestStream_SendsTicksOfOneSymbol(t *testing.T) {
	s, ts := newTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/stream?symbol=XAUUSDm", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}

	// wait until the handler has subscribed, then publish
	for s.Hub.Subscribers() == 0 {
		time.Sleep(time.Millisecond)
	}
	s.Hub.PublishTick(ctx, tick("BTCUSDm", 1))
	s.Hub.PublishTick(ctx, tick("XAUUSDm", 2))

	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var msg TickMsg
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &msg); err != nil {
			t.Fatal(err)
		}
		if msg.Symbol != "XAUUSDm" {
			t.Fatalf("got a tick for %s, want only XAUUSDm", msg.Symbol)
		}
		if msg.Bid != 4100 || msg.TimeMs != tick("XAUUSDm", 2).Time.UnixMilli() {
			t.Errorf("tick = %+v", msg)
		}
		return
	}
	t.Fatalf("stream ended without a tick: %v", sc.Err())
}
