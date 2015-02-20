package taskrunner

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/model/host"
	"10gen.com/mci/util"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

var (
	hostFinderTestConf = mci.TestConfig()
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(hostFinderTestConf))
	if hostFinderTestConf.TaskRunner.LogFile != "" {
		mci.SetLogger(hostFinderTestConf.TaskRunner.LogFile)
	}
}

func TestDBHostFinder(t *testing.T) {

	var users []string
	var taskIds []string
	var hostIds []string
	var hosts []*host.Host
	var hostFinder *DBHostFinder

	Convey("When querying for available hosts", t, func() {

		hostFinder = &DBHostFinder{}

		users = []string{"u1"}
		taskIds = []string{"t1"}
		hostIds = []string{"h1", "h2", "h3"}
		hosts = []*host.Host{
			&host.Host{Id: hostIds[0], StartedBy: mci.MCIUser,
				Status: mci.HostRunning},
			&host.Host{Id: hostIds[1], StartedBy: mci.MCIUser,
				Status: mci.HostRunning},
			&host.Host{Id: hostIds[2], StartedBy: mci.MCIUser,
				Status: mci.HostRunning},
		}

		So(db.Clear(host.Collection), ShouldBeNil)

		Convey("hosts started by users other than the MCI user should not"+
			" be returned", func() {
			hosts[2].StartedBy = users[0]
			for _, host := range hosts {
				util.HandleTestingErr(host.Insert(), t, "Error inserting"+
					" host into database")
			}

			availableHosts, err := hostFinder.FindAvailableHosts()
			util.HandleTestingErr(err, t, "Error finding available hosts")
			So(len(availableHosts), ShouldEqual, 2)
			So(availableHosts[0].Id, ShouldEqual, hosts[0].Id)
			So(availableHosts[1].Id, ShouldEqual, hosts[1].Id)
		})

		Convey("hosts with currently running tasks should not be returned",
			func() {
				hosts[2].RunningTask = taskIds[0]
				for _, host := range hosts {
					util.HandleTestingErr(host.Insert(), t, "Error inserting"+
						" host into database")
				}

				availableHosts, err := hostFinder.FindAvailableHosts()
				util.HandleTestingErr(err, t, "Error finding available hosts")
				So(len(availableHosts), ShouldEqual, 2)
				So(availableHosts[0].Id, ShouldEqual, hosts[0].Id)
				So(availableHosts[1].Id, ShouldEqual, hosts[1].Id)
			})

		Convey("hosts that are not in the 'running' state should not be"+
			" returned", func() {
			hosts[2].Status = mci.HostUninitialized
			for _, host := range hosts {
				util.HandleTestingErr(host.Insert(), t, "Error inserting host"+
					" into database")
			}

			availableHosts, err := hostFinder.FindAvailableHosts()
			util.HandleTestingErr(err, t, "Error finding available hosts")
			So(len(availableHosts), ShouldEqual, 2)
			So(availableHosts[0].Id, ShouldEqual, hosts[0].Id)
			So(availableHosts[1].Id, ShouldEqual, hosts[1].Id)
		})

	})

}
