package v1

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

// GET /api/v1/pending-orders — ประวัติ Outbox ล่าสุด (PENDING/UNKNOWN คือแถวที่
// ต้องเช็คมือใน MT5 tab Trade ก่อนเทรด symbol นั้นต่อ — ดู HANDOFF สำหรับบริบท
// ของเคส EOF ที่เจอจริง)
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

// GET /api/v1/account-info — ข้อมูลบัญชีสดจาก MT5 ตรงๆ (balance, equity,
// account type demo/real, broker, symbol ที่เทรดได้จริงตอนนี้) ไม่ใช่ค่าจาก
// app.env หรือ DB — เรียกใหม่ทุกครั้งที่ hit endpoint นี้ (read-only)
func (h *MonitorHandler) AccountInfo(c *gin.Context) {
	info, err := h.mt5.GetAccountInfo(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, info)
}

// resyncPositionsRequest คือ body ของ POST /api/v1/risk/resync-positions —
// symbols คือ symbol -> จำนวน position ที่เปิดอยู่จริงตอนนี้ (เช็คจาก MT5 tab
// Trade เอง) เช่น {"XAUUSDm": 1}. ส่ง object ว่าง {} ถ้าไม่มี position เปิดอยู่เลย
type resyncPositionsRequest struct {
	Symbols map[string]int `json:"symbols"`
}

// POST /api/v1/risk/resync-positions — แก้ RiskGuard.openPositionsCount /
// openPositionSymbols ให้ตรงกับความจริงใน MT5 ตรงๆ แบบ live ไม่ต้อง restart
// service และไม่ต้องแตะ database มือ — ใช้ตอน internal tracking เพี้ยนไปจาก
// MT5 จริง (เจอเคสจริง 2026-10-01: count ค้างที่ 5 ทั้งที่ MT5 มี position
// เปิดอยู่แค่ 1 ตัว บล็อก signal ใหม่ทุกตัวเพราะชน MAX_OPEN_POSITIONS)
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
