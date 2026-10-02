package domain

import "time"

const (
	SessionAsian           = "ASIAN"
	SessionLondon          = "LONDON"
	SessionLondonNYOverlap = "LONDON_NY_OVERLAP"
	SessionNewYork         = "NEW_YORK"
)

// MarketSessionFromUTC classifies t into one of the 4 standard FX trading
// sessions based on its UTC hour (เพิ่มจากบทสนทนา 2026-10-01 — ดู ROADMAP.md
// "Market Session Tracking"):
//
//	00:00–07:59 UTC  Asian (Tokyo/Sydney)
//	08:00–12:59 UTC  London
//	13:00–16:59 UTC  London/New York overlap (ปกติ volume/volatility สูงสุดของวัน)
//	17:00–21:59 UTC  New York
//	22:00–23:59 UTC  Asian (Sydney เปิดแล้ว, นับรวมกับ Asian แทนแยกเป็น session ที่ 5)
//
// สมมติฐานสำคัญ: ฟังก์ชันนี้ต้องการให้ t เป็นเวลา UTC จริงๆ — เช็คจาก
// timestamp จริงที่เจอ 2026-09-30 แล้วว่า server time ของ MT5/Exness demo
// account ที่ใช้ตอนนี้ตรงกับ UTC+0 พอดี แต่ broker/account อื่นอาจรันเวลา
// เซิร์ฟเวอร์เป็นคนละโซน (เช่น UTC+2/+3) — ถ้าเปลี่ยน broker ต้องเช็คอีกครั้ง
// ก่อนเชื่อผลลัพธ์จากฟังก์ชันนี้
func MarketSessionFromUTC(t time.Time) string {
	hour := t.UTC().Hour()
	switch {
	case hour >= 13 && hour < 17:
		return SessionLondonNYOverlap
	case hour >= 8 && hour < 13:
		return SessionLondon
	case hour >= 17 && hour < 22:
		return SessionNewYork
	default: // 22:00-23:59 and 00:00-07:59
		return SessionAsian
	}
}
