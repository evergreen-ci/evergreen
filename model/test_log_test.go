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
	mgobson "gopkg.in/mgo.v2/bson"
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

func TestDeleteWithLimit(t *testing.T) {
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
		require.NoError(t, (&TestLog{Id: mgobson.ObjectIdHex(primitive.NewObjectIDFromTimestamp(time.Now().Add(time.Hour)).Hex()).Hex()}).Insert())
		require.NoError(t, (&TestLog{Id: mgobson.ObjectIdHex(primitive.NewObjectIDFromTimestamp(time.Now().Add(-time.Hour)).Hex()).Hex()}).Insert())

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
		for i := 0; i < 10000; i++ {
			if i%2 == 0 {
				require.NoError(t, (&TestLog{Id: mgobson.ObjectIdHex(primitive.NewObjectIDFromTimestamp(time.Now().Add(time.Hour)).Hex()).Hex()}).Insert())
			} else {
				require.NoError(t, (&TestLog{Id: mgobson.ObjectIdHex(primitive.NewObjectIDFromTimestamp(time.Now().Add(-time.Hour)).Hex()).Hex()}).Insert())
			}
		}
		num, err := db.Count(TestLogCollection, bson.M{})
		require.NoError(t, err)
		assert.Equal(t, 10000, num)

		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, delErr := DeleteTestLogsWithLimit(ctx, env, time.Now(), 1000)
				require.NoError(t, delErr)
			}()
		}
		wg.Wait()

		num, err = db.Count(TestLogCollection, bson.M{})
		require.NoError(t, err)
		assert.Equal(t, 5000, num)
	})
}
