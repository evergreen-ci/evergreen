package monitor

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud/providers/mock"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

func TestMonitorReachability(t *testing.T) {

	testConfig := evergreen.TestConfig()

	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

	Convey("When checking the reachability of hosts", t, func() {

		// reset the db
		testutil.HandleTestingErr(db.ClearCollections(host.Collection),
			t, "error clearing hosts collection")

		Convey("hosts that have been checked up on recently should"+
			" not be checked", func() {

			// mock provider hosts are always returned as reachable. therefore,
			// if this host were picked up by the monitoring check then it
			// would be updated to be reachable
			h := &host.Host{
				Id: "h1",
				LastReachabilityCheck: time.Now().Add(-time.Minute),
				Status:                evergreen.HostUnreachable,
				Provider:              mock.ProviderName,
			}
			testutil.HandleTestingErr(h.Insert(), t, "error inserting host")

			So(monitorReachability(nil), ShouldBeNil)

			// refresh the host - its status should not have been updated
			h, err := host.FindOne(host.ById("h1"))
			So(err, ShouldBeNil)
			So(h.Status, ShouldEqual, evergreen.HostUnreachable)

		})

		Convey("hosts eligible for a check should have their statuses"+
			" updated appropriately", func() {

			// this host should be picked up and updated to running
			host1 := &host.Host{
				Id: "h1",
				LastReachabilityCheck: time.Now().Add(-15 * time.Minute),
				Status:                evergreen.HostUnreachable,
				Provider:              mock.ProviderName,
			}
			testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

			// this host should not be picked up, since it is quarantined
			host2 := &host.Host{
				Id: "h2",
				LastReachabilityCheck: time.Now().Add(-15 * time.Minute),
				Status:                evergreen.HostQuarantined,
				Provider:              mock.ProviderName,
			}
			testutil.HandleTestingErr(host2.Insert(), t, "error inserting host")

			So(monitorReachability(nil), ShouldBeNil)

			// refresh the first host - its status should have been updated
			host1, err := host.FindOne(host.ById("h1"))
			So(err, ShouldBeNil)
			So(host1.Status, ShouldEqual, evergreen.HostUnreachable)

			// refresh the second host - its status should not have been updated
			host1, err = host.FindOne(host.ById("h2"))
			So(err, ShouldBeNil)
			So(host1.Status, ShouldEqual, evergreen.HostQuarantined)

		})

	})

}
