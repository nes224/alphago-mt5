package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/nes224/alphago-mt5/internal/adapters/config"
	"github.com/nes224/alphago-mt5/internal/adapters/mt5"
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
	if tf.Minutes() == 0 {
		return fmt.Errorf("Unknown timeframe %q (use M1 M5 M15 M30 H1 H4 Daily)", tf)
	}

	cfg, err := config.LoadGateway()
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	client := mt5.NewCommandClient(cfg.CmdAddr(), cfg.MT5Timeout, log)
	defer client.Close()

	probe, err := client.RatesPage(ctx, symbol, tf, time.Time{}, time.Time{}, 1)
	if err != nil {
		return fmt.Errorf("probe: %w", err)
	}
	now := probe.ServerTime
	from := now.Add(-back)

	start := time.Now()
	candles, err := client.Rates(ctx, symbol, tf, from, now)
	if err != nil {
		return err
	}

	closed := 0
	for _, c := range candles {
		if c.Closed {
			closed++
		}
	}

	const layout = "2006-01-02 15:04"
	fmt.Printf("symbol %s tf %s (tf_m_%d)\n", symbol, tf, tf.Minutes())
	fmt.Printf("server time %s (Thai %s)\n", now.Format("2006-01-02 15:04:05"), now.Add(7*time.Hour).Format("15:04:05"))
	fmt.Printf("range %s - > %s\n", from.Format(layout), now.Format(layout))
	fmt.Printf("candles %d (closed %d, doing %d) in %v\n", len(candles), closed, len(candles)-closed, time.Since(start).Round(time.Millisecond))
	if len(candles) == 0 {
		fmt.Println("(There is no candle stick in time period - market closed?)")
		return nil
	}

	fmt.Println()
	fmt.Println("open time open high low close ticks closed")
	show := func(c domain.Candle) {
		fmt.Printf("%s %-11.3f %-11.3f %-11.3f %11.3f %-6d %v\n", c.OpenTime.Format(layout), c.Open, c.High, c.Low, c.Close, c.TickCount, c.Closed)
	}
	if len(candles) <= 6 {
		for _, c := range candles {
			show(c)
		}

		return nil
	}

	for _, c := range candles[:3] {
		show(c)
	}

	fmt.Printf("... (%d candlestick) ...\n", len(candles)-6)
	for _, c := range candles[len(candles)-3:] {
		show(c)
	}
	return nil
}
