package util

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMinInts(t *testing.T) {
	Convey("Min should return the minimum of the inputs passed in, or the largest possible int with no inputs", t, func() {
		So(Min(), ShouldEqual, int(^uint(0)>>1))
		So(Min(1), ShouldEqual, 1)
		So(Min(1, 5, 2, -10, 0), ShouldEqual, -10)
	})
}
