// Client for the gateway API (Go service in cmd/gateway).

export const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8081";

export const TIMEFRAMES = [
  { id: "M1", label: "M1", seconds: 60 },
  { id: "M5", label: "M5", seconds: 300 },
  { id: "M15", label: "M15", seconds: 900 },
  { id: "M30", label: "M30", seconds: 1800 },
  { id: "H1", label: "H1", seconds: 3600 },
  { id: "H4", label: "H4", seconds: 14400 },
  { id: "Daily", label: "D1", seconds: 86400 },
] as const;

export type TimeframeId = (typeof TIMEFRAMES)[number]["id"];

/** One stored candle. t = open time in unix seconds (broker server time). */
export type Candle = { t: number; o: number; h: number; l: number; c: number };

/** One live price. t = unix milliseconds (broker server time). */
export type Tick = { symbol: string; bid: number; ask: number; t: number };

export type BackfillReport = { symbol: string; finished_at: string; error?: string };

export type Health = {
  status: string;
  mt5_stream: boolean;
  mt5_cmd: boolean;
  backfill?: { running: boolean; runs: number; last?: BackfillReport[] };
};

/** The trading account and the symbols the broker offers (MT5 Market Watch). */
export type BrokerInfo = {
  broker: string;
  login: number;
  account_type: string;
  currency: string;
  balance: number;
  equity: number;
  leverage: number;
  symbols: string[];
};

async function getJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(API_URL + path, { cache: "no-store", ...init });
  const body = await res.json().catch(() => null);
  if (!res.ok) throw new Error(body?.error ?? res.statusText);
  return body as T;
}

/** Tracked symbols. */
export const fetchSymbols = () => getJSON<string[]>("/api/symbols");

export const fetchBroker = () => getJSON<BrokerInfo>("/api/broker");

/** Starts tracking a symbol; returns the new tracked list. */
export const addSymbol = (symbol: string) =>
  getJSON<string[]>("/api/symbols", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ symbol }),
  });

/** Stops tracking a symbol (stored candles are kept); returns the new tracked list. */
export const removeSymbol = (symbol: string) =>
  getJSON<string[]>(`/api/symbols/${encodeURIComponent(symbol)}`, { method: "DELETE" });

export const fetchCandles = (symbol: string, tf: TimeframeId, limit = 1500) =>
  getJSON<Candle[]>(`/api/candles?symbol=${encodeURIComponent(symbol)}&tf=${tf}&limit=${limit}`);

export const fetchHealth = () => getJSON<Health>("/health");

export const streamURL = (symbol: string) => `${API_URL}/api/stream?symbol=${encodeURIComponent(symbol)}`;
