package options

import (
	"os"
	"testing"

	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoggerProducers(t *testing.T) {
	defer func() {
		os.RemoveAll("logs.txt")
	}()

	for _, test := range []struct {
		name     string
		producer LoggerProducer
	}{
		{
			name: LogDefault,
			producer: &DefaultLoggerOptions{
				Base: BaseOptions{Format: LogFormatPlain},
			},
		},
		{
			name: LogFile,
			producer: &FileLoggerOptions{
				Filename: "logs.txt",
				Base:     BaseOptions{Format: LogFormatPlain},
			},
		},
		{
			name: LogInherited,
			producer: &InheritedLoggerOptions{
				Base: BaseOptions{Format: LogFormatPlain},
			},
		},
		{
			name: LogInMemory,
			producer: &InMemoryLoggerOptions{
				InMemoryCap: 1024,
				Base:        BaseOptions{Format: LogFormatPlain},
			},
		},
		{
			name: LogSumoLogic,
			producer: &SumoLogicLoggerOptions{
				SumoEndpoint: "https://endpoint",
				Base:         BaseOptions{Format: LogFormatPlain},
			},
		},
		{
			name: LogSplunk,
			producer: &SplunkLoggerOptions{
				Splunk: send.SplunkConnectionInfo{
					ServerURL: "https://splunk",
					Token:     "token",
				},
				Base: BaseOptions{Format: LogFormatPlain},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.name, test.producer.Type())
			sender, err := test.producer.Configure()
			require.NoError(t, err)
			require.NotNil(t, sender)
			assert.NoError(t, sender.Close())
		})
	}
}
