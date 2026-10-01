package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nes224/alphago-mt5/internal/adapters/database"
	"github.com/nes224/alphago-mt5/internal/core/ports"
	"github.com/nes224/alphago-mt5/internal/core/services/pipeline"
	"github.com/nes224/alphago-mt5/internal/core/services/risk"
)

// MonitorHandler exposes read-only insight into what the QuantEngine is
// currently seeing (latest metrics, RiskGuard status, recent signals,
// per-strategy win-rate) over HTTP, so this can be checked without grepping
// server logs.
type MonitorHandler struct {
	engine     ports.QuantEngine
	execRouter *pipeline.ExecutionRouter
	riskGuard  *risk.RiskGuard
	store      *database.Store
	symbol     string
}

func NewMonitorHandler(engine ports.QuantEngine, execRouter *pipeline.ExecutionRouter, riskGuard *risk.RiskGuard, store *database.Store, symbol string) *MonitorHandler {
	return &MonitorHandler{
		engine:     engine,
		execRouter: execRouter,
		riskGuard:  riskGuard,
		store:      store,
		symbol:     symbol,
	}
}

// GET /api/v1/status
func (h *MonitorHandler) Status(c *gin.Context) {
	metrics := h.engine.GetLatestMetrics(h.symbol)

	resp := gin.H{
		"symbol":  h.symbol,
		"metrics": metrics,
	}
	if h.riskGuard != nil {
		resp["risk_guard"] = h.riskGuard.Status()
	}

	c.JSON(http.StatusOK, resp)
}

// GET /api/v1/signals
func (h *MonitorHandler) RecentSignals(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"symbol":  h.symbol,
		"signals": h.execRouter.RecentSignals(),
	})
}

// GET /api/v1/strategy-stats — win-rate สะสมต่อ strategy จาก trade_outcomes
// (ต้องมี trade_closed event จาก EA ไหลเข้ามาก่อนถึงจะมีข้อมูล)
func (h *MonitorHandler) StrategyStats(c *gin.Context) {
	stats, err := h.store.WinRateByStrategy()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"strategies": stats,
	})
}
