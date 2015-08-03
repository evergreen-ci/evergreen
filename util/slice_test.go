package util

import (
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestStringSliceIntersection(t *testing.T) {
	Convey("With two test string arrays [A B C D], [C D E]", t, func() {
		a := []string{"A", "B", "C", "D"}
		b := []string{"C", "D", "E"}

		Convey("intersection [C D] should be returned", func() {
			So(len(StringSliceIntersection(a, b)), ShouldEqual, 2)
			So(StringSliceIntersection(a, b), ShouldContain, "C")
			So(StringSliceIntersection(a, b), ShouldContain, "D")
		})
	})
}
