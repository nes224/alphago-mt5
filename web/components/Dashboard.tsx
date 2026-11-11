"use client";

import { useEffect, useState } from "react";
import type { CandlestickData, UTCTimestamp } from "lightweight-charts";
import PriceChart from "./PriceChart";
import SymbolManager from "./SymbolManager";
import {
  TIMEFRAMES,
  fetchCandles,
  fetchHealth,
  fetchSymbols,
  streamURL,
  type Candle,
  type Health,
  type Tick,
  type TimeframeId,
} from "@/lib/api";

type Loaded = { symbol: string; tf: TimeframeId; candles: Candle[] };

const decimals = (n: number) => (String(n).split(".")[1] ?? "").length;

export default function Dashboard() {
  const [symbols, setSymbols] = useState<string[]>([]);
  const [symbol, setSymbol] = useState<string | null>(null);
  const [tf, setTf] = useState<TimeframeId>("M15");
  const [loaded, setLoaded] = useState<Loaded | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [tick, setTick] = useState<Tick | null>(null);
  const [live, setLive] = useState(false);
  const [health, setHealth] = useState<Health | null>(null);
  const [hover, setHover] = useState<CandlestickData<UTCTimestamp> | null>(null);
  const [managing, setManaging] = useState(false);

  // tracked symbols changed in the Symbols panel
  const onSymbolsChange = (list: string[], added?: string) => {
    setSymbols(list);
    setSymbol((cur) => added ?? (cur && list.includes(cur) ? cur : (list[0] ?? null)));
  };

  // tracked symbols
  useEffect(() => {
    fetchSymbols()
      .then((s) => {
        setSymbols(s);
        setSymbol((cur) => cur ?? s[0] ?? null);
      })
      .catch((e: Error) => setError(`cannot reach the API: ${e.message}`));
  }, []);

  const report = health?.backfill?.last?.find((r) => r.symbol === symbol);

  // stored candles for the chosen symbol and timeframe, reloaded after each backfill
  // (symbols without a live feed only move when the backfill stores new candles)
  useEffect(() => {
    if (!symbol) return;
    let cancelled = false;
    setError(null);
    fetchCandles(symbol, tf)
      .then((candles) => !cancelled && setLoaded({ symbol, tf, candles }))
      .catch((e: Error) => !cancelled && setError(`could not load candles: ${e.message}`));
    return () => {
      cancelled = true;
    };
  }, [symbol, tf, report?.finished_at]);

  // live ticks for the chosen symbol
  useEffect(() => {
    setTick(null);
    if (!symbol) return;
    const es = new EventSource(streamURL(symbol));
    es.onopen = () => setLive(true);
    es.onerror = () => setLive(false);
    es.onmessage = (ev) => setTick(JSON.parse(ev.data) as Tick);
    return () => {
      es.close();
      setLive(false);
    };
  }, [symbol]);

  // connection and backfill status
  useEffect(() => {
    const poll = () =>
      fetchHealth()
        .then(setHealth)
        .catch(() => setHealth(null));
    poll();
    const id = setInterval(poll, 10_000);
    return () => clearInterval(id);
  }, []);

  const ready = loaded !== null && loaded.symbol === symbol && loaded.tf === tf;
  const candles = loaded?.candles ?? [];
  const digits = Math.min(5, Math.max(2, ...candles.slice(-50).map((c) => decimals(c.c))));
  const tfSeconds = TIMEFRAMES.find((x) => x.id === (loaded?.tf ?? tf))!.seconds;
  const fmt = (v?: number) => (v === undefined ? "-" : v.toFixed(digits));

  const backfill = health?.backfill?.running
    ? "backfill running"
    : report
      ? `backfill ${report.error ? "failed" : "ok"} ${new Date(report.finished_at).toLocaleTimeString()}`
      : "backfill -";

  let message: string | null = error;
  if (!message && !ready) message = "loading...";
  if (!message && !symbol) message = "no symbols tracked: click Symbols to add one";
  if (!message && candles.length === 0) message = "no candles stored for this timeframe yet";

  return (
    <div className="page">
      <header className="bar">
        <span className="brand">AlphaGo</span>
        <select aria-label="Symbol" value={symbol ?? ""} onChange={(e) => setSymbol(e.target.value)}>
          {symbols.map((s) => (
            <option key={s} value={s}>
              {s}
            </option>
          ))}
        </select>
        <button onClick={() => setManaging(true)}>Symbols</button>
        <div className="tfs" role="group" aria-label="Timeframe">
          {TIMEFRAMES.map((x) => (
            <button key={x.id} className={x.id === tf ? "on" : ""} onClick={() => setTf(x.id)}>
              {x.label}
            </button>
          ))}
        </div>
        <div className="prices">
          <span>
            <em>Bid</em>
            {fmt(tick?.bid)}
          </span>
          <span>
            <em>Ask</em>
            {fmt(tick?.ask)}
          </span>
          <span>
            <em>Spread</em>
            {tick ? fmt(tick.ask - tick.bid) : "-"}
          </span>
        </div>
        <div className="status">
          <span>
            <i className={live && tick ? "dot ok" : "dot"} />
            {!live ? "offline" : tick ? "live" : "no live feed"}
          </span>
          <span>
            <i className={health?.mt5_stream ? "dot ok" : "dot"} />
            MT5
          </span>
          <span>{backfill}</span>
        </div>
      </header>
      <main className="main">
        <div className="legend">
          {hover &&
            `${symbol} ${tf}  ${new Date(hover.time * 1000).toISOString().replace("T", " ").slice(0, 16)} (server)  ` +
              `O ${fmt(hover.open)}  H ${fmt(hover.high)}  L ${fmt(hover.low)}  C ${fmt(hover.close)}`}
        </div>
        <PriceChart
          candles={candles}
          tick={ready ? tick : null}
          tfSeconds={tfSeconds}
          digits={digits}
          onHover={setHover}
        />
        {message && <div className="message">{message}</div>}
      </main>
      {managing && (
        <SymbolManager tracked={symbols} onChange={onSymbolsChange} onClose={() => setManaging(false)} />
      )}
    </div>
  );
}
