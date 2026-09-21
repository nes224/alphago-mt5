package services_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/services"
)

type MockMT5Adapter struct {
	SendOrderFunc func(ctx context.Context, req domain.TradeRequest) (*domain.TradeResponse, error)
	CloseFunc     func() error
}

func (m *MockMT5Adapter) SendOrder(ctx context.Context, req domain.TradeRequest) (*domain.TradeResponse, error) {
	if m.SendOrderFunc != nil {
		return m.SendOrderFunc(ctx, req)
	}

	return &domain.TradeResponse{Success: true, Ticket: 100001, Message: "Success"}, nil
}

func (m *MockMT5Adapter) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}

	return nil
}

func TestTradeService_ExecuteTrade_Validation(t *testing.T) {
	mockAdapter := &MockMT5Adapter{}
	service := services.NewTradeService(mockAdapter)
	ctx := context.Background()

	tests := []struct {
		name    string
		req     domain.TradeRequest
		wantErr string
	}{
		{
			name: "Empty Symbol Should Fail",
			req: domain.TradeRequest{
				Action: "BUY", Symbol: "", Volume: 0.01,
			},
			wantErr: "symbol cannot be empty",
		},
		{
			name: "Zero or Negative Volume Should Fail",
			req: domain.TradeRequest{
				Action: "BUY", Symbol: "EURUSD", Volume: 0.0,
			},
			wantErr: "trade volume must be greater than 0",
		},
		{
			name: "Invalid Action Should Fail",
			req: domain.TradeRequest{
				Action: "HOLD", Symbol: "EURUSD", Volume: 0.01,
			},
			wantErr: "invalid trade action",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.ExecuteTrade(ctx, tt.req)
			if err == nil {
				t.Fatalf("expected error containing %v, got nil", tt.wantErr)
			}
			// 👈 ใช้ errors.Is เช็กผ่าน Wrapped Error (%w)
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestTradeService_ExecuteTrade_RiskGuard(t *testing.T) {
	mockAdapter := &MockMT5Adapter{}
	service := services.NewTradeService(mockAdapter)

	req := domain.TradeRequest{
		Action: "BUY", Symbol: "EURUSD", Volume: 15.0,
	}

	_, err := service.ExecuteTrade(context.Background(), req)
	if err == nil {
		t.Fatal("expected risk guard error, got nil")
	}

	if !errors.Is(err, services.ErrRiskGuardTriggered) && !strings.Contains(err.Error(), "risk") {
		t.Errorf("expected risk guard error, got: %v", err)
	}
}

func TestTradeService_ExecuteTrade_Success(t *testing.T) {
	mockAdapter := &MockMT5Adapter{
		SendOrderFunc: func(ctx context.Context, req domain.TradeRequest) (*domain.TradeResponse, error) {
			return &domain.TradeResponse{
				Success:   true,
				Ticket:    99999,
				Price:     1.0850,
				Message:   "Order Executed",
				Timestamp: time.Now(),
			}, nil
		},
	}

	service := services.NewTradeService(mockAdapter)
	ctx := context.Background()

	req := domain.TradeRequest{
		Action:   "BUY",
		Symbol:   "EURUSD",
		Volume:   0.1,
		MagicNum: 123456,
	}

	resp, err := service.ExecuteTrade(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !resp.Success || resp.Ticket != 99999 {
		t.Errorf("expected ticket 99999, got %d", resp.Ticket)
	}

}
