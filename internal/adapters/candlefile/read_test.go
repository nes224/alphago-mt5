package candlefile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

const GoldFuture = "XAUUSDm"

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "2026-10.21.csv")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadFile(t *testing.T) {
	path := writeTemp(t,
		"time,open,high,low,close,tick_volume\n"+
			"2026.10.21 00:00:00,4113.774,4114.250,4112.073,4113.832,37\n"+
			"2026.10.21 00:01:00,4113.832,4115.010,4113.500,4114.900,42\n")
	got, err := ReadFile(path, GoldFuture, domain.TimeframeM1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d candles, want 2", len(got))
	}

	want := domain.Candle{
		Symbol: GoldFuture, Timeframe: domain.TimeframeM1,
		OpenTime: time.Date(2026, 10, 21, 0, 1, 0, 0, time.UTC),
		Open:     4113.832, High: 4115.010, Low: 4113.500, Close: 4114.900,
		TickCount: 42, Closed: true,
	}
	if got[1] != want {
		t.Errorf("got %+v\nwant %+v", got[1], want)
	}
}

func TestReadFile_MissingFileIsEmpty(t *testing.T) {
	got, err := ReadFile(filepath.Join(t.TempDir(), "nil.csv"), GoldFuture, domain.TimeframeM1)
	if err != nil || got != nil {
		t.Fatalf("got %v, %v; want nil, nil", got, err)
	}
}

func TestReadFile_Windows_CRLF(t *testing.T) {
	path := writeTemp(t, "time,open,high,low,close,tick_volume\r\n2026.10.21 00:00:00,1,2,0.5,1.5,3\r\n")
	got, err := ReadFile(path, GoldFuture, domain.TimeframeM1)
	if err != nil || len(got) != 1 {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestReadFile_Errors(t *testing.T) {
	tests := map[string]struct {
		content string
		wantErr string
	}{
		"wrong header": {
			"date,o,h,l,c,v\n",
			"unexpected header",
		},
		"bad time on line 3": {
			"time,open,high,low,close,tick_volume\n2026.10.21 00:00:00,1,2,0.5,1.5,3\n2026-10-21T00:01:00Z,1,2,0.5,1.5,3\n",
			"line 3: time",
		},
		"bad price": {
			"time,open,high,low,close,tick_volume\n2026.10.21 00:00:00,1,abc,0.5,1.5,3\n",
			"line 2: high",
		},
		"missing column": {
			"time,open,high,low,close,tick_volume\n2026.10.21 00:00:00,1,2,0.5,1.5\n",
			"wrong number of fields",
		},
	}

	for name, tt := range tests {
		_, err := ReadFile(writeTemp(t, tt.content), GoldFuture, domain.TimeframeM1)
		if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
			t.Errorf("%s: err = %v, want containing %q", name, err, tt.wantErr)
		}
	}
}
