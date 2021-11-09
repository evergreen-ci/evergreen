package utility

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRandom(t *testing.T) {
	t.Run("Legacy", func(t *testing.T) {
		r1 := RandomString()
		r2 := RandomString()
		assert.NotEqual(t, r1, r2)
		assert.Len(t, RandomString(), 32)
	})
	t.Run("Sized", func(t *testing.T) {
		assert.Len(t, MakeRandomString(32), 64)
		assert.Len(t, MakeRandomString(64), 128)
		assert.Len(t, MakeRandomString(2), 4)
		assert.Len(t, MakeRandomString(1), 2)
		assert.Len(t, MakeRandomString(3), 6)

		for i := 1; i < 100; i++ {
			a := MakeRandomString(i)
			b := MakeRandomString(i)
			assert.Equal(t, len(a), len(b))
			assert.NotEqual(t, a, b)
			assert.Len(t, a, 2*i)
		}
	})
}
