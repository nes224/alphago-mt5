package pipeline

import (
	"context"
	"log"

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
						log.Printf("[Worker %d] Risk calculation rejected signal: %v", workerID, err)
						continue
					}

					if r.riskGuard != nil {
						if err := r.riskGuard.ValidateOrder(*preparedOrder, metrics); err != nil {
							log.Printf("[Worker %d] RISK GUARD BLOCKED signal for %s: %v", workerID, signal.Symbol, err)
							continue
						}
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
