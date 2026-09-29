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
  - [x] TickMetrics & ZScoreStrategy (คำนวณ Standard Deviation สวนเข้าหาค่าเฉลี่ย)
  - [x] Open Interest (OI) & Velocity Engine: เพิ่มฟิลด์ OpenInterest / OIDelta ใน tick.go และ tick_metrics.go เพื่อทำ OIExpansionStrategy
  - [] Liquidity Sweep Detection: เขียนโมเดลตรวจจับการกวาด Stop Loss บริเวณ High/Low ย้อนหลัง
  - [] Market Regime Filter: เพิ่ม Hurst Exponent หรือ Trend Slope แยกแยะช่วง Sideway ($Z$-Score) กับ Trend (OIExpansion)
  - [] Microstructure S&R Window: สร้าง Rolling High/Low N-Ticks Buffer

- [] Risk & Position Sizing Engine (สำคัญมากก่อนยิงจริง)
  - [x] Dynamic Position Sizing: คำนวณ Lot Size ตาม Risk % ของ Equity และระยะ Stop Loss จากค่า $\sigma$ (StdDev)
  - [] Risk Guard / Drawdown Control: ล็อคระบบไม่ให้ยิง Order เพิ่มหาก Daily Loss ทะลุ Threshold ที่ตั้งไว้

- [] Strategy Engine & Execution Wireup (Event Loop)
  - [x] สร้าง QuantStrategy Interface และ SignalChannel()
  - [x] Signal Execution Dispatcher: ดึง OrderSignal จาก SignalChannel() ผ่าน Risk Guard แล้วส่งให้ tcp_client.go ยิง Order เข้า MT5 (alphago_mt5.mq)
  - [x] Main Wireup & Integration Tests: ประกอบระบบทั้งหมดใน cmd/app/main.go และเขียน quant_engine_test.go

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
