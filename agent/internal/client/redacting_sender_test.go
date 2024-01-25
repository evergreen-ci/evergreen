package client

import (
	"testing"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedactingSender(t *testing.T) {
	for name, test := range map[string]struct {
		substitutions map[string]string
		inputString   string
		expected      string
	}{
		"MultipleSubstitutions": {
			substitutions: map[string]string{
				"secret_key": "secret_val",
			},
			inputString: "secret_val secret_val",
			expected:    "<redacted:secret_key> <redacted:secret_key>",
		},
		"MultipleValues": {
			substitutions: map[string]string{
				"secret_key1": "secret_val1",
				"secret_key2": "secret_val2",
			},
			inputString: "secret_val2 secret_val1",
			expected:    "<redacted:secret_key2> <redacted:secret_key1>",
		},
		"OverlappingSubstitutions": {
			substitutions: map[string]string{
				"secret_key": "cryptic",
			},
			inputString: "crypticryptic",
			expected:    "<redacted:secret_key>ryptic",
		},
		"MultipleInstancesOfVal": {
			substitutions: map[string]string{
				"secret_key1": "secret_val",
				"secret_key2": "secret_val",
			},
			inputString: "secret_val",
			expected:    "<redacted:secret_key1>",
		},
	} {
		t.Run(name, func(t *testing.T) {
			wrappedSender, err := send.NewInternalLogger("", send.LevelInfo{Threshold: level.Info, Default: level.Info})
			require.NoError(t, err)

			newRedactingSender(wrappedSender, test.substitutions).Send(message.NewDefaultMessage(level.Info, test.inputString))
			assert.Equal(t, test.expected, wrappedSender.GetMessage().Message.String())
		})
	}
}
