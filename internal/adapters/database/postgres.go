package database

import (
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Connect เปิด connection ไปยัง PostgreSQL และรัน AutoMigrate ให้ schema
// ทั้ง 7 ตารางครบก่อนคืนค่ากลับ — เรียกครั้งเดียวตอน service start
func Connect(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}

	if err := db.AutoMigrate(
		&AccountStateModel{},
		&RiskGuardStateModel{},
		&SignalRecordModel{},
		&TradeOutcomeModel{},
		&OpenPositionSymbolModel{},
		&PendingAttributionModel{},
		&PendingOrderModel{},
	); err != nil {
		return nil, fmt.Errorf("failed to auto-migrate schema: %w", err)
	}

	return db, nil
}
