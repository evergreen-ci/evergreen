package send

import (
	"testing"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnnotatingSender(t *testing.T) {
	insend, err := NewInternalLogger("annotatingSender", LevelInfo{Threshold: level.Debug, Default: level.Debug})
	require.NoError(t, err)

	annotate := NewAnnotatingSender(insend, map[string]interface{}{"a": "b"})

	annotate.Send(message.NewSimpleFields(level.Notice, message.Fields{"b": "a"}))
	msg, ok := insend.GetMessageSafe()
	require.True(t, ok)
	assert.Contains(t, msg.Rendered, "a='b'")
	assert.Contains(t, msg.Rendered, "b='a'")

}
