package util

import (
	"math/rand"
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

// JitterInterval returns a duration that some value between the
// interval and 2x the interval.
func JitterInterval(interval time.Duration) time.Duration {
	return time.Duration(rand.Float64()*float64(interval)) + interval
}

// ParseRoundPartOfHour produces a time value with the minute value
// rounded down to the most recent interval.
func RoundPartOfHour(num int) time.Time { return findPartMin(time.Now(), num) }

// ParseRoundPartOfMinute produces a time value with the second value
// rounded down to the most recent interval.
func RoundPartOfMinute(num int) time.Time { return findPartSec(time.Now(), num) }

// this implements the logic of RoundPartOfHour, but takes time as an
// argument for testability.
func findPartMin(now time.Time, num int) time.Time {
	var min int

	if num > now.Minute() || num > 30 {
		min = 0
	} else {
		min = now.Minute() - (now.Minute() % num)
	}

	return time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), min, 0, 0, time.UTC)
}

// this implements the logic of RoundPartOfMinute, but takes time as an
// argument for testability.
func findPartSec(now time.Time, num int) time.Time {
	var sec int

	if num > now.Second() || num > 30 {
		sec = 0
	} else {
		sec = now.Second() - (now.Second() % num)
	}

	return time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), sec, 0, time.UTC)

}
