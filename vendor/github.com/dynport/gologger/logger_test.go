package gologger

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestLogging(t *testing.T) {
	logger := New()
	assert.Equal(t, logger.Colored, true)
	assert.Equal(t, logger.LogLevel, INFO)
	assert.NotNil(t, logger)
	prefix := logger.logPrefix(DEBUG)
	assert.Contains(t, prefix, "\033")
	assert.NotContains(t, prefix, " [")
}

func TestLoggingWithoutColors(t *testing.T) {
	logger := New()
	logger.Colored = false
	prefix := logger.logPrefix(DEBUG)
	assert.NotContains(t, prefix, "\033")
}

func TestLoggingWithTiming(t *testing.T) {
	logger := New()
	logger.Start()
	prefix := logger.logPrefix(DEBUG)
	assert.Contains(t, prefix, " [")
	logger.Stop()
	prefix = logger.logPrefix(DEBUG)
	assert.NotContains(t, prefix, " [")
}

func TestLoggingWithPrefixStack(t *testing.T) {
	logger := New()

	logger.PushPrefix("foo")
	v := logger.logPrefix(DEBUG)
	assert.Contains(t, v, "[foo]")

	logger.PushPrefix("bar")
	v = logger.logPrefix(DEBUG)
	assert.Contains(t, v, "[foo]")
	assert.Contains(t, v, "[bar]")
	assert.Equal(t, logger.PopPrefix(), "bar")

	assert.Equal(t, logger.PopPrefix(), "foo")

	v = logger.logPrefix(DEBUG)
	assert.NotContains(t, v, "[foo]")
	assert.NotContains(t, v, "[bar]")
}
