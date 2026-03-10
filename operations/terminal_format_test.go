package operations

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/taskexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadAndRenderStream(t *testing.T) {
	t.Run("ParsesNDJSONStream", func(t *testing.T) {
		success := true
		durationMs := int64(1200)
		nextStep := 4
		lines := []taskexec.StreamLine{
			{Channel: taskexec.ExecChannel, Step: 3, Message: "Running 'shell.exec'"},
			{Channel: taskexec.TaskChannel, Step: 3, Message: "+ go test -v ./..."},
			{Channel: taskexec.TaskChannel, Step: 3, Message: "PASS"},
			{Channel: taskexec.DoneChannel, Step: 3, Success: &success, DurationMs: &durationMs, NextStep: &nextStep},
		}

		var input bytes.Buffer
		for _, line := range lines {
			data, err := json.Marshal(line)
			require.NoError(t, err)
			input.Write(data)
			input.WriteByte('\n')
		}

		var output bytes.Buffer
		renderer := newTerminalRendererWithColor(&output, false)
		result, err := readAndRenderStream(&input, renderer)
		require.NoError(t, err)

		assert.NotNil(t, result)
		assert.True(t, result.Success)
		assert.Empty(t, result.Error)

		rendered := output.String()
		assert.Contains(t, rendered, "Running 'shell.exec'")
		assert.Contains(t, rendered, "+ go test -v ./...")
		assert.Contains(t, rendered, "PASS")
	})

	t.Run("HandlesFailure", func(t *testing.T) {
		success := false
		durationMs := int64(500)
		nextStep := 2
		lines := []taskexec.StreamLine{
			{Channel: taskexec.TaskChannel, Step: 2, Message: "error output"},
			{Channel: taskexec.DoneChannel, Step: 2, Success: &success, DurationMs: &durationMs, NextStep: &nextStep, Error: "exit code 1"},
		}

		var input bytes.Buffer
		for _, line := range lines {
			data, _ := json.Marshal(line)
			input.Write(data)
			input.WriteByte('\n')
		}

		var output bytes.Buffer
		renderer := newTerminalRendererWithColor(&output, false)
		result, err := readAndRenderStream(&input, renderer)
		require.NoError(t, err)

		assert.NotNil(t, result)
		assert.False(t, result.Success)
		assert.Equal(t, "exit code 1", result.Error)
	})

	t.Run("HandlesInvalidJSON", func(t *testing.T) {
		input := strings.NewReader("not json\n{\"ch\":\"task\",\"step\":0,\"msg\":\"valid\"}\n")

		var output bytes.Buffer
		renderer := newTerminalRendererWithColor(&output, false)
		result, err := readAndRenderStream(input, renderer)
		require.NoError(t, err)

		rendered := output.String()
		assert.Contains(t, rendered, "not json")
		assert.Contains(t, rendered, "valid")
		assert.Nil(t, result)
	})

	t.Run("HandlesEmptyStream", func(t *testing.T) {
		input := strings.NewReader("")

		var output bytes.Buffer
		renderer := newTerminalRendererWithColor(&output, false)
		result, err := readAndRenderStream(input, renderer)
		require.NoError(t, err)
		assert.Nil(t, result)
	})
}
