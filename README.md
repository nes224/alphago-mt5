# AlphaGo MT5

Trading system on MetaTrader 5, written as a portfolio project.

| Folder | What it is | Stack |
| --- | --- | --- |
| `api/` | Backend: MT5 gateway, candle files, backfill, backtest, HTTP API | Go |
| `web/` | Frontend: dashboard with a live candlestick chart | Next.js, TypeScript |
| `*.mq5` | Expert Advisor that runs inside MT5 (ports 5555 / 5556) | MQL5 |
| `docs/` | Results and notes | |

## Run everything

```bash
cp api/app.env.example api/app.env   # set SYMBOLS, MT5_TIMEOUT_SECONDS=30, ...
docker compose up --build
```

- web dashboard: http://localhost:3000
- API: http://localhost:8081/health

MT5 keeps running on the host; the API container reaches it at `host.docker.internal`.

## Symbols

The symbols you can trade are the ones in MT5's **Market Watch** (right click → Symbols to show more).
The EA reports them with the `ACCOUNT_INFO` command.

- In the dashboard, click **Symbols** to add or remove tracked symbols.
- Tracked symbols are saved in `api/data/symbols.json`. `SYMBOLS` in `app.env` only seeds that file on the first run.
- Live ticks come only from the chart the EA is attached to. Other symbols update from MT5 history after every closed M1 candle of that chart (about once a minute).

| Endpoint | What it does |
| --- | --- |
| `GET /api/broker` | account info and every symbol in Market Watch |
| `GET /api/symbols` | tracked symbols |
| `POST /api/symbols` `{"symbol":"BTCUSDm"}` | track a symbol (must be in Market Watch), then backfill it |
| `DELETE /api/symbols/{symbol}` | stop tracking (candle files are kept) |

## Run without Docker

```bash
cd api && set -a && source app.env && set +a && go run ./cmd/gateway
cd web && npm install && npm run dev
```

## Backend tools

```bash
cd api
go test -race ./...
go run ./cmd/backtest -symbol XAUUSDm -tf H1 -filter Daily -split 0.7
go run ./cmd/mt5rates -symbol XAUUSDm -tf M1 -back 1h
```
