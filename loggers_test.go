package evergreen

import (
	"strings"
	"testing"

	"github.com/mongodb/grip/send"
	"github.com/mongodb/grip/slogger"
	. "github.com/smartystreets/goconvey/convey"
)

func TestLoggingWriter(t *testing.T) {
	Convey("With a Logger", t, func() {
		Convey("data written to Writer should match data appended", func() {
			sender := send.MakeInternalLogger()
			testLogger := &slogger.Logger{
				Name:      "",
				Appenders: []send.Sender{sender},
			}
			logWriter := NewInfoLoggingWriter(testLogger)

			testLogLine := "blah blah %x %fkjabsddg"
			_, err := logWriter.Write([]byte(testLogLine + "\n"))
			So(err, ShouldBeNil)
			So(sender.Len(), ShouldEqual, 1)
			So(strings.HasSuffix(sender.GetMessage().Rendered, testLogLine), ShouldBeTrue)
		})

		Convey("writer should cache data until a new line is sent", func() {
			sender := send.MakeInternalLogger()
			testLogger := &slogger.Logger{
				Name:      "",
				Appenders: []send.Sender{sender},
			}
			logWriter := NewInfoLoggingWriter(testLogger)

			msgs := []string{"foo bar", "bar", "foo", "baz baz baz!!!\n"}
			for _, msg := range msgs {
				content := []byte(msg)
				n, err := logWriter.Write(content)
				So(err, ShouldBeNil)
				So(n, ShouldEqual, len(content))

			}
			So(sender.Len(), ShouldEqual, 1)
			So(sender.GetMessage().Rendered, ShouldEndWith, strings.Trim(strings.Join(msgs, ""), "\n"))
		})
	})

}
