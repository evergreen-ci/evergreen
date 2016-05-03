package util

import (
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRunFunctionWithTimeout(t *testing.T) {

	Convey("When running a function with a timeout", t, func() {

		funcDone := false

		Convey("if the function times out, ErrTimedOut should be"+
			" returned", func() {
			f := func() error {
				time.Sleep(time.Minute)
				funcDone = true
				return nil
			}
			So(RunFunctionWithTimeout(f, time.Millisecond*500), ShouldResemble,
				ErrTimedOut)
			So(funcDone, ShouldBeFalse)
		})

		Convey("if the inner function returns an error, the error should not"+
			" be swallowed", func() {
			funcErr := fmt.Errorf("I'm an error")
			f := func() error {
				return funcErr
			}
			So(RunFunctionWithTimeout(f, time.Minute), ShouldResemble, funcErr)
		})

		Convey("if the function does not return an error, nil should be"+
			" returned", func() {
			f := func() error {
				funcDone = true
				return nil
			}
			So(RunFunctionWithTimeout(f, time.Minute), ShouldBeNil)
			So(funcDone, ShouldBeTrue)
		})

	})

}
