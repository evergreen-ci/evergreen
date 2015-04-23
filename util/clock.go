package util

import (
	"time"
)

// A layer of indirection around time.Now() useful for testing code that
// depends on the current time
type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (c SystemClock) Now() time.Time {
	return time.Now()
}
