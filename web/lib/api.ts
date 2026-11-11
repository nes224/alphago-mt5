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

async function getJSON<T>(path: string): Promise<T> {
  const res = await fetch(API_URL + path, { cache: "no-store" });
  const body = await res.json();
  if (!res.ok) throw new Error(body?.error ?? res.statusText);
  return body as T;
}

export const fetchSymbols = () => getJSON<string[]>("/api/symbols");

export const fetchCandles = (symbol: string, tf: TimeframeId, limit = 1500) =>
  getJSON<Candle[]>(`/api/candles?symbol=${encodeURIComponent(symbol)}&tf=${tf}&limit=${limit}`);

export const fetchHealth = () => getJSON<Health>("/health");

export const streamURL = (symbol: string) => `${API_URL}/api/stream?symbol=${encodeURIComponent(symbol)}`;
