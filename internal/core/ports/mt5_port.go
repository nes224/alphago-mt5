package ports

import (
	"context"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)
type MT5Port interface {
	SendOrder(ctx context.Context, req domain.TradeRequest) (*domain.TradeResponse, error)
	Close() error
}
