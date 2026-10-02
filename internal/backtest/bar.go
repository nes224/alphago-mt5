package backtest

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// barTimeLayout matches the CSV export format "2006.01.02 15:04" used by
// the XAU_{1m,5m,...}_data.csv files this package was built to replay.
const barTimeLayout = "2006.01.02 15:04"

// Bar is one OHLCV row from a semicolon-delimited CSV export
// ("Date;Open;High;Low;Close;Volume").
type Bar struct {
	Time   time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume int64
}

// ParseBarLine parses one semicolon-delimited OHLCV row. It does not expect
// or skip the header row ("Date;Open;High;Low;Close;Volume") -- callers
// (e.g. StreamOHLCVCSV) are responsible for not feeding it the header.
func ParseBarLine(line string) (Bar, error) {
	fields := strings.Split(line, ";")
	if len(fields) != 6 {
		return Bar{}, fmt.Errorf("expected 6 semicolon-delimited fields, got %d: %q", len(fields), line)
	}

	t, err := time.Parse(barTimeLayout, fields[0])
	if err != nil {
		return Bar{}, fmt.Errorf("parse timestamp %q: %w", fields[0], err)
	}

	open, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return Bar{}, fmt.Errorf("parse open %q: %w", fields[1], err)
	}
	high, err := strconv.ParseFloat(fields[2], 64)
	if err != nil {
		return Bar{}, fmt.Errorf("parse high %q: %w", fields[2], err)
	}
	low, err := strconv.ParseFloat(fields[3], 64)
	if err != nil {
		return Bar{}, fmt.Errorf("parse low %q: %w", fields[3], err)
	}
	closePrice, err := strconv.ParseFloat(fields[4], 64)
	if err != nil {
		return Bar{}, fmt.Errorf("parse close %q: %w", fields[4], err)
	}

	volume, err := strconv.ParseInt(fields[5], 10, 64)
	if err != nil {
		return Bar{}, fmt.Errorf("parse volume %q: %w", fields[5], err)
	}

	return Bar{
		Time:   t,
		Open:   open,
		High:   high,
		Low:    low,
		Close:  closePrice,
		Volume: volume,
	}, nil
}
