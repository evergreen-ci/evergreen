package log

import (
	"fmt"
	"io"
	"time"

	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
)

// LogIteratorReader implements the io.Reader interface for log lines with
// additional utility functionality.
type LogIteratorReader struct {
	it             LogIterator
	opts           LogIteratorReaderOptions
	leftOver       []byte
	totalBytesRead int
	lastItem       LogLine
}

// LogIteratorReaderOptions describes the options for creating a new
// LogIteratorReader.
type LogIteratorReaderOptions struct {
	// PrintTime, when true, prints the timestamp of each log line along
	// with the line in the following format:
	//		[2006/01/02 15:04:05.000] This is a log line.
	PrintTime bool
	// TimeZone is the time zone to use when printing the timestamp of each
	// each log line. Optional. Defaults to UTC.
	TimeZone *time.Location
	// PrintPriority, when true, prints the priority of each log line along
	// with the line in the following format:
	//		[P: 30] This is a log line.
	// If PrintTime is also set to true, priority will be printed first:
	//		[P:100] [2006/01/02 15:04:05.000] This is a log line.
	PrintPriority bool
	// SoftSizeLimit assists with pagination of long logs. When set the
	// reader will attempt to read as close to the limit as possible while
	// also reading every line for each timestamp reached. Optional.
	SoftSizeLimit int
}

// NewLogIteratorReader returns a reader that reads the log lines from the
// iterator with the given options. The reader will attempt to close the
// iterator.
func NewLogIteratorReader(it LogIterator, opts LogIteratorReaderOptions) *LogIteratorReader {
	if opts.TimeZone == nil {
		opts.TimeZone = time.UTC
	}

	return &LogIteratorReader{
		it:   it,
		opts: opts,
	}
}

// NextTimestamp returns the Unix nanosecond timestamp of the next unread line,
// if applicable. This is mostly used for pagination as the next unread line's
// timestamp is the start key of the next page—call after the reader
// successfully returns an io.EOF error for this use case.
func (r *LogIteratorReader) NextTimestamp() *int64 {
	if r.it.Exhausted() {
		return nil
	}
	return utility.ToInt64Ptr(r.it.Item().Timestamp)
}

func (r *LogIteratorReader) Read(p []byte) (int, error) {
	n := 0

	if r.leftOver != nil {
		data := r.leftOver
		r.leftOver = nil
		n = r.writeToBuffer(data, p, n)
		if n == len(p) {
			return n, nil
		}
	}

	for r.it.Next() {
		if r.opts.SoftSizeLimit > 0 && r.totalBytesRead >= r.opts.SoftSizeLimit && r.lastItem.Timestamp != r.it.Item().Timestamp {
			break
		}

		r.lastItem = r.it.Item()
		data := r.it.Item().Data
		if r.opts.PrintTime {
			data = fmt.Sprintf("[%s] %s", time.Unix(0, r.it.Item().Timestamp).In(r.opts.TimeZone).Format("2006/01/02 15:04:05.000"), data)
		}
		if r.opts.PrintPriority {
			data = fmt.Sprintf("[P:%3d] %s", r.it.Item().Priority, data)
		}
		n = r.writeToBuffer([]byte(data+"\n"), p, n)
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

func (r *LogIteratorReader) writeToBuffer(data, buffer []byte, n int) int {
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
