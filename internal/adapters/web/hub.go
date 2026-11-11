// Package web serves the dashboard: a candlestick chart of the stored candles
// that keeps updating from live ticks pushed over Server-Sent Events (SSE).
package web

import (
	"context"
	"sync"

	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/ports"
)

// TickMsg is one live price as sent to the browser.
type TickMsg struct {
	Symbol string  `json:"symbol"`
	Bid    float64 `json:"bid"`
	Ask    float64 `json:"ask"`
	TimeMs int64   `json:"t"` // broker server time, unix milliseconds
}

// Hub is a MarketPublisher that copies every tick to the connected dashboards.
// A slow browser never blocks the gateway: when its buffer is full, ticks for it are dropped.
type Hub struct {
	mu   sync.Mutex
	subs map[chan TickMsg]struct{}
}

var _ ports.MarketPublisher = (*Hub)(nil)

func NewHub() *Hub { return &Hub{subs: make(map[chan TickMsg]struct{})} }

func (h *Hub) PublishTick(_ context.Context, t domain.Tick) {
	msg := TickMsg{Symbol: t.Symbol, Bid: t.Bid, Ask: t.Ask, TimeMs: t.Time.UnixMilli()}
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- msg:
		default: // this browser is behind; skip the tick for it
		}
	}
}

func (h *Hub) PublishCandle(context.Context, domain.Candle)           {}
func (h *Hub) PublishTradeClosed(context.Context, domain.TradeClosed) {}

// Subscribe returns a channel of ticks and a function that ends the subscription.
func (h *Hub) Subscribe() (<-chan TickMsg, func()) {
	ch := make(chan TickMsg, 256)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
	}
}

// Subscribers is the number of connected dashboards.
func (h *Hub) Subscribers() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}
