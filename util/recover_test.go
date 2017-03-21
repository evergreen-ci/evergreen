package util

import (
	"os"
	"testing"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
	. "github.com/smartystreets/goconvey/convey"
)

func TestRecoverHandler(t *testing.T) {
	grip.Error(os.Setenv("EVERGREEN_TEST", "true"))

	Convey("Recover handler should handle panics", t, func() {
		Convey("without a panic there should be no log messages", func() {
			sender := send.MakeInternalLogger()
			grip.SetSender(sender)
			RecoverAndLogStackTrace()
			grip.SetSender(send.MakeNative())
			msg := sender.GetMessage()
			So(msg, ShouldBeNil)

		})

		Convey("with a panic there should be log messages", func() {
			sender := send.MakeInternalLogger()
			grip.SetSender(sender)

			defer RecoverAndLogStackTrace()
			panic("sorry")
			msgs := []interface{}{}
			grip.SetSender(send.MakeNative())

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
