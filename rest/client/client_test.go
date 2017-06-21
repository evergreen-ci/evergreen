package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEvergreenRESTConstructor(t *testing.T) {
	client := NewEvergreenREST("url", "hostID", "hostSecret")
	c, ok := client.(*evergreenREST)
	assert.True(t, ok, true)
	assert.Equal(t, "hostID", c.hostID)
	assert.Equal(t, "hostSecret", c.hostSecret)
	assert.Equal(t, defaultMaxAttempts, c.maxAttempts)
	assert.Equal(t, defaultTimeoutStart, c.timeoutStart)
	assert.Equal(t, defaultTimeoutMax, c.timeoutMax)
}
