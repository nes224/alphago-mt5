package api

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
)

func do(t *testing.T, method, url, body string) (int, []byte) {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var raw json.RawMessage
	_ = json.NewDecoder(resp.Body).Decode(&raw)
	return resp.StatusCode, raw
}

func TestBroker_ListsSymbols(t *testing.T) {
	_, ts := newTestServer(t)
	code, body := do(t, "GET", ts.URL+"/api/broker", "")
	var info BrokerInfo
	_ = json.Unmarshal(body, &info)
	if code != 200 || info.Broker != "Test Broker" || len(info.Symbols) != 2 {
		t.Fatalf("code=%d body=%s", code, body)
	}
}

func TestSymbols_AddAndRemove(t *testing.T) {
	s, ts := newTestServer(t)
	added := 0
	s.OnSymbolAdded = func() { added++ }

	if code, _ := do(t, "POST", ts.URL+"/api/symbols", `{"symbol":"BTCUSDm"}`); code != 201 {
		t.Fatalf("add code = %d", code)
	}
	if added != 1 {
		t.Errorf("OnSymbolAdded called %d times", added)
	}
	if code, _ := do(t, "POST", ts.URL+"/api/symbols", `{"symbol":"BTCUSDm"}`); code != 409 {
		t.Errorf("duplicate add code = %d, want 409", code)
	}
	if code, _ := do(t, "POST", ts.URL+"/api/symbols", `{"symbol":"EURUSDm"}`); code != 422 {
		t.Errorf("unknown symbol code = %d, want 422", code)
	}
	if code, _ := do(t, "POST", ts.URL+"/api/symbols", `{}`); code != 400 {
		t.Errorf("empty body code = %d, want 400", code)
	}

	code, body := do(t, "GET", ts.URL+"/api/symbols", "")
	var list []string
	_ = json.Unmarshal(body, &list)
	if code != 200 || !slices.Equal(list, []string{"XAUUSDm", "BTCUSDm"}) {
		t.Fatalf("list = %s", body)
	}

	if code, _ := do(t, "DELETE", ts.URL+"/api/symbols/XAUUSDm", ""); code != 200 {
		t.Errorf("remove code = %d", code)
	}
	if code, _ := do(t, "DELETE", ts.URL+"/api/symbols/XAUUSDm", ""); code != 404 {
		t.Errorf("remove again code = %d, want 404", code)
	}
}
