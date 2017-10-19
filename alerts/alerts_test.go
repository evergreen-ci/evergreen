package alerts

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/alert"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"gopkg.in/mgo.v2/bson"
)

var testConf = testutil.TestConfig()

func init() {
	db.SetGlobalSessionProvider(testConf.SessionFactory())
}

func TestAlertQueue(t *testing.T) {
	Convey("After queueing a few alerts", t, func() {
		testutil.HandleTestingErr(db.Clear(alert.Collection), t, "problem clearing collections")
		now := time.Now()

		// a bunch of alerts, in order of oldest to newest
		testAlerts := []*alert.AlertRequest{
			{Id: bson.NewObjectId(), Display: "test1", CreatedAt: now.Add(time.Millisecond)},
			{Id: bson.NewObjectId(), Display: "test2", CreatedAt: now.Add(2 * time.Millisecond)},
			{Id: bson.NewObjectId(), Display: "test3", CreatedAt: now.Add(3 * time.Millisecond)},
		}

		for _, a := range testAlerts {
			err := alert.EnqueueAlertRequest(a)
			So(err, ShouldBeNil)
		}

		Convey("dequeuing should return them in order, oldest first", func() {
			found := 0
			for {
				nextAlert, err := alert.DequeueAlertRequest()
				So(err, ShouldBeNil)
				if nextAlert == nil {
					break
				}
				So(nextAlert.Display, ShouldEqual, testAlerts[found].Display)
				So(nextAlert.QueueStatus, ShouldEqual, alert.InProgress)
				found++
			}
			So(found, ShouldEqual, len(testAlerts))
		})
	})
}
