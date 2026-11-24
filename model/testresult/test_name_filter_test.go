package testresult

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTestNameFilter(t *testing.T) {
	t.Run("AddedTestsShouldBeContainedWithFewFalsePositives", func(t *testing.T) {
		const numTests = 2000
		filter := NewTestNameFilter(t.Context(), numTests)
		for i := range numTests {
			filter.Add(fmt.Sprintf("jstests/core/test%d.js", i))
		}
		for i := range numTests {
			assert.True(t, filter.Contains(fmt.Sprintf("jstests/core/test%d.js", i)))
		}

		const numChecks = 100000
		falsePositives := 0
		for i := range numChecks {
			if filter.Contains(fmt.Sprintf("jstests/core/other%d.js", i)) {
				falsePositives++
			}
		}
		assert.Less(t, float64(falsePositives)/numChecks, 0.02)
	})

	t.Run("SizeShouldBeBounded", func(t *testing.T) {
		assert.Len(t, NewTestNameFilter(t.Context(), 0).Bits, minTestNameFilterTests*testNameFilterBitsPerTest/8)
		assert.Len(t, NewTestNameFilter(t.Context(), 1500).Bits, 1500*testNameFilterBitsPerTest/8)
		assert.Len(t, NewTestNameFilter(t.Context(), 10000000).Bits, maxTestNameFilterTests*testNameFilterBitsPerTest/8)
	})
}
