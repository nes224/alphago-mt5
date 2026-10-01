package v1

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/services"
)

type RiskConfigHandler struct {
	riskConfigService *services.RiskConfigService
}

func NewRiskConfigHandler(riskConfigService *services.RiskConfigService) *RiskConfigHandler {
	return &RiskConfigHandler{riskConfigService: riskConfigService}
}

// PUT /api/v1/risk/config — ตั้ง risk policy แบบ live (ไม่ต้อง restart) แทน
// การแก้ app.env — มีผลทันทีกับ RiskManager/RiskGuard และ persist ลง DB ด้วย
func (h *RiskConfigHandler) SetRiskConfig(c *gin.Context) {
	var req domain.RiskConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}

	resp, err := h.riskConfigService.SetRiskConfig(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"config": resp,
	})
}

// GET /api/v1/risk/config — ค่า risk policy ล่าสุดที่ persist ไว้ (404 ถ้ายัง
// ไม่เคยเรียก PUT เลยสักครั้ง — แปลว่าตอนนี้ยังรันด้วยค่า seed จาก app.env อยู่)
func (h *RiskConfigHandler) GetRiskConfig(c *gin.Context) {
	cfg, err := h.riskConfigService.GetRiskConfig(c.Request.Context())
	if err != nil {
		if errors.Is(err, services.ErrRiskConfigNotSet) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, cfg)
}
