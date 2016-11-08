package slogger

import (
	"bytes"
	"fmt"
	"os"
)

type Appender interface {
	Append(log *Log) error
}

func FormatLog(log *Log) string {
	year, month, day := log.Timestamp.Date()
	hour, min, sec := log.Timestamp.Clock()

	return fmt.Sprintf("[%.4d/%.2d/%.2d %.2d:%.2d:%.2d] [%v.%v] [%v:%d] %v\n",
		year, month, day,
		hour, min, sec,
		log.Prefix, log.Level.Type(),
		log.Filename, log.Line,
		log.Message())
}

type WriteStringer interface {
	WriteString(str string) (int, error)
}

// WriteStringer is implemented by *os.File.
type FileAppender struct {
	WriteStringer
}

func (self FileAppender) Append(log *Log) error {
	_, err := self.WriteString(FormatLog(log))
	return err
}

func StdOutAppender() *FileAppender {
	return &FileAppender{os.Stdout}
}

func StdErrAppender() *FileAppender {
	return &FileAppender{os.Stderr}
}

func DevNullAppender() (*FileAppender, error) {
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		return nil, err
	}

	return &FileAppender{devNull}, nil
}

type StringAppender struct {
	*bytes.Buffer
}

func NewStringAppender(buffer *bytes.Buffer) *StringAppender {
	return &StringAppender{buffer}
}

func (self StringAppender) Append(log *Log) error {
	_, err := self.WriteString(FormatLog(log) + "\n")
	return err
}

// Return true if the log should be passed to the underlying
// `Appender`
type Filter func(log *Log) bool
type FilterAppender struct {
	Appender Appender
	Filter   Filter
}

func (self *FilterAppender) Append(log *Log) error {
	if self.Filter(log) == false {
		return nil
	}

	return self.Appender.Append(log)
}

func LevelFilter(threshold Level, appender Appender) *FilterAppender {
	filterFunc := func(log *Log) bool {
		return log.Level >= threshold
	}

	return &FilterAppender{
		Appender: appender,
		Filter:   filterFunc,
	}
}
