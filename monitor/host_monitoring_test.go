package monitor

import (
	"10gen.com/mci"
	"10gen.com/mci/cloud/providers/mock"
	"10gen.com/mci/db"
	"10gen.com/mci/model"
	"10gen.com/mci/util"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
	"time"
)

func TestMonitorReachability(t *testing.T) {

	testConfig := mci.TestConfig()

	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

	Convey("When checking the reachability of hosts", t, func() {

		// reset the db
		util.HandleTestingErr(db.ClearCollections(model.HostsCollection),
			t, "error clearing hosts collection")

		Convey("hosts that have been checked up on recently should"+
			" not be checked", func() {

			// mock provider hosts are always returned as reachable. therefore,
			// if this host were picked up by the monitoring check then it
			// would be updated to be reachable
			host := &model.Host{
				Id: "h1",
				LastReachabilityCheck: time.Now().Add(-time.Minute),
				Status:                mci.HostUnreachable,
				Provider:              mock.ProviderName,
			}
			util.HandleTestingErr(host.Insert(), t, "error inserting host")

			So(monitorReachability(nil), ShouldBeNil)

			// refresh the host - its status should not have been updated
			host, err := model.FindHost("h1")
			So(err, ShouldBeNil)
			So(host.Status, ShouldEqual, mci.HostUnreachable)

		})

		Convey("hosts eligible for a check should have their statuses"+
			" updated appropriately", func() {

			// this host should be picked up and updated to running
			host1 := &model.Host{
				Id: "h1",
				LastReachabilityCheck: time.Now().Add(-15 * time.Minute),
				Status:                mci.HostUnreachable,
				Provider:              mock.ProviderName,
			}
			util.HandleTestingErr(host1.Insert(), t, "error inserting host")

			// this host should not be picked up, since it is quarantined
			host2 := &model.Host{
				Id: "h2",
				LastReachabilityCheck: time.Now().Add(-15 * time.Minute),
				Status:                mci.HostQuarantined,
				Provider:              mock.ProviderName,
			}
			util.HandleTestingErr(host2.Insert(), t, "error inserting host")

			So(monitorReachability(nil), ShouldBeNil)

			// refresh the first host - its status should have been updated
			host1, err := model.FindHost("h1")
			So(err, ShouldBeNil)
			So(host1.Status, ShouldEqual, mci.HostUnreachable)

			// refresh the second host - its status should not have been updated
			host1, err = model.FindHost("h2")
			So(err, ShouldBeNil)
			So(host1.Status, ShouldEqual, mci.HostQuarantined)

		})

	})

}
