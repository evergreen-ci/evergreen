package utility

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStringSliceIntersection(t *testing.T) {
	a := []string{"A", "B", "C", "D"}
	b := []string{"C", "D", "E"}

	assert.Equal(t, 2, len(StringSliceIntersection(a, b)))
	assert.Contains(t, StringSliceIntersection(a, b), "C")
	assert.Contains(t, StringSliceIntersection(a, b), "D")
}

func TestUniqueStrings(t *testing.T) {
	assert.EqualValues(t, []string{"a", "b", "c", "d", "e"},
		UniqueStrings([]string{"a", "b", "c", "a", "a", "d", "d", "e"}))

	assert.EqualValues(t, []string{"a", "b", "c"},
		UniqueStrings([]string{"a", "b", "c"}))
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
	assert.Contains(t, onlyA, "c")
	assert.Contains(t, onlyA, "f")
	assert.Contains(t, onlyA, "n")
	assert.Len(t, onlyA, 3)
	assert.Contains(t, onlyB, "g")
	assert.Contains(t, onlyB, "y")
	assert.Len(t, onlyB, 2)

	onlyB, onlyA = StringSliceSymmetricDifference(b, a)
	assert.Contains(t, onlyA, "c")
	assert.Contains(t, onlyA, "f")
	assert.Contains(t, onlyA, "n")
	assert.Len(t, onlyA, 3)
	assert.Contains(t, onlyB, "g")
	assert.Contains(t, onlyB, "y")
	assert.Len(t, onlyB, 2)

	onlyA, onlyB = StringSliceSymmetricDifference(a, a)
	assert.Equal(t, []string{}, onlyA)
	assert.Equal(t, []string{}, onlyB)

	empty1, empty2 := StringSliceSymmetricDifference([]string{}, []string{})
	assert.Equal(t, []string{}, empty1)
	assert.Equal(t, []string{}, empty2)
}

func TestGetSetDifference(t *testing.T) {
	assert := assert.New(t)
	setA := []string{"one", "four", "five", "three", "two"}
	setB := []string{"five", "two"}
	difference := GetSetDifference(setA, setB)
	sort.Strings(difference)

	// GetSetDifference returns the elements in A that are not in B
	assert.Equal(3, len(difference))
	assert.Equal("four", difference[0])
	assert.Equal("one", difference[1])
	assert.Equal("three", difference[2])
}

func TestIndexOf(t *testing.T) {
	assert.Equal(t, 3, IndexOf([]string{"a", "b", "c", "d", "e"}, "d"))
	assert.Equal(t, 0, IndexOf([]string{"a", "b", "c", "d", "e"}, "a"))
	assert.Equal(t, -1, IndexOf([]string{"a", "b", "c", "d", "e"}, "f"))
	assert.Equal(t, -1, IndexOf([]string{"a", "b", "c", "d", "e"}, "1"))
	assert.Equal(t, -1, IndexOf([]string{"a", "b", "c", "d", "e"}, "æ"))
}
