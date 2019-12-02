package apm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContext(t *testing.T) {
	t.Run("Simple", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		t.Run("SetTags", func(t *testing.T) {
			ctx = SetTags(ctx, "one")
			assert.NotNil(t, ctx.Value(tagsContextKey))
		})
		t.Run("GetTags", func(t *testing.T) {
			tags := GetTags(ctx)
			require.Len(t, tags, 1)
			assert.Equal(t, "one", tags[0])
		})
	})
	t.Run("NilGet", func(t *testing.T) {
		ctx := context.Background()
		var tags []string
		assert.NotPanics(t, func() { tags = GetTags(ctx) })
		require.Len(t, tags, 0)
	})
	t.Run("WrongType", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), tagsContextKey, true)

		var tags []string
		assert.NotPanics(t, func() { tags = GetTags(ctx) })
		require.Len(t, tags, 0)
	})
}
