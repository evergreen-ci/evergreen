package redactor

import (
	"context"
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/util"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedactingSender(t *testing.T) {
	for name, test := range map[string]struct {
		expansions         map[string]string
		expansionsToRedact []string
		internalRedactions map[string]string
		inputString        string
		expected           string
	}{
		"MultipleSubstitutions": {
			expansions: map[string]string{
				"secret_key": "secret_val",
			},
			internalRedactions: map[string]string{
				"another_secret_key": "another_secret_val",
			},
			expansionsToRedact: []string{"secret_key"},
			inputString:        "secret_val secret_val another_secret_val",
			expected:           fmt.Sprintf("%s %s %s", fmt.Sprintf(redactedVariableTemplate, "secret_key"), fmt.Sprintf(redactedVariableTemplate, "secret_key"), fmt.Sprintf(redactedVariableTemplate, "another_secret_key")),
		},
		"MultipleValues": {
			expansions: map[string]string{
				"secret_key1": "secret_val1",
				"secret_key2": "secret_val2",
			},
			internalRedactions: map[string]string{
				"secret_key3": "secret_val3",
			},
			expansionsToRedact: []string{"secret_key1", "secret_key2"},
			inputString:        "secret_val2 secret_val1 secret_val3",
			expected:           fmt.Sprintf("%s %s %s", fmt.Sprintf(redactedVariableTemplate, "secret_key2"), fmt.Sprintf(redactedVariableTemplate, "secret_key1"), fmt.Sprintf(redactedVariableTemplate, "secret_key3")),
		},
		"OverlappingSubstitutions": {
			expansions: map[string]string{
				"secret_key": "cryptic",
			},
			expansionsToRedact: []string{"secret_key"},
			inputString:        "crypticryptic",
			expected:           fmt.Sprintf("%sryptic", fmt.Sprintf(redactedVariableTemplate, "secret_key")),
		},
		"MultipleInstancesOfVal": {
			expansions: map[string]string{
				"secret_key1": "secret_val",
				"secret_key2": "secret_val",
			},
			expansionsToRedact: []string{"secret_key1", "secret_key2"},
			inputString:        "secret_val",
			expected:           fmt.Sprintf(redactedVariableTemplate, "secret_key1"),
		},
		"LargerExpansionRedactedFirst": {
			expansions: map[string]string{
				"secret_key1": "value",
				"secret_key2": "longer_value",
			},
			expansionsToRedact: []string{"secret_key1", "secret_key2"},
			inputString:        "longer_value",
			expected:           fmt.Sprintf(redactedVariableTemplate, "secret_key2"),
		},
		"NonexistentExpansion": {
			expansions: map[string]string{
				"secret_key": "secret_val",
			},
			expansionsToRedact: []string{"nan"},
			inputString:        "secret_val",
			expected:           "secret_val",
		},
		"NoMatches": {
			expansions: map[string]string{
				"secret_key": "secret_val",
			},
			internalRedactions: map[string]string{
				"another_secret_key": "another_secret_val",
			},
			expansionsToRedact: []string{"secret_key"},
			inputString:        "nothing to see here",
			expected:           "nothing to see here",
		},
	} {
		t.Run(name, func(t *testing.T) {
			wrappedSender, err := send.NewInternalLogger("", send.LevelInfo{Threshold: level.Info, Default: level.Info})
			require.NoError(t, err)

			opts := RedactionOptions{
				Expansions:         util.NewDynamicExpansions(test.expansions),
				Redacted:           test.expansionsToRedact,
				InternalRedactions: util.NewDynamicExpansions(test.internalRedactions),
			}
			s := NewRedactingSender(wrappedSender, opts)
			s.Send(context.Background(), message.NewDefaultMessage(level.Info, test.inputString))
			assert.Equal(t, test.expected, wrappedSender.GetMessage().Message.String())
		})
	}
}

func TestRedactingSenderPicksUpDynamicRedactions(t *testing.T) {
	t.Run("WithoutPreloadPicksUpNewRedactions", func(t *testing.T) {
		wrappedSender, err := send.NewInternalLogger("", send.LevelInfo{Threshold: level.Info, Default: level.Info})
		require.NoError(t, err)

		expansions := util.NewDynamicExpansions(map[string]string{})
		s := NewRedactingSender(wrappedSender, RedactionOptions{
			Expansions: expansions,
		})

		expansions.PutAndRedact("github_token", "ghp_secret123")

		s.Send(context.Background(), message.NewDefaultMessage(level.Info, "token is ghp_secret123"))
		assert.Equal(t, fmt.Sprintf("token is %s", fmt.Sprintf(redactedVariableTemplate, "github_token")), wrappedSender.GetMessage().Message.String())
	})

	t.Run("WithPreloadDoesNotPickUpNewRedactions", func(t *testing.T) {
		wrappedSender, err := send.NewInternalLogger("", send.LevelInfo{Threshold: level.Info, Default: level.Info})
		require.NoError(t, err)

		expansions := util.NewDynamicExpansions(map[string]string{"existing_key": "existing_val"})
		expansions.PutAndRedact("existing_key", "existing_val")
		s := NewRedactingSender(wrappedSender, RedactionOptions{
			Expansions:        expansions,
			PreloadRedactions: true,
		})

		expansions.PutAndRedact("github_token", "ghp_secret123")

		s.Send(context.Background(), message.NewDefaultMessage(level.Info, "token is ghp_secret123"))
		assert.Equal(t, "token is ghp_secret123", wrappedSender.GetMessage().Message.String())
	})
}
