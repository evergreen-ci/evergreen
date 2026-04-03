package cost

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	t.Run("NonZeroS3ArtifactPutCost", func(t *testing.T) {
		cost := Cost{S3ArtifactPutCost: 0.00005}
		assert.False(t, cost.IsZero())
	})

	t.Run("NonZeroS3LogPutCost", func(t *testing.T) {
		cost := Cost{S3LogPutCost: 0.00003}
		assert.False(t, cost.IsZero())
	})
}

func TestCostJSONIncludesThroughputWhenZero(t *testing.T) {
	// When throughput cost is legitimately 0 (e.g., GP2 volumes or GP3 at baseline),
	// the API should still include the throughput fields with value 0.
	c := Cost{S3ArtifactPutCost: 0.0004}
	data, err := json.Marshal(c)
	require.NoError(t, err)

	var unmarshaled map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &unmarshaled))

	assert.Contains(t, unmarshaled, "on_demand_ebs_throughput_cost")
	assert.Contains(t, unmarshaled, "adjusted_ebs_throughput_cost")
	assert.Equal(t, 0.0, unmarshaled["on_demand_ebs_throughput_cost"])
	assert.Equal(t, 0.0, unmarshaled["adjusted_ebs_throughput_cost"])
}
