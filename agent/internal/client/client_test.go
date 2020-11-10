package client

import (
	"context"
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
	assert.Equal(t, defaultMaxAttempts, c.maxAttempts)
	assert.Equal(t, defaultTimeoutStart, c.timeoutStart)
	assert.Equal(t, defaultTimeoutMax, c.timeoutMax)

	client.SetHostID("hostID")
	client.SetHostSecret("hostSecret")
	assert.Equal(t, "hostID", c.hostID)
	assert.Equal(t, "hostSecret", c.hostSecret)
}

func TestLoggerClose(t *testing.T) {
	assert := assert.New(t)
	comm := NewCommunicator("www.foo.com")
	logger, err := comm.GetLoggerProducer(context.Background(), TaskData{}, nil)
	assert.NoError(err)
	assert.NotNil(logger)
	assert.NoError(logger.Close())
	assert.NotPanics(func() {
		assert.NoError(logger.Close())
	})
}
