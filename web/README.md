# alphago-web

Next.js dashboard for the AlphaGo gateway: a candlestick chart of the stored candles
(M1 to D1) that keeps moving with live ticks, plus MT5 connection and backfill status.

It only talks to the gateway API (`cmd/gateway`):

| Endpoint | Used for |
| --- | --- |
| `GET /api/symbols` | symbol list |
| `GET /api/candles?symbol=&tf=&limit=` | stored candles |
| `GET /api/stream?symbol=` | live ticks (Server-Sent Events) |
| `GET /health` | connection and backfill status |

## Run

```bash
npm install
npm run dev          # http://localhost:3000
```

The gateway must be running (default `http://localhost:8081`). Point the app elsewhere with
`NEXT_PUBLIC_API_URL`; it is read at build time. The gateway allows the browser origin set
in its `WEB_ORIGIN` (default `http://localhost:3000`).

Times on the chart are broker server time (UTC+0).
