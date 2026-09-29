package pipeline

import (
	"context"
	"log"

	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/ports"
	"github.com/nes224/alphago-mt5/internal/core/services/risk"
)

type ExecutionRouter struct {
	engine      ports.QuantEngine
	riskMgr     *risk.RiskManager
	riskGuard   *risk.RiskGuard
	orderSink   chan<- risk.PreparedOrder
	workerCount int
}

func NewExecutionRouter(engine ports.QuantEngine, riskMgr *risk.RiskManager, orderSink chan<- risk.PreparedOrder, workers int) *ExecutionRouter {
	return &ExecutionRouter{
		engine:      engine,
		riskMgr:     riskMgr,
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
						log.Printf("[Worker %d] Risk calculation rejected signal: %v", workerID, err)
						return
					}

					select {
					case r.orderSink <- *preparedOrder:
						log.Printf("[Worker %d] ORDER DISPATCHED: %s %s Lot:%.2f @ %.2f (SL:%.2f, TP:%.2f)",
							workerID, preparedOrder.Action, preparedOrder.Symbol, preparedOrder.LotSize, preparedOrder.EntryPrice, preparedOrder.StopLoss, preparedOrder.TakeProfit)
					default:
						log.Printf("[Worker %d] WARNING: Order Sink Channel is full, dropping order for %s", workerID, preparedOrder.Symbol)
					}
				}
			}
		}(i)
	}
}

func (r *ExecutionRouter) processSignal(signal domain.OrderSignal) {
	metrics := r.engine.GetLatestMetrics(signal.Symbol)

	// 1. คำนวณ Sizing / SL / TP
	preparedOrder, err := r.riskMgr.CalculateOrder(signal, metrics)
	if err != nil {
		log.Printf("[Router] Sizing calculation rejected: %v", err)
		return
	}

	// 2. ✅ ผ่าน RiskGuard เช็กความปลอดภัยเป็นด่านสุดท้าย
	if err := r.riskGuard.ValidateOrder(*preparedOrder, metrics); err != nil {
		log.Printf("[RiskGuard REJECTED] Symbol: %s | Reason: %v", preparedOrder.Symbol, err)
		return
	}

	// 3. Dispatch ยิงต่อไปยัง MT5 TCP Adapter
	select {
	case r.orderSink <- *preparedOrder:
		log.Printf("[ORDER DISPATCHED] %s %s | Lot: %.2f | SL: %.2f | TP: %.2f",
			preparedOrder.Action, preparedOrder.Symbol, preparedOrder.LotSize, preparedOrder.StopLoss, preparedOrder.TakeProfit)
	default:
		log.Printf("[WARNING] Order Sink Channel full, dropped order for %s", preparedOrder.Symbol)
	}
}
