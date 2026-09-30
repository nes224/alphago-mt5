package strategy

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/ports"
)

// dailyRange เก็บ open/high/low ของ symbol สำหรับ "วันนี้" (ตามวันที่ปฏิทิน
// ของเครื่อง) รีเซ็ตอัตโนมัติเมื่อข้ามวัน
type dailyRange struct {
	date string // "2006-01-02"
	open float64
	high float64
	low  float64
}

type QuantEngine struct {
	mu            sync.RWMutex
	strategies    []ports.QuantStrategy
	latestMetrics map[string]domain.TickMetrics
	tickChan      chan domain.Tick
	signalChan    chan domain.OrderSignal
	lastTickMap   map[string]domain.Tick
	windows       map[string]*Window
	windowSize    int

	// longTermWindows คือ Window เดียวกันแต่ยาวกว่ามาก ใช้เป็น proxy
	// "higher timeframe trend" — longTermWindowSize <= 0 คือปิดการทำงานนี้
	longTermWindows    map[string]*Window
	longTermWindowSize int

	dailyRanges map[string]*dailyRange

	cooldownMu     sync.Mutex
	signalCooldown time.Duration
	lastSignalTime map[string]time.Time
}

func NewQuantEngine(bufferSize int, windowSize int) *QuantEngine {
	return &QuantEngine{
		strategies:      make([]ports.QuantStrategy, 0),
		tickChan:        make(chan domain.Tick, bufferSize),
		signalChan:      make(chan domain.OrderSignal, bufferSize),
		latestMetrics:   make(map[string]domain.TickMetrics),
		lastTickMap:     make(map[string]domain.Tick),
		windows:         make(map[string]*Window),
		windowSize:      windowSize,
		longTermWindows: make(map[string]*Window),
		dailyRanges:     make(map[string]*dailyRange),
		lastSignalTime:  make(map[string]time.Time),
	}
}

// SetLongTermWindowSize เปิดใช้งาน Dual-Window Trend Filter — window ที่สองนี้
// ควรยาวกว่า windowSize หลักมาก (เช่น 50-100 เท่า) เพื่อประมาณทิศทาง trend
// ภาพใหญ่กว่า โดยไม่ต้องสร้าง candle aggregator จริง ค่า default คือ 0 (ปิด)
func (e *QuantEngine) SetLongTermWindowSize(n int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.longTermWindowSize = n
}

// SetSignalCooldown ตั้งระยะเวลาต่ำสุดระหว่าง signal ที่จะถูกส่งออกต่อ symbol
// เป็น safety net กันไม่ให้ strategy ที่ threshold ยังไม่ผ่านการ tune ยิง order
// รัวเกินไปในตลาดจริง (ค่า default คือ 0 = ปิดการทำงานนี้)
func (e *QuantEngine) SetSignalCooldown(d time.Duration) {
	e.cooldownMu.Lock()
	defer e.cooldownMu.Unlock()
	e.signalCooldown = d
}

// allowSignal คืน true ถ้ายังไม่มี signal ของ symbol นี้ถูกส่งออกภายในช่วง
// cooldown ที่ตั้งไว้ และจะบันทึกเวลาปัจจุบันไว้เป็น "signal ล่าสุด" ทันทีที่อนุญาต
func (e *QuantEngine) allowSignal(symbol string) bool {
	e.cooldownMu.Lock()
	defer e.cooldownMu.Unlock()

	if e.signalCooldown <= 0 {
		return true
	}

	now := time.Now()
	if last, exists := e.lastSignalTime[symbol]; exists && now.Sub(last) < e.signalCooldown {
		return false
	}
	e.lastSignalTime[symbol] = now
	return true
}

func (e *QuantEngine) RegisterStrategy(s ports.QuantStrategy) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.strategies = append(e.strategies, s)
}

func (e *QuantEngine) PushTick(tick domain.Tick) {
	e.tickChan <- tick
}

func (e *QuantEngine) SignalChannel() <-chan domain.OrderSignal {
	return e.signalChan
}

func (e *QuantEngine) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case tick, ok := <-e.tickChan:
				if !ok {
					return
				}
				e.ProcessTick(tick)
			}
		}
	}()
}

func (e *QuantEngine) calculateMetrics(tick domain.Tick) domain.TickMetrics {
	lastTick, exists := e.lastTickMap[tick.Symbol]
	midPrice := (tick.Ask + tick.Bid) / 2.0

	var oiDelta int64 = 0
	var oiVelocity float64 = 0
	var volumeDelta int64 = 0
	var volumeVelocity float64 = 0
	var priceVelocity float64 = 0

	if exists {
		oiDelta = tick.OpenInterest - lastTick.OpenInterest
		volumeDelta = tick.Volume - lastTick.Volume

		timeDelta := tick.Timestamp.Sub(lastTick.Timestamp).Seconds()
		if timeDelta > 0 {
			oiVelocity = float64(oiDelta) / timeDelta
			volumeVelocity = float64(volumeDelta) / timeDelta
			lastMidPrice := (lastTick.Ask + lastTick.Bid) / 2.0
			priceVelocity = (midPrice - lastMidPrice) / timeDelta
		}
	}

	window := e.getOrCreateWindow(tick.Symbol)
	mean := window.Mean()
	stdDev := window.StdDev()
	trendSlope := window.Slope()

	var zScore float64
	if stdDev > 0 {
		zScore = (midPrice - mean) / stdDev
	}

	window.Push(midPrice)

	var longTermTrendSlope float64
	if ltWindow := e.getOrCreateLongTermWindow(tick.Symbol); ltWindow != nil {
		longTermTrendSlope = ltWindow.Slope()
		ltWindow.Push(midPrice)
	}

	dayOpen, dayHigh, dayLow := e.updateDailyRange(tick.Symbol, midPrice, tick.Timestamp)

	return domain.TickMetrics{
		Symbol:             tick.Symbol,
		Price:              midPrice,
		Mean:               mean,
		StdDev:             stdDev,
		ZScore:             zScore,
		TrendSlope:         trendSlope,
		LongTermTrendSlope: longTermTrendSlope,
		Bid:                tick.Bid,
		Ask:                tick.Ask,
		Spread:             tick.Ask - tick.Bid,
		DailyOpen:          dayOpen,
		DailyHigh:          dayHigh,
		DailyLow:           dayLow,
		OpenInterest:       tick.OpenInterest,
		OIDelta:            oiDelta,
		OIVelocity:         oiVelocity,
		VolumeDelta:        volumeDelta,
		VolumeVelocity:     volumeVelocity,
		PriceVelocity:      priceVelocity,
		Timestamp:          tick.Timestamp,
		WindowSize:         window.Size(),
	}
}

func (e *QuantEngine) ProcessTick(tick domain.Tick) domain.TickMetrics {
	e.mu.Lock()

	metrics := e.calculateMetrics(tick)
	e.latestMetrics[tick.Symbol] = metrics
	e.lastTickMap[tick.Symbol] = tick

	strategies := make([]ports.QuantStrategy, len(e.strategies))
	copy(strategies, e.strategies)

	e.mu.Unlock()

	for _, s := range strategies {
		if signal := s.OnTick(tick, metrics); signal != nil {
			if !e.allowSignal(signal.Symbol) {
				continue
			}

			select {
			case e.signalChan <- *signal:
			default:
				log.Warn().Str("symbol", signal.Symbol).Msg("[QuantEngine] signal channel full, dropping signal")
			}
		}
	}

	return metrics
}

func (e *QuantEngine) GetLatestMetrics(symbol string) domain.TickMetrics {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.latestMetrics[symbol]
}

func (e *QuantEngine) UpdateMetrics(symbol string, m domain.TickMetrics) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.latestMetrics[symbol] = m
}

func (e *QuantEngine) getOrCreateWindow(symbol string) *Window {
	w, exists := e.windows[symbol]
	if !exists {
		w = NewWindow(e.windowSize)
		e.windows[symbol] = w
	}

	return w
}

// getOrCreateLongTermWindow คืน nil ถ้า SetLongTermWindowSize ไม่เคยถูกเรียก
// (ฟีเจอร์นี้ปิดอยู่โดย default)
func (e *QuantEngine) getOrCreateLongTermWindow(symbol string) *Window {
	if e.longTermWindowSize <= 0 {
		return nil
	}

	w, exists := e.longTermWindows[symbol]
	if !exists {
		w = NewWindow(e.longTermWindowSize)
		e.longTermWindows[symbol] = w
	}

	return w
}

// updateDailyRange อัปเดต open/high/low ของ "วันนี้" ให้ symbol นี้ รีเซ็ต
// อัตโนมัติเมื่อ tick.Timestamp ข้ามวันที่ปฏิทิน (เทียบจากวันที่ของ tick เอง
// ไม่ใช่เวลาเครื่อง Go เพื่อให้ตรงกับเวลาตลาดจริงที่ EA ส่งมา)
func (e *QuantEngine) updateDailyRange(symbol string, midPrice float64, tickTime time.Time) (open, high, low float64) {
	today := tickTime.Format("2006-01-02")

	r, exists := e.dailyRanges[symbol]
	if !exists || r.date != today {
		r = &dailyRange{date: today, open: midPrice, high: midPrice, low: midPrice}
		e.dailyRanges[symbol] = r
		return r.open, r.high, r.low
	}

	if midPrice > r.high {
		r.high = midPrice
	}
	if midPrice < r.low {
		r.low = midPrice
	}

	return r.open, r.high, r.low
}
