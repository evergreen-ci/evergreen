package monitor

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
)

func TestWarnSlowProvisioningHosts(t *testing.T) {

	testConfig := testutil.TestConfigWithDefaultAuthTokens()

	Convey("When building warnings for hosts that are taking a long time to"+
		" provision", t, func() {

		// reset the db
		require.NoError(t, db.ClearCollections(host.Collection), "error clearing hosts collection")

		Convey("hosts that have not hit the threshold should not trigger a"+
			" warning", func() {

			host1 := &host.Host{
				Id:           "h1",
				StartedBy:    evergreen.User,
				CreationTime: time.Now().Add(-10 * time.Minute),
			}
			require.NoError(t, host1.Insert(), "error inserting host")

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
			require.NoError(t, host1.Insert(), "error inserting host")

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
			require.NoError(t, host1.Insert(), "error inserting host")

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
			require.NoError(t, host1.Insert(), "error inserting host")

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
