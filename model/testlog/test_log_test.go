package testlog

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func init() {
	testutil.Setup()
}

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

			Convey("but a nonexistent test log should not be found", func() {
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

	now := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)

	t.Run("DetectsOutOfBounds", func(t *testing.T) {
		deletedCount, err := DeleteTestLogsWithLimit(ctx, env, now, maxDeleteCount+1)
		assert.Error(t, err)
		assert.Zero(t, deletedCount)
	})
	t.Run("QueryValidation", func(t *testing.T) {
		require.NoError(t, db.Clear(TestLogCollection))
		require.NoError(t, db.Insert(TestLogCollection, bson.M{"_id": primitive.NewObjectIDFromTimestamp(now.Add(time.Hour)).Hex()}))
		require.NoError(t, db.Insert(TestLogCollection, bson.M{"_id": primitive.NewObjectIDFromTimestamp(now.Add(-time.Hour)).Hex()}))

		num, err := db.Count(TestLogCollection, bson.M{})
		require.NoError(t, err)
		assert.Equal(t, 2, num)

		num, err = DeleteTestLogsWithLimit(ctx, env, now, 2)
		assert.NoError(t, err)
		assert.Equal(t, 1, num)

		num, err = db.Count(TestLogCollection, bson.M{})
		require.NoError(t, err)
		assert.Equal(t, 1, num)
	})
}
