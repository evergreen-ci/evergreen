package bad_test

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFail01(t *testing.T) {
	Convey("This should fail", t, func() {
		time.Sleep(40 * time.Millisecond)
		So(1, ShouldEqual, -1)
	})
}

func TestFail02(t *testing.T) {
	Convey("This should fail", t, func() {
		time.Sleep(555 * time.Millisecond)
		So("Bugs", ShouldEqual, "maybe the easter bunny")
	})
}

func TestSkip01(t *testing.T) {
	time.Sleep(100 * time.Millisecond)
	t.SkipNow()
}

func TestPass01(t *testing.T) {
	Convey("This should pass", t, func() {
		time.Sleep(5 * time.Millisecond)
		So(nil, ShouldBeNil)
	})
}

func TestFail03(t *testing.T) {
	Convey("This should panic", t, func() {
		time.Sleep(252 * time.Millisecond)
		panic("BOOM")
	})
}
