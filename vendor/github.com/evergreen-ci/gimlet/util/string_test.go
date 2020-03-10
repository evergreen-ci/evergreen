package util

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRandomString(t *testing.T) {
	prev := []string{}
	for i := 0; i < 1000; i++ {
		s, err := RandomString()
		require.NoError(t, err)
		assert.Len(t, s, hex.EncodedLen(32))
		assert.NotContains(t, prev, s)
		prev = append(prev, s)
	}
}
