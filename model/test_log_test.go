package model

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestTestLogInsertAndFind(t *testing.T) {
	Convey("With a test log", t, func() {

		testutil.HandleTestingErr(
			db.Clear(TestLogCollection), t,
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
