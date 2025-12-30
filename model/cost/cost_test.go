package cost

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCostIsZero(t *testing.T) {
	t.Run("ZeroValues", func(t *testing.T) {
		cost := Cost{}
		assert.True(t, cost.IsZero())
	})

	t.Run("NonZeroOnDemand", func(t *testing.T) {
		cost := Cost{OnDemandEC2Cost: 1.0}
		assert.False(t, cost.IsZero())
	})

	t.Run("NonZeroAdjusted", func(t *testing.T) {
		cost := Cost{AdjustedEC2Cost: 1.0}
		assert.False(t, cost.IsZero())
	})

	t.Run("NonZeroBoth", func(t *testing.T) {
		cost := Cost{OnDemandEC2Cost: 1.5, AdjustedEC2Cost: 1.2}
		assert.False(t, cost.IsZero())
	})
}
