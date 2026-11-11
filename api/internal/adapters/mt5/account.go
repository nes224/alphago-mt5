package mt5

import (
	"context"
	"encoding/json"
	"fmt"
)

// AccountInfo is the EA's answer to ACCOUNT_INFO.
type AccountInfo struct {
	Broker      string   `json:"broker"`
	Login       int64    `json:"login"`
	AccountType string   `json:"account_type"` // DEMO, REAL or CONTEST
	Currency    string   `json:"currency"`
	Balance     float64  `json:"balance"`
	Equity      float64  `json:"equity"`
	Leverage    int64    `json:"leverage"`
	Symbols     []string `json:"symbols"` // the symbols in MT5's Market Watch
}

// AccountInfo asks the EA for the account and the symbols it can trade
// (everything shown in MT5's Market Watch window).
func (c *CommandClient) AccountInfo(ctx context.Context) (AccountInfo, error) {
	line, err := c.Call(ctx, map[string]string{"action": "ACCOUNT_INFO"})
	if err != nil {
		return AccountInfo{}, err
	}
	var resp struct {
		Status  string `json:"status"`
		Message string `json:"message"`
		AccountInfo
	}
	if err := json.Unmarshal(line, &resp); err != nil {
		return AccountInfo{}, fmt.Errorf("decode ACCOUNT_INFO response: %w", err)
	}
	if resp.Status != "SUCCESS" {
		return AccountInfo{}, &EAError{Message: resp.Message}
	}
	return resp.AccountInfo, nil
}
