package units

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTruncateScriptLogs(t *testing.T) {
	t.Run("ShortLogsAreUnchanged", func(t *testing.T) {
		logs := "line one\nline two\nline three"
		assert.Equal(t, logs, truncateScriptLogs(logs))
	})
	t.Run("LogsAtCapAreUnchanged", func(t *testing.T) {
		logs := strings.Repeat("a", maxScriptLogsSize)
		assert.Equal(t, logs, truncateScriptLogs(logs))
	})
	t.Run("LargeLogsAreTruncatedKeepingHeadAndTail", func(t *testing.T) {
		head := "FIRST_SETUP_STEP"
		tail := "FINAL_ERROR_MESSAGE"
		logs := head + strings.Repeat("x", 2*maxScriptLogsSize) + tail

		truncated := truncateScriptLogs(logs)

		require.Less(t, len(truncated), len(logs))
		assert.LessOrEqual(t, len(truncated), maxScriptLogsSize)
		assert.True(t, strings.HasPrefix(truncated, head), "the beginning of the log should be preserved")
		assert.True(t, strings.HasSuffix(truncated, tail), "the end of the log should be preserved")
		assert.Contains(t, truncated, "[log truncated]")
	})
}
