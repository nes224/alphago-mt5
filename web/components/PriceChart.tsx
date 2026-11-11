"use client";

import { useEffect, useRef } from "react";
import {
  CandlestickSeries,
  ColorType,
  CrosshairMode,
  createChart,
  type CandlestickData,
  type IChartApi,
  type ISeriesApi,
  type UTCTimestamp,
} from "lightweight-charts";
import type { Candle, Tick } from "@/lib/api";

type Props = {
  candles: Candle[];
  tick: Tick | null;
  tfSeconds: number;
  digits: number;
  onHover: (bar: CandlestickData<UTCTimestamp> | null) => void;
};

/** Candlestick chart: draws the stored candles, then moves the newest one with each live tick. */
export default function PriceChart({ candles, tick, tfSeconds, digits, onHover }: Props) {
  const box = useRef<HTMLDivElement>(null);
  const chart = useRef<IChartApi | null>(null);
  const series = useRef<ISeriesApi<"Candlestick"> | null>(null);
  const last = useRef<CandlestickData<UTCTimestamp> | null>(null);
  const hover = useRef(onHover);
  hover.current = onHover;

  // create the chart once
  useEffect(() => {
    if (!box.current) return;
    const c = createChart(box.current, {
      autoSize: true,
      layout: { background: { type: ColorType.Solid, color: "#0f1115" }, textColor: "#8b93a1" },
      grid: { vertLines: { color: "#1c2129" }, horzLines: { color: "#1c2129" } },
      rightPriceScale: { borderColor: "#262c36" },
      timeScale: { borderColor: "#262c36", timeVisible: true, secondsVisible: false },
      crosshair: { mode: CrosshairMode.Normal },
    });
    const s = c.addSeries(CandlestickSeries, {
      upColor: "#26a69a",
      downColor: "#ef5350",
      borderVisible: false,
      wickUpColor: "#26a69a",
      wickDownColor: "#ef5350",
    });
    c.subscribeCrosshairMove((p) => {
      const bar = p.seriesData.get(s) as CandlestickData<UTCTimestamp> | undefined;
      hover.current(bar ?? last.current);
    });
    chart.current = c;
    series.current = s;
    return () => {
      c.remove();
      chart.current = null;
      series.current = null;
    };
  }, []);

  // stored candles
  useEffect(() => {
    const s = series.current;
    if (!s) return;
    const data = candles.map((x) => ({ time: x.t as UTCTimestamp, open: x.o, high: x.h, low: x.l, close: x.c }));
    s.setData(data);
    last.current = data.length ? { ...data[data.length - 1] } : null;
    hover.current(last.current);
    chart.current?.timeScale().scrollToRealTime();
  }, [candles]);

  useEffect(() => {
    series.current?.applyOptions({ priceFormat: { type: "price", precision: digits, minMove: 10 ** -digits } });
  }, [digits]);

  // live tick: update the newest candle or start the next one (candles are bid prices)
  useEffect(() => {
    const s = series.current;
    if (!s || !tick) return;
    const sec = Math.floor(tick.t / 1000);
    const start = (sec - (sec % tfSeconds)) as UTCTimestamp;
    const bar = last.current;
    if (bar && start < bar.time) return;
    if (bar && start === bar.time) {
      bar.high = Math.max(bar.high, tick.bid);
      bar.low = Math.min(bar.low, tick.bid);
      bar.close = tick.bid;
    } else {
      last.current = { time: start, open: tick.bid, high: tick.bid, low: tick.bid, close: tick.bid };
    }
    try {
      s.update(last.current!);
    } catch {
      // the tick is older than the chart's newest bar
    }
  }, [tick, tfSeconds]);

  return <div ref={box} className="chart" />;
}
