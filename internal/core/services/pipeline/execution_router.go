package pipeline

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/nes224/alphago-mt5/internal/core/ports"
	"github.com/nes224/alphago-mt5/internal/core/services/risk"
)

const signalHistoryCapacity = 200

const (
	SignalStatusDispatched        = "DISPATCHED"
	SignalStatusRejectedSizing    = "REJECTED_SIZING"
	SignalStatusRejectedRiskGuard = "REJECTED_RISK_GUARD"
	SignalStatusDroppedQueueFull  = "DROPPED_QUEUE_FULL"
)

// SignalRecord is a lightweight, JSON-friendly snapshot of a signal the
// ExecutionRouter processed, kept in memory (and optionally persisted via a
// SignalStore) for monitoring/debugging (e.g. GET /api/v1/signals) without
// having to grep logs.
type SignalRecord struct {
	Symbol    string    `json:"symbol"`
	Action    string    `json:"action"`
	Status    string    `json:"status"`
	Reason    string    `json:"reason"`
	Detail    string    `json:"detail,omitempty"`
	LotSize   float64   `json:"lot_size,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// SignalStore เป็น port ที่ adapter ชั้นนอก (เช่น internal/adapters/database)
// ต้อง implement เพื่อให้ประวัติ signal รอดจาก restart แทน in-memory ring buffer ล้วนๆ
type SignalStore interface {
	SaveSignal(rec SignalRecord) error
	RecentSignals(limit int) ([]SignalRecord, error)
}

// PendingOrder คือข้อมูลที่จำเป็นต้องเขียนลง Outbox ก่อนจริงๆ จะยิง order เข้า
// MT5 — แยกจาก risk.PreparedOrder เพื่อไม่ให้ package risk ต้องรู้จัก storage
type PendingOrder struct {
	Symbol     string
	Action     string
	LotSize    float64
	StopLoss   float64
	TakeProfit float64
	Reason     string
}

// สถานะ Outbox — ต้องตรงกับค่า string ของ database.PendingOrderStatus* (เก็บ
// เป็น string ธรรมดาไม่ผูก type กับ adapter ชั้นนอกตามหลัก Hexagonal Architecture)
const (
	OutboxStatusFailed = "FAILED"
)

// OutboxStore เป็น port ที่ adapter ชั้นนอกต้อง implement เพื่อรองรับ Outbox
// Pattern — เขียนแถว PENDING ไว้ก่อนส่งจริง กัน order ที่ "ตัดสินใจจะส่งแล้ว"
// หายไปเงียบๆ ถ้า service crash ระหว่างตัดสินใจกับส่งจริงเข้า MT5
type OutboxStore interface {
	CreatePendingOrder(order PendingOrder) (id uint, err error)
	MarkOrderOutcome(id uint, status string, ticket uint64, errMsg string) error
}

type ExecutionRouter struct {
	engine      ports.QuantEngine
	riskMgr     *risk.RiskManager
	riskGuard   *risk.RiskGuard
	orderSink   chan<- risk.PreparedOrder
	workerCount int
	store       SignalStore
	outbox      OutboxStore

	historyMu sync.Mutex
	history   []SignalRecord
}

// AttachStore ผูก SignalStore เข้ากับ ExecutionRouter — จากนั้น RecentSignals()
// จะอ่านจาก store แทน in-memory ring buffer (ทำให้ประวัติรอดจาก restart)
func (r *ExecutionRouter) AttachStore(store SignalStore) {
	r.historyMu.Lock()
	defer r.historyMu.Unlock()
	r.store = store
}

// AttachOutboxStore ผูก OutboxStore เข้ากับ ExecutionRouter — ถ้าไม่ผูกไว้
// (nil) จะข้ามการเขียน outbox row ไปเลย (ทำงานเหมือนเดิมก่อนมี Outbox Pattern)
func (r *ExecutionRouter) AttachOutboxStore(store OutboxStore) {
	r.historyMu.Lock()
	defer r.historyMu.Unlock()
	r.outbox = store
}

func NewExecutionRouter(
	engine ports.QuantEngine,
	riskMgr *risk.RiskManager,
	riskGuard *risk.RiskGuard,
	orderSink chan<- risk.PreparedOrder,
	workers int) *ExecutionRouter {
	return &ExecutionRouter{
		engine:      engine,
		riskMgr:     riskMgr,
		riskGuard:   riskGuard,
		orderSink:   orderSink,
		workerCount: workers,
	}
}

func (r *ExecutionRouter) Start(ctx context.Context) {
	for i := 0; i < r.workerCount; i++ {
		go func(workerID int) {
			for {
				select {
				case <-ctx.Done():
					return
				case signal, ok := <-r.engine.SignalChannel():
					if !ok {
						return
					}

					metrics := r.engine.GetLatestMetrics(signal.Symbol)

					preparedOrder, err := r.riskMgr.CalculateOrder(signal, metrics)
					if err != nil {
						log.Warn().Int("worker", workerID).Err(err).Msg("Risk calculation rejected signal")
						r.recordSignal(SignalRecord{
							Symbol: signal.Symbol,
							Action: string(signal.Action),
							Status: SignalStatusRejectedSizing,
							Reason: signal.Reason,
							Detail: err.Error(),
						})
						continue
					}

					if r.riskGuard != nil {
						if err := r.riskGuard.ValidateOrder(*preparedOrder, metrics); err != nil {
							log.Warn().Int("worker", workerID).Str("symbol", signal.Symbol).Err(err).Msg("RISK GUARD BLOCKED signal")
							r.recordSignal(SignalRecord{
								Symbol:  preparedOrder.Symbol,
								Action:  string(preparedOrder.Action),
								Status:  SignalStatusRejectedRiskGuard,
								Reason:  preparedOrder.Reason,
								Detail:  err.Error(),
								LotSize: preparedOrder.LotSize,
							})
							continue
						}
					}

					// เขียน outbox row ก่อนจริงๆ จะ push เข้า orderSink — ถ้า service
					// crash หลังจุดนี้แต่ก่อน sender goroutine ยิงเข้า MT5 จริง จะยังมี
					// แถว PENDING ค้างให้ตรวจเจอตอน restart แทนที่จะหายไปเงียบๆ
					r.historyMu.Lock()
					outbox := r.outbox
					r.historyMu.Unlock()
					if outbox != nil {
						id, err := outbox.CreatePendingOrder(PendingOrder{
							Symbol:     preparedOrder.Symbol,
							Action:     string(preparedOrder.Action),
							LotSize:    preparedOrder.LotSize,
							StopLoss:   preparedOrder.StopLoss,
							TakeProfit: preparedOrder.TakeProfit,
							Reason:     preparedOrder.Reason,
						})
						if err != nil {
							log.Warn().Int("worker", workerID).Err(err).Msg("[ExecutionRouter] failed to write outbox row, proceeding without it")
						} else {
							preparedOrder.OutboxID = id
						}
					}

					select {
					case r.orderSink <- *preparedOrder:
						if r.riskGuard != nil {
							r.riskGuard.MarkPositionOpened(preparedOrder.Symbol)
						}
						log.Info().
							Int("worker", workerID).
							Str("action", string(preparedOrder.Action)).
							Str("symbol", preparedOrder.Symbol).
							Float64("lot", preparedOrder.LotSize).
							Float64("entry", preparedOrder.EntryPrice).
							Float64("sl", preparedOrder.StopLoss).
							Float64("tp", preparedOrder.TakeProfit).
							Msg("ORDER DISPATCHED")
						r.recordSignal(SignalRecord{
							Symbol:  preparedOrder.Symbol,
							Action:  string(preparedOrder.Action),
							Status:  SignalStatusDispatched,
							Reason:  preparedOrder.Reason,
							LotSize: preparedOrder.LotSize,
						})
					default:
						log.Warn().Int("worker", workerID).Str("symbol", preparedOrder.Symbol).Msg("Order Sink Channel is full, dropping order")
						// Order ไม่มีทางไปถึง MT5 แน่นอน (channel เต็ม ไม่ใช่ crash) — ปิด
						// outbox row เป็น FAILED ทันที กัน restart ครั้งถัดไปเข้าใจผิดว่า
						// เป็น order ค้างที่ต้องเช็คมือ
						if outbox != nil && preparedOrder.OutboxID != 0 {
							if err := outbox.MarkOrderOutcome(preparedOrder.OutboxID, OutboxStatusFailed, 0, "dropped: order sink queue full"); err != nil {
								log.Warn().Int("worker", workerID).Err(err).Msg("[ExecutionRouter] failed to mark dropped outbox row as failed")
							}
						}
						r.recordSignal(SignalRecord{
							Symbol:  preparedOrder.Symbol,
							Action:  string(preparedOrder.Action),
							Status:  SignalStatusDroppedQueueFull,
							Reason:  preparedOrder.Reason,
							LotSize: preparedOrder.LotSize,
						})
					}
				}
			}
		}(i)
	}
}

// recordSignal appends a record to the bounded in-memory history (evicting
// the oldest entry once at capacity), and persists it via SignalStore if one
// is attached — the DB write happens outside the history lock since it's I/O.
func (r *ExecutionRouter) recordSignal(rec SignalRecord) {
	rec.Timestamp = time.Now()

	r.historyMu.Lock()
	r.history = append(r.history, rec)
	if len(r.history) > signalHistoryCapacity {
		r.history = r.history[len(r.history)-signalHistoryCapacity:]
	}
	store := r.store
	r.historyMu.Unlock()

	if store != nil {
		if err := store.SaveSignal(rec); err != nil {
			log.Warn().Err(err).Msg("[ExecutionRouter] failed to persist signal record")
		}
	}
}

// RecentSignals returns the most recently processed signals, newest first.
// Prefers the attached SignalStore (durable across restarts) when present,
// falling back to the in-memory ring buffer otherwise.
func (r *ExecutionRouter) RecentSignals() []SignalRecord {
	r.historyMu.Lock()
	store := r.store
	r.historyMu.Unlock()

	if store != nil {
		recent, err := store.RecentSignals(signalHistoryCapacity)
		if err != nil {
			log.Warn().Err(err).Msg("[ExecutionRouter] failed to read signal history from store")
		} else {
			return recent
		}
	}

	r.historyMu.Lock()
	defer r.historyMu.Unlock()

	out := make([]SignalRecord, len(r.history))
	for i, rec := range r.history {
		out[len(r.history)-1-i] = rec
	}
	return out
}
