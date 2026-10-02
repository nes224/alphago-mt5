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
	SignalStatusRejectedWinRate   = "REJECTED_WIN_RATE"
	SignalStatusDroppedQueueFull  = "DROPPED_QUEUE_FULL"
)

type SignalRecord struct {
	Symbol    string    `json:"symbol"`
	Action    string    `json:"action"`
	Status    string    `json:"status"`
	Reason    string    `json:"reason"`
	Detail    string    `json:"detail,omitempty"`
	LotSize   float64   `json:"lot_size,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}
type SignalStore interface {
	SaveSignal(rec SignalRecord) error
	RecentSignals(limit int) ([]SignalRecord, error)
}
type PendingOrder struct {
	Symbol     string
	Action     string
	LotSize    float64
	StopLoss   float64
	TakeProfit float64
	Reason     string
}

const (
	OutboxStatusFailed = "FAILED"
)

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
	winRateGate *risk.WinRateGate

	historyMu sync.Mutex
	history   []SignalRecord
}

func (r *ExecutionRouter) SetWinRateGate(g *risk.WinRateGate) {
	r.historyMu.Lock()
	defer r.historyMu.Unlock()
	r.winRateGate = g
}

func (r *ExecutionRouter) AttachStore(store SignalStore) {
	r.historyMu.Lock()
	defer r.historyMu.Unlock()
	r.store = store
}

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

					r.historyMu.Lock()
					winRateGate := r.winRateGate
					r.historyMu.Unlock()
					if winRateGate != nil && !winRateGate.Allows(preparedOrder.Reason) {
						log.Warn().Int("worker", workerID).Str("symbol", signal.Symbol).Str("reason", preparedOrder.Reason).Msg("WIN RATE GATE BLOCKED signal")
						r.recordSignal(SignalRecord{
							Symbol:  preparedOrder.Symbol,
							Action:  string(preparedOrder.Action),
							Status:  SignalStatusRejectedWinRate,
							Reason:  preparedOrder.Reason,
							LotSize: preparedOrder.LotSize,
						})
						continue
					}

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
