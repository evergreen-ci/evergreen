package monitor

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

func TestMonitorReachability(t *testing.T) {

	testConfig := testutil.TestConfig()

	db.SetGlobalSessionProvider(testConfig.SessionFactory())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("When checking the reachability of hosts", t, func() {
		mockCloud := cloud.GetMockProvider()
		mockCloud.Reset()

		// reset the db
		testutil.HandleTestingErr(db.ClearCollections(host.Collection),
			t, "error clearing hosts collection")

		Convey("hosts that have been checked up on recently should"+
			" not be checked", func() {

			h := &host.Host{
				Id: "h1",
				LastCommunicationTime: time.Now().Add(-time.Minute),
				Status:                evergreen.HostUnreachable,
				Provider:              evergreen.ProviderNameMock,
				StartedBy:             evergreen.User,
			}

			// mock provider hosts are always returned as reachable. therefore,
			// if this host were picked up by the monitoring check then it
			// would be updated to be reachable
			testutil.HandleTestingErr(h.Insert(), t, "error inserting host")

			So(monitorReachability(ctx, nil), ShouldBeNil)

			// refresh the host - its status should not have been updated
			h, err := host.FindOne(host.ById("h1"))
			So(err, ShouldBeNil)
			So(h.Status, ShouldEqual, evergreen.HostUnreachable)

		})

		Convey("hosts eligible for a check should have their statuses"+
			" updated appropriately", func() {

			m1 := cloud.MockInstance{
				IsUp:           true,
				IsSSHReachable: true,
				Status:         cloud.StatusRunning,
			}
			mockCloud.Set("h1", m1)

			// this host should be picked up and updated to running
			host1 := &host.Host{
				Id: "h1",
				LastCommunicationTime: time.Now().Add(-15 * time.Minute),
				Status:                evergreen.HostUnreachable,
				Provider:              evergreen.ProviderNameMock,
				StartedBy:             evergreen.User,
			}
			testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

			m2 := cloud.MockInstance{
				IsUp:           true,
				IsSSHReachable: true,
				Status:         cloud.StatusRunning,
			}
			mockCloud.Set("h2", m2)

			// this host should not be picked up, since it is quarantined
			host2 := &host.Host{
				Id: "h2",
				LastCommunicationTime: time.Now().Add(-15 * time.Minute),
				Status:                evergreen.HostQuarantined,
				Provider:              evergreen.ProviderNameMock,
				StartedBy:             evergreen.User,
			}
			testutil.HandleTestingErr(host2.Insert(), t, "error inserting host")

			So(monitorReachability(ctx, nil), ShouldBeNil)

			// refresh the first host - its status should have been updated
			host1, err := host.FindOne(host.ById("h1"))
			So(err, ShouldBeNil)
			So(host1.Status, ShouldEqual, evergreen.HostRunning)

			// refresh the second host - its status should not have been updated
			host1, err = host.FindOne(host.ById("h2"))
			So(err, ShouldBeNil)
			So(host1.Status, ShouldEqual, evergreen.HostQuarantined)

		})

	})

}
