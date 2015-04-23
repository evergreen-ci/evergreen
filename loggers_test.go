package mci

import (
	"github.com/10gen-labs/slogger/v1"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestLoggingWriter(t *testing.T) {
	Convey("With a LoggingWriter that appends into a SliceAppender", t, func() {
		testAppender := &SliceAppender{}
		testLogger := &slogger.Logger{
			Prefix:    "",
			Appenders: []slogger.Appender{testAppender},
		}

		logWriter := NewInfoLoggingWriter(testLogger)

		testLogLine := "blah blah %x %fkjabsddg"
		logWriter.Write([]byte(testLogLine))
		Convey("data written to Writer should match data appended", func() {
			So(len(testAppender.Messages), ShouldEqual, 1)
			So(testAppender.Messages[0].Message(), ShouldEqual, testLogLine)
		})
	})
}
