package candlefile

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

// WriteCandles เขียนแท่งลงไฟล์ตามกติกา:
//   - เขียนเฉพาะแท่งที่ปิดแล้ว (Closed == true)
//   - แบ่งลงไฟล์ตามช่วงเวลา (FilePath)
//   - merge กับของเดิม: เวลาตรงกัน = ของใหม่ทับ, แถวอื่นอยู่ครบ
//   - เขียนแบบ atomic: พังกลางทาง ไฟล์เดิมไม่เสีย

func WriteCandles(root, symbol string, tf domain.Timeframe, candles []domain.Candle) error {
	groups := make(map[string][]domain.Candle)
	for _, c := range candles {
		if !c.Closed {
			continue
		}
		if c.Symbol != symbol {
			return fmt.Errorf("candlefile: candle %v has symbol %s, want %s", c.OpenTime, c.Symbol, symbol)
		}
		if c.Timeframe != tf {
			return fmt.Errorf("candlefile: candle %v has timeframe %s, want %s", c.OpenTime, c.Timeframe, tf)
		}

		path, err := FilePath(root, symbol, tf, c.OpenTime)
		if err != nil {
			return err
		}

		groups[path] = append(groups[path], c)
	}

	// เขียนไฟล์เก่าก่อนไฟล์ใหม่ (ชื่อไฟล์เป็นวันที่ เรียงตามตัวอักษร = เรียงตามเวลา)
	paths := make([]string, 0, len(groups))
	for p := range groups {
		paths = append(paths, p)
	}

	sort.Strings(paths)

	for _, p := range paths {
		if err := mergeFile(p, symbol, tf, groups[p]); err != nil {
			return err
		}
	}
	return nil
}

func mergeFile(path, symbol string, tf domain.Timeframe, incoming []domain.Candle) error {
	existing, err := ReadFile(path, symbol, tf)
	if err != nil {
		return err
	}

	// key = วินาที unix ของ OpenTime (ใช้ time.Time เป็น key ตรงๆ ไม่ได้ เพราะ location ต่างกันจะถือว่าคนละค่า)
	byTime := make(map[int64]domain.Candle, len(existing)+len(incoming))

	for _, c := range existing {
		byTime[c.OpenTime.Unix()] = c
	}

	for _, c := range incoming {
		byTime[c.OpenTime.Unix()] = c
	}

	merged := make([]domain.Candle, 0, len(byTime))
	for _, c := range byTime {
		merged = append(merged, c)
	}
	sort.Slice(merged, func(i, j int) bool { return merged[i].OpenTime.Before(merged[j].OpenTime) })
	return writeAtomic(path, merged)
}

func writeAtomic(path string, candles []domain.Candle) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()

	w := csv.NewWriter(tmp)
	if err := w.Write(header); err != nil {
		return err
	}

	for _, c := range candles {
		row := []string{
			c.OpenTime.UTC().Format(timeLayout),
			formatPrice(c.Open),
			formatPrice(c.High),
			formatPrice(c.Low),
			formatPrice(c.Close),
			strconv.Itoa(c.TickCount),
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return err
	}

	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func formatPrice(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
