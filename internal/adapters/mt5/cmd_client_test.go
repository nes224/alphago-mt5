package mt5

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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
