package cost

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
