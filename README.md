---

## 🛠️ Project Roadmap & Sprints

### 🟢 Phase 1: The Hand (`pkg/mt5`) — Go MT5 Library
*Focus: Low-level Infrastructure, Socket Communication, and Order Execution.*

- [ ] **MQL5 IPC Server Setup:** Implement ZeroMQ / WebSockets server on MT5 EA for REQ/REP and PUB/SUB patterns.
- [ ] **Go Transport Layer:** Build thread-safe ZMQ client with automatic reconnection and health checks.
- [ ] **Real-time Tick Streaming:** Stream Bid/Ask prices into Go Channels via Goroutines.
- [ ] **Order Management API:** Implement functions for `SendOrder`, `ModifyOrder`, `ClosePosition`, and `GetAccountSummary`.
- [ ] **Integration Tests:** Comprehensive test suite executing trades on demo accounts.

---

### 🔵 Phase 2: The Brain (`internal/quant`) — Quant & Risk Engine
*Focus: Time-Series Processing, Technical Analysis, and Automated Trading Strategy.*

- [ ] **In-Memory Time Series Buffer:** Design ring-buffer data structures for efficient price storage.
- [ ] **Technical Analysis Library:** Pure Go implementation of SMA, EMA, RSI, MACD, and ATR.
- [ ] **Risk Management Module:**
  - Dynamic Lot Size Calculation based on account equity and Stop Loss distance.
  - Daily Loss Cap & Maximum Drawdown Guard.
- [ ] **Strategy Engine:** Event-loop execution engine (e.g., Moving Average Crossover / Mean Reversion).
- [ ] **Backtesting Framework:** Minimalist engine for historical strategy verification.

---

### 🟣 Phase 3: The Intelligence (`internal/ai`) — Claude AI Integration
*Focus: LLM-based Sentiment Analysis and Adaptive Risk Controls.*

- [ ] **Economic Data Ingestion:** Fetch economic calendar and financial market news APIs.
- [ ] **Claude Prompt Pipeline:** Process financial headlines into structured sentiment scores (`RiskLevel: LOW | MED | HIGH`).
- [ ] **Adaptive Parameter Adjuster:** Dynamically scale position sizes or halt trading during high-impact news events.
- [ ] **Trade Guard Integration:** Final confirmation check via Claude API prior to executing high-exposure trades.

---

## 💻 Tech Stack

- **Primary Language:** Go (Golang)
- **Execution Platform:** MetaTrader 5 (MQL5)
- **Messaging Protocol:** ZeroMQ / WebSockets
- **AI / LLM:** Anthropic Claude API
- **Data Interchange:** JSON / Protocol Buffers