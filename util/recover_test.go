package util

import (
	"os"
	"runtime"
	"testing"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
	. "github.com/smartystreets/goconvey/convey"
)

func TestRecoverHandler(t *testing.T) {
	if runtime.Compiler == "gccgo" && runtime.GOARCH == "s390x" {
		t.Skip("test encounters runtime bug on gccgo on zLinux; see EVG-1689")
	}
	grip.Error(os.Setenv("EVERGREEN_TEST", "true"))

	Convey("Recover handler should handle panics", t, func() {
		Convey("without a panic there should be no log messages", func() {
			sender := send.MakeInternalLogger()
			So(grip.SetSender(sender), ShouldBeNil)
			RecoverAndLogStackTrace()
			So(grip.SetSender(send.MakeNative()), ShouldBeNil)
			msg := sender.GetMessage()
			So(msg, ShouldBeNil)

		})

		Convey("with a panic there should be log messages", func() {
			sender := send.MakeInternalLogger()
			So(grip.SetSender(sender), ShouldBeNil)

			func() {
				defer RecoverAndLogStackTrace()
				panic("sorry")
			}()

			msgs := []interface{}{}
			So(grip.SetSender(send.MakeNative()), ShouldBeNil)

			for {
				m := sender.GetMessage()
				if m == nil {
					break
				}

				msgs = append(msgs, m)
			}

			So(len(msgs), ShouldBeGreaterThan, 2)
		})

	})
}
