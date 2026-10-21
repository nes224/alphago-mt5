package services

import (
	"testing"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

func ts(s string) time.Time {
	t, err := time.Parse("2006.01.02 15:04:05", s)
	if err != nil {
		panic(err)
	}
	return t
}

func tick(sym string, bid float64, at string) domain.Tick {
	return domain.Tick{Symbol: sym, Bid: bid, Ask: bid + 0.0002, Time: ts(at)}
}

func TestCandleAggregator_BuildsOHLCWithinMinute(t *testing.T) {
	a := NewCandleAggregator(domain.TimeframeM1)

	for _, tk := range []domain.Tick{
		tick("EURUSD", 1.1000, "2026.10.12 10:13:01"),
		tick("EURUSD", 1.1010, "2026.10.12 10:13:20"),
		tick("EURUSD", 1.0990, "2026.10.12 10:13:40"),
	} {
		if u := a.OnTick(tk); u.Closed != nil {
			t.Fatalf("unexpected closed candle: %+v", u.Closed)
		}
	}
	u := a.OnTick(tick("EURUSD", 1.1005, "2026.10.12 10:13:59"))

	c := u.Current
	if c.OpenTime != ts("2026.10.12 10:13:00") {
		t.Errorf("open time = %v", c.OpenTime)
	}
	if c.Open != 1.1000 || c.High != 1.1010 || c.Low != 1.0990 || c.Close != 1.1005 {
		t.Errorf("OHLC = %v %v %v %v", c.Open, c.High, c.Low, c.Close)
	}
	if c.TickCount != 4 || c.Closed {
		t.Errorf("tickCount=%d closed=%v", c.TickCount, c.Closed)
	}

}

func TestCandleAggregator_ClosesOnNextMinute(t *testing.T) {
	a := NewCandleAggregator(domain.TimeframeM1)
	a.OnTick(tick("EURUSD", 1.1000, "2026.10.12 10:13:01"))
	a.OnTick(tick("EURUSD", 1.1020, "2026.10.12 10:13:30"))

	u := a.OnTick(tick("EURUSD", 1.1030, "2026.10.12 10:14:00"))
	if u.Closed == nil {
		t.Fatal("expected closed candle")
	}
	if !u.Closed.Closed || u.Closed.Close != 1.1020 || u.Closed.OpenTime != ts("2026.10.12 10:13:00") {
		t.Errorf("closed = %+v", u.Closed)
	}
	// แท่งใหม่เปิดที่ราคาของ tick ที่ทำให้แท่งเก่าปิด
	if u.Current.Open != 1.1030 || u.Current.OpenTime != ts("2026.10.12 10:14:00") || u.Current.TickCount != 1 {
		t.Errorf("current = %+v", u.Current)
	}
}

func TestCandleAggregator_GapSkipsEmptyMinutes(t *testing.T) {
	a := NewCandleAggregator(domain.TimeframeM1)
	a.OnTick(tick("EURUSD", 1.1000, "2026.10.12 10:13:01"))
	u := a.OnTick(tick("EURUSD", 1.1050, "2026.10.12 10:20:05"))
	if u.Closed == nil || u.Closed.OpenTime != ts("2026.10.12 10:13:00") {
		t.Fatalf("closed = %+v", u.Closed)
	}
	if u.Current.OpenTime != ts("2026.10.12 10:20:00") {
		t.Errorf("current open = %v", u.Current.OpenTime)
	}
}

func TestCandleAggregator_DropsOutOfOrderTick(t *testing.T) {
	a := NewCandleAggregator(domain.TimeframeM1)
	a.OnTick(tick("EURUSD", 1.1000, "2026.10.12 10:13:01"))
	a.OnTick(tick("EURUSD", 1.1010, "2026.10.12 10:14:01"))

	u := a.OnTick(tick("EURUSD", 1.2000, "2026.10.12 10:13:59"))
	if !u.Dropped || u.Closed != nil {
		t.Fatalf("expected dropped, got %+v", u)
	}
	if u.Current.High != 1.1010 {
		t.Errorf("late tick leaked into current candle: %+v", u.Current)
	}
}

func TestCandleAggregator_SeparatesSymbols(t *testing.T) {
	a := NewCandleAggregator(domain.TimeframeM1)
	a.OnTick(tick("EURUSD", 1.1000, "2026.10.12 10:13:01"))
	u := a.OnTick(tick("XAUUSD", 2400.50, "2026.10.12 10:14:01"))
	if u.Closed != nil {
		t.Fatal("tick of another symbol must not close EURUSD candle")
	}
	eur, _ := a.Current("EURUSD")
	if eur.Close != 1.1000 || eur.OpenTime != ts("2026.10.12 10:13:00") {
		t.Errorf("EURUSD = %+v", eur)
	}
}
