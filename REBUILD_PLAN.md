# Rebuild Plan

เอกสารนี้ไม่ commit ให้ — ผู้ใช้จัดการเอง (ตามรูปแบบเดียวกับ HANDOFF เดิม)

วันที่เริ่ม: 2026-10-10

## กติกาการทำงานใหม่ (ตกลงกันไว้วันนี้)

1. ทำงานแบบ step-by-step ทีละขั้นเท่านั้น
2. AI จะไม่ลงมือเขียนโค้ดเองจนกว่าจะได้รับการอนุมัติในแต่ละขั้นตอนอย่างชัดเจน
3. แผนงาน/มติ/ความคืบหน้าทุกอย่างบันทึกไว้ในไฟล์นี้ ไม่ใช่แค่ในแชท
4. ก่อนเขียนโค้ดจริงของแต่ละ step — สรุปใน .md นี้ก่อนว่าจะทำอะไร เพื่อให้อนุมัติก่อนลงมือ

## สถานะปัจจุบันของระบบ (ข้อเท็จจริงล้วน ไม่ตีความ)

อิงจาก commit ล่าสุด `4ff3e16` branch `feat/live-trading-hardening`

### เปิดใช้งานจริงใน live ตอนนี้

- Multi-Timeframe Filter (`mtfFilter`) — เปิดเสมอ (`MultiTimeframeMinSlope=0.001`) เช็ค Daily/H4 bias ก่อนยืนยันด้วย M30/M15 ก่อนปล่อย signal ทุกตัว
- Win-Rate Gate — เปิดเสมอ บล็อก strategy ที่มี ≥30 เทรดสะสมจาก `trade_outcomes` และ win rate <50%
- SL/TP sizing — ใช้ `SizingVolatility` (StdDev ของ M15 window) × `VolatilityMultiplier` เป็นตัวขับหลัก (ค่า default ตั้งแต่ก่อนเซสชันนี้)

### สร้างเสร็จแล้วแต่ "ปิดอยู่" โดยตั้งใจ (inert by design)

- ATR-based sizing — `UseATRForSizing=false` by default ต้องเปิดเองผ่าน `PUT /api/v1/risk/config`
- Liquidity Confluence Filter (`lcFilter`) — โค้ดเขียนเสร็จ มี test ครบ แต่ **ไม่ได้ถูกเรียกใช้งานเลยใน `setupQuantEngine()`** (comment ปิดไว้) เพราะยังไม่เคยดูค่า CVD/POC จริงมาตั้ง threshold

### คำนวณโชว์เฉยๆ ไม่ได้ gate อะไร

- CVD, Volume Profile/POC, ATR value, VelocityTrendSlope, VolatilityTrendSlope — โชว์ผ่าน `/api/v1/status` ทั้งหมด

### เครื่องมือเสริมแยกต่างหาก (ไม่กระทบ live service)

- `cmd/backtest` — backtest engine รันจาก CSV ไฟล์
- `cmd/import-tick-history` — import CSV เข้า `tick_history` table

## คำถามที่ต้องตอบก่อนเริ่ม Step 1

(ตอบในแชทได้เลย แล้วจะสรุปกลับมาใส่ไว้ตรงนี้)

1. **ผลลัพธ์ที่ "ไม่ตรงกับที่คาดหวัง" คืออะไรโดยเฉพาะเจาะจง?** — ออเดอร์เข้าแบบไหนที่ไม่ควรเข้า / SL-TP ผิดยังไง / หรืออย่างอื่น
2. **"ลื้อระบบ" หมายถึงอะไรกันแน่?**
   - (ก) รื้อกลับไปจุดใดจุดหนึ่งที่เคยนิ่งกว่านี้ (เช่น ก่อนเริ่มทำ ATR/CVD/MTF/WinRateGate ทั้งหมด)
   - (ข) ทบทวนของที่มีอยู่ทีละตัว แล้วตัดบางส่วนทิ้งที่ไม่จำเป็น
   - (ค) ออกแบบใหม่ทั้งหมดตั้งแต่ต้น
3. **เป้าหมายของระบบตอนนี้ที่ชัดที่สุด 1 ข้อ** (ไม่ใช่ list ยาว) — เพื่อให้ step 1 เริ่มถูกจุด

## กติกาเพิ่มเติม (2026-10-10)

- ห้าม `git commit` ให้ — ไม่ว่ากรณีไหน จนกว่าจะสั่งตรงๆ ทีละครั้ง
- ห้าม `git push` เอง
- อนุญาตให้ลบ logic code ทั้งหมดได้ — แต่ต้องสรุป scope ให้ชัดก่อนลงมือ (ดู Step 1 ด้านล่าง)

## Step 1 (เสนอ — รอ confirm ก่อนลงมือ)

**ข้อเสนอ**: ลบ "logic code" ทั้งหมด = โค้ดฝั่ง Brain/Strategy/Risk (ของที่คิด/ตัดสินใจ) เก็บไว้แค่ Hand/Transport layer (ของที่ต่อ MT5/DB/HTTP ได้ แต่ยังไม่มี logic ตัดสินใจอะไรข้างใน)

**จะลบ:**
- `internal/core/domain/` ทั้งโฟลเดอร์ (TickMetrics, RiskConfig, OrderSignal, MultiTimeframeState, StrategyTag, MarketSession ฯลฯ)
- `internal/core/services/` ทั้งโฟลเดอร์ (strategy/, risk/, pipeline/, risk_config_service.go, trade_service.go)
- `internal/core/ports/` ทั้งโฟลเดอร์
- `internal/backtest/` ทั้งโฟลเดอร์
- `cmd/backtest/`, `cmd/import-tick-history/` (พึ่ง internal/backtest)
- โค้ดใน `cmd/app/main.go` ส่วนที่ wire ของข้างบนเข้าด้วยกัน (quant engine, risk manager, execution router ฯลฯ)

**จะเก็บไว้ (ไม่แตะ):**
- `internal/adapters/mt5/` — ต่อ MT5 (TCP/stream)
- `internal/adapters/database/` — ต่อ Postgres (แต่ method ส่วนใหญ่จะ compile ไม่ผ่านเพราะพึ่ง type จาก core ที่ลบไป ต้องตัดตามไปด้วยหรือ stub ไว้)
- `internal/adapters/http/v1/` — เช่นเดียวกัน จะพังเพราะพึ่ง core — ต้องตัดสินใจว่าตัดทิ้งไปก่อนหรือ stub
- `internal/adapters/config/`, `internal/adapters/logging/`
- `alphago_mt5.mq` (EA ฝั่ง MT5 — คนละภาษา ไม่ใช่ Go logic)

**ผลที่จะเกิดขึ้นทันที**: `go build ./...` จะ**พังทั้งโปรเจกต์** หลังลบ (เพราะ adapters ยัง reference type ที่หายไป) — ต้องตามไปตัด/stub `internal/adapters/database`, `internal/adapters/http/v1`, และ `cmd/app/main.go` ให้ compile ผ่านอีกครั้งในขั้นต่อไป ไม่ใช่จบใน step เดียว

**คำถามก่อนลงมือ**: ตกลงตาม scope นี้ไหม หรืออยากให้เก็บบางส่วนไว้ (เช่น เก็บ `internal/backtest` ไว้ก่อนเพราะเพิ่งทำเสร็จและ verify กับ real data ไปแล้ว)?

## Step log

### Step 1 — ลบ logic code ทั้งหมด (เสร็จแล้ว)

ลบ `internal/core/`, `internal/backtest/`, `internal/adapters/`, `cmd/backtest/`, `cmd/import-tick-history/` ทั้งหมด เหลือ `cmd/app/main.go` ไฟล์เดียว เขียนใหม่เป็น stdlib `net/http` ล้วน มีแค่ `GET /health` → `{"status":"ok"}` ไม่พึ่ง package ไหนในโปรเจกต์เลย `go mod tidy` แล้ว — `go.mod` เหลือ 3 บรรทัด ไม่มี dependency ภายนอก

ยืนยันแล้วว่ารันได้จริง (`go build` ผ่าน, ทดสอบ `/health` ได้ `200 {"status":"ok"}` บน port อื่นเพราะ port 8080 บนเครื่องนี้ถูก Apache ตัวอื่นใช้อยู่ก่อน — ยังไม่ได้แก้ port ใน `main.go` รอคำสั่ง)

ยังไม่แตะ: `alphago_mt5.mq`, `docker-compose.yml`, `app.env`, `REBUILD_PLAN.md`

### Step 2 — วางโครง Hexagonal เปล่า (เสร็จแล้ว)

สร้างโฟลเดอร์ + `doc.go` (แค่ `package xxx` ไม่มีโค้ด ไม่มี comment) รอไว้ 8 จุด:

- `internal/core/domain/`
- `internal/core/ports/`
- `internal/core/services/`
- `internal/adapters/config/`
- `internal/adapters/database/`
- `internal/adapters/http/v1/`
- `internal/adapters/logging/`
- `internal/adapters/mt5/`

ยังไม่มี logic ข้างในเลยสักไฟล์ — `cmd/app/main.go` ยังไม่ import อะไรจากโฟลเดอร์พวกนี้ ยืนยันแล้วว่า `go build`/`go vet` ผ่านทั้งโปรเจกต์

### Step 3 — รอคำสั่งถัดไป

_(ยังไม่เริ่ม)_
