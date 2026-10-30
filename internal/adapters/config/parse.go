package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

// parseSymbols splits "XAUUSDm, BTCUSDm" into ["XAUUSDm", "BTCUSDm"].
// Empty input gives an empty list; duplicates and empty items are errors.
func parseSymbols(s string) ([]string, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var out []string
	seen := map[string]bool{}
	for _, part := range strings.Split(s, ",") {
		sym := strings.TrimSpace(part)
		if sym == "" {
			return nil, fmt.Errorf("empty symbol in %q", s)
		}
		if seen[sym] {
			return nil, fmt.Errorf("duplicate symbol %q", sym)
		}
		seen[sym] = true
		out = append(out, sym)
	}

	return out, nil
}

func parseLookback(s string) (map[domain.Timeframe]time.Duration, error) {
	out := map[domain.Timeframe]time.Duration{}
	if strings.TrimSpace(s) == "" {
		return out, nil
	}

	for _, part := range strings.Split(s, ",") {
		key, val, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			return nil, fmt.Errorf("%q: want TF=duration, e.g. M1=7d", part)
		}
		tf := domain.Timeframe(strings.TrimSpace(key))
		if tf.Minutes() == 0 {
			return nil, fmt.Errorf("%q: unknown timeframe %q", part, tf)
		}
		d, err := parseDays(strings.TrimSpace(val))
		if err != nil {
			return nil, fmt.Errorf("%q: %w", part, err)
		}
		out[tf] = d
	}

	return out, nil
}

func parseDays(s string) (time.Duration, error) {
	var d time.Duration
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil {
			return 0, fmt.Errorf("bad number of days %q", s)
		}
		d = time.Duration(n) * 24 * time.Hour
	} else {
		var err error
		if d, err = time.ParseDuration(s); err != nil {
			return 0, fmt.Errorf("bad duration %q", s)
		}
	}

	if d <= 0 {
		return 0, fmt.Errorf("duration %q must be > 0", s)
	}

	return d, nil
}
