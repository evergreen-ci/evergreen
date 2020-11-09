package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEvergreenCommunicatorConstructor(t *testing.T) {
	client := NewCommunicator("url")
	defer client.Close()

	c, ok := client.(*communicatorImpl)
	assert.True(t, ok, true)
	assert.Empty(t, c.apiUser)
	assert.Empty(t, c.apiKey)
	assert.Equal(t, defaultMaxAttempts, c.maxAttempts)
	assert.Equal(t, defaultTimeoutStart, c.timeoutStart)
	assert.Equal(t, defaultTimeoutMax, c.timeoutMax)

	client.SetAPIUser("apiUser")
	client.SetAPIKey("apiKey")
	assert.Equal(t, "apiUser", c.apiUser)
	assert.Equal(t, "apiKey", c.apiKey)
}
