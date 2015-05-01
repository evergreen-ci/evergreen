package model

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
	"time"
)

var (
	notifyTimesTestConfig = evergreen.TestConfig()
)

func init() {
	db.SetGlobalSessionProvider(
		db.SessionFactoryFromConfig(notifyTimesTestConfig))
}

func TestSetLastNotificationsEventTime(t *testing.T) {

	Convey("When setting the last notification event time", t, func() {

		util.HandleTestingErr(db.Clear(NotifyTimesCollection),
			t, "Error clearing '%v' collection", NotifyTimesCollection)

		Convey("the last notification time for only the specified project"+
			" should be set to the specified time", func() {

			projectOne := "projectOne"
			projectTwo := "projectTwo"

			timeOne := time.Now()
			timeTwo := timeOne.Add(time.Second)

			So(SetLastNotificationsEventTime(projectOne, timeOne), ShouldBeNil)
			lastEventTime, err := LastNotificationsEventTime(projectOne)
			So(err, ShouldBeNil)
			So(lastEventTime.Round(time.Second).Equal(
				timeOne.Round(time.Second)), ShouldBeTrue)

			So(SetLastNotificationsEventTime(projectTwo, timeTwo), ShouldBeNil)
			lastEventTime, err = LastNotificationsEventTime(projectTwo)
			So(err, ShouldBeNil)
			So(lastEventTime.Round(time.Second).Equal(
				timeTwo.Round(time.Second)), ShouldBeTrue)

			// make sure the time for project one wasn't changed
			lastEventTime, err = LastNotificationsEventTime(projectOne)
			So(err, ShouldBeNil)
			So(lastEventTime.Round(time.Second).Equal(
				timeOne.Round(time.Second)), ShouldBeTrue)

			// now change it
			So(SetLastNotificationsEventTime(projectOne, timeTwo), ShouldBeNil)
			lastEventTime, err = LastNotificationsEventTime(projectOne)
			So(err, ShouldBeNil)
			So(lastEventTime.Round(time.Second).Equal(
				timeTwo.Round(time.Second)), ShouldBeTrue)
		})

	})

}

func TestLastNotificationsEventTime(t *testing.T) {

	Convey("When checking the last notifications event time for a"+
		" project", t, func() {

		util.HandleTestingErr(db.Clear(NotifyTimesCollection),
			t, "Error clearing '%v' collection", NotifyTimesCollection)

		Convey("if there are no times stored, the earliest date to consider"+
			" should be returned", func() {

			projectName := "project"

			lastEventTime, err := LastNotificationsEventTime(projectName)
			So(err, ShouldBeNil)
			So(lastEventTime.Round(time.Second).Equal(
				EarliestDateToConsider.Round(time.Second)), ShouldBeTrue)

		})

		Convey("if a time has previously been recorded, it should be"+
			" returned", func() {

			projectName := "project"
			timeToUse := time.Now()

			So(SetLastNotificationsEventTime(projectName, timeToUse), ShouldBeNil)

			lastEventTime, err := LastNotificationsEventTime(projectName)
			So(err, ShouldBeNil)
			So(lastEventTime.Round(time.Second).Equal(
				timeToUse.Round(time.Second)), ShouldBeTrue)
		})

	})

}
