package model

import (
	"sync"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestTestLogInsertAndFind(t *testing.T) {
	Convey("With a test log", t, func() {

		require.NoError(t, db.Clear(TestLogCollection),
			"error clearing test log collection")

		log := &TestLog{
			Name:          "TestNothing",
			Task:          "TestTask1000",
			TaskExecution: 5,
			Lines: []string{
				"did some stuff",
				"did some other stuff",
				"finished doing stuff",
			},
		}

		Convey("inserting that test log into the db", func() {
			err := log.Insert()
			So(err, ShouldBeNil)

			Convey("the test log should be findable in the db", func() {
				logFromDB, err := FindOneTestLog("TestNothing", "TestTask1000", 5)
				So(err, ShouldBeNil)
				So(logFromDB, ShouldResemble, log)
			})

			Convey("but a nonexistant test log should not be found", func() {
				logFromDB, err := FindOneTestLog("blech", "blah", 1)
				So(logFromDB, ShouldBeNil)
				So(err, ShouldBeNil)
			})

		})

	})

}

func TestDeleteTestLogsWithLimit(t *testing.T) {
	env := evergreen.GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()

	t.Run("DetectsOutOfBounds", func(t *testing.T) {
		assert.Panics(t, func() {
			_, _ = DeleteTestLogsWithLimit(ctx, env, time.Now(), 200*1000)
		})
		assert.NotPanics(t, func() {
			_, _ = DeleteTestLogsWithLimit(ctx, env, time.Now(), 1)
		})
	})
	t.Run("QueryValidation", func(t *testing.T) {
		require.NoError(t, db.Clear(TestLogCollection))
		require.NoError(t, db.Insert(TestLogCollection, bson.M{"_id": primitive.NewObjectIDFromTimestamp(time.Now().Add(time.Hour)).Hex()}))
		require.NoError(t, db.Insert(TestLogCollection, bson.M{"_id": primitive.NewObjectIDFromTimestamp(time.Now().Add(-time.Hour)).Hex()}))

		num, err := db.Count(TestLogCollection, bson.M{})
		require.NoError(t, err)
		assert.Equal(t, 2, num)

		num, err = DeleteTestLogsWithLimit(ctx, env, time.Now(), 2)
		assert.NoError(t, err)
		assert.Equal(t, 1, num)

		num, err = db.Count(TestLogCollection, bson.M{})
		require.NoError(t, err)
		assert.Equal(t, 1, num)
	})
	t.Run("Parallel", func(t *testing.T) {
		require.NoError(t, db.Clear(TestLogCollection))
		for i := 0; i < 1000; i++ {
			if i%2 == 0 {
				require.NoError(t, db.Insert(TestLogCollection, bson.M{"_id": primitive.NewObjectIDFromTimestamp(time.Now().Add(time.Hour)).Hex()}))
			} else {
				require.NoError(t, db.Insert(TestLogCollection, bson.M{"_id": primitive.NewObjectIDFromTimestamp(time.Now().Add(-time.Hour)).Hex()}))
			}
		}
		num, err := db.Count(TestLogCollection, bson.M{})
		require.NoError(t, err)
		assert.Equal(t, 1000, num)

		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, delErr := DeleteTestLogsWithLimit(ctx, env, time.Now(), 100)
				require.NoError(t, delErr)
			}()
		}
		wg.Wait()

		num, err = db.Count(TestLogCollection, bson.M{})
		require.NoError(t, err)
		assert.Equal(t, 500, num)
	})
}
