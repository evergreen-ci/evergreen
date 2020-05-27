package pail

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConstructorLegacyGridFS(t *testing.T) {
	t.Run("NilSessionsFallBack", func(t *testing.T) {
		_, err := NewLegacyGridFSBucketWithSession(nil, GridFSOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "without a URI")
	})
	t.Run("RequireValidURI", func(t *testing.T) {
		_, err := NewLegacyGridFSBucket(GridFSOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "without a URI")
	})
	t.Run("InvalidURIErrors", func(t *testing.T) {
		t.Parallel()
		_, err := NewLegacyGridFSBucket(GridFSOptions{MongoDBURI: "foo"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "problem connecting")
	})
	t.Run("CreateConnectionFromURI", func(t *testing.T) {
		t.Parallel()
		bucket, err := NewLegacyGridFSBucket(GridFSOptions{MongoDBURI: "mongodb://localhost:27017"})
		assert.NoError(t, err)
		assert.NotNil(t, bucket)
	})
	t.Run("CheckCommandEnforcesSession", func(t *testing.T) {
		bucket := &gridfsLegacyBucket{}
		err := bucket.Check(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no session defined")
	})
	t.Run("Iterator", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		iter := &legacyGridFSIterator{
			ctx: ctx,
		}
		t.Run("ErrorsWithCancledContext", func(t *testing.T) {
			tctx, tcancel := context.WithCancel(ctx)
			tcancel()
			assert.False(t, iter.Next(tctx))
		})
		t.Run("ErrorsIfContext", func(t *testing.T) {
			cancel()
			ctx := context.Background()
			assert.False(t, iter.Next(ctx))
		})
	})

}
