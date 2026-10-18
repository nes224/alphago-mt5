package services

import (
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

type CandleAggregator struct {
	tf      domain.Timeframe
	current map[string]*domain.Candle
}

func NewCandleAggregator(tf domain.Timeframe) *CandleAggregator {
	if tf.Minutes() <= 0 {
		panic("candle aggregator: unsupported timeframe " + string(tf))
	}
	return &CandleAggregator{tf: tf, current: make(map[string]*domain.Candle)}
}

type CandleUpdate struct {
	Closed  *domain.Candle
	Current domain.Candle
	Dropped bool
}

// OnTick ป้อน tick เข้าไป แล้วคืนแท่งที่ปิด + แท่งปัจจุบัน
func (a *CandleAggregator) OnTick(t domain.Tick) CandleUpdate {
	openTime := t.Time.Truncate(time.Duration(a.tf.Minutes()))
	price := t.Bid

	cur, ok := a.current[t.Symbol]
	if ok && openTime.Before(cur.OpenTime) {
		return CandleUpdate{Current: *cur, Dropped: true}
	}

	var closed *domain.Candle
	if ok && openTime.After(cur.OpenTime) {
		c := *cur
		c.Closed = true
		closed = &c
		ok = false
	}

	if !ok {
		cur = &domain.Candle{
			Symbol:    t.Symbol,
			Timeframe: a.tf,
			OpenTime:  openTime,
			Open:      price,
			High:      price,
			Low:       price,
		}
		a.current[t.Symbol] = cur
	}

	if price > cur.High {
		cur.High = price
	}

	if price < cur.Low {
		cur.Low = price
	}
	cur.Close = price
	cur.TickCount++
	cur.Volume += t.Volume

	return CandleUpdate{Closed: closed, Current: *cur}
}

// Current คืน snapshot แท่งที่กำลังก่อตัวของ symbol (ok=false ถ้ายังไม่เคยมี tick)
func (a *CandleAggregator) Current(symbol string) (domain.Candle, bool) {
	c, ok := a.current[symbol]
	if !ok {
		return domain.Candle{}, false
	}

	return *c, true
}
