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

func TestWarnExpiringSpawnedHosts(t *testing.T) {

	testConfig := testutil.TestConfig()

	db.SetGlobalSessionProvider(testConfig.SessionFactory())

	Convey("When building warnings for spawned hosts that will be expiring"+
		" soon", t, func() {

		// reset the db
		testutil.HandleTestingErr(db.ClearCollections(host.Collection),
			t, "error clearing hosts collection")

		Convey("any hosts not expiring within a threshold should not trigger"+
			" warnings", func() {

			// this host does not expire within the first notification
			// threshold
			host1 := &host.Host{
				Id:             "h1",
				ExpirationTime: time.Now().Add(time.Hour * 15),
			}
			testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

			warnings, err := spawnHostExpirationWarnings(testConfig)
			So(err, ShouldBeNil)
			So(len(warnings), ShouldEqual, 0)

		})

		Convey("any thresholds for which warnings have already been sent"+
			" should be ignored", func() {

			// this host meets the first notification warning threshold
			host1 := &host.Host{
				Id:             "h1",
				ExpirationTime: time.Now().Add(time.Hour * 10),
				Notifications: map[string]bool{
					"720": true, // the first threshold in minutes
				},
			}
			testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

			warnings, err := spawnHostExpirationWarnings(testConfig)
			So(err, ShouldBeNil)
			So(len(warnings), ShouldEqual, 0)

		})

		Convey("the most recent threshold crossed should be used to create"+
			" the warning", func() {

			// this host meets both notification warning thresholds
			host1 := &host.Host{
				Id:             "h1",
				ExpirationTime: time.Now().Add(time.Minute * 10),
			}
			testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

			warnings, err := spawnHostExpirationWarnings(testConfig)
			So(err, ShouldBeNil)
			So(len(warnings), ShouldEqual, 1)

			// execute the callback, make sure the correct threshold is set
			So(warnings[0].callback(warnings[0].host, warnings[0].threshold), ShouldBeNil)
			host1, err = host.FindOne(host.ById("h1"))
			So(err, ShouldBeNil)
			So(host1.Notifications["120"], ShouldBeTrue)

		})

		Convey("hosts that are quarantined or have already expired should not"+
			" merit warnings", func() {

			// quarantined host
			host1 := &host.Host{
				Id:             "h1",
				Status:         evergreen.HostQuarantined,
				ExpirationTime: time.Now().Add(time.Minute * 10),
			}
			testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

			// terminated host
			host2 := &host.Host{
				Id:             "h2",
				Status:         evergreen.HostTerminated,
				ExpirationTime: time.Now().Add(time.Minute * 10),
			}
			testutil.HandleTestingErr(host2.Insert(), t, "error inserting host")

			// past the expiration. no warning needs to be sent since this host
			// is theoretically about to be terminated, at which time a
			// notification will be sent
			host3 := &host.Host{
				Id:             "h3",
				ExpirationTime: time.Now().Add(-time.Minute * 10),
			}
			testutil.HandleTestingErr(host3.Insert(), t, "error inserting host")

			warnings, err := spawnHostExpirationWarnings(testConfig)
			So(err, ShouldBeNil)
			So(len(warnings), ShouldEqual, 0)

		})

	})

}

func TestWarnSlowProvisioningHosts(t *testing.T) {

	testConfig := testutil.TestConfig()

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

			warnings, err := slowProvisioningWarnings(testConfig)
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

			warnings, err := slowProvisioningWarnings(testConfig)
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

			warnings, err := slowProvisioningWarnings(testConfig)
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

			warnings, err := slowProvisioningWarnings(testConfig)
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
