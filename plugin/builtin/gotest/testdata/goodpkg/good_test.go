package goodpkg

import (
	. "github.com/smartystreets/goconvey/convey"
	"testing"
	"time"
)

func TestPass01(t *testing.T) {
	Convey("This should pass", t, func() {
		time.Sleep(40 * time.Millisecond)
		So(ReturnOne(), ShouldEqual, 1)
	})
}

func TestPass02(t *testing.T) {
	Convey("This should pass also", t, func() {
		time.Sleep(222 * time.Millisecond)
		So(-1, ShouldEqual, -1)
	})
}
