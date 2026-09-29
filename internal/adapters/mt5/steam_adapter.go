package mt5

import (
	"bufio"
	"context"
	"encoding/json"
	"log"
	"net"
	"sync"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

type StreamAdapter struct {
	addr       string
	conn       net.Conn
	tickChan   chan domain.Tick
	reconnectD time.Duration
	mu         sync.Mutex
	isClosed   bool
}

type rawTick struct {
	Symbol string  `json:"symbol"`
	Bid    float64 `json:"bid"`
	Ask    float64 `json:"ask"`
	Time   string  `json:"time"`
}

func NewStreamAdapter(addr string) *StreamAdapter {
	return &StreamAdapter{
		addr:       addr,
		tickChan:   make(chan domain.Tick, 1000),
		reconnectD: 3 * time.Second,
	}
}

func (s *StreamAdapter) SubscribeTicks(ctx context.Context) (<-chan domain.Tick, error) {
	go s.connectAndStream(ctx)
	return s.tickChan, nil
}

func (s *StreamAdapter) connectAndStream(ctx context.Context) {
	// เมื่อ Goroutine นี้จบทำงาน (Shutdown หรือ Context Cancel) ให้ปิด tickChan ที่นี่จุดเดียว
	defer close(s.tickChan)

	for {
		select {
		case <-ctx.Done():
			log.Println("[StreamAdapter] Context canceled, stopping stream worker...")
			return
		default:
		}

		s.mu.Lock()
		if s.isClosed {
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()

		log.Printf("[StreamAdapter] Connecting to MT5 Tick Streamer at %s...", s.addr)

		// ใช้ DialContext เพื่อให้ยกเลิกการ Dial ได้ทันทีเมื่อ Context ถูก cancel
		var dialer net.Dialer
		conn, err := dialer.DialContext(ctx, "tcp", s.addr)
		if err != nil {
			log.Printf("[StreamAdapter] Connection failed: %v. Retrying in %v...", err, s.reconnectD)
			if !s.sleepWithContext(ctx, s.reconnectD) {
				return // หลุดลูปทันทีถ้า Context ยกเลิก
			}
			continue
		}

		s.mu.Lock()
		s.conn = conn
		s.mu.Unlock()

		log.Println("[StreamAdapter] Successfully connected to MT5 Tick Stream!")

		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			line := scanner.Bytes()
			var raw rawTick
			if err := json.Unmarshal(line, &raw); err != nil {
				log.Printf("[StreamAdapter] JSON Unmarshal Error: %v", err)
				continue
			}

			parsedTime, err := time.Parse("2006.01.02 15:04:05", raw.Time)
			if err != nil {
				log.Printf("[StreamAdapter] Time Parse Error: %v", err)
				continue
			}

			tick := domain.Tick{
				Symbol:    raw.Symbol,
				Bid:       raw.Bid,
				Ask:       raw.Ask,
				Timestamp: parsedTime,
			}

			// ส่งเข้า Channel อย่างปลอดภัย
			select {
			case <-ctx.Done():
				conn.Close()
				return
			case s.tickChan <- tick:
			default:
				log.Println("[StreamAdapter] Warning: Tick channel buffer full, dropping tick")
			}
		}

		if err := scanner.Err(); err != nil {
			log.Printf("[StreamAdapter] Read error: %v", err)
		}

		s.cleanConn()

		s.mu.Lock()
		if s.isClosed {
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()

		if !s.sleepWithContext(ctx, s.reconnectD) {
			return
		}
	}
}

func (s *StreamAdapter) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isClosed {
		return nil
	}

	s.isClosed = true
	var err error
	if s.conn != nil {
		err = s.conn.Close()
		s.conn = nil
	}
	// ไม่ปิด s.tickChan ที่นี่ ให้ defer ใน connectAndStream เป็นผู้ปิด
	return err
}

func (s *StreamAdapter) cleanConn() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}
}

// sleepWithContext ช่วยให้สั่งรัน Sleep โดยยกเลิกได้ทันทีถ้า Context Done
func (s *StreamAdapter) sleepWithContext(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
