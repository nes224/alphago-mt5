package mt5

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAccountInfo_ParsesSymbols(t *testing.T) {
	ea := newFakeEA(t, func(_ int, req map[string]any) string {
		if req["action"] != "ACCOUNT_INFO" {
			t.Errorf("action = %v", req["action"])
		}
		return `{"status":"SUCCESS","balance":1000.50,"equity":998.20,"currency":"USD","leverage":2000,` +
			`"account_type":"DEMO","broker":"Exness Technologies Ltd","login":414353575,` +
			`"symbols":["XAUUSDm","BTCUSDm","DXYm","USOILm"]}`
	})
	c := newTestClient(ea.addr(), time.Second)
	defer c.Close()

	info, err := c.AccountInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Broker != "Exness Technologies Ltd" || info.AccountType != "DEMO" || info.Login != 414353575 {
		t.Errorf("info = %+v", info)
	}
	if len(info.Symbols) != 4 || info.Symbols[2] != "DXYm" {
		t.Errorf("symbols = %v", info.Symbols)
	}
}

func TestAccountInfo_EAFailure(t *testing.T) {
	ea := newFakeEA(t, func(int, map[string]any) string {
		return `{"status":"FAILED","message":"not logged in"}`
	})
	c := newTestClient(ea.addr(), time.Second)
	defer c.Close()

	_, err := c.AccountInfo(context.Background())
	var eaErr *EAError
	if !errors.As(err, &eaErr) || eaErr.Message != "not logged in" {
		t.Fatalf("err = %v, want EAError(not logged in)", err)
	}
}
