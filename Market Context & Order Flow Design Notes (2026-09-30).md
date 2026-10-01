📝 Market Context & Order Flow Design Notes
บันทึกบทสนทนา: 2026-09-30

หมายเหตุ: ไฟล์นี้ยังไม่ commit — ผู้ใช้จะจัดการ commit เอง (ต่างจาก ROADMAP.md ที่มีสรุปแบบย่อของหัวข้อเหล่านี้อยู่แล้ว)

---

## 🔍 คำถามที่ถามไว้: "ในโค้ดตอนนี้มีการคำนวณย้อนหลังกลับไปดูไหม?"

**คำตอบตรงๆ**: มีนิดเดียว ไม่ใช่ของที่คุยกันในเซสชันนี้เลย

สิ่งที่มีอยู่จริงตอนนี้ (`QuantEngine`):
- `DailyOpen`/`DailyHigh`/`DailyLow` — จำได้แค่ 3 ตัวเลข อัปเดตทุก tick ตั้งแต่เปิดวัน รีเซ็ตข้ามวันอัตโนมัติ (`updateDailyRange`)
- `Window` (20 tick) และ `longTermWindows` (2000 tick) — มองย้อนหลังแค่ "N tick ล่าสุด" ไม่ใช่ทั้งวัน
- `LiquiditySweepDetector` — buffer 300 tick ล่าสุด (ไม่ใช่ทั้งวันเช่นกัน)

สิ่งที่**ยังไม่มีในโค้ดเลย** (ทั้งหมดเป็นแค่การคุยออกแบบในเซสชันนี้ ยังไม่ได้เขียนสักบรรทัด):
- Volume Profile / Point of Control (POC)
- Cumulative Volume Delta (CVD) / Order Flow proxy
- Swept-Levels History (จำว่าวันนี้เคยกวาดจุดไหนมาก่อน)
- Multi-Timeframe Bias/Confirmation/M1 Filter (เดิมมี Dual-Window แต่พังแล้ว ถูก supersede ไปแล้ว)
- Reversal leading indicators (Momentum Deceleration, Volatility Contraction, Hurst Exponent, Divergence)
- News Blackout Window

---

## 🎯 เชื่อมกับ Architecture เดิมของโปรเจกต์

เอกสาร `Pure Quantitative Strategy Engine Architecture Document.md` (เขียนไว้ตั้งแต่ต้นโปรเจกต์) ระบุไว้ชัดว่า:
- ตัด Candlestick/OHLC ออกจาก critical path โดยสิ้นเชิง (เพื่อ latency < 1ms)
- เป้าหมายคือ "Order Flow & Tick Velocity Dynamics" และ "Mathematical SMC & Liquidity Sweep"

**สิ่งที่ออกแบบในเซสชันนี้ยังคง "no candlestick" ตามเจตนารมณ์เดิม** — ใช้ time-bounded rolling window บน tick ดิบแทนการสร้าง OHLC candle จริง (ดูหัวข้อ Multi-Timeframe ด้านล่าง) ยกเว้นถ้าจะทำ candlestick pattern recognition (Order Block/ICT) ในอนาคตซึ่งจำเป็นต้องมี OHLC จริง

---

## 1. Multi-Timeframe Entry Flow (Top-Down, ไม่ใช่ Dual-Window)

**ปัญหาของ Dual-Window เดิม** (ดูรายละเอียดใน ROADMAP.md): นับเป็นจำนวน tick ไม่ใช่เวลาจริง + threshold เดาผิดไป 60 เท่า จนบล็อก signal ทุกตัว → ปิดไปแล้ว (`MinLongTermTrendSlope=0`)

**หลักการใหม่**: Top-Down Analysis — timeframe ใหญ่กำหนดทิศทาง (bias), timeframe เล็กจับจังหวะเข้า ไม่ใช่ "โหวต" ให้ทุก timeframe เท่ากันหมด (จะเข้มเกินไปเพราะ M1/M5 มี noise ตามธรรมชาติ)

```
Tick เข้า
  → News Blackout Window?           [ใหม่ ยังไม่สร้าง] ถ้าใช่ → หยุด ไม่เทรด
  → Bias: Daily + H4 ตรงกันไหม?      [ใหม่ ยังไม่สร้าง] ถ้าไม่ตรง → หยุด
  → Confirmation: M30 หรือ M15       [ใหม่ ยังไม่สร้าง] ต้องตรงกับ Bias ถ้าไม่ → หยุด
  → M1 Filter: ไม่สวนทาง signal      [ใหม่ ยังไม่สร้าง] ถ้าสวน → หยุด
  → Entry Trigger (tick-level)       [มีอยู่แล้ว] VolumeExpansionStrategy / LiquiditySweepStrategy
  → Signal Cooldown (30s/symbol)     [มีอยู่แล้ว]
  → RiskManager: Sizing              [มีอยู่แล้ว]
  → RiskGuard: Circuit + Position    [มีอยู่แล้ว]
  → ส่งเข้า MT5 จริง
```

**Implementation note**: ไม่ต้องสร้าง candle/OHLC builder เลย — ใช้ `Window` เดิมที่มีอยู่ แต่เพิ่มโหมด "หมดอายุตามเวลาจริง" (duration-based eviction) แทน/เพิ่มจากโหมด "จำนวน tick" เดิม แล้วสร้าง Time-Window 3-4 ระดับต่อ symbol (Daily, H4, M30/M15, M1) — slope คำนวณตรงจาก tick ในช่วงเวลานั้นๆ

---

## 2. Reversal Detection (Leading Indicators)

Slope กลับเครื่องหมาย = สัญญาณ **lagging** (รู้ตอนกลับตัวไปแล้ว) — เสนอ 2 ตัวที่ใช้วัตถุดิบที่มีอยู่แล้ว (เร่งทำก่อน):

1. **Momentum Deceleration** — เก็บ trend ของ `PriceVelocity` เอง (อนุพันธ์อันดับ 2) velocity ยังบวกแต่ค่าลดลงเรื่อยๆ = สัญญาณเตือนก่อน slope จะกลับจริง
2. **Volatility Contraction** — เก็บ trend ของ `StdDev` เอง (คล้าย Bollinger Band Squeeze) มักเกิดก่อนกลับตัว/ระเบิดทิศทาง

เก็บไว้ทีหลัง (ยังไม่ priority):
3. **Hurst Exponent** — วัด trend persistence ทางสถิติ (H>0.5 persistent, H<0.5 mean-reverting) — เคยพูดถึงตอนทำ Market Regime Filter แล้วว่า "เก็บไว้อัปเกรดทีหลังถ้า slope ไม่พอ"
4. **Divergence** — ราคาทำจุดสุดขั้วใหม่แต่ momentum/volume ไม่ทำตาม

---

## 3. Order Flow & Daily Profile

**คำถามเดิม**: "ทำ order flow ได้ไหมโดยไม่ต้องมี DOM จาก broker" — คำตอบ: ได้ ผ่าน **Tick Rule** (ราคาขึ้น = สันนิษฐานฝั่งซื้อกด, ราคาลง = ฝั่งขายกด) คำนวณจาก `bid`/`ask`/`volume` ที่มีอยู่แล้ว ไม่ต้องพึ่ง Level 2 (เช็คแล้วว่า Exness ไม่น่าจะมี DOM ให้ XAUUSDm เหมือนกรณี Open Interest ที่เจอไปแล้ว)

**คำถามที่สอง**: "ต้องทำ distributed system ไหมเพื่อคำนวณย้อนหลังทั้งวัน" — คำตอบ: **ไม่ต้อง**

เหตุผลเชิงตัวเลข: Gold tick หนาแน่นสุด ~20 tick/วินาที → 1 วัน ~1.7 ล้าน tick เก็บ price+volume ต่อ tick ใช้ memory ไม่กี่ MB คำนวณ incremental (อัปเดตทีละ tick สะสมไปเรื่อยๆ เหมือน `DailyOpen/High/Low` ที่มีอยู่แล้ว) ใช้เวลาระดับ nanosecond ต่อ tick — ไม่ใช่ big-data scale ไม่ต้องมีเครื่องที่สองหรือ distributed worker เลย รันใน `QuantEngine` process เดียวกันได้สบาย

**ออกแบบ**: ขยาย `dailyRange` struct เดิมให้เป็น "Daily Profile" เพิ่ม 3 ส่วน (ทั้งหมด incremental ต่อ tick เหมือน Open/High/Low เดิม):

| ส่วนขยาย | คืออะไร | ใช้ทำอะไร |
|---|---|---|
| **Volume Profile** | สะสม volume ที่แต่ละระดับราคา (bucketed) ตลอดวัน | หา **POC** (ราคาที่มี volume มากสุดวันนี้) — มักเป็นแนวรับ/ต้านจริง |
| **Cumulative Delta** | Tick Rule สะสม buy-volume ลบ sell-volume ทั้งวัน | รู้ว่าทั้งวันฝั่งไหนกดดันมากกว่ากัน (Order Flow proxy) |
| **Swept Levels History** | list ของราคาที่เคยโดน `LiquiditySweepDetector` ตรวจพบว่าถูกกวาดมาแล้ววันนี้ | ให้ `LiquiditySweepStrategy` เช็คว่าจุดนี้ "เคยสำคัญมาก่อน" ไหม ไม่ใช่แค่เทียบ 300 tick ล่าสุด |

**เข้ากับ flow ตรงไหน**: ไม่ใช่ด่านใหม่ในสาย Bias→Confirmation→M1 — เป็น **context เสริม** ให้ `LiquiditySweepStrategy` ฉลาดขึ้น (เพิ่มน้ำหนักให้ signal ที่ sweep ตรงกับ POC หรือจุดที่เคย sweep มาก่อน)

---

## 4. News Blackout Window

ของเดิมที่มีอยู่แล้วช่วยได้บางส่วน: `RiskGuard.MaxSpreadPips` บล็อก order อัตโนมัติถ้า spread กว้างผิดปกติ (มักเกิดตอนข่าวแรง) — แต่เป็นการป้องกันแบบ **reactive** (รู้ทีหลังว่า spread กว้างแล้ว)

**เสนอ**: News Blackout Window — เก็บรายการเวลาข่าวสำคัญ (มือหรือดึงจาก economic calendar API เบาๆ ไม่ต้องใช้ Claude AI) แล้วหยุดยิง signal ล่วงหน้า/หลัง N นาทีรอบเวลาข่าว เป็นการป้องกันแบบ **proactive** ทำได้โดยไม่ต้องรอเปิด Phase 3 (Claude AI Integration ที่ pause ไว้)

---

## สรุปลำดับที่คุยกันไว้ (ยังไม่ตัดสินใจ priority สุดท้าย)

1. Multi-Timeframe Entry Flow (Bias → Confirmation → M1 Filter)
2. Reversal Detection: Momentum Deceleration + Volatility Contraction
3. Daily Profile: Volume Profile/POC + Cumulative Delta + Swept-Levels History
4. News Blackout Window

ทั้งหมดนี้ **ยังไม่ได้เขียนโค้ดสักบรรทัดเดียว** เป็นการออกแบบที่ตกลงกันไว้ในบทสนทนาเท่านั้น รอ confirm ก่อนเริ่ม implement จริง
