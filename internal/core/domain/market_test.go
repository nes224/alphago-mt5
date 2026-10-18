package domain

import (
	"testing"
	"time"
)

func TestTimeframeDuration(t *testing.T) {
	for tf, want := range map[Timeframe]time.Duration{
		TimeframeM1:    time.Minute,
		TimeframeM5:    5 * time.Minute,
		TimeframeM15:   15 * time.Minute,
		TimeframeM30:   30 * time.Minute,
		TimeframeH1:    time.Hour,
		TimeframeH4:    4 * time.Hour,
		TimeframeDaily: 24 * time.Hour,
		"W1":           0,
	} {
		if got := tf.Minutes(); got != int(want) {
			t.Errorf("%s.Duration() = %v, want %v", tf, got, want)
		}
	}
}
