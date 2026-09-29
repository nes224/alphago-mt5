package strategy

import (
	"math"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

type ZScoreStrategy struct {
	symbol      string
	threshold   float64
	priceBuffer []float64
	bufferCap   int
}

func NewZScoreStrategy(symbol string, threshold float64, bufferCap int) *ZScoreStrategy {
	return &ZScoreStrategy{
		symbol:      symbol,
		threshold:   threshold,
		priceBuffer: make([]float64, 0, bufferCap),
		bufferCap:   bufferCap,
	}
}

func (s *ZScoreStrategy) ID() string {
	return "QUANT_ZSCORE_MEAN_REVERSION"
}

func (s *ZScoreStrategy) OnTick(tick domain.Tick, metrics domain.Tick) *domain.OrderSignal {
	if tick.Symbol != s.symbol {
		return nil
	}

	midPrice := (tick.Bid + tick.Ask) / 2.0
	s.priceBuffer = append(s.priceBuffer, midPrice)
	if len(s.priceBuffer) > s.bufferCap {
		s.priceBuffer = s.priceBuffer[1:]
	}

	if len(s.priceBuffer) < s.bufferCap {
		return nil
	}

	mean, stdDev := s.calculateStats(s.priceBuffer)
	if stdDev == 0 {
		return nil
	}

	zScore := (midPrice - mean) / stdDev

	if zScore <= -s.threshold {
		return &domain.OrderSignal{
			Symbol:    s.symbol,
			Action:    domain.SignalAction(domain.ActionBuy),
			Price:     tick.Ask,
			Reason:    "STATISTICAL_OVEREXTENDED_BUY",
			Timestamp: time.Now(),
		}
	} else if zScore >= s.threshold {
		return &domain.OrderSignal{
			Symbol:    s.symbol,
			Action:    domain.SignalAction(domain.ActionSell),
			Price:     tick.Bid,
			Reason:    "STATISTICAL_OVEREXTENDED_SELL",
			Timestamp: time.Now(),
		}
	}

	return nil
}

func (s *ZScoreStrategy) calculateStats(data []float64) (mean float64, stdDev float64) {
	var sum float64
	for _, v := range data {
		sum += v
	}

	mean = sum / float64(len(data))
	var varianceSum float64
	for _, v := range data {
		varianceSum += math.Pow(v-mean, 2)
	}
	stdDev = math.Sqrt(varianceSum / float64(len(data)))
	return mean, stdDev
}
