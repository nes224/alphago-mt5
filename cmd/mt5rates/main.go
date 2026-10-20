package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

// mt5rates — เครื่องมือทดสอบ: ขอแท่งย้อนหลังจาก EA ผ่าน command port แล้วพิมพ์สรุป
//
//	go run ./cmd/mt5rates -symbol XAUUSDm -tf M1 -back 24h
//	go run ./cmd/mt5rates -symbol BTCUSDm -tf H4 -back 720h
//
// ใช้ MT5_HOST / MT5_PORT / MT5_TIMEOUT_SECONDS เหมือน Gateway
// หมายเหตุ: EA รับ command client ได้ทีละตัว — ปิด Gateway (หรือ nc) ที่ต่อ 5555 อยู่ก่อน

func main() {
	symbol := flag.String("symbol", "", "symbol as the in mt5 for example XAUUSDm (maybe?)")
	tf := flag.String("tf", "M1", "timeframe: M1 M5 M15 M30 H1m H4 Daily")
	back := flag.Duration("back", 24*time.Hour, "look back for time duration server in the present to know how much for example 3h, 24h, 720h")
	flag.Parse()

	if *symbol == "" {
		fmt.Fprintln(os.Stderr, "Identify symbol as XAUUSDm")
		os.Exit(2)
	}

	if err := run(*symbol, domain.Timeframe(*tf), *back); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(symbol string, tf domain.Timeframe, back time.Duration) error {
	return nil
}
