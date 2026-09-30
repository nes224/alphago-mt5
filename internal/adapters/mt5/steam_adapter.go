package mt5

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

type StreamAdapter struct {
	addr            string
	conn            net.Conn
	tickChan        chan domain.Tick
	tradeClosedChan chan domain.TradeClosedEvent
	reconnectD      time.Duration
	mu              sync.Mutex
	isClosed        bool
	startOnce       sync.Once
}

// rawMessage คือ envelope ที่ EA (AlphaGo_Listener.mq5) ส่งมาทาง Stream Socket
// — "type" บอกว่าเป็น tick ราคา หรือ trade_closed (จาก OnTradeTransaction);
// field ที่ไม่เกี่ยวกับ type นั้นๆ จะเป็น zero value เฉยๆ ไม่ error
type rawMessage struct {
	Type         string  `json:"type"`
	Symbol       string  `json:"symbol"`
	Bid          float64 `json:"bid"`
	Ask          float64 `json:"ask"`
	Volume       float64 `json:"volume"`
	OpenInterest int64   `json:"open_interest"`
	Ticket       uint64  `json:"ticket"`
	Profit       float64 `json:"profit"`
	Time         string  `json:"timestamp"` // EA ส่งมาเป็น key "timestamp" ไม่ใช่ "time"
}

func NewStreamAdapter(addr string) *StreamAdapter {
	return &StreamAdapter{
		addr:            addr,
		tickChan:        make(chan domain.Tick, 1000),
		tradeClosedChan: make(chan domain.TradeClosedEvent, 100),
		reconnectD:      3 * time.Second,
	}
}

func (s *StreamAdapter) SubscribeTicks(ctx context.Context) (<-chan domain.Tick, error) {
	s.startOnce.Do(func() { go s.connectAndStream(ctx) })
	return s.tickChan, nil
}

// SubscribeTradeEvents คืน channel ของ "position ปิดแล้ว" event (win/loss จริง)
// ใช้ TCP connection เดียวกับ SubscribeTicks (คนละ channel แต่อ่านจาก stream
// เดียวกัน) — เรียกได้ทั้งคู่โดย connectAndStream loop จะรันแค่ครั้งเดียวเท่านั้น
// (กันไม่ให้ dial ซ้ำสองครั้งถ้าเรียกทั้ง SubscribeTicks และ SubscribeTradeEvents)
func (s *StreamAdapter) SubscribeTradeEvents(ctx context.Context) (<-chan domain.TradeClosedEvent, error) {
	s.startOnce.Do(func() { go s.connectAndStream(ctx) })
	return s.tradeClosedChan, nil
}

func (s *StreamAdapter) connectAndStream(ctx context.Context) {
	// เมื่อ Goroutine นี้จบทำงาน (Shutdown หรือ Context Cancel) ให้ปิด channel ที่นี่จุดเดียว
	defer close(s.tickChan)
	defer close(s.tradeClosedChan)

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("[StreamAdapter] Context canceled, stopping stream worker...")
			return
		default:
		}

		s.mu.Lock()
		if s.isClosed {
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()

		log.Info().Str("addr", s.addr).Msg("[StreamAdapter] Connecting to MT5 Tick Streamer...")

		// ใช้ DialContext เพื่อให้ยกเลิกการ Dial ได้ทันทีเมื่อ Context ถูก cancel
		var dialer net.Dialer
		conn, err := dialer.DialContext(ctx, "tcp", s.addr)
		if err != nil {
			log.Warn().Err(err).Dur("retry_in", s.reconnectD).Msg("[StreamAdapter] Connection failed")
			if !s.sleepWithContext(ctx, s.reconnectD) {
				return // หลุดลูปทันทีถ้า Context ยกเลิก
			}
			continue
		}

		s.mu.Lock()
		s.conn = conn
		s.mu.Unlock()

		log.Info().Msg("[StreamAdapter] Successfully connected to MT5 Tick Stream!")

		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			line := scanner.Bytes()
			var raw rawMessage
			if err := json.Unmarshal(line, &raw); err != nil {
				log.Warn().Err(err).Msg("[StreamAdapter] JSON Unmarshal Error")
				continue
			}

			parsedTime, err := time.Parse("2006.01.02 15:04:05", raw.Time)
			if err != nil {
				// EA ไม่ได้ส่ง time มา (หรือ format ไม่ตรง) — ใช้เวลาที่รับฝั่ง Go แทน
				// เพื่อไม่ให้ข้อความหายไปทั้งเส้น (กระทบความแม่นยำของ PriceVelocity/
				// OIVelocity เล็กน้อยเพราะเวลาไม่ตรงเป๊ะกับฝั่ง MT5 แต่ยังดีกว่า drop ทิ้งทั้งหมด)
				log.Debug().Err(err).Msg("[StreamAdapter] Time Parse Error (falling back to time.Now())")
				parsedTime = time.Now()
			}

			if raw.Type == "trade_closed" {
				event := domain.TradeClosedEvent{
					Symbol:    raw.Symbol,
					Ticket:    raw.Ticket,
					Profit:    raw.Profit,
					Timestamp: parsedTime,
				}

				select {
				case <-ctx.Done():
					conn.Close()
					return
				case s.tradeClosedChan <- event:
				default:
					log.Warn().Msg("[StreamAdapter] Trade-closed channel buffer full, dropping event")
				}
				continue
			}

			tick := domain.Tick{
				Symbol:       raw.Symbol,
				Bid:          raw.Bid,
				Ask:          raw.Ask,
				Volume:       int64(raw.Volume),
				OpenInterest: raw.OpenInterest,
				Timestamp:    parsedTime,
			}

			// ส่งเข้า Channel อย่างปลอดภัย
			select {
			case <-ctx.Done():
				conn.Close()
				return
			case s.tickChan <- tick:
			default:
				log.Warn().Msg("[StreamAdapter] Tick channel buffer full, dropping tick")
			}
		}

		if err := scanner.Err(); err != nil {
			log.Warn().Err(err).Msg("[StreamAdapter] Read error")
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
	// ไม่ปิด channel ที่นี่ ให้ defer ใน connectAndStream เป็นผู้ปิด
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
