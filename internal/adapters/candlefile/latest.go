package candlefile

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

// LatestCandle returns the newest stored candle for (symbol, tf).
// ok=false means nothing is stored yet (no directory, or no rows in any file).
func LatestCandle(root, symbol string, tf domain.Timeframe) (c domain.Candle, ok bool, err error) {
	dir, err := Dir(root, symbol, tf)
	if err != nil {
		return domain.Candle{}, false, err
	}
	fmt.Println(dir)

	return domain.Candle{}, false, nil
}

// periodFiles lists the candle files in dir, oldest first.
// Anything not named like a period file (e.g. *.tmp, .DS_Store) is ignored.

func periodFiles(dir string, tf domain.Timeframe) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	layout := periodLayout(tf)
	var names []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".csv") {
			continue
		}

		if _, err := time.Parse(layout, strings.TrimSuffix(name, ".csv")); err != nil {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}
