package mt5

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/ports"
)

type TCPAdapter struct {
	addr    string
	timeout time.Duration
	conn    net.Conn
	mu      sync.Mutex
}

var _ ports.MT5Port = (*TCPAdapter)(nil)

func NewTCPAdapter(addr string, timeout time.Duration) *TCPAdapter {
	return &TCPAdapter{
		addr:    addr,
		timeout: timeout,
	}
}

func (a *TCPAdapter) connect() error {
	if a.conn != nil {
		return nil
	}

	conn, err := net.DialTimeout("tcp", a.addr, a.timeout)
	if err != nil {
		return fmt.Errorf("failed to connect to MT5 listener at %s: %w", a.addr, err)
	}

	a.conn = conn
	return nil
}

func (a *TCPAdapter) SendOrder(ctx context.Context, req domain.TradeRequest) (*domain.TradeResponse, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if err := a.connect(); err != nil {
		return nil, err
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(a.timeout)
	}
	_ = a.conn.SetDeadline(deadline)

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal trade request: %w", err)
	}
	payload = append(payload, '\n')

	if _, err := a.conn.Write(payload); err != nil {
		a.closeConn()
		return nil, fmt.Errorf("failed to write to MT5 socket: %w", err)
	}

	reader := bufio.NewReader(a.conn)
	respBytes, err := reader.ReadBytes('\n')
	if err != nil {
		a.closeConn()
		return nil, fmt.Errorf("failed to read response from MT5: %w", err)
	}

	var resp domain.TradeResponse
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal MT5 response: %w", err)
	}

	return &resp, nil
}

func (a *TCPAdapter) closeConn() {
	if a.conn != nil {
		_ = a.conn.Close()
		a.conn = nil
	}
}

func (a *TCPAdapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.closeConn()
	return nil
}