package units

import (
	"sync"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestDeleteCollectionWithLimit(t *testing.T) {
	env := evergreen.GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()

	t.Run("DetectsOutOfBounds", func(t *testing.T) {
		assert.Panics(t, func() {
			j := NewDbCleanupJob(utility.RoundPartOfMinute(2), model.TestLogCollection, false)
			_, _ = j.DeleteCollectionWithLimit(ctx, env, time.Now(), 200*1000)
		})
		assert.NotPanics(t, func() {
			j := NewDbCleanupJob(utility.RoundPartOfMinute(2), model.TestLogCollection, false)
			_, _ = j.DeleteCollectionWithLimit(ctx, env, time.Now(), 1)
		})
	})
	t.Run("QueryValidation", func(t *testing.T) {
		require.NoError(t, db.Clear(model.TestLogCollection))
		require.NoError(t, db.Insert(model.TestLogCollection, bson.M{"_id": primitive.NewObjectIDFromTimestamp(time.Now().Add(time.Hour)).Hex()}))
		require.NoError(t, db.Insert(model.TestLogCollection, bson.M{"_id": primitive.NewObjectIDFromTimestamp(time.Now().Add(-time.Hour)).Hex()}))

		num, err := db.Count(model.TestLogCollection, bson.M{})
		require.NoError(t, err)
		assert.Equal(t, 2, num)

		j := NewDbCleanupJob(utility.RoundPartOfMinute(2), model.TestLogCollection, false)
		num, err = j.DeleteCollectionWithLimit(ctx, env, time.Now(), 2)
		assert.NoError(t, err)
		assert.Equal(t, 1, num)

		num, err = db.Count(model.TestLogCollection, bson.M{})
		require.NoError(t, err)
		assert.Equal(t, 1, num)
	})
	t.Run("Parallel", func(t *testing.T) {
		require.NoError(t, db.Clear(model.TestLogCollection))
		for i := 0; i < 10000; i++ {
			if i%2 == 0 {
				require.NoError(t, db.Insert(model.TestLogCollection, bson.M{"_id": primitive.NewObjectIDFromTimestamp(time.Now().Add(time.Hour)).Hex()}))
			} else {
				require.NoError(t, db.Insert(model.TestLogCollection, bson.M{"_id": primitive.NewObjectIDFromTimestamp(time.Now().Add(-time.Hour)).Hex()}))
			}
		}
		num, err := db.Count(model.TestLogCollection, bson.M{})
		require.NoError(t, err)
		assert.Equal(t, 10000, num)

		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				j := NewDbCleanupJob(utility.RoundPartOfMinute(2), model.TestLogCollection, false)
				_, delErr := j.DeleteCollectionWithLimit(ctx, env, time.Now(), 1000)
				require.NoError(t, delErr)
			}()
		}
		wg.Wait()

		num, err = db.Count(model.TestLogCollection, bson.M{})
		require.NoError(t, err)
		assert.Equal(t, 5000, num)
	})
}
