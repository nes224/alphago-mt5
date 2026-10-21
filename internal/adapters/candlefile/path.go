package candlefile

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

// Package candlefile เก็บแท่งเทียนเป็นไฟล์ CSV:
//	<root>/<symbol ตัวเล็ก>/tf_m_<นาที>/<ช่วงเวลา>.csv
// แบ่งไฟล์ตามขนาด TF: M1–M30 รายวัน, H1–H4 รายเดือน, D1 รายปี

func FilePath(root, symbol string, tf domain.Timeframe, openTime time.Time) (string, error) {
	dir, err := Dir(root, symbol, tf)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fileName(tf, openTime)), nil
}

func Dir(root, symbol string, tf domain.Timeframe) (string, error) {
	if symbol == "" || strings.ContainsAny(symbol, `/\.`) {
		return "", fmt.Errorf("candlefile: invalid symbol %q", symbol)
	}

	if tf.Minutes() == 0 {
		return "", errors.New("candlefile: unsupported timeframe " + string(tf))
	}

	tfDir := fmt.Sprintf("tf_m_%d", tf.Minutes())
	return filepath.Join(root, strings.ToLower(symbol), tfDir), nil
}

func fileName(tf domain.Timeframe, openTime time.Time) string {
	t := openTime.UTC()
	switch {
	case tf.Minutes() <= 30:
		return t.Format("2006-01-02") + ".csv"
	case tf.Minutes() < 1440:
		return t.Format("2006-01") + ".csv"
	default:
		return t.Format("2006") + ".csv"
	}
}