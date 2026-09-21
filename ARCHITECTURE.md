# 🚀 High-Performance Trading Engine Architecture

เอกสารสรุปสถาปัตยกรรมระบบรับส่งข้อมูลราคาและส่งคำสั่งซื้อขาย (Order Execution) ประสิทธิภาพสูง ออกแบบโดยแบ่ง Layer ออกเป็น Hot Path, Warm Path และ Outbox Execution เพื่อรองรับ Low Latency และความถูกต้องของข้อมูล (Data Consistency)

---

## 🏗️ System Architecture Overview

[ Market Data / WebSocket ]
│
├──► [ Hot Path: Ring Buffer (RAM) ] ──► Strategy Engine / Backtest
│
└──► [ Warm Path: TimescaleDB ] ─────► Continuous Aggregations (1m, 5m Candles)

[ User / Strategy Engine ]
│
▼
[ Cold Path / Order Flow ]
│
(Tx Start)
├──► Insert orders (PENDING)
└──► Insert outbox_events (ORDER_CREATED)
(Tx Commit)
│
▼
[ Go Outbox Relay Worker ] ──► [ MT5 / Broker Gateway API ]

---

## 📚 Table of Contents

1. [Hot Path: In-Memory Ring Buffer](#1-hot-path-in-memory-ring-buffer)
2. [Warm Path: TimescaleDB & Continuous Aggregates](#2-warm-path-timescaledb--continuous-aggregates)
3. [Cold Path: Transactional Outbox Pattern](#3-cold-path-transactional-outbox-pattern)
4. [Tech Stack](#4-tech-stack)

---

## ⚡ 1. Hot Path: In-Memory Ring Buffer

**วัตถุประสงค์:** ประมวลผลข้อมูล Tick Data ล่าสุดในหน่วยความจำ (RAM) เพื่อให้ Strategy Engine อ่านราคาและคำนวณ Indicator ได้เร็วที่สุดโดยไม่ต้องรอ I/O ของ Database

* **Data Structure:** Fixed-size Ring Buffer (Circular Queue)
* **Concurrency:** Thread-safe / Mutex Lock-free (or RWMutex)
* **Key Features:**
  * เก็บ Tick/Price Data แบบ FIFO ตามช่วงเวลา sliding window
  * Zero-memory allocation ในช่วง runtime (ป้องกัน GC Overhead)

---

## 📊 2. Warm Path: TimescaleDB & Continuous Aggregations

**วัตถุประสงค์:** จัดเก็บข้อมูล Tick Data ย้อนหลังอย่างเป็นระบบ และแปลงสภาพข้อมูลเป็น OHLCV Candles (1m, 5m, 1h) สำหรับวิเคราะห์หรือทำ Backtest

* **Hypertables:** สร้าง Partition ตามช่วงเวลา (`time` column) เพื่อเร่งความเร็ว Query และ Insert
* **Continuous Aggregations:** ให้ TimescaleDB ทำ Materialized Views คำนวณ High, Low, Open, Close, Volume แบบ Real-time อัตโนมัติ
* **Retention Policy:** ตั้งค่าลบข้อมูล Tick เก่า หรือบีบอัด (Compression Policy) เพื่อประหยัดพื้นที่จัดเก็บ

---

## 🔒 3. Cold Path: Transactional Outbox Pattern

**วัตถุประสงค์:** รับประกันความถูกต้องของสถานะคำสั่งซื้อขาย (Order State) ป้องกันปัญหา Dual-Write Failure เมื่อส่งคำสั่งไปยัง MT5 หรือ Broker Gateway

* **Atomic Transaction:** บันทึกข้อมูลคำสั่งซื้อขายลงตาราง `orders` พร้อมเขียน Event ลงตาราง `outbox_events` ภายใน Database Transaction เดียวกัน
* **Outbox Relay Worker:**
  * ดึง Event ที่ยังไม่ประมวลผล (`PENDING`) ไปยิงผ่าน Broker API
  * รองรับ Retry Mechanism แบบ Exponential Backoff
  * อัปเดตสถานะเป็น `PROCESSED` หรือ `FAILED` พร้อมกับรับประกัน **At-Least-Once Delivery**

---

## 🛠️ Tech Stack

* **Language:** Go (Golang)
* **Database:** PostgreSQL + TimescaleDB Extension
* **Caching / In-Memory:** Lock-Free Ring Buffer in Go
* **External Integrations:** MetaTrader 5 (MT5) / Broker REST & FIX API

------------------------------------------------------------------

# 🏗️ Market Data & Architecture Strategy: AlphaGo System

---

## 📌 Executive Summary

การประมวลผลข้อมูลราคาตลาด (Tick / Candle Data) ที่มีความถี่สูง (High-Frequency Stream) **ไม่ควรใช้ Transactional Outbox Pattern** เนื่องจาก Outbox Pattern ออกแบบมาแก้ปัญหา *Dual-Write* บน Transactional Database ซึ่งจะสร้าง **DB Disk I/O Bottleneck** และ **Latency มหาศาล** จากการเขียน/อ่าน Disk ตลอดเวลา

**โซลูชันที่ถูกต้อง:** แยกสถาปัตยกรรมออกเป็น **Streaming Pipeline (In-Memory First / Pub-Sub)** สำหรับ Market Data และใช้ **Outbox Pattern เฉพาะส่วน Trading Execution (Order State)** เท่านั้น

---

## 🔄 1. Market Data Pipeline (Hot, Warm & Cold Storage)

ออกแบบการไหลของข้อมูลตาม **Data Lifecycle** เพื่อให้ระบบรองรับ throughput สูง Latency ต่ำ และสเกลเป็น Distributed System ได้ง่าย

[MT5 EA / Feed Engine] ──(TCP / WebSocket Stream)──> [Go Ingestion Engine]
│
┌─────────────────┴─────────────────┐
▼                                   ▼
[Channel A: Hot Path]       [Channel B: Warm Path]
│                                   │
▼                                   ▼
[In-Memory Ring Buffer]    [Batch Persister (Bulk)]
(ใช้คำนวน Indicator ทันที)             │
▼
[TimescaleDB / ClickHouse]
│
▼ (Older than 30d)
[Cold: Parquet on S3]

### 🔹 Layer Storage Breakdown

| Layer | Storage Technology | Purpose & Usage | Performance Target |
| :--- | :--- | :--- | :--- |
| **Hot Path** | **In-Memory Ring Buffer (Go RAM)** / Redis Streams | เก็บข้อมูล Tick/Candle ล่าสุด (เช่น 1,000 Bars) ไว้ใน RAM สำหรับ Strategy Engine คำนวณสัญญาณยิง Order | Sub-millisecond / Microseconds |
| **Warm Path** | **TimescaleDB** หรือ **ClickHouse** | บันทึก Tick/Candle Data ย้อนหลัง (30-90 วัน) สำหรับทำ Charting, Backtesting และ Query วิเคราะห์ | Millisecond Query Latency |
| **Cold Path** | **Parquet Files on Object Storage (S3 / MinIO)** | Export ข้อมูลเก่ากว่า 30 วันเป็นไฟล์ `.parquet` เก็บใน Data Lake สำหรับฝึก AI / Machine Learning Models | Cost-optimized Long-term Storage |

---

## 🛠️ 2. Go Architecture Implementation Strategy

เพื่อคงสถาปัตยกรรม **Hexagonal Architecture** ใน Go:

1. **Inbound Adapter:** รับ Stream Ticks จาก MT5 ผ่าน Socket
2. **Internal Event Dispatcher (Fan-out Pattern):**
   * ใช้ **Go Channels + Worker Pool** กระจายข้อมูลแบบ Non-blocking
   * **Channel A (Strategy Worker):** อัปเดต In-Memory Ring Buffer ทันที
   * **Channel B (Persistence Worker):** พักข้อมูลใน Batch Buffer (เช่น ทุกๆ 100 Ticks หรือทุกๆ 500ms) แล้วยิง `Bulk Insert` / `CopyFrom` ลง Database
3. **Continuous Aggregations:** ให้ Database (TimescaleDB) ช่วยแปลง Tick Data เป็น Time-bucketed Candles (1m, 5m, 1h) อัตโนมัติในฝั่ง DB

---

## 🎯 3. การใช้งาน Transactional Outbox Pattern ที่ถูกต้อง

ใช้ Outbox Pattern **เฉพาะตอนที่ Quant Engine ตัดสินใจสั่งเปิด/ปิด Order** เพื่อป้องกันปัญหา Message Loss เมื่อระบบล่มระหว่างยิง Order หา Broker/MT5

### 📋 State & Flow Diagram for Order Execution

[Quant Engine Signal]
│
▼
┌─────────────────────────────────────────────────────────────┐
│  PostgreSQL Atomic Transaction                              │
│  1. INSERT INTO orders (status = 'PENDING')                 │
│  2. INSERT INTO outbox_events (event_type = 'EXECUTE_ORDER') │
└─────────────────────────────────────────────────────────────┘
│
▼ (Committed)
┌─────────────────────────────────────────────────────────────┐
│  Outbox Relay (Go Background Worker / Poller)               │
└─────────────────────────────────────────────────────────────┘
│
▼
[Send Order via MT5 TCP Adapter] ───> [MT5 / Broker Execution]

---

