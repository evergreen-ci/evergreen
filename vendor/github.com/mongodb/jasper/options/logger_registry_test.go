package options

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoggerRegistry(t *testing.T) {
	registry := NewBasicLoggerRegistry()
	registeredFactories := map[string]LoggerProducerFactory{}

	registeredFactories[LogDefault] = NewDefaultLoggerProducer
	assert.False(t, registry.Check(LogDefault))
	registry.Register(NewDefaultLoggerProducer)
	assert.True(t, registry.Check(LogDefault))
	factory, ok := registry.Resolve(LogDefault)
	assert.Equal(t, NewDefaultLoggerProducer(), factory())
	assert.True(t, ok)

	registeredFactories[LogFile] = NewFileLoggerProducer
	assert.False(t, registry.Check(LogFile))
	registry.Register(NewFileLoggerProducer)
	assert.True(t, registry.Check(LogFile))
	factory, ok = registry.Resolve(LogFile)
	assert.Equal(t, NewFileLoggerProducer(), factory())
	assert.True(t, ok)

	factories := registry.Names()
	require.Len(t, factories, len(registeredFactories))
	for _, factoryName := range factories {
		_, ok := registeredFactories[factoryName]
		require.True(t, ok)
	}
}
