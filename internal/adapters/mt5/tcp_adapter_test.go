package mt5_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/nes224/alphago-mt5/internal/adapters/mt5"
	"github.com/nes224/alphago-mt5/internal/core/domain"
)

func startMockMT5Server(t *testing.T, address string) func() {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatalf("Failed to start mock server: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		reader := bufio.NewReader(conn)
		reqBytes, err := reader.ReadBytes('\n')
		if err != nil {
			return
		}

		var req domain.TradeRequest
		if err := json.Unmarshal(reqBytes, &req); err != nil {
			return
		}

		resp := domain.TradeResponse{
			Success:   true,
			Ticket:    88888,
			Price:     1.0850,
			Message:   "Mock Order Placed Successfully",
			Timestamp: time.Now(),
		}

		respBytes, _ := json.Marshal(resp)
		conn.Write(append(respBytes, '\n'))
	}()

	return func() {
		listener.Close()
		<-done
	}
}

func TestTCPAdapter_Integration(t *testing.T) {
	serverAddr := "127.0.0.1:5555"
	stopServer := startMockMT5Server(t, serverAddr)
	defer stopServer()

	time.Sleep(50 * time.Millisecond)

	adapter := mt5.NewTCPAdapter("127.0.0.1:5555", 2*time.Second)
	defer adapter.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req := domain.TradeRequest{
		Action:   "BUY",
		Symbol:   "EURUSD",
		Volume:   0.01,
		MagicNum: 12456,
	}

	resp, err := adapter.SendOrder(ctx, req)
	if err != nil {
		t.Fatalf("Integration test failed on SendOrder: %v", err)
	}

	if !resp.Success {
		t.Errorf("Expected success response, got false")
	}

	if resp.Ticket != 88888 {
		t.Errorf("Expected ticket 88888, got %d", resp.Ticket)
	}
}
