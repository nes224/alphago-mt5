package strategy

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/ports"
)

type dailyRange struct {
	date string // "2006-01-02"
	open float64
	high float64
	low  float64
}

type QuantEngine struct {
	mu            sync.RWMutex
	strategies    []ports.QuantStrategy // list ของ strategy ที่ลงทะเบียนไว้ (VolumeExpansionStrategy, LiquiditySweepStrategy) ถูกเรียก OnTick ทุก tick
	latestMetrics map[string]domain.TickMetrics // — metrics ล่าสุดของแต่ละ symbol (key=symbol) ที่ /api/v1/status ดึงไปโชว์
	tickChan      chan domain.Tick // signalChan||tickChan — channel รับ tick เข้า (PushTick) และส่ง signal ออก (ExecutionRouter ดึงไปคำนวณ order)
	signalChan    chan domain.OrderSignal 
	lastTickMap   map[string]domain.Tick // tick ก่อนหน้าของแต่ละ symbol ใช้คำนวณ delta/velocity (ราคา, volume, OI เทียบกับ tick ก่อน)
	windows       map[string]*Window // windows / windowSize — window นับทีละ tick (default 20 ตัว) ใช้คำนวณ Mean/StdDev/ZScore/TrendSlope
	windowSize    int
	longTermWindows    map[string]*Window
	longTermWindowSize int
	mtfFilter *MultiTimeframeFilter
	velocityWindows   map[string]*Window
	volatilityWindows map[string]*Window
	sizingWindows map[string]*TimeWindow
	atrCalculators map[string]*ATRCalculator
	cvdWindows      map[string]*RollingCVD
	cvdTrendWindows map[string]*Window
	volumeProfiles  map[string]*RollingVolumeProfile
	lcFilter        *LiquidityConfluenceFilter

	dailyRanges map[string]*dailyRange

	cooldownMu     sync.Mutex
	signalCooldown time.Duration
	lastSignalTime map[string]time.Time
}

func NewQuantEngine(bufferSize int, windowSize int) *QuantEngine {
	return &QuantEngine{
		strategies:        make([]ports.QuantStrategy, 0),
		tickChan:          make(chan domain.Tick, bufferSize),
		signalChan:        make(chan domain.OrderSignal, bufferSize),
		latestMetrics:     make(map[string]domain.TickMetrics),
		lastTickMap:       make(map[string]domain.Tick),
		windows:           make(map[string]*Window),
		windowSize:        windowSize,
		longTermWindows:   make(map[string]*Window),
		velocityWindows:   make(map[string]*Window),
		volatilityWindows: make(map[string]*Window),
		sizingWindows:     make(map[string]*TimeWindow),
		atrCalculators:    make(map[string]*ATRCalculator),
		cvdWindows:        make(map[string]*RollingCVD),
		cvdTrendWindows:   make(map[string]*Window),
		volumeProfiles:    make(map[string]*RollingVolumeProfile),
		dailyRanges:       make(map[string]*dailyRange),
		lastSignalTime:    make(map[string]time.Time),
	}
}

func (e *QuantEngine) SetLongTermWindowSize(n int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.longTermWindowSize = n
}

func (e *QuantEngine) SetMultiTimeframeFilter(f *MultiTimeframeFilter) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.mtfFilter = f
}

func (e *QuantEngine) SetSignalCooldown(d time.Duration) {
	e.cooldownMu.Lock()
	defer e.cooldownMu.Unlock()
	e.signalCooldown = d
}

func (e *QuantEngine) allowSignal(symbol string, at time.Time) bool {
	e.cooldownMu.Lock()
	defer e.cooldownMu.Unlock()

	if e.signalCooldown <= 0 {
		return true
	}

	if last, exists := e.lastSignalTime[symbol]; exists && at.Sub(last) < e.signalCooldown {
		return false
	}
	e.lastSignalTime[symbol] = at
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

	velocityWindow := e.getOrCreateVelocityWindow(tick.Symbol)
	velocityTrendSlope := velocityWindow.Slope()
	velocityWindow.Push(priceVelocity)

	volatilityWindow := e.getOrCreateVolatilityWindow(tick.Symbol)
	volatilityTrendSlope := volatilityWindow.Slope()
	volatilityWindow.Push(stdDev)

	var longTermTrendSlope float64
	if ltWindow := e.getOrCreateLongTermWindow(tick.Symbol); ltWindow != nil {
		longTermTrendSlope = ltWindow.Slope()
		ltWindow.Push(midPrice)
	}

	sizingWindow := e.getOrCreateSizingWindow(tick.Symbol)
	sizingVolatility := sizingWindow.StdDev()
	sizingWindow.Push(tick.Timestamp, midPrice)

	atrCalc := e.getOrCreateATR(tick.Symbol)
	atrValue, atrReady := atrCalc.Value()
	atrCalc.Push(tick.Timestamp, midPrice)
	cvd := e.getOrCreateCVD(tick.Symbol)
	cvdReady := cvd.Span() >= time.Duration(float64(cvdWindowDuration)*minDirectionWarmupFraction)
	cvdValue := cvd.Value()
	cvd.Push(tick.Timestamp, midPrice, volumeDelta)

	cvdTrendWindow := e.getOrCreateCVDTrendWindow(tick.Symbol)
	cvdTrendSlope := cvdTrendWindow.Slope()
	cvdTrendWindow.Push(cvdValue)

	profile := e.getOrCreateVolumeProfile(tick.Symbol)
	profileReady := profile.Span() >= time.Duration(float64(volumeProfileDuration)*minDirectionWarmupFraction)
	pocPrice, pocVolume, pocOk := profile.POC()
	volumeAtCurrentPrice := profile.VolumeAt(midPrice)
	absVolumeDelta := volumeDelta
	if absVolumeDelta < 0 {
		absVolumeDelta = -absVolumeDelta
	}
	profile.Push(tick.Timestamp, midPrice, float64(absVolumeDelta))

	var distanceToPOC float64
	if pocOk {
		distanceToPOC = midPrice - pocPrice
	}

	dayOpen, dayHigh, dayLow := e.updateDailyRange(tick.Symbol, midPrice, tick.Timestamp)

	if e.mtfFilter != nil {
		e.mtfFilter.PushTick(tick.Symbol, midPrice, tick.Timestamp)
	}

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

		VelocityTrendSlope:   velocityTrendSlope,
		VolatilityTrendSlope: volatilityTrendSlope,
		SizingVolatility:     sizingVolatility,
		ATR:                  atrValue,
		ATRReady:             atrReady,

		CVD:           cvdValue,
		CVDReady:      cvdReady,
		CVDTrendSlope: cvdTrendSlope,

		POCPrice:             pocPrice,
		POCVolume:            pocVolume,
		POCReady:             profileReady && pocOk,
		DistanceToPOC:        distanceToPOC,
		VolumeAtCurrentPrice: volumeAtCurrentPrice,

		Timestamp:  tick.Timestamp,
		WindowSize: window.Size(),
	}
}

func (e *QuantEngine) ProcessTick(tick domain.Tick) domain.TickMetrics {
	e.mu.Lock()

	metrics := e.calculateMetrics(tick)
	e.latestMetrics[tick.Symbol] = metrics
	e.lastTickMap[tick.Symbol] = tick

	strategies := make([]ports.QuantStrategy, len(e.strategies))
	copy(strategies, e.strategies)
	mtfFilter := e.mtfFilter
	lcFilter := e.lcFilter

	e.mu.Unlock()

	for _, s := range strategies {
		if signal := s.OnTick(tick, metrics); signal != nil {
			if mtfFilter != nil && !mtfFilter.Allows(signal.Symbol, signal.Action) {
				continue
			}

			if lcFilter != nil && !lcFilter.Allows(metrics, signal.Action) {
				continue
			}

			if !e.allowSignal(signal.Symbol, tick.Timestamp) {
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

func (e *QuantEngine) Backfill(ticks []domain.Tick) {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, tick := range ticks {
		metrics := e.calculateMetrics(tick)
		e.latestMetrics[tick.Symbol] = metrics
		e.lastTickMap[tick.Symbol] = tick
	}
}

func (e *QuantEngine) GetLatestMetrics(symbol string) domain.TickMetrics {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.latestMetrics[symbol]
}

func (e *QuantEngine) GetMultiTimeframeState(symbol string) domain.MultiTimeframeState {
	e.mu.RLock()
	f := e.mtfFilter
	e.mu.RUnlock()

	if f == nil {
		return domain.MultiTimeframeState{AllowsBuy: true, AllowsSell: true}
	}
	return f.State(symbol)
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

func (e *QuantEngine) getOrCreateVelocityWindow(symbol string) *Window {
	w, exists := e.velocityWindows[symbol]
	if !exists {
		w = NewWindow(e.windowSize)
		e.velocityWindows[symbol] = w
	}

	return w
}

func (e *QuantEngine) getOrCreateVolatilityWindow(symbol string) *Window {
	w, exists := e.volatilityWindows[symbol]
	if !exists {
		w = NewWindow(e.windowSize)
		e.volatilityWindows[symbol] = w
	}

	return w
}

const sizingWindowDuration = 15 * time.Minute

func (e *QuantEngine) getOrCreateSizingWindow(symbol string) *TimeWindow {
	w, exists := e.sizingWindows[symbol]
	if !exists {
		w = NewTimeWindow(sizingWindowDuration)
		e.sizingWindows[symbol] = w
	}

	return w
}

const (
	atrPeriodDuration = 5 * time.Minute
	atrNumPeriods     = 14
)

func (e *QuantEngine) getOrCreateATR(symbol string) *ATRCalculator {
	a, exists := e.atrCalculators[symbol]
	if !exists {
		a = NewATRCalculator(atrPeriodDuration, atrNumPeriods)
		e.atrCalculators[symbol] = a
	}

	return a
}

const (
	cvdWindowDuration       = 15 * time.Minute
	volumeProfileDuration   = 60 * time.Minute
	volumeProfileBucketSize = 0.50
)

func (e *QuantEngine) getOrCreateCVD(symbol string) *RollingCVD {
	c, exists := e.cvdWindows[symbol]
	if !exists {
		c = NewRollingCVD(cvdWindowDuration)
		e.cvdWindows[symbol] = c
	}

	return c
}

func (e *QuantEngine) getOrCreateCVDTrendWindow(symbol string) *Window {
	w, exists := e.cvdTrendWindows[symbol]
	if !exists {
		w = NewWindow(e.windowSize)
		e.cvdTrendWindows[symbol] = w
	}

	return w
}

func (e *QuantEngine) getOrCreateVolumeProfile(symbol string) *RollingVolumeProfile {
	p, exists := e.volumeProfiles[symbol]
	if !exists {
		p = NewRollingVolumeProfile(volumeProfileBucketSize, volumeProfileDuration)
		e.volumeProfiles[symbol] = p
	}

	return p
}

func (e *QuantEngine) SetLiquidityConfluenceFilter(f *LiquidityConfluenceFilter) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.lcFilter = f
}

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
