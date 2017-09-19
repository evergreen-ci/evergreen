package monitor

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud/providers/mock"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	modelUtil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
)

func TestFlaggingDecommissionedHosts(t *testing.T) {

	testConfig := testutil.TestConfig()

	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

	Convey("When flagging decommissioned hosts", t, func() {

		Convey("only hosts in the database who are marked decommissioned"+
			" should be returned", func() {

			// reset the db
			testutil.HandleTestingErr(db.ClearCollections(host.Collection),
				t, "error clearing hosts collection")

			// insert hosts with different statuses

			host1 := &host.Host{
				Id:     "h1",
				Status: evergreen.HostRunning,
			}
			testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

			host2 := &host.Host{
				Id:     "h2",
				Status: evergreen.HostTerminated,
			}
			testutil.HandleTestingErr(host2.Insert(), t, "error inserting host")

			host3 := &host.Host{
				Id:     "h3",
				Status: evergreen.HostDecommissioned,
			}
			testutil.HandleTestingErr(host3.Insert(), t, "error inserting host")

			host4 := &host.Host{
				Id:     "h4",
				Status: evergreen.HostDecommissioned,
			}
			testutil.HandleTestingErr(host4.Insert(), t, "error inserting host")

			host5 := &host.Host{
				Id:     "h5",
				Status: evergreen.HostQuarantined,
			}
			testutil.HandleTestingErr(host5.Insert(), t, "error inserting host")

			// flag the decommissioned hosts - there should be 2 of them
			decommissioned, err := flagDecommissionedHosts(nil, testConfig)
			So(err, ShouldBeNil)
			So(len(decommissioned), ShouldEqual, 2)
			var ids []string
			for _, h := range decommissioned {
				ids = append(ids, h.Id)
			}
			So(util.SliceContains(ids, host3.Id), ShouldBeTrue)
			So(util.SliceContains(ids, host4.Id), ShouldBeTrue)
		})

	})

}

func TestFlaggingIdleHosts(t *testing.T) {

	testConfig := testutil.TestConfig()

	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

	Convey("When flagging idle hosts to be terminated", t, func() {

		// reset the db
		testutil.HandleTestingErr(db.ClearCollections(host.Collection),
			t, "error clearing hosts collection")
		testutil.HandleTestingErr(modelUtil.AddTestIndexes(host.Collection,
			true, true, host.RunningTaskKey), t, "error adding host index")
		Convey("hosts currently running a task should never be"+
			" flagged", func() {

			// insert a host that is currently running a task - but whose
			// creation time would otherwise indicate it has been idle a while
			host1 := host.Host{
				Id:           "h1",
				Provider:     mock.ProviderName,
				CreationTime: time.Now().Add(-30 * time.Minute),
				RunningTask:  "t1",
				Status:       evergreen.HostRunning,
				StartedBy:    evergreen.User,
			}
			testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

			// finding idle hosts should not return the host
			idle, err := flagIdleHosts(nil, nil)
			So(err, ShouldBeNil)
			So(len(idle), ShouldEqual, 0)

			Convey("even if they have a last communication time > 10 minutes", func() {
				h2 := host.Host{
					Id:                    "anotherhost",
					Provider:              mock.ProviderName,
					CreationTime:          time.Now().Add(-30 * time.Minute),
					RunningTask:           "t3",
					Status:                evergreen.HostRunning,
					LastCommunicationTime: time.Now().Add(-30 * time.Minute),
					StartedBy:             evergreen.User,
				}
				testutil.HandleTestingErr(h2.Insert(), t, "error inserting host")
				// finding idle hosts should not return the host
				idle, err := flagIdleHosts(nil, nil)
				So(err, ShouldBeNil)
				So(len(idle), ShouldEqual, 0)

			})

		})

		Convey("hosts not currently running a task should be flagged if they"+
			" have been idle at least 15 minutes and will incur a payment in"+
			" less than 10 minutes", func() {

			// insert two hosts - one whose last task was more than 15 minutes
			// ago, one whose last task was less than 15 minutes ago

			host1 := host.Host{
				Id:                    "h2",
				Provider:              mock.ProviderName,
				LastTaskCompleted:     "t1",
				LastTaskCompletedTime: time.Now().Add(-time.Minute * 20),
				LastCommunicationTime: time.Now(),
				Status:                evergreen.HostRunning,
				StartedBy:             evergreen.User,
			}
			testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

			host2 := host.Host{
				Id:                    "h3",
				Provider:              mock.ProviderName,
				LastTaskCompleted:     "t2",
				LastTaskCompletedTime: time.Now().Add(-time.Minute * 5),
				LastCommunicationTime: time.Now(),
				Status:                evergreen.HostRunning,
				StartedBy:             evergreen.User,
			}
			testutil.HandleTestingErr(host2.Insert(), t, "error inserting host")

			// finding idle hosts should only return the first host
			idle, err := flagIdleHosts(nil, nil)
			So(err, ShouldBeNil)
			So(len(idle), ShouldEqual, 1)
			So(idle[0].Id, ShouldEqual, "h2")

		})
		Convey("hosts not currently running a task with a last communication time greater"+
			"than 10 mins should be marked as idle", func() {
			anotherHost := host.Host{
				Id:                    "h1",
				Provider:              mock.ProviderName,
				LastCommunicationTime: time.Now().Add(-time.Minute * 20),
				Status:                evergreen.HostRunning,
				StartedBy:             evergreen.User,
			}
			So(anotherHost.Insert(), ShouldBeNil)
			// finding idle hosts should only return the first host
			idle, err := flagIdleHosts(nil, nil)
			So(err, ShouldBeNil)
			So(len(idle), ShouldEqual, 1)
			So(idle[0].Id, ShouldEqual, "h1")
		})

	})

}

func TestFlaggingExcessHosts(t *testing.T) {

	testConfig := testutil.TestConfig()

	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

	Convey("When flagging excess hosts to be terminated", t, func() {

		Convey("with two separate distros containing hosts", func() {

			// reset the db
			testutil.HandleTestingErr(db.ClearCollections(host.Collection),
				t, "error clearing hosts collection")

			// mock up the distros

			distro1 := distro.Distro{
				Id:       "d1",
				PoolSize: 2,
			}
			distro2 := distro.Distro{
				Id:       "d2",
				PoolSize: 1,
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
					Provider:  mock.ProviderName,
				}
				testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

				host2 := &host.Host{
					Id:        "h2",
					Distro:    distro.Distro{Id: "d2"},
					Status:    evergreen.HostRunning,
					StartedBy: evergreen.User,
					Provider:  mock.ProviderName,
				}
				testutil.HandleTestingErr(host2.Insert(), t, "error inserting host")

				// flag the excess hosts - there should not be any
				excess, err := flagExcessHosts(distros, nil)
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
					Provider:  mock.ProviderName,
				}
				testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

				host2 := &host.Host{
					Id:        "h2",
					Distro:    distro.Distro{Id: "d2"},
					Status:    evergreen.HostRunning,
					StartedBy: evergreen.User,
					Provider:  mock.ProviderName,
				}
				testutil.HandleTestingErr(host2.Insert(), t, "error inserting host")

				host3 := &host.Host{
					Id:        "h3",
					Distro:    distro.Distro{Id: "d2"},
					Status:    evergreen.HostRunning,
					StartedBy: evergreen.User,
					Provider:  mock.ProviderName,
				}
				testutil.HandleTestingErr(host3.Insert(), t, "error inserting host")

				host4 := &host.Host{
					Id:        "h4",
					Distro:    distro.Distro{Id: "d2"},
					Status:    evergreen.HostRunning,
					StartedBy: evergreen.User,
					Provider:  mock.ProviderName,
				}
				testutil.HandleTestingErr(host4.Insert(), t, "error inserting host")

				// flag the excess hosts - there should be 2, both from
				// the second distro
				excess, err := flagExcessHosts(distros, nil)
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
					Provider:  mock.ProviderName,
				}
				testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

				host2 := &host.Host{
					Id:        "h2",
					Distro:    distro.Distro{Id: "d1"},
					Status:    evergreen.HostRunning,
					StartedBy: evergreen.User,
					Provider:  mock.ProviderName,
				}
				testutil.HandleTestingErr(host2.Insert(), t, "error inserting host")

				host3 := &host.Host{
					Id:        "h3",
					Distro:    distro.Distro{Id: "d1"},
					Status:    evergreen.HostRunning,
					StartedBy: evergreen.User,
					Provider:  mock.ProviderName,
				}
				testutil.HandleTestingErr(host3.Insert(), t, "error inserting host")

				host4 := &host.Host{
					Id:        "h4",
					Distro:    distro.Distro{Id: "d2"},
					Status:    evergreen.HostRunning,
					StartedBy: evergreen.User,
					Provider:  mock.ProviderName,
				}
				testutil.HandleTestingErr(host4.Insert(), t, "error inserting host")

				host5 := &host.Host{
					Id:        "h5",
					Distro:    distro.Distro{Id: "d2"},
					Status:    evergreen.HostRunning,
					StartedBy: evergreen.User,
					Provider:  mock.ProviderName,
				}
				testutil.HandleTestingErr(host5.Insert(), t, "error inserting host")

				host6 := &host.Host{
					Id:        "h6",
					Distro:    distro.Distro{Id: "d2"},
					Status:    evergreen.HostRunning,
					StartedBy: evergreen.User,
					Provider:  mock.ProviderName,
				}
				testutil.HandleTestingErr(host6.Insert(), t, "error inserting host")

				// find the excess hosts - there should be one for the first
				// distro and two for the second
				excess, err := flagExcessHosts(distros, nil)
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
					Provider:    mock.ProviderName,
				}
				testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

				host2 := &host.Host{
					Id:          "h2",
					Distro:      distro.Distro{Id: "d1"},
					Status:      evergreen.HostRunning,
					StartedBy:   evergreen.User,
					RunningTask: "t2",
					Provider:    mock.ProviderName,
				}
				testutil.HandleTestingErr(host2.Insert(), t, "error inserting host")

				host3 := &host.Host{
					Id:          "h3",
					Distro:      distro.Distro{Id: "d2"},
					Status:      evergreen.HostRunning,
					StartedBy:   evergreen.User,
					RunningTask: "t3",
					Provider:    mock.ProviderName,
				}
				testutil.HandleTestingErr(host3.Insert(), t, "error inserting host")

				host4 := &host.Host{
					Id:          "h4",
					Distro:      distro.Distro{Id: "d2"},
					Status:      evergreen.HostRunning,
					StartedBy:   evergreen.User,
					RunningTask: "t4",
					Provider:    mock.ProviderName,
				}
				testutil.HandleTestingErr(host4.Insert(), t, "error inserting host")

				// find the excess hosts - there should be none, since all of
				// the hosts are running tasks and cannot safely be terminated
				excess, err := flagExcessHosts(distros, nil)
				So(err, ShouldBeNil)
				So(len(excess), ShouldEqual, 0)

			})

		})

	})

}

func TestFlaggingUnprovisionedHosts(t *testing.T) {

	testConfig := testutil.TestConfig()

	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

	Convey("When flagging unprovisioned hosts to be terminated", t, func() {

		// reset the db
		testutil.HandleTestingErr(db.ClearCollections(host.Collection),
			t, "error clearing hosts collection")

		Convey("hosts that have not hit the provisioning limit should"+
			" be ignored", func() {

			host1 := &host.Host{
				Id:           "h1",
				StartedBy:    evergreen.User,
				CreationTime: time.Now().Add(-time.Minute * 10),
			}
			testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

			unprovisioned, err := flagUnprovisionedHosts(nil, nil)
			So(err, ShouldBeNil)
			So(len(unprovisioned), ShouldEqual, 0)

		})

		Convey("hosts that are already terminated should be ignored", func() {

			host1 := &host.Host{
				Id:           "h1",
				StartedBy:    evergreen.User,
				CreationTime: time.Now().Add(-time.Minute * 60),
				Status:       evergreen.HostTerminated,
			}
			testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

			unprovisioned, err := flagUnprovisionedHosts(nil, nil)
			So(err, ShouldBeNil)
			So(len(unprovisioned), ShouldEqual, 0)

		})

		Convey("hosts that are already provisioned should be ignored", func() {

			host1 := &host.Host{
				Id:           "h1",
				StartedBy:    evergreen.User,
				CreationTime: time.Now().Add(-time.Minute * 60),
				Provisioned:  true,
			}
			testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

			unprovisioned, err := flagUnprovisionedHosts(nil, nil)
			So(err, ShouldBeNil)
			So(len(unprovisioned), ShouldEqual, 0)

		})

		Convey("hosts that have hit the provisioning limit should be"+
			" flagged", func() {

			host1 := &host.Host{
				Id:           "h1",
				StartedBy:    evergreen.User,
				CreationTime: time.Now().Add(-time.Minute * 60),
			}
			testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

			unprovisioned, err := flagUnprovisionedHosts(nil, nil)
			So(err, ShouldBeNil)
			So(len(unprovisioned), ShouldEqual, 1)
			So(unprovisioned[0].Id, ShouldEqual, "h1")

		})

	})
}

func TestFlaggingProvisioningFailedHosts(t *testing.T) {

	testConfig := testutil.TestConfig()

	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

	Convey("When flagging hosts whose provisioning failed", t, func() {

		// reset the db
		testutil.HandleTestingErr(db.ClearCollections(host.Collection),
			t, "error clearing hosts collection")

		Convey("only hosts whose provisioning failed should be"+
			" picked up", func() {

			host1 := &host.Host{
				Id:     "h1",
				Status: evergreen.HostRunning,
			}
			testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

			host2 := &host.Host{
				Id:     "h2",
				Status: evergreen.HostUninitialized,
			}
			testutil.HandleTestingErr(host2.Insert(), t, "error inserting host")

			host3 := &host.Host{
				Id:     "h3",
				Status: evergreen.HostProvisionFailed,
			}
			testutil.HandleTestingErr(host3.Insert(), t, "error inserting host")

			unprovisioned, err := flagProvisioningFailedHosts(nil, nil)
			So(err, ShouldBeNil)
			So(len(unprovisioned), ShouldEqual, 1)
			So(unprovisioned[0].Id, ShouldEqual, "h3")

		})

	})
}

func TestFlaggingExpiredHosts(t *testing.T) {

	testConfig := testutil.TestConfig()

	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

	Convey("When flagging expired hosts to be terminated", t, func() {

		// reset the db
		testutil.HandleTestingErr(db.ClearCollections(host.Collection),
			t, "error clearing hosts collection")

		Convey("hosts started by the default user should be filtered"+
			" out", func() {

			host1 := &host.Host{
				Id:        "h1",
				Status:    evergreen.HostRunning,
				StartedBy: evergreen.User,
			}
			testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

			expired, err := flagExpiredHosts(nil, nil)
			So(err, ShouldBeNil)
			So(len(expired), ShouldEqual, 0)

		})

		Convey("hosts that are terminated or quarantined should be filtered"+
			" out", func() {

			host1 := &host.Host{
				Id:     "h1",
				Status: evergreen.HostQuarantined,
			}
			testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

			host2 := &host.Host{
				Id:     "h2",
				Status: evergreen.HostTerminated,
			}
			testutil.HandleTestingErr(host2.Insert(), t, "error inserting host")

			expired, err := flagExpiredHosts(nil, nil)
			So(err, ShouldBeNil)
			So(len(expired), ShouldEqual, 0)

		})

		Convey("hosts should be returned if their expiration threshold has"+
			" been reached", func() {

			// not expired
			host1 := &host.Host{
				Id:             "h1",
				Status:         evergreen.HostRunning,
				ExpirationTime: time.Now().Add(time.Minute * 10),
			}
			testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

			// expired
			host2 := &host.Host{
				Id:             "h2",
				Status:         evergreen.HostRunning,
				ExpirationTime: time.Now().Add(-time.Minute * 10),
			}
			testutil.HandleTestingErr(host2.Insert(), t, "error inserting host")

			expired, err := flagExpiredHosts(nil, nil)
			So(err, ShouldBeNil)
			So(len(expired), ShouldEqual, 1)
			So(expired[0].Id, ShouldEqual, "h2")

		})

	})

}
