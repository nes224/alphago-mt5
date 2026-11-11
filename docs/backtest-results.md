# Backtest results: SuperTrend + SMA on XAUUSDm (run 2026-11-11)

Data: the gateway's candle files on 2026-11-05. Costs: spread 0.25 on every trade, no commission.
Results are in R (1R = the stop distance). Signals on candle close, entry at the next open,
stop before target when both fall inside one candle.

## Verdict

No reliable edge yet. The small profits on Daily and H1 come from long trades during gold's
bull run (price rose from 1,249 to 4,065 over the Daily sample); short trades lost.
On M15 and M5 the results are noise and the best in-sample parameters fail out-of-sample.

## Default parameters (ATR 10, x3, SMA 200, SL 1.5 ATR, TP 2R)

| TF / filter | Period | Trades | Win rate | Total R | In-sample R (70%) | Out-of-sample R (30%) |
| --- | --- | --- | --- | --- | --- | --- |
| Daily / none | 2018-06 to 2026-11 | 35 | 42.9% | +10.0 | +3.0 | +7.0 |
| H1 / Daily | 2024-11 to 2026-11 | 92 | 35.9% | +6.2 | -7.3 | +13.5 |
| H1 / none | 2024-11 to 2026-11 | 141 | 34.0% | +0.6 | -7.4 | +8.0 |
| M30 / H1 | 2026-05 to 2026-11 | 29 | 37.9% | +4.0 | +8.0 | -4.0 |
| M15 / H1 | 2026-05 to 2026-11 | 103 | 29.1% | -13.0 | -2.0 | -11.0 |
| M5 / H1 | 2026-05 to 2026-11 | 307 | 35.2% | +18.8 | +20.0 | -1.2 |

Long vs short with default parameters:

| TF / filter | Long | Short |
| --- | --- | --- |
| Daily / none | 25 trades, +8.0R | 10 trades, +2.0R |
| H1 / Daily | 39 trades, +15.0R | 53 trades, -8.8R |
| H1 / none | 79 trades, +9.5R | 62 trades, -8.9R |

## Parameter grid (960 combinations, first 70% in-sample)

| TF / filter | Positive in-sample | Positive out-of-sample | Positive in both | Mean out-of-sample |
| --- | --- | --- | --- | --- |
| Daily / none | 84% | 81% | 71% | +4.0R |
| H1 / Daily | 63% | 73% | 44% | +5.5R |
| H1 / none | 61% | 43% | 26% | -0.8R |
| M15 / H1 | 64% | 15% | 10% | -7.4R |
| M5 / H1 | 63% | 54% | 29% | +1.3R |

On M15, every one of the top 10 in-sample combinations made a loss or broke even out-of-sample:
choosing parameters by past results there is overfitting.

## Limits of this test

- Only one market and one regime: gold rose strongly over the whole Daily and H1 sample.
- Small samples: Daily out-of-sample has 11 trades; M5/M15/M30 cover only 6 months.
- Scale: +10R over 8 years at 1% risk per trade is about +10%, far below holding gold.

## Reproduce

```bash
go run ./cmd/backtest -symbol XAUUSDm -tf Daily -split 0.7
go run ./cmd/backtest -symbol XAUUSDm -tf H1 -filter Daily -split 0.7
go run ./cmd/backtest -symbol XAUUSDm -tf H1 -filter Daily -split 0.7 -grid
```
