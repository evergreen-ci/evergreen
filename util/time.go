package util

import (
	"time"
)

// ZeroTime represents 0 in epoch time
var ZeroTime time.Time = time.Unix(0, 0)

// IsZeroTime checks that a time is either equal to golang ZeroTime or
// UTC ZeroTime.
func IsZeroTime(t time.Time) bool {
	return t.Equal(ZeroTime) || t.IsZero()
}
