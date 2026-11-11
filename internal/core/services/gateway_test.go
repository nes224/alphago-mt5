package services

import (
	"context"
	"testing"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

type nopPublisher struct{}

func (nopPublisher) PublishTick(context.Context, domain.Tick)               {}
func (nopPublisher) PublishCandle(context.Context, domain.Candle)           {}
func (nopPublisher) PublishTradeClosed(context.Context, domain.TradeClosed) {}

// OnCandleClose fires once per closed M1 candle, not on every tick
func TestGatewayService_OnCandleCloseOncePerMinute(t *testing.T) {
	gw := NewGatewayService(nopPublisher{}, discardLog())
	closes := 0
	gw.OnCandleClose = func() { closes++ }

	base := time.Date(2026, 11, 5, 4, 0, 0, 0, time.UTC)
	tick := func(sec int) {
		gw.HandleTick(context.Background(), domain.Tick{Symbol: "XAUUSDm", Bid: 4100, Ask: 4100.25, Time: base.Add(time.Duration(sec) * time.Second)})
	}

	for _, sec := range []int{0, 10, 59} { // all inside 04:00
		tick(sec)
	}
	if closes != 0 {
		t.Fatalf("closes = %d before the minute ended, want 0", closes)
	}
	tick(61)  // 04:01 starts -> 04:00 closes
	tick(90)  // still 04:01
	tick(125) // 04:02 starts -> 04:01 closes
	if closes != 2 {
		t.Errorf("closes = %d, want 2", closes)
	}
}
