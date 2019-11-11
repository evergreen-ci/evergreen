package ftdc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSampleIterator(t *testing.T) {
	t.Run("CanceledContextCreator", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		chunk := &Chunk{
			nPoints: 2,
		}
		out := chunk.streamDocuments(ctx)
		assert.NotNil(t, out)
		for {
			doc, ok := <-out
			if ok {
				continue
			}

			assert.False(t, ok)
			assert.Nil(t, doc)
			break
		}

	})
	t.Run("CloserOperations", func(t *testing.T) {
		iter := &sampleIterator{}
		assert.Panics(t, func() {
			iter.Close()
		})
		counter := 0
		iter.closer = func() { counter++ }
		assert.NotPanics(t, func() {
			iter.Close()
		})
		assert.Equal(t, 1, counter)

	})

}
