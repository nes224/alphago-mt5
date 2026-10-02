package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nes224/alphago-mt5/internal/adapters/database"
	"github.com/nes224/alphago-mt5/internal/core/ports"
	"github.com/nes224/alphago-mt5/internal/core/services/pipeline"
	"github.com/nes224/alphago-mt5/internal/core/services/risk"
)

type MonitorHandler struct {
	engine     ports.QuantEngine
	execRouter *pipeline.ExecutionRouter
	riskGuard  *risk.RiskGuard
	store      *database.Store
	mt5        ports.MT5Port
	symbol     string
}

func NewMonitorHandler(engine ports.QuantEngine, execRouter *pipeline.ExecutionRouter, riskGuard *risk.RiskGuard, store *database.Store, mt5 ports.MT5Port, symbol string) *MonitorHandler {
	return &MonitorHandler{
		engine:     engine,
		execRouter: execRouter,
		riskGuard:  riskGuard,
		store:      store,
		mt5:        mt5,
		symbol:     symbol,
	}
}

func (h *MonitorHandler) Status(c *gin.Context) {
	metrics := h.engine.GetLatestMetrics(h.symbol)

	resp := gin.H{
		"symbol":          h.symbol,
		"metrics":         metrics,
		"multi_timeframe": h.engine.GetMultiTimeframeState(h.symbol),
	}
	if h.riskGuard != nil {
		resp["risk_guard"] = h.riskGuard.Status()
	}

	c.JSON(http.StatusOK, resp)
}

func (h *MonitorHandler) RecentSignals(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"symbol":  h.symbol,
		"signals": h.execRouter.RecentSignals(),
	})
}

func (h *MonitorHandler) StrategyStats(c *gin.Context) {
	stats, err := h.store.WinRateByStrategy()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	sessionStats, err := h.store.WinRateBySession()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"strategies": stats,
		"sessions":   sessionStats,
	})
}

func (h *MonitorHandler) PendingOrders(c *gin.Context) {
	orders, err := h.store.PendingOrders(50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"pending_orders": orders,
	})
}

func (h *MonitorHandler) AccountInfo(c *gin.Context) {
	info, err := h.mt5.GetAccountInfo(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, info)
}

type resyncPositionsRequest struct {
	Symbols map[string]int `json:"symbols"`
}

func (h *MonitorHandler) ResyncPositions(c *gin.Context) {
	var req resyncPositionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}

	h.riskGuard.ResyncPositions(req.Symbols)

	c.JSON(http.StatusOK, gin.H{
		"status":     "ok",
		"risk_guard": h.riskGuard.Status(),
	})
}
