package monitor

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

func TestWarnSlowProvisioningHosts(t *testing.T) {

	testConfig := testutil.TestConfigWithDefaultAuthTokens()

	db.SetGlobalSessionProvider(testConfig.SessionFactory())

	Convey("When building warnings for hosts that are taking a long time to"+
		" provision", t, func() {

		// reset the db
		testutil.HandleTestingErr(db.ClearCollections(host.Collection),
			t, "error clearing hosts collection")

		Convey("hosts that have not hit the threshold should not trigger a"+
			" warning", func() {

			host1 := &host.Host{
				Id:           "h1",
				StartedBy:    evergreen.User,
				CreationTime: time.Now().Add(-10 * time.Minute),
			}
			testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

			warnings, err := SlowProvisioningWarnings(testConfig)
			So(err, ShouldBeNil)
			So(len(warnings), ShouldEqual, 0)

		})

		Convey("hosts that have already triggered a notification should not"+
			" trigger another", func() {

			host1 := &host.Host{
				Id:           "h1",
				StartedBy:    evergreen.User,
				CreationTime: time.Now().Add(-1 * time.Hour),
				Notifications: map[string]bool{
					slowProvisioningWarning: true,
				},
			}
			testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

			warnings, err := SlowProvisioningWarnings(testConfig)
			So(err, ShouldBeNil)
			So(len(warnings), ShouldEqual, 0)

		})

		Convey("terminated hosts should not trigger a warning", func() {

			host1 := &host.Host{
				Id:           "h1",
				StartedBy:    evergreen.User,
				Status:       evergreen.HostTerminated,
				CreationTime: time.Now().Add(-1 * time.Hour),
			}
			testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

			warnings, err := SlowProvisioningWarnings(testConfig)
			So(err, ShouldBeNil)
			So(len(warnings), ShouldEqual, 0)

		})

		Convey("hosts that are at the threshold and have not previously"+
			" triggered a warning should trigger one", func() {

			host1 := &host.Host{
				Id:           "h1",
				StartedBy:    evergreen.User,
				CreationTime: time.Now().Add(-1 * time.Hour),
			}
			testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

			warnings, err := SlowProvisioningWarnings(testConfig)
			So(err, ShouldBeNil)
			So(len(warnings), ShouldEqual, 1)

			// make sure running the callback sets the notification key
			So(warnings[0].callback(warnings[0].host, warnings[0].threshold), ShouldBeNil)
			host1, err = host.FindOne(host.ById("h1"))
			So(err, ShouldBeNil)
			So(host1.Notifications[slowProvisioningWarning], ShouldBeTrue)

		})

	})
}
