# Variable definitions
BINARY_NAME=alphago-mt5
MAIN_PATH=./cmd/app/main.go
COVERAGE_FILE=coverage.out

.PHONY: all build run run-air test test-v coverage clean fmt help

## default: แสดงรายการคำสั่งทั้งหมด
all: help

## build: คอมไพล์โปรเจกต์เป็น Binary file
build:
	@echo "==> Building binary $(BINARY_NAME)..."
	go build -o bin/$(BINARY_NAME) $(MAIN_PATH)

## run: รันแอปพลิเคชันโดยตรง
run:
	@echo "==> Running application..."
	go run $(MAIN_PATH)

## run-air: รันแอปพลิเคชันด้วย Air (Live Reload)
run-air:
	@echo "==> Running application with Air (Live Reload)..."
	@if command -v air > /dev/null; then \
		air; \
	else \
		echo "Air is not installed. Installing Air..."; \
		go install github.com/air-verse/air@latest; \
		air; \
	fi

## test: รัน Unit Test ทั้งหมดแบบเงียบ
test:
	@echo "==> Running tests..."
	go test ./...

## test-v: รัน Unit Test แบบ Verbose (แสดงรายละเอียดทุกกรณีทดสอบ)
test-v:
	@echo "==> Running tests with verbose output..."
	go test -v -cover ./...

## coverage: รัน Test พร้อมสร้าง HTML Report สำหรับดู Code Coverage ในบราวเซอร์
coverage:
	@echo "==> Generating test coverage report..."
	go test -coverprofile=$(COVERAGE_FILE) ./...
	go tool cover -html=$(COVERAGE_FILE)

## fmt: จัดฟอร์แมตโค้ด Go ทั้งโปรเจกต์ตามมาตรฐาน gofmt
fmt:
	@echo "==> Formatting code..."
	go fmt ./...

## clean: ลบไฟล์ Build binary, tmp จาก air และ Coverage report เก่า
clean:
	@echo "==> Cleaning build artifacts..."
	rm -rf bin/
	rm -rf tmp/
	rm -f $(COVERAGE_FILE)

## help: แสดงคำสั่งที่สามารถใช้งานได้ทั้งหมด
help:
	@echo "Usage:"
	@echo "  make <target>"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' |  sed -e 's/^/ /'