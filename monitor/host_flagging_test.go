package monitor

import (
	"10gen.com/mci"
	"10gen.com/mci/cloud/providers/mock"
	"10gen.com/mci/db"
	"10gen.com/mci/model"
	"10gen.com/mci/model/host"
	"10gen.com/mci/util"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
	"time"
)

func TestFlaggingDecommissionedHosts(t *testing.T) {

	testConfig := mci.TestConfig()

	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

	Convey("When flagging decommissioned hosts", t, func() {

		Convey("only hosts in the database who are marked decommissioned"+
			" should be returned", func() {

			// reset the db
			util.HandleTestingErr(db.ClearCollections(host.Collection),
				t, "error clearing hosts collection")

			// insert hosts with different statuses

			host1 := &host.Host{
				Id:     "h1",
				Status: mci.HostRunning,
			}
			util.HandleTestingErr(host1.Insert(), t, "error inserting host")

			host2 := &host.Host{
				Id:     "h2",
				Status: mci.HostTerminated,
			}
			util.HandleTestingErr(host2.Insert(), t, "error inserting host")

			host3 := &host.Host{
				Id:     "h3",
				Status: mci.HostDecommissioned,
			}
			util.HandleTestingErr(host3.Insert(), t, "error inserting host")

			host4 := &host.Host{
				Id:     "h4",
				Status: mci.HostDecommissioned,
			}
			util.HandleTestingErr(host4.Insert(), t, "error inserting host")

			host5 := &host.Host{
				Id:     "h5",
				Status: mci.HostQuarantined,
			}
			util.HandleTestingErr(host5.Insert(), t, "error inserting host")

			// flag the decommissioned hosts - there should be 2 of them
			decommissioned, err := flagDecommissionedHosts(nil, testConfig)
			So(err, ShouldBeNil)
			So(len(decommissioned), ShouldEqual, 2)
			So(util.SliceContains(decommissioned, *host3), ShouldBeTrue)
			So(util.SliceContains(decommissioned, *host4), ShouldBeTrue)

		})

	})

}

func TestFlaggingIdleHosts(t *testing.T) {

	testConfig := mci.TestConfig()

	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

	Convey("When flagging idle hosts to be terminated", t, func() {

		// reset the db
		util.HandleTestingErr(db.ClearCollections(host.Collection),
			t, "error clearing hosts collection")

		Convey("hosts currently running a task should never be"+
			" flagged", func() {

			// insert a host that is currently running a task - but whose
			// creation time would otherwise indicate it has been idle a while
			host1 := host.Host{
				Id:           "h1",
				Provider:     mock.ProviderName,
				CreationTime: time.Now().Add(-30 * time.Minute),
				RunningTask:  "t1",
				Status:       mci.HostRunning,
				StartedBy:    mci.MCIUser,
			}
			util.HandleTestingErr(host1.Insert(), t, "error inserting host")

			// finding idle hosts should not return the host
			idle, err := flagIdleHosts(nil, nil)
			So(err, ShouldBeNil)
			So(len(idle), ShouldEqual, 0)

		})

		Convey("hosts not currently running a task should be flagged if they"+
			" have been idle at least 15 minutes and will incur a payment in"+
			" less than 10 minutes", func() {

			// insert two hosts - one whose last task was more than 15 minutes
			// ago, one whose last task was less than 15 minutes ago

			host1 := host.Host{
				Id:                    "h1",
				Provider:              mock.ProviderName,
				LastTaskCompleted:     "t1",
				LastTaskCompletedTime: time.Now().Add(-time.Minute * 20),
				Status:                mci.HostRunning,
				StartedBy:             mci.MCIUser,
			}
			util.HandleTestingErr(host1.Insert(), t, "error inserting host")

			host2 := host.Host{
				Id:                    "h2",
				Provider:              mock.ProviderName,
				LastTaskCompleted:     "t2",
				LastTaskCompletedTime: time.Now().Add(-time.Minute * 5),
				Status:                mci.HostRunning,
				StartedBy:             mci.MCIUser,
			}
			util.HandleTestingErr(host2.Insert(), t, "error inserting host")

			// finding idle hosts should only return the first host
			idle, err := flagIdleHosts(nil, nil)
			So(err, ShouldBeNil)
			So(len(idle), ShouldEqual, 1)
			So(idle[0].Id, ShouldEqual, "h1")

		})

	})

}

func TestFlaggingExcessHosts(t *testing.T) {

	testConfig := mci.TestConfig()

	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

	Convey("When flagging excess hosts to be terminated", t, func() {

		Convey("with two separate distros containing hosts", func() {

			// reset the db
			util.HandleTestingErr(db.ClearCollections(host.Collection),
				t, "error clearing hosts collection")

			// mock up the distros

			distro1 := model.Distro{
				Name:     "d1",
				MaxHosts: 2,
			}
			distro2 := model.Distro{
				Name:     "d2",
				MaxHosts: 1,
			}
			distros := map[string]model.Distro{
				"d1": distro1,
				"d2": distro2,
			}

			Convey("if neither distro has excess hosts, no hosts should be"+
				" flagged to be terminated", func() {

				// insert one host for each distro

				host1 := &host.Host{
					Id:        "h1",
					Distro:    "d1",
					Status:    mci.HostRunning,
					StartedBy: mci.MCIUser,
					Provider:  mock.ProviderName,
				}
				util.HandleTestingErr(host1.Insert(), t, "error inserting host")

				host2 := &host.Host{
					Id:        "h2",
					Distro:    "d2",
					Status:    mci.HostRunning,
					StartedBy: mci.MCIUser,
					Provider:  mock.ProviderName,
				}
				util.HandleTestingErr(host2.Insert(), t, "error inserting host")

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
					Distro:    "d1",
					Status:    mci.HostRunning,
					StartedBy: mci.MCIUser,
					Provider:  mock.ProviderName,
				}
				util.HandleTestingErr(host1.Insert(), t, "error inserting host")

				host2 := &host.Host{
					Id:        "h2",
					Distro:    "d2",
					Status:    mci.HostRunning,
					StartedBy: mci.MCIUser,
					Provider:  mock.ProviderName,
				}
				util.HandleTestingErr(host2.Insert(), t, "error inserting host")

				host3 := &host.Host{
					Id:        "h3",
					Distro:    "d2",
					Status:    mci.HostRunning,
					StartedBy: mci.MCIUser,
					Provider:  mock.ProviderName,
				}
				util.HandleTestingErr(host3.Insert(), t, "error inserting host")

				host4 := &host.Host{
					Id:        "h4",
					Distro:    "d2",
					Status:    mci.HostRunning,
					StartedBy: mci.MCIUser,
					Provider:  mock.ProviderName,
				}
				util.HandleTestingErr(host4.Insert(), t, "error inserting host")

				// flag the excess hosts - there should be 2, both from
				// the second distro
				excess, err := flagExcessHosts(distros, nil)
				So(err, ShouldBeNil)
				So(len(excess), ShouldEqual, 2)
				for _, host := range excess {
					So(host.Distro, ShouldEqual, "d2")
				}

			})

			Convey("if both distros have excess hosts, the appropriate number"+
				" of hosts from each distro should be flagged", func() {

				// insert three hosts for each distro

				host1 := &host.Host{
					Id:        "h1",
					Distro:    "d1",
					Status:    mci.HostRunning,
					StartedBy: mci.MCIUser,
					Provider:  mock.ProviderName,
				}
				util.HandleTestingErr(host1.Insert(), t, "error inserting host")

				host2 := &host.Host{
					Id:        "h2",
					Distro:    "d1",
					Status:    mci.HostRunning,
					StartedBy: mci.MCIUser,
					Provider:  mock.ProviderName,
				}
				util.HandleTestingErr(host2.Insert(), t, "error inserting host")

				host3 := &host.Host{
					Id:        "h3",
					Distro:    "d1",
					Status:    mci.HostRunning,
					StartedBy: mci.MCIUser,
					Provider:  mock.ProviderName,
				}
				util.HandleTestingErr(host3.Insert(), t, "error inserting host")

				host4 := &host.Host{
					Id:        "h4",
					Distro:    "d2",
					Status:    mci.HostRunning,
					StartedBy: mci.MCIUser,
					Provider:  mock.ProviderName,
				}
				util.HandleTestingErr(host4.Insert(), t, "error inserting host")

				host5 := &host.Host{
					Id:        "h5",
					Distro:    "d2",
					Status:    mci.HostRunning,
					StartedBy: mci.MCIUser,
					Provider:  mock.ProviderName,
				}
				util.HandleTestingErr(host5.Insert(), t, "error inserting host")

				host6 := &host.Host{
					Id:        "h6",
					Distro:    "d2",
					Status:    mci.HostRunning,
					StartedBy: mci.MCIUser,
					Provider:  mock.ProviderName,
				}
				util.HandleTestingErr(host6.Insert(), t, "error inserting host")

				// find the excess hosts - there should be one for the first
				// distro and two for the second
				excess, err := flagExcessHosts(distros, nil)
				So(err, ShouldBeNil)
				So(len(excess), ShouldEqual, 3)
				excessFirst := 0
				excessSecond := 0
				for _, host := range excess {
					if host.Distro == "d1" {
						excessFirst++
					}
					if host.Distro == "d2" {
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
					Distro:      "d1",
					Status:      mci.HostRunning,
					StartedBy:   mci.MCIUser,
					RunningTask: "t1",
					Provider:    mock.ProviderName,
				}
				util.HandleTestingErr(host1.Insert(), t, "error inserting host")

				host2 := &host.Host{
					Id:          "h2",
					Distro:      "d1",
					Status:      mci.HostRunning,
					StartedBy:   mci.MCIUser,
					RunningTask: "t2",
					Provider:    mock.ProviderName,
				}
				util.HandleTestingErr(host2.Insert(), t, "error inserting host")

				host3 := &host.Host{
					Id:          "h3",
					Distro:      "d2",
					Status:      mci.HostRunning,
					StartedBy:   mci.MCIUser,
					RunningTask: "t3",
					Provider:    mock.ProviderName,
				}
				util.HandleTestingErr(host3.Insert(), t, "error inserting host")

				host4 := &host.Host{
					Id:          "h4",
					Distro:      "d2",
					Status:      mci.HostRunning,
					StartedBy:   mci.MCIUser,
					RunningTask: "t4",
					Provider:    mock.ProviderName,
				}
				util.HandleTestingErr(host4.Insert(), t, "error inserting host")

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

	testConfig := mci.TestConfig()

	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

	Convey("When flagging unprovisioned hosts to be terminated", t, func() {

		// reset the db
		util.HandleTestingErr(db.ClearCollections(host.Collection),
			t, "error clearing hosts collection")

		Convey("hosts that have not hit the provisioning limit should"+
			" be ignored", func() {

			host1 := &host.Host{
				Id:           "h1",
				StartedBy:    mci.MCIUser,
				CreationTime: time.Now().Add(-time.Minute * 10),
			}
			util.HandleTestingErr(host1.Insert(), t, "error inserting host")

			unprovisioned, err := flagUnprovisionedHosts(nil, nil)
			So(err, ShouldBeNil)
			So(len(unprovisioned), ShouldEqual, 0)

		})

		Convey("hosts that are already terminated should be ignored", func() {

			host1 := &host.Host{
				Id:           "h1",
				StartedBy:    mci.MCIUser,
				CreationTime: time.Now().Add(-time.Minute * 40),
				Status:       mci.HostTerminated,
			}
			util.HandleTestingErr(host1.Insert(), t, "error inserting host")

			unprovisioned, err := flagUnprovisionedHosts(nil, nil)
			So(err, ShouldBeNil)
			So(len(unprovisioned), ShouldEqual, 0)

		})

		Convey("hosts that are already provisioned should be ignored", func() {

			host1 := &host.Host{
				Id:           "h1",
				StartedBy:    mci.MCIUser,
				CreationTime: time.Now().Add(-time.Minute * 40),
				Provisioned:  true,
			}
			util.HandleTestingErr(host1.Insert(), t, "error inserting host")

			unprovisioned, err := flagUnprovisionedHosts(nil, nil)
			So(err, ShouldBeNil)
			So(len(unprovisioned), ShouldEqual, 0)

		})

		Convey("hosts that have hit the provisioning limit should be"+
			" flagged", func() {

			host1 := &host.Host{
				Id:           "h1",
				StartedBy:    mci.MCIUser,
				CreationTime: time.Now().Add(-time.Minute * 40),
			}
			util.HandleTestingErr(host1.Insert(), t, "error inserting host")

			unprovisioned, err := flagUnprovisionedHosts(nil, nil)
			So(err, ShouldBeNil)
			So(len(unprovisioned), ShouldEqual, 1)
			So(unprovisioned[0].Id, ShouldEqual, "h1")

		})

	})
}

func TestFlaggingProvisioningFailedHosts(t *testing.T) {

	testConfig := mci.TestConfig()

	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

	Convey("When flagging hosts whose provisioning failed", t, func() {

		// reset the db
		util.HandleTestingErr(db.ClearCollections(host.Collection),
			t, "error clearing hosts collection")

		Convey("only hosts whose provisioning failed should be"+
			" picked up", func() {

			host1 := &host.Host{
				Id:     "h1",
				Status: mci.HostRunning,
			}
			util.HandleTestingErr(host1.Insert(), t, "error inserting host")

			host2 := &host.Host{
				Id:     "h2",
				Status: mci.HostUninitialized,
			}
			util.HandleTestingErr(host2.Insert(), t, "error inserting host")

			host3 := &host.Host{
				Id:     "h3",
				Status: mci.HostProvisionFailed,
			}
			util.HandleTestingErr(host3.Insert(), t, "error inserting host")

			unprovisioned, err := flagProvisioningFailedHosts(nil, nil)
			So(err, ShouldBeNil)
			So(len(unprovisioned), ShouldEqual, 1)
			So(unprovisioned[0].Id, ShouldEqual, "h3")

		})

	})
}

func TestFlaggingExpiredHosts(t *testing.T) {

	testConfig := mci.TestConfig()

	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

	Convey("When flagging expired hosts to be terminated", t, func() {

		// reset the db
		util.HandleTestingErr(db.ClearCollections(host.Collection),
			t, "error clearing hosts collection")

		Convey("hosts started by the default user should be filtered"+
			" out", func() {

			host1 := &host.Host{
				Id:        "h1",
				Status:    mci.HostRunning,
				StartedBy: mci.MCIUser,
			}
			util.HandleTestingErr(host1.Insert(), t, "error inserting host")

			expired, err := flagExpiredHosts(nil, nil)
			So(err, ShouldBeNil)
			So(len(expired), ShouldEqual, 0)

		})

		Convey("hosts that are terminated or quarantined should be filtered"+
			" out", func() {

			host1 := &host.Host{
				Id:     "h1",
				Status: mci.HostQuarantined,
			}
			util.HandleTestingErr(host1.Insert(), t, "error inserting host")

			host2 := &host.Host{
				Id:     "h2",
				Status: mci.HostTerminated,
			}
			util.HandleTestingErr(host2.Insert(), t, "error inserting host")

			expired, err := flagExpiredHosts(nil, nil)
			So(err, ShouldBeNil)
			So(len(expired), ShouldEqual, 0)

		})

		Convey("hosts should be returned if their expiration threshold has"+
			" been reached", func() {

			// not expired
			host1 := &host.Host{
				Id:             "h1",
				Status:         mci.HostRunning,
				ExpirationTime: time.Now().Add(time.Minute * 10),
			}
			util.HandleTestingErr(host1.Insert(), t, "error inserting host")

			// expired
			host2 := &host.Host{
				Id:             "h2",
				Status:         mci.HostRunning,
				ExpirationTime: time.Now().Add(-time.Minute * 10),
			}
			util.HandleTestingErr(host2.Insert(), t, "error inserting host")

			expired, err := flagExpiredHosts(nil, nil)
			So(err, ShouldBeNil)
			So(len(expired), ShouldEqual, 1)
			So(expired[0].Id, ShouldEqual, "h2")

		})

	})

}
