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

// fromNanoSeconds returns milliseconds of a duration for queries in the database.
func FromNanoseconds(duration time.Duration) int64 {
	return int64(duration) / 1000000
}

// fromNanoSeconds returns milliseconds of a duration for queries in the database.
func ToNanoseconds(duration time.Duration) time.Duration {
	return duration * 1000000
}
