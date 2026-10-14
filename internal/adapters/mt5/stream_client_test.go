package mt5

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

type recorder struct {
	mu    sync.Mutex
	ticks []domain.Tick
	got   chan struct{}
}

func TestParseEvent_Tick(t *testing.T) {
	line := `{"type":"tick","symbol":"EURUSD","bid":1.08523,"ask":1.08531,"volume":0,"timestamp":"2026.10.12 10:13:05"}`
	ev, err := parseEvent([]byte(line))
	if err != nil {
		t.Fatal(err)
	}

	tk, ok := ev.(domain.Tick)
	if !ok {
		t.Fatalf("got %T", ev)
	}
	want := time.Date(2026, 10, 12, 10, 13, 5, 0, time.UTC)
	if tk.Symbol != "EURUSD" || tk.Bid != 1.08523 || tk.Ask != 1.08531 || !tk.Time.Equal(want) {
		t.Errorf("tick = %+v", tk)
	}
}

func TestParseEvent_TradeClosed(t *testing.T) {
	line := `{"type":"trade_closed","symbol":"XAUUSD","ticket":123456789,"profit":-12.40,"timestamp":"2026.10.12 10:15:00"}`
	ev, err := parseEvent([]byte(line))
	if err != nil {
		t.Fatal(err)
	}

	tc, ok := ev.(domain.TradeClosed)
	if !ok || tc.Ticket != 123456789 || tc.Profit != -12.40 {
		t.Errorf("got %#v", ev)
	}
}

func TestParseEvent_Errors(t *testing.T) {
	for name, line := range map[string]string{
		"garbage":      `not json`,
		"unknown type": `{"type":"foo","symbol":"EURUSD","timestamp":"2026.10.12 10:13:05"}`,
		"bad time":     `{"type":"tick","symbol":"EURUSD","bid":1,"ask":1,"timestamp":"2026-10-12T10:13:05Z"}`,
		"zero price":   `{"type":"tick","symbol":"EURUSD","bid":0,"ask":0,"timestamp":"2026.10.12 10:13:05"}`,
		"no symbol":    `{"type":"tick","bid":1,"ask":1,"timestamp":"2026.10.12 10:13:05"}`,
	} {
		if _, err := parseEvent([]byte(line)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func (r *recorder) HandleTick(_ context.Context, t domain.Tick) {
	r.mu.Lock()
	r.ticks = append(r.ticks, t)
	r.mu.Unlock()
	r.got <- struct{}{}
}

func (r *recorder) HandleTradeClosed(context.Context, domain.TradeClosed) {}

func TestStreamClient_ReconnectsAfterEADrops(t *testing.T) {
	In, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer In.Close()

	go func() {
		for i := 0; i<2; i++ {
			conn, err := In.Accept()
			if err != nil {
				return
			}

			io.WriteString(conn, `{"type":"tick","symbol":"EURUSD","bid":1.1,"ask":1.1002,"volume":0,"timestamp":"2026.10.12 10:13:05"}`+"\n")
			io.WriteString(conn, "garbage line should be skipped\n")
			time.Sleep(20 * time.Millisecond)
			conn.Close()
		}
	}()

	rec := &recorder{got: make(chan struct{}, 4)}
	c := NewStreamClient(In.Addr().String(), rec, slog.New(slog.NewTextHandler(io.Discard, nil)))
	c.minBackoff = 10 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 3 * time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx)}()

	for i := 0; i < 2; i++ {
		select {
		case <-rec.got:
		case <-ctx.Done():
			t.Fatalf("only got %d ticks before timeout", i)
		}
	}
	cancel()
	if err := <-done; err != context.Canceled {
		t.Errorf("Run returned %v", err)
	}

}

