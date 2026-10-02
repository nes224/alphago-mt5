package backtest

import (
	"bufio"
	"fmt"
	"os"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

func StreamOHLCVCSV(path, symbol string, from, to time.Time, spreadHalf float64, onTick func(domain.Tick)) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// 1-minute bars over years of history produce long files but short
	// lines; the default scanner buffer is already plenty, no need to grow it.

	lineNum := 0
	if scanner.Scan() {
		lineNum++ // header, discarded
	}

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if line == "" {
			continue
		}

		bar, err := ParseBarLine(line)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNum, err)
		}

		if bar.Time.Before(from) || !bar.Time.Before(to) {
			continue
		}

		for _, tick := range SynthesizeTicks(bar, symbol, spreadHalf) {
			onTick(tick)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	return nil
}

// LastBarTime returns the timestamp of the last data row in path -- used to
// default a backtest's "-to" flag to the file's actual last date without the
// caller needing to know it up front. Reads the whole file (simple, and fine
// for a one-shot CLI tool; this is not called from the hot streaming path).
func LastBarTime(path string) (time.Time, error) {
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var lastLine string
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if lineNum == 1 {
			continue // header
		}
		if line := scanner.Text(); line != "" {
			lastLine = line
		}
	}
	if err := scanner.Err(); err != nil {
		return time.Time{}, fmt.Errorf("read %s: %w", path, err)
	}
	if lastLine == "" {
		return time.Time{}, fmt.Errorf("%s has no data rows", path)
	}

	bar, err := ParseBarLine(lastLine)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse last line: %w", err)
	}
	return bar.Time, nil
}
