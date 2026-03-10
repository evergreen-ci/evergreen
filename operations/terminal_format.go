package operations

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/evergreen-ci/evergreen/agent/taskexec"
)

// ANSI color codes
const (
	ansiReset = "\033[0m"
	ansiDim   = "\033[2m"
)

// terminalRenderer handles formatting and rendering of NDJSON stream lines.
type terminalRenderer struct {
	out      io.Writer
	useColor bool
}

func newTerminalRenderer(out io.Writer) *terminalRenderer {
	useColor := true
	if f, ok := out.(*os.File); ok {
		fi, err := f.Stat()
		if err != nil || (fi.Mode()&os.ModeCharDevice) == 0 {
			useColor = false
		}
	} else {
		useColor = false
	}

	if os.Getenv("NO_COLOR") != "" {
		useColor = false
	}

	return &terminalRenderer{
		out:      out,
		useColor: useColor,
	}
}

func newTerminalRendererWithColor(out io.Writer, useColor bool) *terminalRenderer {
	return &terminalRenderer{
		out:      out,
		useColor: useColor,
	}
}

func (r *terminalRenderer) color(code, text string) string {
	if !r.useColor {
		return text
	}
	return code + text + ansiReset
}

// RenderTaskMessage renders a task channel (stdout/stderr) message.
func (r *terminalRenderer) RenderTaskMessage(msg string) {
	fmt.Fprintln(r.out, msg)
}

// RenderExecMessage renders an exec channel message (dimmed).
func (r *terminalRenderer) RenderExecMessage(msg string) {
	fmt.Fprintln(r.out, r.color(ansiDim, msg))
}

// streamResponseResult holds the final status from processing a stream.
type streamResponseResult struct {
	Success bool
	Error   string
}

// readAndRenderStream reads an NDJSON response body and renders each line to the terminal.
// It returns the final result from the "done" message(s). For multi-step execution,
// only the last done message determines overall success.
func readAndRenderStream(body io.Reader, renderer *terminalRenderer) (*streamResponseResult, error) {
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
			// If it's not valid JSON, print it as-is (graceful degradation).
			fmt.Fprintln(renderer.out, line)
			continue
		}

		switch sl.Channel {
		case taskexec.TaskChannel:
			renderer.RenderTaskMessage(sl.Message)
		case taskexec.ExecChannel:
			renderer.RenderExecMessage(sl.Message)
		case taskexec.DoneChannel:
			success := sl.Success != nil && *sl.Success
			lastResult = &streamResponseResult{
				Success: success,
				Error:   sl.Error,
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return lastResult, err
	}

	return lastResult, nil
}
