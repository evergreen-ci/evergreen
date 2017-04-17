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

// FromPythonTime returns a time.Time that corresponds to the float style
// python time which is <seconds>.<fractional_seconds> from unix epoch.
func FromPythonTime(pyTime float64) time.Time {
	sec := int64(pyTime)
	toNano := int64(1000000000)
	asNano := int64(pyTime * float64(toNano))
	nano := asNano % toNano
	res := time.Unix(sec, nano)
	return res
}

// ToPythonTime returns a number in the format that python's time.time() returns
// as a float with <seconds>.<fractional_seconds>
func ToPythonTime(t time.Time) float64 {
	if IsZeroTime(t) {
		return float64(0)
	}
	timeAsInt64 := t.UnixNano()
	fromNano := float64(1000000000)

	res := float64(timeAsInt64) / fromNano
	return res
}
