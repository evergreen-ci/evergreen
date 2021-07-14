package client

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/stretchr/testify/assert"
)

func TestEvergreenCommunicatorConstructor(t *testing.T) {
	client := NewHostCommunicator("url", "hostID", "hostSecret")
	defer client.Close()

	c, ok := client.(*hostCommunicator)
	assert.True(t, ok, true)
	assert.Equal(t, "hostID", c.reqHeaders[evergreen.HostHeader])
	assert.Equal(t, "hostSecret", c.reqHeaders[evergreen.HostSecretHeader])
	assert.Equal(t, defaultMaxAttempts, c.retry.MaxAttempts)
	assert.Equal(t, defaultTimeoutStart, c.retry.MinDelay)
	assert.Equal(t, defaultTimeoutMax, c.retry.MaxDelay)
}

func TestLoggerClose(t *testing.T) {
	assert := assert.New(t)
	comm := NewHostCommunicator("www.foo.com", "hostID", "hostSecret")
	logger, err := comm.GetLoggerProducer(context.Background(), TaskData{}, nil)
	assert.NoError(err)
	assert.NotNil(logger)
	assert.NoError(logger.Close())
	assert.NotPanics(func() {
		assert.NoError(logger.Close())
	})
}
