package agent

import (
	"math/rand"
	"time"
)

// fuzzInterval returns a duration that is gs
func fuzzInterval(interval time.Duration) time.Duration {
	val := (rand.Float64() * float64(interval)) + interval

	return time.Duration(val)
}
