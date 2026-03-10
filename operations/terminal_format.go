package operations

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/evergreen-ci/evergreen/agent/taskexec"
)

// streamResponseResult holds the final status from processing a stream.
type streamResponseResult struct {
	Success bool
	Error   string
}

// readAndRenderStream reads an NDJSON response body and prints each line to out.
// It returns the final result from the "done" message(s). For multi-step execution,
// only the last done message determines overall success.
func readAndRenderStream(body io.Reader, out io.Writer) (*streamResponseResult, error) {
	scanner := bufio.NewScanner(body)
	// Allow large lines (e.g. long command output).
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var lastResult *streamResponseResult

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var sl taskexec.StreamLine
		if err := json.Unmarshal([]byte(line), &sl); err != nil {
			fmt.Fprintln(out, line)
			continue
		}

		switch sl.Channel {
		case taskexec.DoneChannel:
			success := sl.Success != nil && *sl.Success
			lastResult = &streamResponseResult{
				Success: success,
				Error:   sl.Error,
			}
		default:
			if sl.Message != "" {
				fmt.Fprintln(out, sl.Message)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return lastResult, err
	}

	return lastResult, nil
}
