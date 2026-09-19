package ports

import (
	"context"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

type MT5Port interface {
	ExecuteOrder(ctx context.Context, order domain.TradeOrder) (*domain.OrderResult, error)
	Close() error
}
