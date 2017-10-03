package util

import (
	"errors"
	"os"
	"runtime"
	"strings"
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
			RecoverLogStackTraceAndExit()
			So(grip.SetSender(send.MakeNative()), ShouldBeNil)
			msg := sender.GetMessage()
			So(msg, ShouldBeNil)

		})

		Convey("with a panic-and-exit there should be log messages", func() {
			sender := send.MakeInternalLogger()
			So(grip.SetSender(sender), ShouldBeNil)

			func() {
				defer RecoverLogStackTraceAndExit()
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

			So(len(msgs), ShouldEqual, 1)
		})

		Convey("with a panic-and-continue there should be log messages", func() {
			sender := send.MakeInternalLogger()
			So(grip.SetSender(sender), ShouldBeNil)

			func() {

				defer RecoverLogStackTraceAndContinue("op")
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

			So(len(msgs), ShouldEqual, 1)
		})

		Convey("recover and error should modify error var, if the error is set", func() {
			var err error
			So(err, ShouldBeNil)
			waiter := make(chan struct{})
			err = func() (err error) {
				err = errors.New("foo")
				defer close(waiter)
				defer func() { err = HandlePanicWithError(recover(), err, "error set") }()
				panic("sorry one")
			}()

			<-waiter
			So(err, ShouldNotBeNil)
			So(strings.Contains(err.Error(), "sorry"), ShouldBeTrue)
			So(strings.Contains(err.Error(), "foo"), ShouldBeTrue)
		})

		Convey("recover and error should modify error var, if the error is not set", func() {
			var err error
			So(err, ShouldBeNil)
			waiter := make(chan struct{})
			err = func() (err error) {
				defer close(waiter)
				defer func() { err = HandlePanicWithError(recover(), err, "error not set") }()
				panic("sorry two")
			}()

			<-waiter
			So(err, ShouldNotBeNil)
			So(strings.Contains(err.Error(), "sorry"), ShouldBeTrue)
		})

	})
}
