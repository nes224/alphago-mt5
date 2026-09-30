package logging

import (
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Init ตั้งค่า global zerolog logger ตัวเดียวที่ทุก package เรียกผ่าน
// github.com/rs/zerolog/log — เรียกครั้งเดียวตอนเริ่ม main()
//
// appEnv == "production" -> JSON logs (เหมาะกับ log aggregator)
// อย่างอื่น (dev/staging/ว่าง) -> console pretty-print อ่านง่ายตอน develop
func Init(appEnv string) {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	if appEnv == "production" {
		log.Logger = zerolog.New(os.Stdout).With().Timestamp().Caller().Logger()
		return
	}

	log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: "15:04:05"}).With().Timestamp().Logger()
}
