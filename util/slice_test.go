package util

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
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

func TestUniqueStrings(t *testing.T) {
	Convey("With a test string slice ", t, func() {
		Convey("[a b c a a d d e] should become [a b c d e]", func() {
			in := []string{"a", "b", "c", "a", "a", "d", "d", "e"}
			out := UniqueStrings(in)
			So(out, ShouldResemble, []string{"a", "b", "c", "d", "e"})
		})
		Convey("[a b c] should remain [a b c]", func() {
			in := []string{"a", "b", "c"}
			out := UniqueStrings(in)
			So(out, ShouldResemble, []string{"a", "b", "c"})
		})
	})
}
