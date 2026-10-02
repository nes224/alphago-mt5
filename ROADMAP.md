# 🚀 AlphaGo - Hexagonal Architecture Checklist & Roadmap

โปรเจกต์ระบบเทรดอัตโนมัติ (Algorithmic Trading & Quant Engine) ด้วย Go, MT5 (ZeroMQ/TCP Socket), Hexagonal Architecture และ Claude AI

---

🟢 Phase 1: The Hand (Go MT5 Lib & Transport Layer)
[x] Project & Architecture Initialization
[x] จัดโครงสร้างโฟลเดอร์ตาม Hexagonal Architecture (internal/core/domain, internal/core/ports, internal/adapters, cmd/app)
[x] ตั้งค่า Config Adapter ดึงค่าด้วย Viper / Environment Variables
[x] ตั้งค่า Makefile และ .air.toml สำหรับ Hot Reloading และ Developer Experience
[x] บันทึก Version Control ผ่าน Git Commits
[x] Core Domain & Use Cases
[x] นิยาม Domain Models ใน internal/core/domain (TradeRequest, TradeResponse)
[x] เขียน TradeService ใน internal/core/services สำหรับสั่ง Execute Trade
[x] อิมพลีเมนต์ Validation Rules (Symbol, Volume > 0, Action) และ Risk Guard Logic (Max Lot Limit)
[x] เขียน Unit Test & Integration Test ฝั่ง Go ครอบคลุมทุก Edge Case และผ่าน 100%
[x] Secondary Adapter (MT5 Listener - MQL5 Side)
[x] เขียน MQL5 Expert Advisor (EA) ทำหน้าที่เป็น TCP Socket Listener (Port 5555)
[x] จัดการ Parsing JSON Request (action, symbol, type, volume) และส่ง OrderSend()
[x] ส่ง JSON Response กลับหา Go Client
[x] Secondary Adapter (MT5 Client - Go Side)
[x] สร้าง internal/adapters/mt5/tcp_adapter.go อิมพลีเมนต์ ports.MT5Port
[x] เขียน Timeout, Auto-reconnect และ Error Handling สำหรับ Socket Connection

---

## 🧠 Phase 2: The Brain (Go Quant Engine & Strategy)
- [x] **Real-time Market Data Stream**
  - [x] เพิ่ม Socket Port (เช่น 5556) สำหรับ PUB/SUB หรือ Stream Price จาก MT5
  - [x] สร้าง PureQuantEngine ประมวลผล Tick สดในระดับ Sub-millisecond
  - [x] สร้าง Sliding Window In-Memory Buffer สำหรับคำนวณ Z-Score สถิติ   
- [x] Pure Quant & Microstructure Metrics (แทนที่ Indicators เก่า)
  - [x] TickMetrics & ZScoreStrategy (คำนวณ Standard Deviation สวนเข้าหาค่าเฉลี่ย) — **ลบ `zscore_strategy.go` ทิ้งแล้ว** (โค้ดพัง ไม่ implement `ports.QuantStrategy` มาตั้งแต่ต้น ไม่เคยถูกใช้งานจริง — concept มัน cover อยู่แล้วใน `VolumeExpansionStrategy`/`LiquiditySweepStrategy`)
  - [x] Open Interest (OI) & Velocity Engine: เพิ่มฟิลด์ OpenInterest / OIDelta ใน tick.go และ tick_metrics.go — **เปลี่ยนเป็น `VolumeExpansionStrategy` แทน `OIExpansionStrategy` แล้ว** เพราะ Exness/โบรกเกอร์ CFD ไม่ส่ง Open Interest จริงมาให้ (`OIDelta` จะเป็น 0 เสมอ) ใช้ `VolumeDelta`/`VolumeVelocity` จาก tick volume จริงแทน — `minVolumeVelocity` ยังเป็นค่าประมาณ ต้อง tune จากข้อมูลจริง
  - [x] Liquidity Sweep Detection: เขียนโมเดลตรวจจับการกวาด Stop Loss บริเวณ High/Low ย้อนหลัง (`liquidity_sweep_detector.go` + ผูกเป็น `LiquiditySweepStrategy` เข้า QuantEngine แล้ว — fade แบบ Reversal/Stop-Hunt, register อยู่ใน `main.go`)
  - [x] Market Regime Filter: เพิ่ม Trend Slope (least-squares regression บน `Window` เดิม) แยกแยะ Sideway/Trend แล้วปิดสวิตช์ `LiquiditySweepStrategy` เวลา Trend (`window.go:Slope()`, `TickMetrics.TrendSlope`) — **ยังไม่ได้ผ่าน backtest จริง threshold เป็นค่าประมาณ** ยังไม่ได้ทำ Hurst Exponent (เก็บไว้อัปเกรดทีหลังถ้า slope ไม่พอ)
  - [x] Microstructure S&R Window: สร้าง Rolling High/Low N-Ticks Buffer (`LiquiditySweepDetector.HighLow()`)

- [x] Risk & Position Sizing Engine (สำคัญมากก่อนยิงจริง)
  - [x] Dynamic Position Sizing: คำนวณ Lot Size ตาม Risk % ของ Equity และระยะ Stop Loss จากค่า $\sigma$ (StdDev)
  - [x] Risk Guard / Drawdown Control: ล็อคระบบไม่ให้ยิง Order เพิ่มหาก Daily Loss ทะลุ Threshold ที่ตั้งไว้ (ต่อเข้า `ExecutionRouter.Start()` แล้ว — เช็ค `riskGuard.ValidateOrder()` หลังคำนวณ lot size ก่อนส่งเข้า `orderSink`)
  - [x] **Volatility-Adaptive Position Sizing** (เพิ่มจากบทสนทนา 2026-10-01 — เขียนเสร็จแล้ว): แก้ปัญหาที่เจอจริงว่า SL/TP แคบติดพื้น `MinSLDistance` เกือบตลอดเวลา (คำนวณจาก `StdDev` ของ window แค่ 20 tick ซึ่งสั้นเกินไปเทียบกับทองที่วิ่ง 80-200 จุด/วันตอนนี้ ไม่ใช่ 20-50 จุดแบบตอนตั้งค่าเดิม) —
    - เพิ่ม `TickMetrics.SizingVolatility` (StdDev ของ M15 `TimeWindow` แยกต่างหากจาก `mtfFilter` โดยตั้งใจ — คนละหน้าที่กัน) ใช้แทน `StdDev` เดิมในการคำนวณ `slDistance = VolatilityMultiplier × SizingVolatility` — fallback ไป `StdDev` เดิมถ้า M15 ยังไม่มีข้อมูลพอ (เพิ่ง restart)
    - `MinSLDistance`/`MaxSLDistance` เปลี่ยนบทบาทจาก "ตัวขับหลัก" เป็น "ราวกันตก" สองข้างเท่านั้น
    - เพิ่ม `VolatilityMultiplier` (ค่าเริ่มต้น 2.0 — ยังไม่ผ่าน backtest เป็นค่าประมาณ) เป็น field ใหม่ใน `RiskConfig` ปรับผ่าน `PUT /api/v1/risk/config` ได้แบบ live
    - **จุดสำคัญที่แก้พร้อมกัน**: เดิมถ้า lot ที่คำนวณได้ต่ำกว่า `MinLotSize` (บัญชีเล็กเกินไปเทียบกับความผันผวน) ระบบจะปัดขึ้นเป็น `MinLotSize` เงียบๆ ทำให้ความเสี่ยงจริงเกิน `RiskPerTradePercent` ที่ตั้งใจไว้โดยไม่มี warning — ตอนนี้ `CalculateOrder` return error แทน (ข้ามไม้นั้นไปเลย ใช้ error-handling path เดิมของ `ExecutionRouter` ที่มีอยู่แล้ว ไม่ต้องเขียนใหม่)

- [x] Strategy Engine & Execution Wireup (Event Loop)
  - [x] สร้าง QuantStrategy Interface และ SignalChannel()
  - [x] Signal Execution Dispatcher: ดึง OrderSignal จาก SignalChannel() ผ่าน Risk Guard แล้วส่งให้ tcp_client.go ยิง Order เข้า MT5 (alphago_mt5.mq)
  - [x] Main Wireup & Integration Tests: ประกอบระบบทั้งหมดใน cmd/app/main.go และเขียน quant_engine_test.go

- [x] **Market Context Awareness** (เพิ่มเข้ามาจากบทสนทนา 2026-09-30) — ครบทุก sub-item แล้ว (2026-10-01) แต่ threshold หลายตัวยังเป็นค่าประมาณ รอ backtest/tune จากข้อมูลจริง
  - [x] ~~Multi-Timeframe Confirmation: Dual-Window Trend Filter~~ — **SUPERSEDED (2026-09-30)** ลองใช้งานจริงแล้วพัง: นับเป็นจำนวน tick ไม่ใช่เวลาจริง (`LongTermWindowSize=2000` tick อาจกิน 2 นาทีหรือ 20 นาทีก็ได้แล้วแต่ความคึกของตลาด ไม่ตรงกับ timeframe ไหนเลยจริงๆ) กับ threshold ที่เดาไว้ (`0.05`) เข้มกว่าค่าจริงที่วัดได้ (`~0.0009`) เกือบ 60 เท่า จนบล็อก signal ทุกตัว ปิดไปแล้ว (`MinLongTermTrendSlope=0`) ตอนนี้ถูกแทนที่ด้วยข้อถัดไป
  - [x] **Multi-Timeframe Confirmation (แบบใหม่ — Top-Down, ไม่ใช่ Dual-Window)**: ใช้หลัก Top-Down Analysis แทน "โหวต" แบบเดิม — เพราะ timeframe เล็ก (M1/M5) มี noise ตามธรรมชาติ ถ้าบังคับให้ตรงกับ timeframe ใหญ่ตลอดจะเจอปัญหาเข้มเกินไปแบบ Dual-Window ซ้ำ **เขียนเสร็จแล้ว (2026-10-01)** — เพิ่ม `TimeWindow` (duration-based eviction, `time_window.go`) ไม่ต้องสร้าง candle/OHLC builder จริง, `MultiTimeframeFilter` (`multi_timeframe_filter.go`) เก็บ Time-Window 4 ระดับต่อ symbol (Daily, H4, M30, M15) ใช้กฎ **Bias** (Daily+H4 ต้องชี้ทางเดียวกัน) → **Confirmation** (M30 หรือ M15 อันใดอันหนึ่งพอ) ผูกเข้า `QuantEngine` แล้วผ่าน `SetMultiTimeframeFilter()` คั่นก่อนเช็ค cooldown — **fail-open โดยตั้งใจ** (ไม่มีทิศทางชัดเจน/ข้อมูลไม่พอ = ไม่ขวาง) กันไม่ให้พังซ้ำแบบ Dual-Window เดิมถ้า threshold (`MultiTimeframeMinSlope` ใน `main.go`) ยังเดาไม่แม่น — **ยังไม่ได้ผ่าน backtest จริง** threshold เป็นค่าประมาณ ต้องเก็บ slope จริงจากตลาดก่อนค่อยปรับ
  - [x] **Reversal Detection (Leading Indicators)**: slope กลับเครื่องหมายเป็นสัญญาณ lagging (รู้ว่ากลับตัวหลังจากกลับไปแล้ว) — เพิ่ม 2 ตัวที่ใช้วัตถุดิบที่มีอยู่แล้ว (StdDev, PriceVelocity) ไม่ต้องสร้างของใหม่ **เขียนเสร็จแล้ว (2026-10-01)** — เพิ่ม `TickMetrics.VelocityTrendSlope`/`VolatilityTrendSlope` คำนวณจาก Rolling Window ของ PriceVelocity/StdDev เอง (ไม่ใช่ราคา) เปิดเผยผ่าน `/api/v1/status` — **เป็นแค่ metric ให้ดูเฉยๆ ยังไม่ได้เอาไปเป็นเงื่อนไขกรอง signal ใน strategy ไหนโดยตรง** (เหมือน DailyOpen/High/Low ตอนเพิ่มเข้ามาครั้งแรก):
    - **Momentum Deceleration**: เก็บ trend ของ `PriceVelocity` เอง (อนุพันธ์อันดับ 2) — velocity ยังเป็นบวกอยู่แต่ค่าลดลงเรื่อยๆ คือสัญญาณเตือนก่อน slope จะกลับเครื่องหมายจริง
    - **Volatility Contraction**: เก็บ trend ของ `StdDev` เอง — StdDev หดตัวต่อเนื่อง (คล้าย Bollinger Band Squeeze) มักเกิดก่อนการกลับตัว/ระเบิดทิศทางแรงๆ
    - เก็บไว้ทีหลัง (ยังไม่ priority เท่า Multi-TF): Hurst Exponent (วัด trend persistence ทางสถิติ), Divergence (ราคาทำจุดสุดขั้วใหม่แต่ momentum ไม่ทำตาม)
  - [x] Session High/Low & Daily Structure: `QuantEngine.updateDailyRange()` เก็บ `DailyOpen`/`DailyHigh`/`DailyLow` ต่อ symbol ใน `TickMetrics` รีเซ็ตอัตโนมัติข้ามวันตาม timestamp ของ tick — เปิดเผยเป็น metric แล้ว (ดูผ่าน `/api/v1/status`) ยังไม่ได้เอาไปใช้เป็นเงื่อนไขใน strategy ไหนโดยตรง
  - [x] Adaptive Learning (win-rate per strategy): เพิ่มตาราง `trade_outcomes` — main.go จับคู่ ticket ↔ signal Reason (`ticketToReason sync.Map`) ตอน dispatch order แล้ว attribute ผลแพ้/ชนะกลับไปหา `StrategyTag` ตอน trade_closed event มาถึง คำนวณ win-rate ผ่าน `Store.WinRateByStrategy()` ดูได้ที่ `GET /api/v1/strategy-stats` — **ยังไม่ได้ทำ auto-pause/ลด lot อัตโนมัติตาม win-rate** (แค่เก็บข้อมูลให้ดู เป็น decision ที่ต้องคุยเรื่อง threshold/action ก่อนต่อ) ข้อจำกัด: ticketToReason เป็น in-memory ไม่ persist ข้าม restart
  - [x] **Market Session Tracking (Asian/London/US)** (เพิ่มเข้ามาจากบทสนทนา 2026-10-01): เก็บว่าแต่ละ trade เกิดช่วง market session ไหน — คำนวณจาก timestamp ที่มีอยู่แล้วตรงๆ (UTC hour range) ไม่ต้องพึ่งข้อมูลภายนอกใหม่ ต่อยอดจาก Adaptive Learning ด้านบนให้แยก win-rate ได้ตาม session ด้วย ไม่ใช่แค่ตาม strategy **เขียนเสร็จแล้ว (2026-10-01)** — เพิ่ม `domain.MarketSessionFromUTC()` แบ่ง 4 session (`ASIAN`/`LONDON`/`LONDON_NY_OVERLAP`/`NEW_YORK`) ตัดสินใจทั้ง 2 ข้อที่ค้างไว้แล้ว: (1) เก็บที่ระดับ `trade_outcomes` อย่างเดียว (คอลัมน์ `session` ใหม่) ไม่ได้แตะ `signal_records` (2) London/US overlap (13:00-16:59 UTC) นับเป็น session ที่ 4 แยกออกมาเลย — เพิ่ม `Store.WinRateBySession()` โชว์ผ่าน `GET /api/v1/strategy-stats` (key `sessions` คู่กับ `strategies` เดิม) — **สมมติฐานที่ต้องเช็คซ้ำถ้าเปลี่ยน broker**: คำนวณ session จาก `event.Timestamp` ตรงๆ โดยถือว่าเป็น UTC อยู่แล้ว (เช็คจาก timestamp จริงของ Exness demo account ที่ใช้ตอนนี้ว่า server time ตรงกับ UTC+0 พอดี — broker อื่นอาจรันเวลาเซิร์ฟเวอร์เป็นคนละโซน)
  - [x] Order Flow proxy (จากบทสนทนา): ยังไม่ได้เขียน — แนวทางที่ตกลงกันคือ Cumulative Volume Delta (CVD) จาก tick rule (เทียบราคาขึ้น/ลงระหว่าง tick เพื่อเดาฝั่ง buy/sell aggressor) คำนวณจาก `bid`/`ask`/`volume` ที่มีอยู่แล้ว ไม่ต้องพึ่ง DOM/Level2 จาก broker (เช็คแล้วว่า Exness ไม่น่าจะมี DOM ให้ XAUUSDm เหมือนกรณี Open Interest)
  - [x] **Tick History Recording** (เพิ่มเข้ามาจากบทสนทนา 2026-10-01 — เขียนเสร็จแล้ว): รากฐานที่ Order Flow/POC/ATR/backtest ในอนาคตต้องใช้ — เก็บ tick ดิบทุกตัวลงตาราง `tick_history` ใหม่ ผ่าน **background goroutine แยกต่างหาก** (`recordTickHistory`) ไม่ใช่ inline ในจังหวะประมวลผล tick ปกติ — `consumeTicks` ส่งต่อแบบ non-blocking เข้า channel (`TickHistoryChanBuffer=5000`, เต็มแล้วดรอป tick ทิ้งแทนที่จะหน่วง live trading path) เขียนลง DB เป็น batch (ทุก 200 แถวหรือทุก 2 วินาที แล้วแต่อย่างไหนถึงก่อน — ไม่ insert ทีละ tick) — **ยังไม่มี retention policy** ตารางนี้จะโตเรื่อยๆ ไม่มีการลบข้อมูลเก่าทิ้ง ต้องกลับมาทำทีหลัง
  - [x] News/Economic Calendar — track อยู่ใน [Phase 3](#-phase-3-the-intelligence-claude-ai-integration--️-paused) แล้ว (ตอนนี้ pause ไว้)

---

## 🟣 Phase 3: The Intelligence (Claude AI Integration) — ⏸️ PAUSED

> หยุดไว้ก่อนตามคำสั่ง (2026-09-30) — โฟกัส Phase 2 (tune threshold จากข้อมูลจริง) และ Phase 4 (hardening) ก่อน ค่อยกลับมาเปิดทีหลัง

- [ ] **Claude AI Secondary Adapter**
  - [ ] สร้าง `internal/adapters/claude/` เชื่อมต่อ Anthropic Claude API
  - [ ] นิยาม `ports.AIPort` สำหรับวิเคราะห์ Sentiment และ Risk Level
- [ ] **News & Fundamental Analysis**
  - [ ] ดึงข่าวเศรษฐกิจ / Economic Calendar เข้าไปวิเคราะห์ผ่าน Claude Prompt
  - [ ] แปลง AI Output เป็น Struct (`RiskLevel`, `TradeAllowed`, `LotMultiplier`)
- [ ] **Dynamic Risk Guard**
  - [ ] นำค่าจาก Claude AI ไป Override Risk Parameters ของ Quant Engine ก่อนสั่งเทรดจริง

---

## 🧪 Phase 4: Testing & Hardening

- [ ] **Unit & Integration Testing**
  - [x] เขียน Unit Test สำหรับ `TradeService` โดยใช้ Mock `MT5Port` (ไม่ต้องต่อ MT5 จริง) — `trade_service_test.go` มี `MockMT5Adapter` ครบ 5 เคส
  - [x] ทำ Integration Test บน บัญชี Demo ของ MT5 — ทดสอบยิงจริงผ่าน Exness Demo แล้ว (manual, ไม่ใช่ automated test)
- [x] **Logging & Monitoring**
  - [x] ติดตั้ง Structured Logger (`zerolog`) แทน `log.Printf` ทั้งระบบ — `internal/adapters/logging/logging.go` (console pretty-print ตอน dev, JSON ตอน `APP_ENV=production`)
- [x] **Persistence Layer (PostgreSQL + GORM)**: `internal/adapters/database/` — เก็บ Account Balance, RiskGuard state (daily equity/circuit breaker/consecutive losses), และ Signal/Trade history ให้รอดจาก restart แทน in-memory ล้วนๆ (`risk.StateStore`, `pipeline.SignalStore` ports + `docker-compose.yml` สำหรับรัน Postgres local)
- [x] **Trade Outcome Tracking (Win/Loss)**: EA ส่ง `trade_closed` event ผ่าน `OnTradeTransaction()` บน stream socket เดียวกับ tick (แยกด้วย field `"type"`) → `StreamAdapter.SubscribeTradeEvents()` → `RiskGuard.RecordTradeResult()` + อัปเดต `account_state.balance` ใน DB ตาม P/L จริง — **`ACCOUNT_BALANCE` ตอนนี้ dynamic เต็มรูปแบบแล้ว** ขยับตามผลเทรดจริงอัตโนมัติ ไม่ต้องแก้มืออีก (ยังต้องแก้ EA ฝั่ง MQL5 เพิ่มก่อนถึงจะใช้งานได้จริง — ดูโค้ดที่แนบให้)

------------------------------------------------------------------------------------------------------------------------------------------------

Indicator Set ถูกออกแบบมาเพื่อช่วยลดการตัดสินใจจากอารมณ์ โดยใช้โครงสร้างราคา Donchian Channel ร่วมกับค่า Volatility และระบบยืนยันแนวโน้มหลายเงื่อนไข
⸻
1. MTRADER Donchian Counter Trend
จับจังหวะราคาตึงตัว เพื่อหาจุดกลับตัวหรือจุดพักฐาน
อินดิเคเตอร์ตัวนี้เหมาะสำหรับใช้สังเกตจังหวะที่ราคาเคลื่อนออกจากโซนสมดุลมากเกินไป และเริ่มมีโอกาสย่อตัวกลับเข้าสู่กรอบ
ระบบจะแสดงข้อมูลสำคัญจากหลายองค์ประกอบ เช่น
� Donchian Channel
� Moving Average
� Stochastic
� Parabolic SAR
� Standard Deviation
� Envelope
� MACD
� RSI
� CCI
� Volatility Regime
พร้อมแสดงผลเป็น Summary Panel เพื่อช่วยดูว่าฝั่ง Buy หรือ Sell มีน้ำหนักมากกว่า 50% หรือไม่
จุดเด่นคือ ไม่จำเป็นต้องอ่านอินดิเคเตอร์ทุกตัวแยกกันทีละเส้น ระบบจะสรุปภาพรวมให้เห็นทันทีว่า ตลาดเริ่มเอียงไปทางฝั่งใด
เหมาะสำหรับ
� หาจังหวะพักฐานในเทรนด์
� จับจุดกลับตัวระยะสั้น
� วางแผน Take Profit
� ลดการเข้าออเดอร์ในจังหวะที่ราคาตึงเกินไป
Counter Trend ไม่ได้มีไว้สวนเทรนด์แบบสุ่ม แต่ใช้เพื่อมองหาจุดที่ราคาเริ่มมีโอกาสกลับเข้าสู่ Mean อย่างเป็นระบบ
⸻
2. MTRADER Donchian Adaptive Trend Mix
หาแนวโน้มหลักด้วย Donchian + SuperTrend แบบปรับตาม Volatility
ตัวนี้คือแกนหลักสำหรับสาย Trend Following
ระบบผสม Donchian Channel เข้ากับ SuperTrend เพื่อช่วยกรองสัญญาณ Breakout และดูว่าตลาดกำลังมีแรงส่งต่อเนื่องหรือไม่
บนกราฟจะแสดงข้อมูลสำคัญ เช่น
� Trend ปัจจุบัน: Bull Trend หรือ Bear Trend
� Trend Score
� Volatility Regime
� ATR %
� ATR Percentile
� SuperTrend Factor
� Signal Mode
ตัวอย่างเช่น หากระบบแสดงว่า
* Trend = BEAR TREND
* Score = -4
* Signal Mode = Breakout + SuperTrend
หมายความว่าโครงสร้างราคากำลังให้น้ำหนักไปทางขาลง และมีเงื่อนไขยืนยันจากระบบ Breakout ร่วมกับ SuperTrend
เหมาะสำหรับ
� เทรดตามแนวโน้มหลัก
� หาจังหวะ Breakout
� ใช้เป็นตัวกรองก่อนเปิดออเดอร์
� ลดการเข้าเทรดสวนเทรนด์
� ช่วยวางระบบ Trailing Stop ตามโครงสร้างราคา
⸻
3. MTRADER ATR % SD Trend
วัดความผันผวนเป็นเปอร์เซ็นต์ เพื่อเลือกกลยุทธ์ให้ตรงกับตลาด
ราคาแต่ละสินค้ามีระดับราคาไม่เท่ากัน การดู ATR เป็นตัวเลขราคาเพียงอย่างเดียวอาจทำให้เปรียบเทียบได้ยาก
ATR % SD Trend จึงเปลี่ยนค่า ATR ให้อยู่ในรูปแบบ เปอร์เซ็นต์ของราคา เพื่อให้เห็นชัดว่าตลาดกำลังแกว่งแรงหรือเบาเมื่อเทียบกับพฤติกรรมย้อนหลัง
ระบบแสดงข้อมูล เช่น
� ATR % Price
� ATR % Average
� ATR Percentile
� Volatility Regime
� Market Mode
� Lookback 252 Bars
ตัวอย่างเช่น
* ATR % Price = 0.663%
* ATR % Average = 0.808%
* Regime = LOW VOL
* Market Mode = Range / Grid / Mean Reversion
ข้อมูลนี้ช่วยให้แยกออกว่า ช่วงไหนควรรอ Breakout และช่วงไหนเหมาะกับการเล่นรอบในกรอบราคา
เหมาะสำหรับ
� เลือกกลยุทธ์ให้เหมาะกับ Volatility
� วัดระยะ Stop Loss และ Take Profit
� ใช้กรองสัญญาณก่อนเข้าเทรด
� แยกช่วง Trend ออกจากช่วง Range
� ลดการใช้ระยะ Grid หรือ Stop Loss แบบเดิมในทุกสภาวะตลาด
⸻
ใช้งานร่วมกันอย่างไร?
แนวทางใช้งานแบบง่ายมี 3 ขั้นตอน
Step 1: เช็ก Volatility ก่อน
ใช้ ATR % SD Trend เพื่อดูว่า ตลาดอยู่ในช่วง Low Vol, Normal Vol หรือ High Vol
Step 2: เช็กแนวโน้มหลัก
ใช้ Donchian Adaptive Trend Mix เพื่อดูว่าโครงสร้างหลักเป็น Bull Trend หรือ Bear Trend และมี Breakout ยืนยันหรือไม่
Step 3: เลือกจังหวะเข้าให้ได้เปรียบ
ใช้ Donchian Counter Trend เพื่อหาจังหวะย่อ จังหวะพักฐาน หรือจุดที่ราคาเริ่มตึงตัว แทนการไล่เข้าตามราคาแบบไม่มีแผน