package monitor

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/smartystreets/goconvey/convey/reporting"
)

func init() {
	reporting.QuietMode()

	if !util.StringSliceContains(evergreen.ProviderSpawnable, evergreen.ProviderNameMock) {
		evergreen.ProviderSpawnable = append(evergreen.ProviderSpawnable, evergreen.ProviderNameMock)
	}
}

func TestFlaggingExcessHosts(t *testing.T) {

	testConfig := testutil.TestConfig()

	db.SetGlobalSessionProvider(testConfig.SessionFactory())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("When flagging excess hosts to be terminated", t, func() {

		Convey("with two separate distros containing hosts", func() {

			// reset the db
			testutil.HandleTestingErr(db.ClearCollections(host.Collection),
				t, "error clearing hosts collection")

			// mock up the distros

			distro1 := distro.Distro{
				Id:       "d1",
				PoolSize: 2,
				Provider: evergreen.ProviderNameMock,
			}
			distro2 := distro.Distro{
				Id:       "d2",
				PoolSize: 1,
				Provider: evergreen.ProviderNameMock,
			}
			distros := []distro.Distro{distro1, distro2}

			Convey("if neither distro has excess hosts, no hosts should be"+
				" flagged to be terminated", func() {

				// insert one host for each distro

				host1 := &host.Host{
					Id:        "h1",
					Distro:    distro.Distro{Id: "d1"},
					Status:    evergreen.HostRunning,
					StartedBy: evergreen.User,
					Provider:  evergreen.ProviderNameMock,
				}
				testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

				host2 := &host.Host{
					Id:        "h2",
					Distro:    distro.Distro{Id: "d2"},
					Status:    evergreen.HostRunning,
					StartedBy: evergreen.User,
					Provider:  evergreen.ProviderNameMock,
				}
				testutil.HandleTestingErr(host2.Insert(), t, "error inserting host")

				// flag the excess hosts - there should not be any
				excess, err := flagExcessHosts(ctx, distros, nil)
				So(err, ShouldBeNil)
				So(len(excess), ShouldEqual, 0)

			})

			Convey("if only one distro has excess hosts, the appropriate"+
				" number of hosts from that distro should be flagged", func() {

				// insert one host for the first distro, and three for the
				// second distro

				host1 := &host.Host{
					Id:        "h1",
					Distro:    distro.Distro{Id: "d1"},
					Status:    evergreen.HostRunning,
					StartedBy: evergreen.User,
					Provider:  evergreen.ProviderNameMock,
				}
				testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

				host2 := &host.Host{
					Id:        "h2",
					Distro:    distro.Distro{Id: "d2"},
					Status:    evergreen.HostRunning,
					StartedBy: evergreen.User,
					Provider:  evergreen.ProviderNameMock,
				}
				testutil.HandleTestingErr(host2.Insert(), t, "error inserting host")

				host3 := &host.Host{
					Id:        "h3",
					Distro:    distro.Distro{Id: "d2"},
					Status:    evergreen.HostRunning,
					StartedBy: evergreen.User,
					Provider:  evergreen.ProviderNameMock,
				}
				testutil.HandleTestingErr(host3.Insert(), t, "error inserting host")

				host4 := &host.Host{
					Id:        "h4",
					Distro:    distro.Distro{Id: "d2"},
					Status:    evergreen.HostRunning,
					StartedBy: evergreen.User,
					Provider:  evergreen.ProviderNameMock,
				}
				testutil.HandleTestingErr(host4.Insert(), t, "error inserting host")

				// flag the excess hosts - there should be 2, both from
				// the second distro
				excess, err := flagExcessHosts(ctx, distros, nil)
				So(err, ShouldBeNil)
				So(len(excess), ShouldEqual, 2)
				for _, host := range excess {
					So(host.Distro.Id, ShouldEqual, "d2")
				}

			})

			Convey("if both distros have excess hosts, the appropriate number"+
				" of hosts from each distro should be flagged", func() {

				// insert three hosts for each distro

				host1 := &host.Host{
					Id:        "h1",
					Distro:    distro.Distro{Id: "d1"},
					Status:    evergreen.HostRunning,
					StartedBy: evergreen.User,
					Provider:  evergreen.ProviderNameMock,
				}
				testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

				host2 := &host.Host{
					Id:        "h2",
					Distro:    distro.Distro{Id: "d1"},
					Status:    evergreen.HostRunning,
					StartedBy: evergreen.User,
					Provider:  evergreen.ProviderNameMock,
				}
				testutil.HandleTestingErr(host2.Insert(), t, "error inserting host")

				host3 := &host.Host{
					Id:        "h3",
					Distro:    distro.Distro{Id: "d1"},
					Status:    evergreen.HostRunning,
					StartedBy: evergreen.User,
					Provider:  evergreen.ProviderNameMock,
				}
				testutil.HandleTestingErr(host3.Insert(), t, "error inserting host")

				host4 := &host.Host{
					Id:        "h4",
					Distro:    distro.Distro{Id: "d2"},
					Status:    evergreen.HostRunning,
					StartedBy: evergreen.User,
					Provider:  evergreen.ProviderNameMock,
				}
				testutil.HandleTestingErr(host4.Insert(), t, "error inserting host")

				host5 := &host.Host{
					Id:        "h5",
					Distro:    distro.Distro{Id: "d2"},
					Status:    evergreen.HostRunning,
					StartedBy: evergreen.User,
					Provider:  evergreen.ProviderNameMock,
				}
				testutil.HandleTestingErr(host5.Insert(), t, "error inserting host")

				host6 := &host.Host{
					Id:        "h6",
					Distro:    distro.Distro{Id: "d2"},
					Status:    evergreen.HostRunning,
					StartedBy: evergreen.User,
					Provider:  evergreen.ProviderNameMock,
				}
				testutil.HandleTestingErr(host6.Insert(), t, "error inserting host")

				// find the excess hosts - there should be one for the first
				// distro and two for the second
				excess, err := flagExcessHosts(ctx, distros, nil)
				So(err, ShouldBeNil)
				So(len(excess), ShouldEqual, 3)
				excessFirst := 0
				excessSecond := 0
				for _, host := range excess {
					if host.Distro.Id == "d1" {
						excessFirst++
					}
					if host.Distro.Id == "d2" {
						excessSecond++
					}
				}
				So(excessFirst, ShouldEqual, 1)
				So(excessSecond, ShouldEqual, 2)

			})

			Convey("hosts currently running a task should not be"+
				" flagged", func() {

				// insert two hosts for each distro, with running tasks

				host1 := &host.Host{
					Id:          "h1",
					Distro:      distro.Distro{Id: "d1"},
					Status:      evergreen.HostRunning,
					StartedBy:   evergreen.User,
					RunningTask: "t1",
					Provider:    evergreen.ProviderNameMock,
				}
				testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

				host2 := &host.Host{
					Id:          "h2",
					Distro:      distro.Distro{Id: "d1"},
					Status:      evergreen.HostRunning,
					StartedBy:   evergreen.User,
					RunningTask: "t2",
					Provider:    evergreen.ProviderNameMock,
				}
				testutil.HandleTestingErr(host2.Insert(), t, "error inserting host")

				host3 := &host.Host{
					Id:          "h3",
					Distro:      distro.Distro{Id: "d2"},
					Status:      evergreen.HostRunning,
					StartedBy:   evergreen.User,
					RunningTask: "t3",
					Provider:    evergreen.ProviderNameMock,
				}
				testutil.HandleTestingErr(host3.Insert(), t, "error inserting host")

				host4 := &host.Host{
					Id:          "h4",
					Distro:      distro.Distro{Id: "d2"},
					Status:      evergreen.HostRunning,
					StartedBy:   evergreen.User,
					RunningTask: "t4",
					Provider:    evergreen.ProviderNameMock,
				}
				testutil.HandleTestingErr(host4.Insert(), t, "error inserting host")

				// find the excess hosts - there should be none, since all of
				// the hosts are running tasks and cannot safely be terminated
				excess, err := flagExcessHosts(ctx, distros, nil)
				So(err, ShouldBeNil)
				So(len(excess), ShouldEqual, 0)

			})
		})
	})
}
