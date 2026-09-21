package mt5

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/ports"
)

type tcpAdapter struct {
	conn net.Conn
}

var _ ports.MT5Port = (*tcpAdapter)(nil)

func NewTCPAdapter(host string, port int, timeoutSeconds int) (ports.MT5Port, error) {
	address := fmt.Sprintf("%s:%d", host, port)
	conn, err := net.DialTimeout("tcp", address, time.Duration(timeoutSeconds)*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MT5 socket: %w", err)
	}

	return &tcpAdapter{conn: conn}, nil
}

func (a *tcpAdapter) SendOrder(ctx context.Context, req domain.TradeRequest) (*domain.TradeResponse, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal trade request: %w", err)
	}

	if deadline, ok := ctx.Deadline(); ok {
		_ = a.conn.SetDeadline(deadline)
	}

	_, err = a.conn.Write(append(data, '\n'))
	if err != nil {
		return nil, fmt.Errorf("failed to send order to MT5: %w", err)
	}

	reader := bufio.NewReader(a.conn)
	resBytes, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read response from MT5: %w", err)
	}

	var resp domain.TradeResponse
	if err := json.Unmarshal(resBytes, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal MT5 response: %w", err)
	}

	return &resp, nil
}

func (a *tcpAdapter) Close() error {
	if a.conn != nil {
		return a.conn.Close()
	}
	return nil
}