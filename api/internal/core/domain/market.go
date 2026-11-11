package domain

import "time"

var AllTimeframes = []Timeframe{
	TimeframeM1, TimeframeM5, TimeframeM15, TimeframeM30,
	TimeframeH1, TimeframeH4, TimeframeDaily,
}

// Tick ราคาหนึ่งจังหวะจาก MT5
type Tick struct {
	Symbol string
	Bid    float64
	Ask    float64
	Volume int64
	Time   time.Time // server ของ broker
}
type Candle struct {
	Symbol    string
	Timeframe Timeframe
	OpenTime  time.Time
	Open      float64
	High      float64
	Low       float64
	Close     float64
	TickCount int
	Volume    int64
	Closed    bool
}

type TradeClosed struct {
	Symbol string
	Ticket uint64
	Profit float64
	Time   time.Time
}

func (t Tick) Mid() float64 { return (t.Bid + t.Ask) / 2 }

func (t Tick) Spread() float64 { return t.Ask - t.Bid }

type Timeframe string

const (
	TimeframeM1    Timeframe = "M1"
	TimeframeM5    Timeframe = "M5"
	TimeframeM15   Timeframe = "M15"
	TimeframeM30   Timeframe = "M30"
	TimeframeH1    Timeframe = "H1"
	TimeframeH4    Timeframe = "H4"
	TimeframeDaily Timeframe = "Daily"
)

// Minutes คืนความยาวของ TF เป็นนาที
func (tf Timeframe) Minutes() int {
	switch tf {
	case TimeframeM1:
		return 1
	case TimeframeM5:
		return 5
	case TimeframeM15:
		return 15
	case TimeframeM30:
		return 30
	case TimeframeH1:
		return 60
	case TimeframeH4:
		return 240
	case TimeframeDaily:
		return 1440
	default:
		return 0
	}
}

func (tf Timeframe) Duration() time.Duration { return time.Duration(tf.Minutes() * int(time.Minute)) }
