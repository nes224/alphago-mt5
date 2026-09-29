📐 Architectural Design Document: Data Pipeline & Memory Management
System: AlphaGo Quant Engine

Module: Market Data Pipeline & Strategy State

Language/Tech: Go, MT5 Socket Stream, Distributed Architecture

🚨 1. Problem Statement (ปัญหาคืออะไร?)
ในการสร้างระบบ Quant / Algorithmic Trading System แบบ Real-time ตัว engine จำเป็นต้องรับข้อมูลราคา (Tick Data) จาก MT5 ผ่าน Socket Interface มาคำนวณ Indicator (เช่น EMA, RSI, MACD, ATR) และสร้างสัญญาณซื้อขาย (Trading Signals)

หากเราเลือกใช้ Traditional Architecture โดยการ Save/Query ข้อมูลราคาทั้งหมดผ่าน Database (RDBMS/TimescaleDB) โดยตรงทุกๆ Tick จะพบปัญหาคอขวดหลัก 3 ประการ:

1.1 High Query Latency & Network OverheadIndicator ส่วนใหญ่ต้องการข้อมูลราคาย้อนหลัง $N$ แท่งล่าสุด (เช่น EMA 200 ต้องใช้ราคาปิด 200 แท่งล่าสุด)หากสตรีม Tick เข้ามา 10–50 Ticks/Sec แล้ว Go Service ต้องส่ง Query SELECT ... ORDER BY time DESC LIMIT 200 ไปยัง Database ทุกครั้ง จะเกิด Round-Trip Network Latency ($5 - 50\text{ ms}$) ซึ่งช้าเกินไปสำหรับระบบ High-Speed Trading

1.2 Database Bottleneck & IOPS Exhaustion
การเขียน (Insert) ทุก Tick ลง Database พร้อมกับการอ่าน (Query) เพื่อคำนวณ Signal ซ้ำๆ ใน Thread เดียวกัน จะทำให้ Database เกิด I/O Bottleneck และ CPU Usage พุ่งสูงจนระบบค้าง

1.3 Memory Allocation & Garbage Collector (GC) Pauses (ถ้าใช้ Go Dynamic Slices)
หากพยายามเก็บข้อมูลไว้ใน RAM ของ Go เอง แต่ใช้วิธี append() ใส่ Dynamic Slice ไปเรื่อยๆ แล้วคอย Slice หัวทิ้ง (slice = slice[1:]) จะทำให้ Go Runtime ต้องคอย Alloc/Dealloc Memory ตลอดเวลา

ส่งผลให้ Go Garbage Collector (GC) ทำงานบ่อยขึ้น เกิด GC Stop-the-World Pauses ทำให้ระบบกระตุกแบบสุ่ม (Latency Spikes)

🛠️ 2. The Solution (เราจะแก้ปัญหานี้อย่างไร?)
ทางแก้ปัญหาที่เป็นมาตรฐานระดับ Institutional Quant Infrastructure คือการแบ่งการทำงานออกเป็น 2 Paths (Hot Path vs Cold Path) และใช้ In-Memory Ring Buffer ร่วมกับ Event-Driven Messaging Bus

┌──> [In-Memory Ring Buffer] ──> [Strategy Engine] 
                                          │    (RAM / Zero-Alloc)          (Hot Path: < 1ms)
                                          │
[MT5 Tick Streamer] ──> [StreamAdapter] ──┼──> [Kafka / NATS JetStream] ──> [Worker Process] ──> [TimescaleDB]
                                          │    (Message Broker)            (Throttling)          (Cold Path: Storage)
                                          │
                                          └──> [WebSocket Hub] ──> [Dashboard UI]

Solution Component 1: In-Memory Ring Buffer (Hot Path)หน้าที่: เก็บข้อมูลงวดล่าสุด $N$ ช่อง ไว้ใน RAM ของ Go Process เอง โดยมีคุณสมบัติดังนี้:Fixed Size Array: ล็อคขนาด Memory ไว้ตั้งแต่เริ่มทำงาน เช่น ขนาด 500 หรือ 1,000 ช่องOverwrite Oldest Data ($O(1)$): เมื่อข้อมูลเต็ม Pointer จะวนกลับมาเขียนทับข้อมูลเก่าที่สุดทันทีZero-Allocation: ไม่มี append เพิ่มขนาด Memory ในระหว่างที่ระบบรัน ทำให้ GC ไม่ทำงานหนัก ค่า Latency นิ่งระดับ Nanoseconds

Code Conceptual: In-Memory Ring Buffer (Go)

package buffer

import "sync"

type Tick struct {
	Bid       float64
	Ask       float64
	Timestamp int64
}

type RingBuffer struct {
	mu     sync.RWMutex
	data   []Tick
	capacity int
	head   int
	isFull bool
}

func NewRingBuffer(capacity int) *RingBuffer {
	return &RingBuffer{
		data:     make([]Tick, capacity),
		capacity: capacity,
	}
}

// Push เพิ่ม Tick ใหม่ด้วย O(1) Time Complexity (Thread-safe)
func (r *RingBuffer) Push(tick Tick) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.data[r.head] = tick
	r.head = (r.head + 1) % r.capacity
	if r.head == 0 {
		r.isFull = true
	}
}

// GetLatestN ดึงข้อมูล N รายการล่าสุดออกมารวดเดียวโดยไม่ต้อง query DB
func (r *RingBuffer) GetLatestN(n int) []Tick {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if n > r.capacity {
		n = r.capacity
	}

	result := make([]Tick, 0, n)
	count := r.capacity
	if !r.isFull {
		count = r.head
	}

	if n > count {
		n = count
	}

	// Calculate start index for chronological read (Oldest -> Newest)
	start := (r.head - n + r.capacity) % r.capacity
	for i := 0; i < n; i++ {
		idx := (start + i) % r.capacity
		result = append(result, r.data[idx])
	}

	return result
}

Solution Component 2: Distributed Message Bus & DB Persistence (Cold Path)
หน้าที่: แยกงานจัดเก็บข้อมูลถาวรออกจาก Loop คำนวณ Trade Signal

Kafka / NATS JetStream (Throttling & Buffer):

ทำหน้าที่เป็น Message Queue คั่นกลางระหว่าง StreamAdapter กับ Database

ช่วยซับแรงกระแทกเวลามีข่าวออกแล้ว Ticks พุ่งสูง (Tick Spikes)

TimescaleDB / PostgreSQL (Persistence):

ใช้ Worker ดึงข้อมูลจาก Kafka/NATS แบบ Batch มาบันทึกลง Database

ใช้สำหรับ Backtesting, Historical Analysis และ Warm-up state ตอนที่ Go Engine เพิ่ง Restart ใหม่

📊 3. Comparison Matrix (เปรียบเทียบแต่ละรูปแบบ)
มิติ                     Direct Database Query            Dynamic Slice (append)In-Memory            Ring Buffer (แนะนำ)
Latency.              $5 - 50\text ms (High)                 < 1\text ms  (Unstable)             < 0.001\text ms  100 ns
I/O Complexity	       Network & Disk I/O	                      RAM Only	                          RAM Only
Memory Allocation	 High (DB Driver allocations)	       High (Triggers GC Frequent)	        Zero Additional Allocations
Data Retention	           Permanent	                      In-Memory (Temporary)	               In-Memory (Temporary)
Use Case	             Reports & History	                     Small projects	                  Production Trading Engine

🎯 4. Summary & Implementation Steps
ใช้ Ring Buffer ใน Go Process เพื่อส่งราคาเข้า Indicator และ Strategy Engine (ได้ความเร็วสูงสุด Latency ต่ำที่สุด)
ใช้ Kafka/NATS + DB (TimescaleDB) ทำงานแบบ Asynchronous อยู่เบื้องหลัง เพื่อเก็บประวัติราคาไว้ทำ Backtesting และ Warm up ระบบเมื่อเปิดใหม่
รองรับ Volatile Market ช่วงข่าวออกได้