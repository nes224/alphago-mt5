package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/services"
)

type TradeHandler struct {
	tradeService *services.TradeService
}

func NewTradeHandler(tradeService *services.TradeService) *TradeHandler {
	return &TradeHandler{tradeService: tradeService}
}

// POST /api/v1/trade
func (h *TradeHandler) PlaceOrder(c *gin.Context) {
	var req domain.TradeRequest

	// Bind JSON Body เข้า domain.TradeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid trade request body: " + err.Error(),
		})
		return
	}

	// ส่งเข้า Core Domain Service
	resp, err := h.tradeService.ExecuteTrade(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *TradeHandler) CloseOrder(c *gin.Context) {
	var req domain.TradeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body:" + err.Error()})
		return
	}

	req.Action = domain.ActionClose

	resp, err := h.tradeService.ExecuteTrade(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *TradeHandler) ModifyOder(c *gin.Context) {
	var req domain.TradeRequest
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body:" + err.Error()})
		return
	}

	req.Action = domain.ActionModify

	resp, err := h.tradeService.ExecuteTrade(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}
