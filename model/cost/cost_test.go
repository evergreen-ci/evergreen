package cost

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSumPerChildVersionAdjustedTotals(t *testing.T) {
	one := Cost{AdjustedEC2Cost: 1.0}
	two := Cost{AdjustedEBSStorageCost: 2.0}
	three := Cost{AdjustedS3LogPutCost: 0.5}
	t.Run("IndicesAndNils", func(t *testing.T) {
		a, p := SumPerChildVersionAdjustedTotals(3, func(i int) (actual, predicted *Cost) {
			switch i {
			case 0:
				return &one, &two
			case 1:
				return &two, nil
			case 2:
				return nil, &three
			}
			return nil, nil
		})
		// 1+2+0 actual; 2+0+0.5 predicted
		assert.InDelta(t, 3.0, a, 1e-9)
		assert.InDelta(t, 2.5, p, 1e-9)
	})
	t.Run("NZero", func(t *testing.T) {
		a, p := SumPerChildVersionAdjustedTotals(0, func(int) (actual, predicted *Cost) {
			return nil, nil
		})
		assert.InDelta(t, 0, a, 1e-9)
		assert.InDelta(t, 0, p, 1e-9)
	})
}

func TestCostTotalAdjusted(t *testing.T) {
	c := Cost{
		AdjustedEC2Cost:               1,
		AdjustedEBSThroughputCost:     2,
		AdjustedEBSStorageCost:        4,
		AdjustedS3ArtifactPutCost:     0.1,
		AdjustedS3LogPutCost:          0.2,
		AdjustedS3ArtifactStorageCost: 0.3,
		AdjustedS3LogStorageCost:      0.4,
		OnDemandEC2Cost:               100,
	}
	assert.InDelta(t, 8.0, c.TotalAdjusted(), 1e-9)

	withChildren := c
	withChildren.ChildPatchesTotalCost = 2
	assert.InDelta(t, 10.0, withChildren.TotalAdjusted(), 1e-9)
}

func TestRoundCost(t *testing.T) {
	t.Run("ZeroInputReturnsZero", func(t *testing.T) {
		assert.Equal(t, 0.0, RoundCost(0))
	})
	t.Run("SmallValueWithFloatNoiseRoundsToTwoSigFigs", func(t *testing.T) {
		// Floating-point noise after the 2nd significant digit should be removed.
		assert.Equal(t, 0.00000037, RoundCost(0.00000036750000000000004))
	})
	t.Run("ValueBelowOneCentRoundsToTwoSigFigs", func(t *testing.T) {
		assert.Equal(t, 0.0087, RoundCost(0.008724004762715199))
	})
	t.Run("SmallValueRoundsUpToTwoSigFigs", func(t *testing.T) {
		assert.Equal(t, 0.0058, RoundCost(0.005819905908980206))
	})
	t.Run("ValueAboveOneCentRoundsToTwoDecimalPlaces", func(t *testing.T) {
		// Values >= 0.01 are rounded to 2 decimal places.
		assert.Equal(t, 1.23, RoundCost(1.2345678))
	})
}

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

	t.Run("NonZeroOnDemandS3ArtifactPutCost", func(t *testing.T) {
		cost := Cost{OnDemandS3ArtifactPutCost: 0.00005}
		assert.False(t, cost.IsZero())
	})

	t.Run("NonZeroOnDemandS3LogPutCost", func(t *testing.T) {
		cost := Cost{OnDemandS3LogPutCost: 0.00003}
		assert.False(t, cost.IsZero())
	})

	t.Run("NonZeroChildPatchesTotalCost", func(t *testing.T) {
		cost := Cost{ChildPatchesTotalCost: 1.0}
		assert.False(t, cost.IsZero())
	})
}

func TestCostJSONIncludesEBSThroughputFieldsWhenZero(t *testing.T) {
	// Adjusted EBS throughput and storage JSON keys omit `omitempty`, so zero values still serialize (for API stability).
	c := Cost{}
	data, err := json.Marshal(c)
	require.NoError(t, err)

	var unmarshaled map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &unmarshaled))

	assert.NotContains(t, unmarshaled, "on_demand_ebs_throughput_cost")
	assert.NotContains(t, unmarshaled, "on_demand_ebs_storage_cost")

	assert.Contains(t, unmarshaled, "adjusted_ebs_throughput_cost")
	assert.Contains(t, unmarshaled, "adjusted_ebs_storage_cost")
	assert.Equal(t, 0.0, unmarshaled["adjusted_ebs_throughput_cost"])
	assert.Equal(t, 0.0, unmarshaled["adjusted_ebs_storage_cost"])
}

func TestCostJSONSerializesTotalWhenSet(t *testing.T) {
	c := Cost{OnDemandEC2Cost: 1, AdjustedEC2Cost: 0.5}
	c.Total = c.TotalAdjusted()
	data, err := json.Marshal(c)
	require.NoError(t, err)
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &m))
	assert.InDelta(t, 0.5, m["total"], 1e-9)
	assert.InDelta(t, 1.0, m["on_demand_ec2_cost"], 1e-9)
}
