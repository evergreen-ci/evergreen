package util

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
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

func TestSplitCommas(t *testing.T) {
	for testName, testCase := range map[string]func(t *testing.T){
		"ReturnsUnmodifiedStringsWithoutCommas": func(t *testing.T) {
			input := []string{"foo", "bar", "bat"}
			assert.Equal(t, input, SplitCommas(input))
		},
		"ReturnsSplitCommaStrings": func(t *testing.T) {
			input := []string{"foo,bar", "bat", "baz,qux,quux"}
			expected := []string{"foo", "bar", "bat", "baz", "qux", "quux"}
			assert.Equal(t, expected, SplitCommas(input))
		},
	} {
		t.Run(testName, func(t *testing.T) {
			testCase(t)
		})
	}
}

func TestStringSliceSymmetricDifference(t *testing.T) {
	a := []string{"a", "c", "f", "n", "q"}
	b := []string{"q", "q", "g", "y", "a"}

	onlyA, onlyB := StringSliceSymmetricDifference(a, b)
	assert.EqualValues(t, []string{"c", "f", "n"}, onlyA)
	assert.EqualValues(t, []string{"g", "y"}, onlyB)

	onlyB, onlyA = StringSliceSymmetricDifference(b, a)
	assert.EqualValues(t, []string{"c", "f", "n"}, onlyA)
	assert.EqualValues(t, []string{"g", "y"}, onlyB)

	onlyA, onlyB = StringSliceSymmetricDifference(a, a)
	assert.Equal(t, []string{}, onlyA)
	assert.Equal(t, []string{}, onlyB)

	empty1, empty2 := StringSliceSymmetricDifference([]string{}, []string{})
	assert.Equal(t, []string{}, empty1)
	assert.Equal(t, []string{}, empty2)
}
