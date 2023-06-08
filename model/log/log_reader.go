package log

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// LogReader implements the io.Reader interface for log lines with additional
// utility functionality.
type LogReader interface {
	// NextTimestamp returns the Unix nanosecond timestamp of the next
	// unread line, if applicable. This should be called after the reader
	// is exhausted, succesfully returning an io.EOF error. Useful for
	// pagination.
	NextTimestamp() *int64

	io.Reader
}

// LogIteratorReaderOptions describes the options for creating a LogReader.
type LogIteratorReaderOptions struct {
	// LineLimit limits the number of lines read from the log. Ignored if
	// TailN is also set.
	LineLimit int
	// TailN is the number of lines to read from the tail of the log.
	TailN int
	// PrintTime, when true, prints the timestamp of each log line along
	// with the line in the following format:
	//		[2006/01/02 15:04:05.000] This is a log line.
	PrintTime bool
	// PrintPriority, when true, prints the priority of each log line along
	// with the line in the following format:
	//		[P: 30] This is a log line.
	// If PrintTime is also set to true, priority will be printed first:
	//		[P:100] [2006/01/02 15:04:05.000] This is a log line.
	PrintPriority bool
	// SoftSizeLimit assists with pagination of long logs. When set the
	// reader will attempt to read as close to the limit as possible while
	// also reading every line for each timestamp reached. Ignored if TailN
	// is also set.
	SoftSizeLimit int
}

// NewlogIteratorReader returns a LogReader that reads the log lines from the
// LogIterator with the given options.
func NewLogIteratorReader(ctx context.Context, it LogIterator, opts LogIteratorReaderOptions) LogReader {
	if opts.TailN > 0 {
		if !it.IsReversed() {
			it = it.Reverse()
		}

		return &logIteratorTailReader{
			ctx:  ctx,
			it:   it,
			opts: opts,
		}
	}

	return &logIteratorReader{
		ctx:  ctx,
		it:   it,
		opts: opts,
	}
}

type logIteratorReader struct {
	ctx            context.Context
	it             LogIterator
	opts           LogIteratorReaderOptions
	lineCount      int
	leftOver       []byte
	totalBytesRead int
	lastItem       LogLine
}

func (r *logIteratorReader) NextTimestamp() *int64 {
	if r.it.Exhausted() {
		return nil
	}
	return utility.ToInt64Ptr(r.it.Item().Timestamp)
}

func (r *logIteratorReader) Read(p []byte) (int, error) {
	n := 0

	if r.leftOver != nil {
		data := r.leftOver
		r.leftOver = nil
		n = r.writeToBuffer(data, p, n)
		if n == len(p) {
			return n, nil
		}
	}

	for r.it.Next(r.ctx) {
		r.lineCount++
		if r.opts.LineLimit > 0 && r.lineCount > r.opts.LineLimit {
			break
		}
		if r.opts.SoftSizeLimit > 0 && r.totalBytesRead >= r.opts.SoftSizeLimit && r.lastItem.Timestamp != r.it.Item().Timestamp {
			break
		}

		r.lastItem = r.it.Item()
		data := r.it.Item().Data
		if r.opts.PrintTime {
			data = fmt.Sprintf("[%s] %s", time.Unix(0, r.it.Item().Timestamp).UTC().Format("2006/01/02 15:04:05.000"), data)
		}
		if r.opts.PrintPriority {
			data = fmt.Sprintf("[P:%3d] %s", r.it.Item().Priority, data)
		}
		n = r.writeToBuffer([]byte(data), p, n)
		if n == len(p) {
			return n, nil
		}
	}

	catcher := grip.NewBasicCatcher()
	catcher.Add(r.it.Err())
	catcher.Add(r.it.Close())
	if catcher.HasErrors() {
		return n, catcher.Resolve()
	}

	return n, io.EOF
}

func (r *logIteratorReader) writeToBuffer(data, buffer []byte, n int) int {
	if len(buffer) == 0 {
		return 0
	}

	m := len(data)
	if n+m > len(buffer) {
		m = len(buffer) - n
		r.leftOver = data[m:]
	}
	_ = copy(buffer[n:n+m], data[:m])

	r.totalBytesRead += m

	return n + m
}

type logIteratorTailReader struct {
	ctx  context.Context
	it   LogIterator
	opts LogIteratorReaderOptions
	r    io.Reader
}

func (r *logIteratorTailReader) NextTimestamp() *int64 {
	if r.it.Exhausted() {
		return nil
	}

	return utility.ToInt64Ptr(r.it.Item().Timestamp)
}

func (r *logIteratorTailReader) Read(p []byte) (int, error) {
	if r.r == nil {
		if err := r.getReader(); err != nil {
			return 0, errors.Wrap(err, "reading data")
		}
	}

	if len(p) == 0 {
		return 0, io.EOF
	}

	return r.r.Read(p)
}

func (r *logIteratorTailReader) getReader() error {
	var lines string
	for i := 0; i < r.opts.TailN && r.it.Next(r.ctx); i++ {
		data := r.it.Item().Data
		if r.opts.PrintTime {
			data = fmt.Sprintf("[%s] %s", time.Unix(0, r.it.Item().Timestamp).UTC().Format("2006/01/02 15:04:05.000"), data)
		}
		if r.opts.PrintPriority {
			data = fmt.Sprintf("[P:%3d] %s", r.it.Item().Priority, data)
		}
		lines = data + lines
	}

	catcher := grip.NewBasicCatcher()
	catcher.Add(r.it.Err())
	catcher.Add(r.it.Close())

	r.r = strings.NewReader(lines)

	return catcher.Resolve()
}
