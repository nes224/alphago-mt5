package domain

import (
	"strings"
	"time"
)

type CustomTime struct {
	time.Time
}

const mt5TimeLayout = "2006.01.02 15:04:05"

func (ct *CustomTime) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), "\"")
	if s == "" || s == "null" {
		return nil
	}

	t, err := time.Parse(mt5TimeLayout, s)
	if err != nil {
		return err
	}

	ct.Time = t
	return nil
}
