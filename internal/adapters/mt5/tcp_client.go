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

func NewTCPAdapter(host string, port int, timeoutSeconds int) (ports.MT5Port, error) {
	address := fmt.Sprintf("%s:%d", host, port)
	conn, err := net.DialTimeout("tcp", address, time.Duration(timeoutSeconds)*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MT5 socket: %w", err)
	}

	return &tcpAdapter{conn: conn}, nil
}

func (a *tcpAdapter) ExecuteOrder(ctx context.Context, order domain.TradeOrder) (*domain.OrderResult, error) {
	payload := map[string]interface{}{
		"action": "TRADE",
		"symbol": order.Symbol,
		"type":   string(order.Type),
		"volume": order.Volume,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal order: %w", err)
	}

	_, err = a.conn.Write(append(data, '\n'))
	if err != nil {
		return nil, fmt.Errorf("failed to send order to MT5: %w", err)
	}

	reader := bufio.NewReader(a.conn)
	resBytes, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read MT5 response: %w", err)
	}

	var result domain.OrderResult
	if err := json.Unmarshal(resBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal MT5 response: %w", err)
	}


	return &result, nil
}


func (a *tcpAdapter) Close() error {
	if a.conn != nil {
		return a.conn.Close()
	}

	return nil
}