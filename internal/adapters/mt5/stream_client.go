package mt5

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync/atomic"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/ports"
)

const eaTimeLayout = "2006.01.02 15:04:05"

type StreamClient struct {
	addr        string
	handler     ports.MarketEventHandler
	log         *slog.Logger
	dialTimeout time.Duration
	minBackoff  time.Duration
	maxBackoff  time.Duration
	connected   atomic.Bool
}

type wireEvent struct {
	Type      string  `json:"type"`
	Symbol    string  `json:"symbol"`
	Bid       float64 `json:"bid"`
	Ask       float64 `json:"ask"`
	Volume    int64   `json:"volume"`
	Ticket    uint64  `json:"ticket"`
	Profit    float64 `json:"profit"`
	Timestamp string  `json:"timestamp"`
}

func NewStreamClient(addr string, h ports.MarketEventHandler, log *slog.Logger) *StreamClient {
	return &StreamClient{
		addr:        addr,
		handler:     h,
		log:         log.With("component", "mt5_stream", "addr", addr),
		dialTimeout: 5 * time.Second,
		minBackoff:  500 * time.Millisecond,
		maxBackoff:  10 * time.Second,
	}
}

// Connected บอกว่าตอนนี้ต่อกับ EA อยู่ไหม (สำหรับ / health)
func (c *StreamClient) Connected() bool { return c.connected.Load() }

func (c *StreamClient) Run(ctx context.Context) error {
	backoff := c.minBackoff
	for {
		start := time.Now()
		err := c.session(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if time.Since(start) > 30*time.Second {
			backoff = c.minBackoff
		}
		c.log.Warn("stream disconnected, will retry", "err", err, "retry_in", backoff)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, c.maxBackoff)
	}
}

func (c *StreamClient) session(ctx context.Context) error {
	d := net.Dialer{Timeout: c.dialTimeout, KeepAlive: 15 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", c.addr)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	c.connected.Store(true)
	defer c.connected.Store(false)
	c.log.Info("connected to EA stream")

	// ปิด conn เมื่อ ctx ถูก cancel เพื่อปลด scanner.Scan() ที่ block อยู่
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()

	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 0, 4096), 64*1024)
	for sc.Scan() {
		c.handleLine(ctx, sc.Bytes())
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("read: %w", err)
	}

	return errors.New("EA closed connection (EOF)")
}

func (c *StreamClient) handleLine(ctx context.Context, line []byte) {
	ev, err := parseEvent(line)
	if err != nil {
		c.log.Warn("bad line from EA", "err", err, "line", string(line))
		return
	}
	switch e := ev.(type) {
	case domain.Tick:
		c.handler.HandleTick(ctx, e)
	case domain.TradeClosed:
		c.handler.HandleTradeClosed(ctx, e)
	}

}

func parseEvent(line []byte) (any, error) {
	if len(line) == 0 {
		return nil, errors.New("empty line")
	}

	var w wireEvent
	if err := json.Unmarshal(line, &w); err != nil {
		return nil, fmt.Errorf("json: %w", err)
	}

	if w.Symbol == "" {
		return nil, errors.New("missing symbol")
	}

	ts, err := time.Parse(eaTimeLayout, w.Timestamp)
	if err != nil {
		return nil, fmt.Errorf("timestamp %q: %w", w.Timestamp, err)
	}

	switch w.Type {
	case "tick":
		if w.Bid <= 0 || w.Ask <= 0 {
			return nil, fmt.Errorf("invalid price bid=%v ask=%v", w.Bid, w.Ask)
		}
		return domain.Tick{
			Symbol: w.Symbol, 
			Bid: w.Bid,
			Ask: w.Ask,
			Volume: w.Volume,
			Time: ts,
		}, nil
		case "trade_closed":
			return domain.TradeClosed{
				Symbol: w.Symbol,
				Ticket: w.Ticket,
				Profit: w.Profit,
				Time: ts,
			}, nil
		default:
			return nil, fmt.Errorf("unknown event type %q", w.Type)
	}
}
