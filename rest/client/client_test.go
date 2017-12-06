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
	assert.Empty(t, c.hostID)
	assert.Empty(t, c.hostSecret)
	assert.Empty(t, c.apiUser)
	assert.Empty(t, c.apiKey)
	assert.Equal(t, defaultMaxAttempts, c.maxAttempts)
	assert.Equal(t, defaultTimeoutStart, c.timeoutStart)
	assert.Equal(t, defaultTimeoutMax, c.timeoutMax)

	client.SetHostID("hostID")
	client.SetHostSecret("hostSecret")
	client.SetAPIUser("apiUser")
	client.SetAPIKey("apiKey")
	assert.Equal(t, "hostID", c.hostID)
	assert.Equal(t, "hostSecret", c.hostSecret)
	assert.Equal(t, "apiUser", c.apiUser)
	assert.Equal(t, "apiKey", c.apiKey)
}
