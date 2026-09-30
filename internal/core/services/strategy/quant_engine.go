package strategy

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/ports"
)

type QuantEngine struct {
	mu            sync.RWMutex
	strategies    []ports.QuantStrategy
	latestMetrics map[string]domain.TickMetrics
	tickChan      chan domain.Tick
	signalChan    chan domain.OrderSignal
	lastTickMap   map[string]domain.Tick
	windows       map[string]*Window
	windowSize    int

	cooldownMu     sync.Mutex
	signalCooldown time.Duration
	lastSignalTime map[string]time.Time
}

func NewQuantEngine(bufferSize int, windowSize int) *QuantEngine {
	return &QuantEngine{
		strategies:     make([]ports.QuantStrategy, 0),
		tickChan:       make(chan domain.Tick, bufferSize),
		signalChan:     make(chan domain.OrderSignal, bufferSize),
		latestMetrics:  make(map[string]domain.TickMetrics),
		lastTickMap:    make(map[string]domain.Tick),
		windows:        make(map[string]*Window),
		windowSize:     windowSize,
		lastSignalTime: make(map[string]time.Time),
	}
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

	return domain.TickMetrics{
		Symbol:         tick.Symbol,
		Price:          midPrice,
		Mean:           mean,
		StdDev:         stdDev,
		ZScore:         zScore,
		TrendSlope:     trendSlope,
		Bid:            tick.Bid,
		Ask:            tick.Ask,
		Spread:         tick.Ask - tick.Bid,
		OpenInterest:   tick.OpenInterest,
		OIDelta:        oiDelta,
		OIVelocity:     oiVelocity,
		VolumeDelta:    volumeDelta,
		VolumeVelocity: volumeVelocity,
		PriceVelocity:  priceVelocity,
		Timestamp:      tick.Timestamp,
		WindowSize:     window.Size(),
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
