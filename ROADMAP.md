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

- [ ] **Real-time Market Data Stream**
  - [ ] เพิ่ม Socket Port (เช่น 5556) สำหรับ PUB/SUB หรือ Stream Price จาก MT5
  - [ ] สร้าง In-memory Ring Buffer สำหรับเก็บ Time Series Price Data ใน Go
- [ ] **Technical Analysis Indicators**
  - [ ] เขียน Pure Go Indicator Functions (`SMA`, `EMA`, `RSI`, `MACD`, `ATR`)
- [ ] **Risk & Position Sizing Engine**
  - [ ] คำนวณ Dynamic Lot Size ตาม Account Equity และ Stop Loss
  - [ ] ระบบ Max Daily Loss / Drawdown Guard
- [ ] **Strategy Engine (Event Loop)**
  - [ ] รับ Tick/Candle Data → คำนวณ Signal → สั่งยิง Order ผ่าน `MT5Port`

---

## 🟣 Phase 3: The Intelligence (Claude AI Integration)

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
  - [ ] เขียน Unit Test สำหรับ `TradeService` โดยใช้ Mock `MT5Port` (ไม่ต้องต่อ MT5 จริง)
  - [ ] ทำ Integration Test บน บัญชี Demo ของ MT5
- [ ] **Logging & Monitoring**
  - [ ] ติดตั้ง Structured Logger (เช่น `zerolog` หรือ `zap`) บันทึก Log ทุกการส่งคำสั่งเทรด
