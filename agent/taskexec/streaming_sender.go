package taskexec

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
)

// StreamChannel identifies the type of log line in the NDJSON stream.
type StreamChannel string

const (
	// TaskChannel is for command stdout/stderr output.
	TaskChannel StreamChannel = "task"
	// ExecChannel is for agent framework messages (step started/completed).
	ExecChannel StreamChannel = "exec"
	// DoneChannel signals step completion with status.
	DoneChannel StreamChannel = "done"
)

// StreamLine represents a single line in the NDJSON stream.
type StreamLine struct {
	Channel    StreamChannel `json:"ch"`
	Step       int           `json:"step"`
	StepNumber string        `json:"step_number,omitempty"`
	Message    string        `json:"msg,omitempty"`
	Success    *bool         `json:"success,omitempty"`
	DurationMs *int64        `json:"duration_ms,omitempty"`
	NextStep   *int          `json:"next_step,omitempty"`
	Error      string        `json:"error,omitempty"`
}

// streamWriter is a shared writer that writes NDJSON lines to an HTTP response
// and optionally to a local log file. It uses http.Flusher to push data immediately.
type streamWriter struct {
	mu         sync.Mutex
	w          io.Writer
	flusher    http.Flusher
	logFile    *logFile
	step       int
	stepNumber string
	closed     bool
}

// newStreamWriter creates a writer backed by an HTTP response writer.
// If flusher is available, each write is flushed immediately.
// logFile is optional and may be nil.
func newStreamWriter(w io.Writer, flusher http.Flusher, logFile *logFile, step int) *streamWriter {
	return &streamWriter{
		w:       w,
		flusher: flusher,
		logFile: logFile,
		step:    step,
	}
}

// NewStreamWriterExported creates a stream writer for use by the daemon HTTP handlers.
func NewStreamWriterExported(w io.Writer, flusher http.Flusher, lf *LogFileHandle, step int) *streamWriter {
	var inner *logFile
	if lf != nil {
		inner = lf.inner
	}
	return newStreamWriter(w, flusher, inner, step)
}

// Close marks the stream writer as closed so that subsequent writes to the
// HTTP response are silently dropped.
func (sw *streamWriter) Close() {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.closed = true
}

// SetStep updates the current step index for subsequent writes.
func (sw *streamWriter) SetStep(step int) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.step = step
}

// SetStepNumber updates the current step number string for subsequent writes.
func (sw *streamWriter) SetStepNumber(stepNumber string) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.stepNumber = stepNumber
}

// WriteLine writes a StreamLine as NDJSON to the response and log file.
func (sw *streamWriter) WriteLine(line StreamLine) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if sw.logFile != nil {
		stepStr := line.StepNumber
		if stepStr == "" {
			stepStr = fmt.Sprintf("%d", line.Step)
		}
		sw.logFile.writeLogLine(stepStr, line.Message)
	}

	if sw.closed {
		return
	}

	data, err := json.Marshal(line)
	if err != nil {
		return
	}
	data = append(data, '\n')

	if _, err := sw.w.Write(data); err != nil {
		sw.closed = true
		return
	}
	if sw.flusher != nil {
		sw.flusher.Flush()
	}

}

// WriteChannelMessage is a convenience method for writing a message on a given channel.
func (sw *streamWriter) WriteChannelMessage(ch StreamChannel, msg string) {
	sw.WriteLine(StreamLine{
		Channel:    ch,
		Step:       sw.step,
		StepNumber: sw.stepNumber,
		Message:    msg,
	})
}

// WriteDone writes a completion message for the current step.
func (sw *streamWriter) WriteDone(success bool, durationMs int64, nextStep int, errMsg string) {
	line := StreamLine{
		Channel:    DoneChannel,
		Step:       sw.step,
		StepNumber: sw.stepNumber,
		Success:    &success,
		DurationMs: &durationMs,
		NextStep:   &nextStep,
	}
	if errMsg != "" {
		line.Error = errMsg
	}
	sw.WriteLine(line)
}

// streamingSender implements send.Sender and writes messages as NDJSON to a streamWriter.
type streamingSender struct {
	*send.Base
	channel StreamChannel
	sw      *streamWriter
}

func newStreamingSender(name string, channel StreamChannel, sw *streamWriter) *streamingSender {
	s := &streamingSender{
		Base:    send.NewBase(name),
		channel: channel,
		sw:      sw,
	}
	_ = s.SetLevel(send.LevelInfo{Default: level.Trace, Threshold: level.Trace})
	return s
}

func (s *streamingSender) Send(m message.Composer) {
	if !m.Loggable() {
		return
	}
	s.sw.WriteChannelMessage(s.channel, m.String())
}

func (s *streamingSender) Flush(_ context.Context) error { return nil }

// streamingLoggerProducer implements LoggerProducer backed by a single
// streaming sender. All logger channels (Task, Execution, System) write
// to the same stream.
type streamingLoggerProducer struct {
	sender send.Sender
	sw     *streamWriter
	closed bool
	mu     sync.Mutex
}

// newStreamingLoggerProducer creates a logger producer that routes all logs to the HTTP stream.
func newStreamingLoggerProducer(sw *streamWriter) *streamingLoggerProducer {
	return &streamingLoggerProducer{
		sender: newStreamingSender("output", TaskChannel, sw),
		sw:     sw,
	}
}

func (p *streamingLoggerProducer) Execution() send.Sender { return p.sender }
func (p *streamingLoggerProducer) Task() send.Sender      { return p.sender }
func (p *streamingLoggerProducer) System() send.Sender    { return p.sender }
func (p *streamingLoggerProducer) StreamWriter() *streamWriter {
	return p.sw
}

func (p *streamingLoggerProducer) Flush(_ context.Context) error { return nil }

func (p *streamingLoggerProducer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	return nil
}

func (p *streamingLoggerProducer) Closed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed
}

func (p *streamingLoggerProducer) setSender(s send.Sender) {
	p.sender = s
}
