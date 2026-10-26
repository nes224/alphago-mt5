package mt5

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/ports"
)

type CommandClient struct {
	addr           string
	log            *slog.Logger
	dialTimeout    time.Duration
	reqTimeout     time.Duration
	ratesRetries   int
	ratesRetryWait time.Duration
	mu             sync.Mutex
	conn           net.Conn
	rd             *bufio.Reader
	connected      atomic.Bool
}

type ratesRequest struct {
	Action string `json:"action"`
	Symbol string `json:"symbol"`
	TF     int    `json:"tf"`
	From   int64  `json:"from,omitempty"`
	To     int64  `json:"to,omitempty"`
	Count  int    `json:"count,omitempty"`
}

type ratesResponse struct {
	Status     string       `json:"status"`
	Message    string       `json:"message"`
	Symbol     string       `json:"symbol"`
	TF         int          `json:"tf"`
	ServerTime int64        `json:"server_time"`
	Count      int          `json:"count"`
	HasMore    bool         `json:"has_more"`
	NextFrom   int64        `json:"next_from"`
	Rates      [][6]float64 `json:"rates"` // [time, open, high, low, close, tick_volume]
}

type RatesPage struct {
	Candles    []domain.Candle
	ServerTime time.Time
	HasMore    bool
	NextFrom   time.Time
}

type EAError struct{ Message string }

func (e *EAError) Error() string { return "EA: " + e.Message }

func (e *EAError) retryable() bool { return strings.HasPrefix(e.Message, "CopyRates failed") }

var _ ports.HistoryProvider = (*CommandClient)(nil)

func NewCommandClient(addr string, reqTimeout time.Duration, log *slog.Logger) *CommandClient {
	return &CommandClient{
		addr:           addr,
		log:            log.With("component", "mt5_cmd", "addr", addr),
		dialTimeout:    5 * time.Second,
		reqTimeout:     reqTimeout,
		ratesRetries:   5,
		ratesRetryWait: time.Second,
	}
}

func (c *CommandClient) Connected() bool { return c.connected.Load() }

func (c *CommandClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dropLocked()
	return nil
}

func (c *CommandClient) dropLocked() {
	if c.conn == nil {
		return
	}

	_ = c.conn.Close()
	c.conn = nil
	c.rd = nil
	c.connected.Store(false)
	c.log.Info("command connection closed")
}

func (c *CommandClient) dialLocked(ctx context.Context) error {
	d := net.Dialer{Timeout: c.dialTimeout, KeepAlive: 15 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", c.addr)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	c.conn = conn
	c.rd = bufio.NewReaderSize(conn, 64*1024)
	c.connected.Store(true)
	c.log.Info("connected to EA command port")
	return nil
}

func (c *CommandClient) wrapIOErr(ctx context.Context, op string, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return fmt.Errorf("%s: EA did not respond within %v (another client connected to EA?): %w", op, c.reqTimeout, err)
	}

	return fmt.Errorf("%s: %w", op, err)
}

func (c *CommandClient) Call(ctx context.Context, req any) ([]byte, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}

	payload = append(payload, '\n')
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if c.conn == nil {
		if err := c.dialLocked(ctx); err != nil {
			return nil, err
		}
	}
	deadline := time.Now().Add(c.reqTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}

	_ = c.conn.SetDeadline(deadline)

	conn := c.conn
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer stop()

	if _, err := c.conn.Write(payload); err != nil {
		c.dropLocked()
		return nil, c.wrapIOErr(ctx, "write", err)
	}

	line, err := c.rd.ReadBytes('\n')
	if err != nil {
		c.dropLocked()
		return nil, c.wrapIOErr(ctx, "read", err)
	}

	return line, err
}

func (c *CommandClient) Rates(ctx context.Context, symbol string, tf domain.Timeframe, from, to time.Time) ([]domain.Candle, error) {
	if from.IsZero() {
		return nil, errors.New("rates: from is required")
	}

	if !to.IsZero() && from.After(to) {
		return nil, fmt.Errorf("rates: from %v is after to %v", from, to)
	}

	var all []domain.Candle
	cursor := from
	for {
		page, err := c.ratesPageWithRetry(ctx, symbol, tf, cursor, to)
		if err != nil {
			return all, err
		}
		all = append(all, page.Candles...)

		if !page.HasMore {
			return all, nil
		}

		if !page.NextFrom.After(cursor) {
			return all, fmt.Errorf("rates: EA returned next_from %v not after %v", page.NextFrom, cursor)
		}
		cursor = page.NextFrom
	}
}

func (c *CommandClient) RatesPage(ctx context.Context, symbol string, tf domain.Timeframe, from, to time.Time, count int) (RatesPage, error) {
	minutes := tf.Minutes()
	if minutes == 0 {
		return RatesPage{}, fmt.Errorf("unsupported timeframe %q", tf)
	}

	req := ratesRequest{Action: "RATES", Symbol: symbol, TF: int(minutes)}
	if from.IsZero() {
		req.Count = count
	} else {
		req.From = from.Unix()
		if !to.IsZero() {
			req.To = to.Unix()
		}
	}

	line, err := c.Call(ctx, req)
	if err != nil {
		return RatesPage{}, err
	}

	var resp ratesResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return RatesPage{}, fmt.Errorf("decode RATES response: %w", err)
	}
	if resp.Status != "SUCCESS" {
		return RatesPage{}, &EAError{Message: resp.Message}
	}

	server := serverTime(resp.ServerTime)
	page := RatesPage{ServerTime: server, HasMore: resp.HasMore}
	if resp.HasMore {
		page.NextFrom = serverTime(resp.NextFrom)
	}

	dur := tf.Duration()
	page.Candles = make([]domain.Candle, 0, len(resp.Rates))
	for _, r := range resp.Rates {
		open := serverTime(int64(r[0]))
		page.Candles = append(page.Candles, domain.Candle{
			Symbol:    symbol,
			Timeframe: tf,
			OpenTime:  open,
			Open:      r[1],
			High:      r[2],
			Low:       r[3],
			Close:     r[4],
			TickCount: int(r[5]),
			Closed:    !open.Add(dur).After(server),
		})
	}

	return page, nil
}

func (c *CommandClient) ratesPageWithRetry(ctx context.Context, symbol string, tf domain.Timeframe, from, to time.Time) (RatesPage, error) {
	wait := c.ratesRetryWait
	for attempt := 0; ; attempt++ {
		page, err := c.RatesPage(ctx, symbol, tf, from, to, 0)
		var eaErr *EAError
		if err == nil || !errors.As(err, &eaErr) || !eaErr.retryable() || attempt >= c.ratesRetries {
			return page, err
		}

		c.log.Warn("RATES not ready, retrying", "symbol", symbol, "tf", tf, "from", from, "err", eaErr.Message, "retry_in", wait)
		select {
		case <-ctx.Done():
			return RatesPage{}, ctx.Err()
		case <-time.After(wait):
		}
		wait *= 2
	}
}

// serverTime แปลงวินาที unix ของ "เวลา server" เป็น time.Time ที่ location = UTC
func serverTime(sec int64) time.Time { return time.Unix(sec, 0).UTC() }

// ServerTime returns the broker's current server time, taken from a one-candle RATES request
// (implements ports.HistoryProvider).
func (c *CommandClient) ServerTime(ctx context.Context, symbol string) (time.Time, error) {
	page, err := c.RatesPage(ctx, symbol, domain.TimeframeM1, time.Time{}, time.Time{}, 1)
	if err != nil {
		return time.Time{}, err
	}
	return page.ServerTime, nil
}