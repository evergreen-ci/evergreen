package rehttp

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConstDelay(t *testing.T) {
	want := 2 * time.Second
	fn := ConstDelay(want)
	for i := 0; i < 5; i++ {
		delay := fn(Attempt{Index: i})
		assert.Equal(t, want, delay, "%d", i)
	}
}

func TestExpJitterDelay(t *testing.T) {
	fn := ExpJitterDelay(time.Second, 5*time.Second)
	for i := 0; i < 10; i++ {
		delay := fn(Attempt{Index: i})
		top := math.Pow(2, float64(i)) * float64(time.Second)
		actual := time.Duration(math.Min(float64(5*time.Second), top))
		assert.True(t, delay > 0, "%d: %s <= 0", i, delay)
		assert.True(t, delay <= actual, "%d: %s > %s", i, delay, actual)
	}
}
