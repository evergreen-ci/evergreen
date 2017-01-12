package evergreen

import (
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/tychoish/grip/send"
	"github.com/tychoish/grip/slogger"
)

func TestLoggingWriter(t *testing.T) {
	Convey("With a Logger", t, func() {
		sender := send.MakeInternalLogger()
		testLogger := &slogger.Logger{
			Name:      "",
			Appenders: []send.Sender{sender},
		}

		logWriter := NewInfoLoggingWriter(testLogger)

		testLogLine := "blah blah %x %fkjabsddg"
		logWriter.Write([]byte(testLogLine))
		Convey("data written to Writer should match data appended", func() {
			So(sender.Len(), ShouldEqual, 1)

			So(strings.HasSuffix(sender.GetMessage().Rendered, testLogLine), ShouldBeTrue)
		})
	})
}
