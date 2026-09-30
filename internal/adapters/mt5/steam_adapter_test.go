package mt5_test

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nes224/alphago-mt5/internal/adapters/mt5"
)

// startFakeStreamServer spins up a local TCP listener that writes the given
// raw JSON lines to the first connection it accepts, simulating the EA side
// of the stream socket without needing MT5/Wine.
func startFakeStreamServer(t *testing.T, lines []string) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start fake stream server: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		for _, line := range lines {
			conn.Write([]byte(line + "\n"))
			time.Sleep(5 * time.Millisecond)
		}
		// keep the connection open briefly so the client can finish reading
		time.Sleep(100 * time.Millisecond)
	}()

	return ln.Addr().String()
}

func TestStreamAdapter_RoutesTicksAndTradeClosedEventsSeparately(t *testing.T) {
	addr := startFakeStreamServer(t, []string{
		`{"type":"tick","symbol":"XAUUSDm","bid":4183.30,"ask":4183.55,"volume":12,"timestamp":"2026.09.30 13:15:35"}`,
		`{"type":"trade_closed","symbol":"XAUUSDm","ticket":5184444142,"profit":12.34,"timestamp":"2026.09.30 13:16:00"}`,
	})

	adapter := mt5.NewStreamAdapter(addr)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tickChan, err := adapter.SubscribeTicks(ctx)
	if err != nil {
		t.Fatalf("unexpected error subscribing to ticks: %v", err)
	}
	tradeChan, err := adapter.SubscribeTradeEvents(ctx)
	if err != nil {
		t.Fatalf("unexpected error subscribing to trade events: %v", err)
	}

	select {
	case tick := <-tickChan:
		if tick.Symbol != "XAUUSDm" || tick.Bid != 4183.30 {
			t.Errorf("unexpected tick: %+v", tick)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for tick")
	}

	select {
	case event := <-tradeChan:
		if event.Symbol != "XAUUSDm" || event.Ticket != 5184444142 || event.Profit != 12.34 {
			t.Errorf("unexpected trade closed event: %+v", event)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for trade closed event")
	}
}

func TestStreamAdapter_SubscribingBothDoesNotDialTwice(t *testing.T) {
	var connectCount atomic.Int32
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start fake stream server: %v", err)
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			connectCount.Add(1)
			conn.Write([]byte(`{"type":"tick","symbol":"XAUUSDm","bid":1,"ask":1.1,"volume":1,"timestamp":"2026.09.30 13:15:35"}` + "\n"))
			time.Sleep(200 * time.Millisecond)
			conn.Close()
		}
	}()

	adapter := mt5.NewStreamAdapter(ln.Addr().String())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if _, err := adapter.SubscribeTicks(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := adapter.SubscribeTradeEvents(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	if got := connectCount.Load(); got > 1 {
		t.Errorf("expected at most 1 connection from calling both Subscribe methods, got %d", got)
	}
}
