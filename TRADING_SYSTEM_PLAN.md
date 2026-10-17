# Trading System Plan

เอกสารนี้ไม่ commit ให้ — ผู้ใช้จัดการเอง

## กติกาการทำงาน

- step-by-step ทีละขั้นเท่านั้น
- AI ไม่เขียนโค้ด logic เองจนกว่าจะอนุมัติแต่ละขั้นอย่างชัดเจน
- ห้าม `git commit` / ห้าม `git push` ให้ ไม่ว่ากรณีไหน จนกว่าจะสั่งตรงๆ ทีละครั้ง
- แผนงาน/มติ/ความคืบหน้าทุกอย่างบันทึกไว้ในไฟล์นี้
- **คำสั่งสั้นๆ แบบ "เขียนมาเริ่มกัน" ไม่นับเป็นการอนุมัติให้เขียนโค้ด** (2026-10-12 — เคยเข้าใจผิดว่าเป็นการอนุมัติ เขียนไปแล้ว 7 ไฟล์ + แก้ docker-compose.yml ก่อนจะถูกสั่งถอยกลับ) ต้องถามตรงๆ ให้ชัดว่า "เริ่มเขียนโค้ดได้เลยไหม" แล้วรอคำตอบที่ชัดเจนจริงๆ ก่อนแตะไฟล์ไหนเลย

## สถานะปัจจุบัน

- `cmd/app/main.go` — HTTP server ขั้นต่ำ (stdlib `net/http` ล้วน ไม่พึ่ง dependency ภายนอก) มีแค่ `GET /health` → `{"status":"ok"}`
- โครง Hexagonal ว่างเปล่า (แค่ `package xxx` ไม่มี logic ข้างใน):
  - `internal/core/domain/`
  - `internal/core/ports/`
  - `internal/core/services/`
  - `internal/adapters/config/`
  - `internal/adapters/database/`
  - `internal/adapters/http/v1/`
  - `internal/adapters/logging/`
  - `internal/adapters/mt5/`
- `go.mod` ไม่มี dependency ภายนอกเลย

## เป้าหมายอ้างอิง — องค์ประกอบของระบบเทรดที่ดี

### กลุ่ม 1 — ขาดไม่ได้เลย

1. Market data connection — เชื่อมต่อราคาตลาดแบบเสถียร reconnect เองได้
2. Order execution — ส่ง/แก้/ปิด order ได้แน่นอน รู้ผลจริงจาก broker
3. Risk management — position sizing, SL/TP บังคับทุกไม้, circuit breaker, จำกัด position เปิดพร้อมกัน
4. State persistence — รอดจาก restart/crash
5. Logging พื้นฐาน

### กลุ่ม 2 — ต้องมีก่อนเงินจริง

6. Strategy ที่พิสูจน์แล้วว่ามี edge จริง (backtest บนข้อมูลสมจริง)
7. Reconciliation กับ broker

### ข้อ 6 — ตัดสินใจแล้ว: Strategy ที่เลือก (เพื่อจำกัด scope)

**SuperTrend + SMA confirmation** (แรงบันดาลใจจาก Mutanabby_AI Pine Script ที่รีวิวกันไปแล้ว) — เอาเฉพาะ**กลไก** ไม่เอา cosmetic/marketing ส่วนที่ไม่เกี่ยวกับ signal จริง:

- **Entry**: SuperTrend (ATR-based) cross + ยืนยันทิศทางด้วย SMA confirmation
- **Stop loss**: ATR-based distance
- **Take profit**: R-multiple จาก stop distance (เริ่มจาก TP เดียวก่อน ไม่ใช่ 3 เป้าพร้อมกัน — ตัดสินใจอัตราส่วนตอนออกแบบ Step 3 จริง)
- **ไม่เอา**: RSI bar coloring (cosmetic), "Strong Buy" label (ไม่มีผลต่อ position size จริงในต้นฉบับ), external library import

**กติกาสำคัญที่ยังไม่เปลี่ยน**: นี่คือ**ตัวเลือกเพื่อจำกัด scope** ไม่ใช่การยืนยันว่ามี edge จริง — ต้องสร้าง backtest engine ใหม่ (Step 3+) แล้วพิสูจน์ด้วยข้อมูลจริงก่อนถึงจะเทรดเงินจริงได้ ตามกติกาเดิมทุกประการ

### กลุ่ม 3 — เพิ่มทีหลังได้

8. Order flow / microstructure signals
9. Multi-timeframe confirmation
10. Win-rate gating
11. Multi-strategy


## Target Architecture (ตกลงแล้ว)

**2 service** แยกกันด้วยเหตุผล **failure isolation** (MT5 connection หลุดไม่ควรทำให้ risk/trading engine ตาย) ไม่ใช่เหตุผลเรื่อง scale:

1. **MT5 Gateway service** — คุยกับ MT5 EA โดยตรง (TCP), รับ tick/trade event, ส่ง order command ออกไป ไม่มี business logic ข้างใน
2. **Trading Engine service** — risk management, strategy, execution decision, persistence, HTTP API/dashboard

**การสื่อสารระหว่าง 2 service**: gRPC (แทนที่ custom TCP/JSON protocol เดิม)

**Tick distribution**: message queue (**Kafka** — ตามที่ตั้งใจจะโชว์ใน CV โดยเฉพาะ, NATS เป็นทางเลือกสำรองถ้า deploy ยากเกินไปสำหรับ dev เครื่องเดียว) — Gateway publish tick เข้า topic เดียว มีหลาย consumer อิสระกัน: persistence (บันทึก tick_history), Trading Engine (คำนวณ signal), Dashboard (แสดงราคาสด) — เหตุผลรองรับจริง ไม่ใช่ยัดเข้าไปเฉยๆ (ดูหัวข้อ "เป้าหมายเพิ่มเติม" ด้านบน)

**ต้นทุนที่รู้ไว้แล้ว**: เพิ่ม failure mode ระหว่าง service (เช่น gRPC call timeout ตอนจะส่ง order) ต้องมี retry/idempotency handling เพิ่มจาก monolith — เป็น trade-off ที่แลกกับคุณค่า CV โดยตั้งใจ

## หลักการออกแบบกันงานซ้ำซ้อน (ใช้ตอนออกแบบ Step 3 เป็นหลัก)

ของในกลุ่ม 3 (signal combination, correlation risk, TWAP/VWAP, win-rate gating, research env แยก) **ไม่ implement ตอนนี้** — แต่ 4 จุดนี้ต้องออกแบบ "รูปร่าง" ให้ถูกตั้งแต่แรก เพื่อให้เพิ่มของพวกนั้นทีหลังเป็นการ "เสียบเพิ่ม" ไม่ใช่ "รื้อสร้างใหม่":

1. **Strategy ต้องอยู่หลัง interface** (เช่น `OnTick(...) *Signal`) แม้มีแค่ 1 strategy ตอนนี้ — เพิ่ม strategy ทีหลัง = เพิ่ม struct ใหม่ ไม่แก้ของเดิม
2. **Risk/position tracking ต้อง key ด้วย symbol ตั้งแต่แรก** (`map[symbol]...` ไม่ใช่ตัวแปรเดี่ยว) แม้เทรด symbol เดียวตอนนี้
3. **Order pipeline ต้องเป็น gate ต่อกันเป็นสเตจ** (signal → sizing → gate1 → gate2 → ... → dispatch) ไม่ใช่ฟังก์ชันเดียวเช็ครวด — เพิ่ม gate ใหม่ทีหลัง = เสียบเข้า pipeline
4. **Strategy/domain logic อยู่ใน `internal/core` แยกขาดจาก transport (MT5 Gateway)** — ตรงกับ Step 2 อยู่แล้ว ทำให้ backtest tool ทีหลัง import `internal/core` ใช้ตรงๆได้ ไม่ duplicate logic

ข้อ 1-3 ใช้ตอนออกแบบ Step 3 (Trading Engine) — ข้อ 4 ใช้ตั้งแต่ Step 2 นี้เลย

## ตัดสินใจ: Signal-only mode ก่อน auto-trading

**Auto-trading (ส่ง order จริงเข้า MT5) ปิดไว้ก่อนโดย default** จนกว่าจะสั่งเปิดเองชัดเจน — ลำดับ priority เปลี่ยนเป็น: สร้าง strategy + dashboard ให้เห็น signal ชัดเจนก่อน ค่อยเปิด auto-execution ทีหลัง (ไม่ใช่เปิดอัตโนมัติพอโค้ดเสร็จ)

**Peak Profit / entry-SL-TP บน dashboard ยังโชว์ได้แม้ auto-trading ปิดอยู่** — จำลอง (simulate) trade lifecycle จาก signal ที่เกิดขึ้นจริง (entry → track จนกว่าจะถึง SL/TP ที่คำนวณไว้) แบบ paper trade ไม่ส่ง order จริงเข้า MT5 — ปลอดภัย 100% เห็นภาพครบตามที่ต้องการ

**Build order ที่ปรับใหม่**:
- Step 2 (MT5 Gateway + ราคาสด) — ไม่เปลี่ยน
- Step 3 (Strategy + Multi-TF filter + Dashboard เต็มรูปแบบ, signal-only/paper mode) — คำนวณ signal, จำลอง entry/SL/TP/peak-profit โชว์บน dashboard **ไม่ส่ง order จริง**
- Step 4+ (เปิด auto-trading จริง) — ทำทีหลัง แยกขั้นตอนชัดเจน หลังดู dashboard ไปสักพักแล้วเชื่อใจ

## คำถามเปิด (ยังไม่ตัดสินใจ)

**SuperTrend+SMA กับ Multi-Timeframe Trend Indicator สัมพันธ์กันแบบไหน**:
- (ก) 1 strategy (SuperTrend+SMA) + 1 filter ยืนยันทิศทาง (Multi-TF Trend) — ยังเป็น "1 strategy" ไม่ขัดกับที่ตกลงจำกัด scope ไว้ — **แนวโน้มเลือกทางนี้**
- (ข) 2 strategy อิสระแยกกันเทรด — ขัดกับการจำกัด scope ที่ตกลงไว้ ต้องคุยขอบเขต risk management ใหม่ถ้าจะทำแบบนี้

รอตัดสินใจตอนออกแบบ Step 3

## Step log

### Step 1 — โครงเปล่า + /health (เสร็จแล้ว)

รายละเอียดตาม "สถานะปัจจุบัน" ด้านบน ยืนยันแล้วว่า `go build`/`go vet` ผ่าน และ `/health` ตอบถูกต้องจริง

### Step 2 — MT5 Gateway service (รอ confirm เริ่มเขียนโค้ด)

**ขอบเขต**:
- ต่อ MT5 EA ผ่าน TCP (เหมือนของเดิมที่เคยมี — `alphago_mt5.mq` ยังอยู่ ไม่ต้องแก้ฝั่ง EA ตอนนี้)
- รับ tick แบบ real-time, รวมเป็นแท่งเทียน M1 เอง
- Publish tick เข้า Kafka topic
- gRPC server ฝั่ง Gateway ไว้ให้ Trading Engine เรียกสั่ง order (ยังไม่ต้องมี Trading Engine ฝั่งตรงข้ามในขั้นนี้ แค่เปิด endpoint ไว้)
- Dashboard ขั้นต่ำ: candlestick chart แสดงราคาสดจาก tick ที่ publish (ต่อยอดจาก Kafka topic หรือ gRPC stream ตรงจาก Gateway ก็ได้ — ตัดสินใจตอนเขียนจริง)

#### มติที่ตกลงใน session 2026-10-12 → 2026-10-17

**เป้าหมายโปรเจกต์**: ทำเป็นผลงาน (portfolio/CV) — ไม่ได้หวังกำไรจากการเทรด

**ข้อมูลแท่งเทียน**
- แท่งเทียนเก็บเป็นไฟล์ CSV **ไม่ลง DB เด็ดขาด**
- โครงโฟลเดอร์: `data/candle/<symbol>/tf_m_<นาที>/`
  - tf_m_1, tf_m_5, tf_m_15, tf_m_30 → `yyyy-mm-dd.csv` (รายวัน)
  - tf_m_60, tf_m_240 → `yyyy-mm.csv` (รายเดือน)
  - tf_m_1440 → `yyyy.csv` (รายปี)
- เนื้อในไฟล์: `time,open,high,low,close,tick_volume`
- ขึ้นวัน/เดือน/ปีใหม่ = ไฟล์ใหม่ ไฟล์เก่าต้องอยู่ครบ — ระบบไม่ลบไฟล์เองเด็ดขาด
- เขียนทับแท่งเดิม = merge (อ่านทั้งไฟล์ → แทนแถวเวลาเดียวกัน → เรียง → เขียนไฟล์ชั่วคราวแล้ว rename)
- ทุก TF: M1, M5, M15, M30, H1, H4, D1 — ความลึกย้อนหลังตั้งแยกต่อ TF
- แท่งที่ปิดแล้วใช้ของ broker (จาก RATES) เป็นข้อมูลจริง, ตัวรวมแท่งของ Gateway ใช้แค่แท่งที่กำลังก่อตัว
- tick ดิบไม่เก็บ (script export เก็บไว้เผื่อทำ backtest วันหน้า — tick ควรเป็นไฟล์ ไม่ใช่ DB)
- `data/` อยู่ใน .gitignore

**การเติม gap (อัตโนมัติ ไม่ manual)**
- from = แท่งล่าสุดในไฟล์ (ดึงซ้ำแท่งนั้นด้วย) / ถ้าไม่มีไฟล์ = ปัจจุบัน − N วันของ TF นั้น
- to = นาทีที่แล้ว (ไม่เอาแท่งที่ยังไม่ปิด)
- เติมจากเก่าไปใหม่, ระหว่างเติมยังไม่บันทึกแท่งสด, ไม่ไล่นับนาทีที่หายในอดีต (วันหยุด/ตลาดปิด = ปกติ)
- ทำตอน start, ทุกครั้งที่ต่อ EA ใหม่, และทุกครั้งที่แท่งปิด

**Postgres**: เก็บค่าระบบเท่านั้น / ต่อ DB ไม่ได้ = Gateway ไม่ยอม start

**ยกเลิก**: replay mode จาก CSV, ค่าระบบ data_source (csv/mt5) — ไม่ใช้แล้ว

**EA**: แก้ได้ (ข้อความ "ไม่ต้องแก้ EA" เดิมหมายถึงแค่ขั้นนั้น ไม่ใช่กฎห้าม)

**ข้อเท็จจริงจาก broker**
- symbol คือ `XAUUSDm` (ไม่ใช่ XAUUSD)
- เวลา server ของ broker = UTC+0 (เวลาไทย − 7 ชม.)

#### เสร็จแล้ว
- [x] 2a — TCP stream client + reconnect + ตัวรวมแท่ง M1 + /health (อยู่ในโปรเจกต์ ผู้ใช้แก้บางไฟล์เอง)
- [x] Script `AlphaGo_TickExport.mq5` — export tick ย้อนหลังเป็น CSV (เก็บไว้ใช้ backtest)
- [x] EA v1.30 — คำสั่ง RATES (ทุก TF, ส่งชุดละ 200 แท่ง, has_more/next_from, server_time),
      tick ส่ง time_msc + เวลาของ tick จริง, SendAll() ส่งครบทุก byte — ทดสอบผ่านแล้ว (2026-10-16)

#### ลำดับงานถัดไป
1. [ ] Command client ฝั่ง Gateway — ต่อ 5555 ค้างไว้, ทีละคำสั่ง, timeout, reconnect, วนขอ RATES จนครบ, retry ตอน CopyRates failed
2. [ ] ตัวเขียน/อ่านไฟล์แท่งเทียน (merge + atomic write + เทสต์ข้ามวันไฟล์เก่าไม่หาย)
3. [ ] ตัวรวมแท่งหลาย TF + ตัวเติม gap
4. [ ] ปรับ stream parser ให้ใช้ time_msc
5. [ ] Kafka (franz-go) + KRaft ใน docker-compose
6. [ ] gRPC server stub
7. [ ] Dashboard: แท่งย้อนหลังจากไฟล์ + แท่งสดผ่าน Kafka → SSE

#### ยังไม่ตัดสินใจ
- ชื่อโฟลเดอร์ symbol: `xauusdm` (ตาม broker — แนะนำ) หรือ `xauusd`
- ค่า N (ย้อนหลังกี่วัน) ของแต่ละ TF — ร่างไว้: M1 30 วัน, M5–M30 6 เดือน, H1–H4 2 ปี, D1 10 ปี
- ค่าระบบใน Postgres ตอนนี้จะมีอะไรบ้าง (data_source ถูกยกเลิกแล้ว)

#### งานค้างฝั่ง MT5 (ผู้ใช้ทำ)
- จัดชื่อไฟล์ใหม่: โค้ด EA → `Experts/AlphaGo_Listener.mq5`, script → `Scripts/AlphaGo_TickExport.mq5`
  (ตอนนี้ EA v1.30 รันอยู่ในไฟล์ชื่อ alphago_tickexport)
- อัปเดต `alphago_mt5.mq` ในโปรเจกต์เป็น v1.30

#### เพิ่มเติม (2026-10-17)

**ทดสอบช่วงวันหยุด (เสาร์-อาทิตย์)**
- forex/ทองไม่วิ่ง → ย้าย EA ไปเกาะชาร์ต `BTCUSDm` เพื่อให้มี tick สดไว้ทดสอบ วันธรรมดาย้ายกลับ XAUUSDm
- ห้าม hardcode symbol ในโค้ด → ตั้งผ่านค่าระบบ เช่น `SYMBOLS=XAUUSDm` / `SYMBOLS=BTCUSDm`
- RATES ใช้ได้แม้ตลาดปิด (อ่านจากประวัติ) → ทดสอบ command client / เติม gap ได้ทุกวัน
- EA ส่ง tick ได้ทีละ symbol (เฉพาะชาร์ตที่เกาะ); EA 2 ตัวไม่ได้เพราะ port 5555/5556 ชน

**Correlation (ผู้ใช้สนใจ — ตัดสินใจว่าจะทำ)**
- คู่ที่สนใจ: XAUUSD กับ DXY, USOIL, USTECH, US500
- ทำจากแท่งย้อนหลัง (RATES ขอ symbol ไหนก็ได้) → ไม่ต้องแก้ EA
  ตัวเติม gap วนตามรายการ symbol → ได้ data/candle/<symbol>/... ครบทุกตัว
- แบบสด (ทุก symbol พร้อมกัน) ต้องแก้ EA ให้อ่านหลาย symbol จาก timer — ทำทีหลังถ้าต้องการ
- หลักการคำนวณที่ต้องทำให้ถูก:
  - ใช้ผลตอบแทน (% เปลี่ยน) ไม่ใช่ราคา
  - จับคู่แท่งเฉพาะเวลาที่มีข้อมูลทั้งสองฝั่ง (ตลาดเปิด-ปิดไม่ตรงกัน)
  - แสดงเป็น rolling correlation ตามเวลา ไม่ใช่ตัวเลขเดียว
- ลำดับ: (1) Step 2 แกนหลักเสร็จก่อน โดยรองรับรายการ symbol ตั้งแต่แรก
          (2) correlation จากข้อมูลย้อนหลัง + หน้าใน dashboard
          (3) EA หลาย symbol (ถ้าต้องการแบบสด)
- เดิมอยู่กลุ่ม 3 → เลื่อนขึ้นมาเป็นงานถัดจาก Step 2 แกนหลัก

**ยังไม่ตัดสินใจ / รอข้อมูล**
- รายชื่อ symbol จริงใน Market Watch (DXY มีไหม? ชื่ออะไร — น่าจะลงท้าย m)
  ถ้าไม่มี DXY → ใช้ EURUSD แทนได้

**ยังไม่ทำ step นี้**: risk management, strategy/signal, order execution จริง, persistence ของ trade/account — รอ Step 3 (Trading Engine)

_(รอคำยืนยันให้เริ่มเขียนโค้ดจริง — ตามกติกาที่ตั้งไว้)_

