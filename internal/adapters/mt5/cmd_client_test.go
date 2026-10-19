package mt5

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

type fakeEA struct {
	t        *testing.T
	ln       net.Listener
	handler  func(conn int, req map[string]any) string
	conns    atomic.Int32
	inFlight atomic.Int32
	overlap  atomic.Bool
}

func newFakeEA(t *testing.T, h func(conn int, req map[string]any) string) *fakeEA {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	f := &fakeEA{t: t, ln: ln, handler: h}
	go f.serve()
	t.Cleanup(func() { ln.Close() })
	return f
}

func (f *fakeEA) addr() string { return f.ln.Addr().String() }

func (f *fakeEA) serve() {
	for {
		conn, err := f.ln.Accept()
		if err != nil {
			return
		}

		n := int(f.conns.Add(1))
		go func() {
			defer conn.Close()
			rd := bufio.NewReader(conn)
			for {
				line, err := rd.ReadBytes('\n')
				if err != nil {
					return
				}
				if f.inFlight.Add(1) > 1 {
					f.overlap.Store(true)
				}

				var req map[string]any
				_ = json.Unmarshal(line, &req)
				resp := f.handler(n, req)
				if resp == "" {
					f.inFlight.Add(-1)
					continue
				}

				time.Sleep(time.Millisecond)
				f.inFlight.Add(-1)
				io.WriteString(conn, resp+"\n")
			}
		}()
	}
}

func quiteLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func newTestClient(addr string, timeout time.Duration) *CommandClient {
	c := NewCommandClient(addr, timeout, quiteLog())
	c.ratesRetryWait = 5 * time.Millisecond
	return c
}

const t0 = int64(1792133160) // 2026-10-16T06:46:00 เวลา server

func ratesJSON(serevrTime int64, hasMore bool, nextFrom int64, bars ...int64) string {
	rows := make([]string, len(bars))
	for i, ts := range bars {
		rows[i] = fmt.Sprintf("[%d, 4178.279, 4178.757, 4177.466, 4178.036,179]", ts)
	}
	return fmt.Sprintf(`{"status": "SUCCESS", "symbol": "XAUUSDm", "tf":1, "server_time":%d, "count":%d}, "has_more":%v, "next_from": %d, "rates":[%s]`,
		serevrTime, len(bars), hasMore, nextFrom, strings.Join(rows, ","))
}

func TestRatesPage_ParsesEAResponse(t *testing.T) {
	var got map[string]any
	ea := newFakeEA(t, func(_ int, req map[string]any) string {
		got = req
		return ratesJSON(t0+150, false, 0, t0, t0+60, t0+120)
	})
	c := newTestClient(ea.addr(), time.Second)

	page, err := c.RatesPage(context.Background(), "XAUUSDm",  domain.TimeframeM1, time.Unix(t0, 0).UTC(), time.Time{}, 0)
	if err != nil {
		t.Fatal(err)
	}

	if got["action"] != "RATES" || got["tf"] != float64(1) || got["from"] != float64(t0) || got["to"] != nil {
		t.Errorf("request = %v", got)
	}
	if len(page.Candles) != 3 {
		t.Fatalf("candles = %d", len(page.Candles))
	}

	first := page.Candles[0]
	if first.OpenTime != time.Unix(t0, 0).UTC() || first.Open != 4178.279 || first.Close != 4178.036 || first.TickCount != 179 {
		t.Errorf("first = %+v", first)
	}
	if !page.Candles[0].Closed || !page.Candles[1].Closed || page.Candles[2].Closed {
		t.Errorf("closed flags = %v %v %v", page.Candles[0].Closed, page.Candles[1].Closed, page.Candles[2].Closed)
	}
}

func TestRates_FollowsPagesUntilDone(t *testing.T) {
	var froms []int64
	ea := newFakeEA(t, func(_ int, req map[string]any) string {
		from := int64(req["from"].(float64))
		froms = append(froms, from)
		switch from {
		case t0:
			return ratesJSON(t0+600, true, t0+120, t0, t0+60)
		case t0 + 120:
			return ratesJSON(t0+600, true, t0+240, t0+120, t0+180)
		case t0 + 240:
			return ratesJSON(t0+600, false, 0, t0+240)
		}
		return `{"status": "FAILED", "message": "unexpected from"}`
	})
	c := newTestClient(ea.addr(), time.Second)

	candles, err := c.Rates(context.Background(), "XAUUSDm", domain.TimeframeM1, time.Unix(t0, 0).UTC(), time.Unix(t0+600, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(candles) != 5 {
		t.Fatalf("candles = %d, want 5", len(candles))
	}

	for i, c := range candles {
		if want := time.Unix(t0+int64(i)*60, 0).UTC(); c.OpenTime != want {
			t.Errorf("candle %d open = %v, want %v", i, c.OpenTime, want)
		}
	}

	if len(froms) != 3 || ea.conns.Load() != 1 {
		t.Errorf("requests froms=%v conns=%d (should reuse existing connection)", froms, ea.conns.Load())
	}

}