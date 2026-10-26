package candlefile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

const testSymbol = "XAUUSDm"

func bar(day, hh, mm int, price float64) domain.Candle {
	return domain.Candle{
		Symbol:    testSymbol,
		Timeframe: domain.TimeframeM1,
		OpenTime:  time.Date(2026, 10, day, hh, mm, 0, 0, time.UTC),
		Open:      price - 1, High: price + 1, Low: price - 2, Close: price,
		TickCount: 10,
		Closed:    true,
	}
}

func readBack(t *testing.T, root string, day int) []domain.Candle {
	t.Helper()
	path, _ := FilePath(root, testSymbol, domain.TimeframeM1, time.Date(2026, 10, day, 0, 0, 0, 0, time.UTC))
	got, err := ReadFile(path, testSymbol, domain.TimeframeM1)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func closes(cs []domain.Candle) []float64 {
	out := make([]float64, len(cs))
	for i, c := range cs {
		out[i] = c.Close
	}
	return out
}

func equalFloats(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func TestWriteCandles_CreateFileAndRoundTrips(t *testing.T) {
	root := t.TempDir()
	in := []domain.Candle{bar(21, 0, 0, 4113.832), bar(21, 0, 1, 4114.9)}

	if err := WriteCandles(root, testSymbol, domain.TimeframeM1, in); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(root, "xauusdm", "tf_m_1", "2026-10-21.csv"))
	if err != nil {
		t.Fatal(err)
	}
	want := "time,open,high,low,close,tick_volume\n" +
		"2026.10.21 00:00:00,4112.832,4114.832,4111.832,4113.832,10\n" +
		"2026.10.21 00:01:00,4113.9,4115.9,4112.9,4114.9,10\n"
	if string(raw) != want {
		t.Errorf("file content:\n%s\nwant:\n%s", raw, want)
	}

	got := readBack(t, root, 21)
	if len(got) != 2 || got[0] != in[0] || got[1] != in[1] {
		t.Errorf("round troup:\ngot %+v\nwant %+v", got, in)
	}

}

func TestWriteCandles_MergeNewOverwritesSameTime(t *testing.T) {
	root := t.TempDir()
	first := []domain.Candle{bar(21, 0, 0, 100), bar(21, 0, 1, 101), bar(21, 0, 2, 102)}
	if err := WriteCandles(root, testSymbol, domain.TimeframeM1, first); err != nil {
		t.Fatal(err)
	}

	second := []domain.Candle{bar(21, 0, 3, 103), bar(21, 0, 2, 202)}
	if err := WriteCandles(root, testSymbol, domain.TimeframeM1, second); err != nil {
		t.Fatal(err)
	}

	got := closes(readBack(t, root, 21))
	if want := []float64{100, 101, 202, 103}; !equalFloats(got, want) {
		t.Errorf("closes = %v, want %v", got, want)
	}
}

func TestWriteCandles_NewDayNewFileOldFileKept(t *testing.T) {
	root := t.TempDir()
	day21 := []domain.Candle{bar(21, 23, 57, 1), bar(21, 23, 58, 2)}
	if err := WriteCandles(root, testSymbol, domain.TimeframeM1, day21); err != nil {
		t.Fatal(err)
	}

	// รอบถัดไปมีแท่งคร่อมเที่ยงคืน
	cross := []domain.Candle{bar(21, 23, 59, 3), bar(22, 0, 0, 4), bar(22, 0, 1, 5)}
	if err := WriteCandles(root, testSymbol, domain.TimeframeM1, cross); err != nil {
		t.Fatal(err)
	}

	if got := closes(readBack(t, root, 21)); !equalFloats(got, []float64{1, 2, 3}) {
		t.Errorf("day 21 = %v, want [1 2 3] (old rows must be kept)", got)
	}
	if got := closes(readBack(t, root, 22)); !equalFloats(got, []float64{4, 5}) {
		t.Errorf("day 22 = %v, want [4 5]", got)
	}
}

func TestWriteCandles_SkipsCandlesStillForming(t *testing.T) {
	root := t.TempDir()
	forming := bar(21, 0, 1, 999)
	forming.Closed = false

	if err := WriteCandles(root, testSymbol, domain.TimeframeM1, []domain.Candle{bar(21, 0, 0, 1), forming}); err != nil {
		t.Fatal(err)
	}
	if got := closes(readBack(t, root, 21)); !equalFloats(got, []float64{1}) {
		t.Errorf("closes = %v , want [1]", got)
	}
}

func TestWriteCandles_NoTempFilesLeft(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 3; i++ {
		if err := WriteCandles(root, testSymbol, domain.TimeframeM1, []domain.Candle{bar(21, 0, i, float64(i))}); err != nil {
			t.Fatal(err)
		}
	}

	entries, _ := os.ReadDir(filepath.Join(root, "xauusdm", "tf_m_1"))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}

	if len(entries) != 1 {
		t.Errorf("files = %d, want 1", len(entries))
	}
}

func TestWriteCandles_RefusesToOverwriteCorruptFile(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "xauusdm", "tf_m_1")
	os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, "2026-10-21.csv")
	currupt := "this not our csv\n"
	os.WriteFile(path, []byte(currupt), 0o644)

	err := WriteCandles(root, testSymbol, domain.TimeframeM1, []domain.Candle{bar(21, 0, 0, 1)})
	if err == nil {
		t.Fatal("expected error")
	}

	raw, _ := os.ReadFile(path)
	if string(raw) != currupt {
		t.Errorf("corrupt file was modified: %q", raw)
	}
}

func TestWriteCandles_RejectsWrongTimeframe(t *testing.T) {
	c := bar(21, 0, 0, 1)
	c.Timeframe = domain.TimeframeH1
	if err := WriteCandles(t.TempDir(), testSymbol, domain.TimeframeM1, []domain.Candle{c}); err == nil {
		t.Fatal("expected error")
	}
}

func TestWriteCandles_RejectsWrongSymbol(t *testing.T) {
	root := t.TempDir()
	c := bar(21, 0, 0, 1)
	c.Symbol = "BTCUSDm"
	if err := WriteCandles(root, testSymbol, domain.TimeframeM1, []domain.Candle{c}); err == nil {
		t.Fatal("expected error")
	}
	if got := readBack(t, root, 21); len(got) != 0 {
		t.Errorf("wrote %d candles for wrong symbol", len(got))
	}
}
