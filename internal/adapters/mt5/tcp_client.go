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

// sendAndReceive เขียน payload (JSON + \n) ลง command socket แล้วอ่าน response
// บรรทัดเดียวกลับมา — ใช้ร่วมกันทั้ง SendOrder และ GetAccountInfo เพราะ EA
// คุยผ่าน protocol เดียวกันหมด (JSON line-in, JSON line-out) ไม่ว่า action ไหน
func (a *TCPAdapter) sendAndReceive(ctx context.Context, payload []byte) ([]byte, error) {
	if err := a.connect(); err != nil {
		return nil, err
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(a.timeout)
	}
	_ = a.conn.SetDeadline(deadline)

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

	return respBytes, nil
}

func (a *TCPAdapter) SendOrder(ctx context.Context, req domain.TradeRequest) (*domain.TradeResponse, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal trade request: %w", err)
	}
	payload = append(payload, '\n')

	respBytes, err := a.sendAndReceive(ctx, payload)
	if err != nil {
		return nil, err
	}

	var resp domain.TradeResponse
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal MT5 response: %w", err)
	}

	return &resp, nil
}

// GetAccountInfo ขอข้อมูลบัญชีสดจาก MT5 (balance, equity, account type, symbol
// ที่เทรดได้จริงตอนนี้) ผ่าน command socket เดียวกับ SendOrder — ใช้แทนการอ่าน
// ACCOUNT_BALANCE จาก app.env ตอน seed ครั้งแรก (ดู cmd/app/main.go)
func (a *TCPAdapter) GetAccountInfo(ctx context.Context) (*domain.AccountInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	respBytes, err := a.sendAndReceive(ctx, []byte("{\"action\":\"ACCOUNT_INFO\"}\n"))
	if err != nil {
		return nil, err
	}

	var info domain.AccountInfo
	if err := json.Unmarshal(respBytes, &info); err != nil {
		return nil, fmt.Errorf("failed to unmarshal MT5 account info response: %w", err)
	}
	if !info.IsSuccess() {
		return nil, fmt.Errorf("MT5 returned non-success status for account info: %s", info.Status)
	}

	return &info, nil
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
