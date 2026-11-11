"use client";

import { useEffect, useMemo, useState } from "react";
import { addSymbol, fetchBroker, removeSymbol, type BrokerInfo } from "@/lib/api";

type Props = {
  tracked: string[];
  onChange: (tracked: string[], added?: string) => void;
  onClose: () => void;
};

/** Panel to pick which broker symbols the gateway tracks. */
export default function SymbolManager({ tracked, onChange, onClose }: Props) {
  const [broker, setBroker] = useState<BrokerInfo | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [query, setQuery] = useState("");
  const [busy, setBusy] = useState<string | null>(null);

  useEffect(() => {
    fetchBroker()
      .then(setBroker)
      .catch((e: Error) => setError(e.message));
  }, []);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const available = useMemo(() => {
    const q = query.trim().toLowerCase();
    return (broker?.symbols ?? [])
      .filter((s) => !tracked.includes(s) && s.toLowerCase().includes(q))
      .sort();
  }, [broker, tracked, query]);

  const run = async (symbol: string, action: () => Promise<string[]>, added?: string) => {
    setBusy(symbol);
    setError(null);
    try {
      onChange(await action(), added);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(null);
    }
  };

  return (
    <div className="overlay" onClick={onClose}>
      <div className="panel" role="dialog" aria-label="Symbols" onClick={(e) => e.stopPropagation()}>
        <div className="panel-head">
          <strong>Symbols</strong>
          <button onClick={onClose} aria-label="Close">
            ✕
          </button>
        </div>

        {broker && (
          <p className="muted">
            {broker.broker} · {broker.account_type} #{broker.login} · {broker.symbols.length} symbols in Market Watch
          </p>
        )}
        {error && <p className="err">{error}</p>}

        <h4>Tracking</h4>
        <ul className="sym-list">
          {tracked.map((s) => (
            <li key={s}>
              <span>{s}</span>
              <button disabled={busy !== null} onClick={() => run(s, () => removeSymbol(s))}>
                {busy === s ? "..." : "Remove"}
              </button>
            </li>
          ))}
          {tracked.length === 0 && <li className="muted">none yet</li>}
        </ul>

        <h4>Add from broker</h4>
        <input
          autoFocus
          placeholder="Search, e.g. BTC, EUR, US30"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <ul className="sym-list scroll">
          {!broker && !error && <li className="muted">loading broker symbols...</li>}
          {available.map((s) => (
            <li key={s}>
              <span>{s}</span>
              <button disabled={busy !== null} onClick={() => run(s, () => addSymbol(s), s)}>
                {busy === s ? "..." : "Add"}
              </button>
            </li>
          ))}
          {broker && available.length === 0 && <li className="muted">no match</li>}
        </ul>
        <p className="muted small">
          Missing a symbol? Show it in MT5 first (Market Watch → right click → Symbols). Live prices come only
          from the chart the EA runs on; other symbols update from MT5 history about once a minute.
        </p>
      </div>
    </div>
  );
}
