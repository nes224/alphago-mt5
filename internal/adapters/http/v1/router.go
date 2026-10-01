package v1

import (
	"github.com/gin-gonic/gin"
	"github.com/nes224/alphago-mt5/internal/adapters/database"
	"github.com/nes224/alphago-mt5/internal/adapters/mt5"
	"github.com/nes224/alphago-mt5/internal/core/services"
	"github.com/nes224/alphago-mt5/internal/core/services/pipeline"
	"github.com/nes224/alphago-mt5/internal/core/services/risk"
	"github.com/nes224/alphago-mt5/internal/core/services/strategy"
)

func RouterV1(
	r *gin.Engine,
	mt5Adapter *mt5.TCPAdapter,
	riskGuard *risk.RiskGuard,
	quantEngine *strategy.QuantEngine,
	execRouter *pipeline.ExecutionRouter,
	store *database.Store,
	TradingSymbol string,
) *gin.Engine {
	// 10. Setup HTTP Services & Handlers (REST API)
	tradeService := services.NewTradeService(mt5Adapter, riskGuard)
	tradeHandler := NewTradeHandler(tradeService)
	monitorHandler := NewMonitorHandler(quantEngine, execRouter, riskGuard, store, mt5Adapter, TradingSymbol)

	v1Router := r.Group("/api/v1")
	{
		v1Router.POST("/trade", tradeHandler.PlaceOrder)
		v1Router.POST("/orders/close", tradeHandler.CloseOrder)
		v1Router.PUT("/orders/modify", tradeHandler.ModifyOder)
		v1Router.GET("/status", monitorHandler.Status)
		v1Router.GET("/signals", monitorHandler.RecentSignals)
		v1Router.GET("/strategy-stats", monitorHandler.StrategyStats)
		v1Router.GET("/pending-orders", monitorHandler.PendingOrders)
		v1Router.GET("/account-info", monitorHandler.AccountInfo)
		v1Router.POST("/risk/resync-positions", monitorHandler.ResyncPositions)
	}

	return r
}
