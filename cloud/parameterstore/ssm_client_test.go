package parameterstore

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMakeParamNameBatches(t *testing.T) {
	t.Run("EmptyForNoParamsAndPositiveBatchSize", func(t *testing.T) {
		assert.Empty(t, makeParamNameBatches([]string{}, 5))
	})
	t.Run("EmptyForNoParamsAndInvalidBatchSize", func(t *testing.T) {
		assert.Empty(t, makeParamNameBatches([]string{}, -5))
	})
	t.Run("UsesDefaultBatchSizeForInvalidBatchSize", func(t *testing.T) {
		paramNames := []string{"a", "b", "c"}
		batches := makeParamNameBatches(paramNames, -5)
		require.Len(t, batches, len(paramNames))
		assert.Equal(t, batches[0], []string{"a"})
		assert.Equal(t, batches[1], []string{"b"})
		assert.Equal(t, batches[2], []string{"c"})
	})
	t.Run("CreatesOneBatchForParamsThatFitWithinSingleBatch", func(t *testing.T) {
		paramNames := []string{"a", "b", "c"}
		batches := makeParamNameBatches(paramNames, 5)
		require.Len(t, batches, 1)
		assert.Equal(t, batches[0], paramNames)
	})
	t.Run("CreatesMultipleEvenlySizedBatches", func(t *testing.T) {
		paramNames := []string{"a", "b", "c", "d"}
		batches := makeParamNameBatches(paramNames, 2)
		require.Len(t, batches, 2)
		assert.Equal(t, batches[0], []string{"a", "b"})
		assert.Equal(t, batches[1], []string{"c", "d"})
	})
	t.Run("CreatesMultipleBatchesOfMaxBatchSizeAndOneBatchForRemainder", func(t *testing.T) {
		paramNames := []string{"a", "b", "c", "d", "e"}
		batches := makeParamNameBatches(paramNames, 2)
		require.Len(t, batches, 3)
		assert.Equal(t, batches[0], []string{"a", "b"})
		assert.Equal(t, batches[1], []string{"c", "d"})
		assert.Equal(t, batches[2], []string{"e"})
	})
	t.Run("CreatesProperBatchesForLargeNumberOfParams", func(t *testing.T) {
		const numParams = 1001
		paramNames := make([]string, 0, numParams)
		for i := 0; i < numParams; i++ {
			paramNames = append(paramNames, fmt.Sprintf("%d", i))
		}

		const batchSize = 10
		batches := makeParamNameBatches(paramNames, batchSize)
		require.Len(t, batches, 101)
		for i := 0; i < 100; i++ {
			require.Len(t, batches[i], batchSize)
			assert.Equal(t, batches[i], paramNames[i*batchSize:(i+1)*batchSize])
		}
		require.Len(t, batches[100], 1)
		assert.Equal(t, batches[100], paramNames[1000:])
	})
}
