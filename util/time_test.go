package util

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTimeJitter(t *testing.T) {
	assert := assert.New(t)

	for i := 0; i < 100; i++ {
		t := JitterInterval(time.Second * 15)
		assert.True(t >= 15*time.Second)
		assert.True(t <= 30*time.Second)
	}
}
