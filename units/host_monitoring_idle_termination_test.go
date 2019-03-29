package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	modelUtil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	. "github.com/smartystreets/goconvey/convey"
)

func flagIdleHosts(ctx context.Context, env evergreen.Environment) ([]string, error) {
	queue := queue.NewAdaptiveOrderedLocalQueue(3)
	if err := queue.Start(ctx); err != nil {
		return nil, err
	}
	defer queue.Runner().Close()

	if err := PopulateIdleHostJobs(env)(queue); err != nil {
		return nil, err
	}

	amboy.WaitCtxInterval(ctx, queue, 50*time.Millisecond)

	terminated := []string{}

	for j := range queue.Results(ctx) {
		if ij, ok := j.(*idleHostJob); ok {
			if ij.Terminated {
				terminated = append(terminated, ij.HostID)
			}
		}
	}

	return terminated, nil
}

////////////////////////////////////////////////////////////////////////
//
// legacy test case

func TestFlaggingIdleHosts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testConfig := testutil.TestConfig()
	db.SetGlobalSessionProvider(testConfig.SessionFactory())

	env := evergreen.GetEnvironment()

	Convey("When flagging idle hosts to be terminated", t, func() {

		// reset the db
		testutil.HandleTestingErr(db.ClearCollections(distro.Collection),
			t, "error clearing distros collection")
		testutil.HandleTestingErr(db.ClearCollections(host.Collection),
			t, "error clearing hosts collection")
		testutil.HandleTestingErr(modelUtil.AddTestIndexes(host.Collection,
			true, true, host.RunningTaskKey), t, "error adding host index")

		// insert our reference distro.Distro
		distro1 := distro.Distro{
			Id: "distro1",
		}
		testutil.HandleTestingErr(distro1.Insert(), t, "error inserting distro")

		Convey("hosts currently running a task should never be"+
			" flagged", func() {

			// insert a host that is currently running a task - but whose
			// creation time would otherwise indicate it has been idle a while
			host1 := host.Host{
				Id:           "h1",
				Distro:       distro.Distro{Id: "distro1"},
				Provider:     evergreen.ProviderNameMock,
				CreationTime: time.Now().Add(-30 * time.Minute),
				RunningTask:  "t1",
				Status:       evergreen.HostRunning,
				StartedBy:    evergreen.User,
			}
			testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

			// finding idle hosts should not return the host
			idle, err := flagIdleHosts(ctx, env)
			So(err, ShouldBeNil)
			So(len(idle), ShouldEqual, 0)

			Convey("even if they have a last communication time > 10 minutes", func() {
				h2 := host.Host{
					Id:                    "anotherhost",
					Distro:                distro.Distro{Id: "distro1"},
					Provider:              evergreen.ProviderNameMock,
					CreationTime:          time.Now().Add(-30 * time.Minute),
					RunningTask:           "t3",
					Status:                evergreen.HostRunning,
					LastCommunicationTime: time.Now().Add(-30 * time.Minute),
					StartedBy:             evergreen.User,
				}
				testutil.HandleTestingErr(h2.Insert(), t, "error inserting host")
				// finding idle hosts should not return the host
				idle, err := flagIdleHosts(ctx, env)
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
				Distro:                distro.Distro{Id: "distro1"},
				Provider:              evergreen.ProviderNameMock,
				LastTask:              "t1",
				LastTaskCompletedTime: time.Now().Add(-time.Minute * 20),
				LastCommunicationTime: time.Now(),
				Status:                evergreen.HostRunning,
				StartedBy:             evergreen.User,
				Provisioned:           true,
			}
			testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

			host2 := host.Host{
				Id:                    "h3",
				Distro:                distro.Distro{Id: "distro1"},
				Provider:              evergreen.ProviderNameMock,
				LastTask:              "t2",
				LastTaskCompletedTime: time.Now().Add(-time.Minute * 2),
				LastCommunicationTime: time.Now(),
				Status:                evergreen.HostRunning,
				StartedBy:             evergreen.User,
				Provisioned:           true,
			}

			testutil.HandleTestingErr(host2.Insert(), t, "error inserting host")

			// finding idle hosts should only return the first host
			idle, err := flagIdleHosts(ctx, env)
			So(err, ShouldBeNil)
			So(len(idle), ShouldEqual, 1)
			So(idle[0], ShouldEqual, "h2")

		})
		Convey("hosts not currently running a task with a last communication time greater"+
			"than 10 mins should be marked as idle", func() {
			anotherHost := host.Host{
				Id:                    "h4",
				Distro:                distro.Distro{Id: "distro1"},
				Provider:              evergreen.ProviderNameMock,
				LastCommunicationTime: time.Now().Add(-time.Minute * 20),
				Status:                evergreen.HostRunning,
				StartedBy:             evergreen.User,
			}
			So(anotherHost.Insert(), ShouldBeNil)
			// finding idle hosts should only return the first host
			idle, err := flagIdleHosts(ctx, env)
			So(err, ShouldBeNil)
			So(len(idle), ShouldEqual, 1)
			So(idle[0], ShouldEqual, "h4")
		})
		Convey("hosts that have been provisioned should have the timer reset", func() {
			now := time.Now()
			h5 := host.Host{
				Id:                    "h5",
				Distro:                distro.Distro{Id: "distro1"},
				Provider:              evergreen.ProviderNameMock,
				LastCommunicationTime: now,
				Status:                evergreen.HostRunning,
				StartedBy:             evergreen.User,
				CreationTime:          now.Add(-10 * time.Minute), // created before the cutoff
				ProvisionTime:         now.Add(-2 * time.Minute),  // provisioned after the cutoff
			}
			So(h5.Insert(), ShouldBeNil)

			// h5 should not be flagged as idle
			idle, err := flagIdleHosts(ctx, env)
			So(err, ShouldBeNil)
			So(len(idle), ShouldEqual, 0)
		})
	})
}

////////////////////////////////////////////////////////////////////////
//
// Testing with reference distro ids that are not present in the 'distro' collection in the database
//

func TestFlaggingIdleHostsWithMissingDistroIDs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testConfig := testutil.TestConfig()
	db.SetGlobalSessionProvider(testConfig.SessionFactory())

	env := evergreen.GetEnvironment()

	Convey("When flagging idle hosts to be terminated", t, func() {

		// reset the db
		testutil.HandleTestingErr(db.ClearCollections(distro.Collection),
			t, "error clearing distros collection")
		testutil.HandleTestingErr(db.ClearCollections(host.Collection),
			t, "error clearing hosts collection")

		// insert two reference distro.Distro
		distro1 := distro.Distro{
			Id: "distro1",
			PlannerSettings: distro.PlannerSettings{
				MinimumHosts: 2,
			},
		}
		distro2 := distro.Distro{
			Id: "distro2",
			PlannerSettings: distro.PlannerSettings{
				MinimumHosts: 1,
			},
		}

		testutil.HandleTestingErr(distro1.Insert(), t, "error inserting distro")
		testutil.HandleTestingErr(distro2.Insert(), t, "error inserting distro")

		Convey("Add some hosts with referenced host.Distro that do not exist in the distro collection ", func() {
			host1 := host.Host{
				Id:           "h1",
				Distro:       distro.Distro{Id: "distro2"},
				Provider:     evergreen.ProviderNameMock,
				CreationTime: time.Now().Add(-10 * time.Minute),
				Status:       evergreen.HostRunning,
				StartedBy:    evergreen.User,
			}
			host2 := host.Host{
				Id:           "h2",
				Distro:       distro.Distro{Id: "distro1"},
				Provider:     evergreen.ProviderNameMock,
				CreationTime: time.Now().Add(-20 * time.Minute),
				Status:       evergreen.HostRunning,
				StartedBy:    evergreen.User,
			}
			host3 := host.Host{
				Id:           "h3",
				Distro:       distro.Distro{Id: "c"},
				Provider:     evergreen.ProviderNameMock,
				CreationTime: time.Now().Add(-30 * time.Minute),
				Status:       evergreen.HostRunning,
				StartedBy:    evergreen.User,
			}
			host4 := host.Host{
				Id:           "h4",
				Distro:       distro.Distro{Id: "a"},
				Provider:     evergreen.ProviderNameMock,
				CreationTime: time.Now().Add(-30 * time.Minute),
				Status:       evergreen.HostRunning,
				StartedBy:    evergreen.User,
			}
			host5 := host.Host{
				Id:           "h5",
				Distro:       distro.Distro{Id: "z"},
				Provider:     evergreen.ProviderNameMock,
				CreationTime: time.Now().Add(-20 * time.Minute),
				Status:       evergreen.HostRunning,
				StartedBy:    evergreen.User,
			}
			testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")
			testutil.HandleTestingErr(host2.Insert(), t, "error inserting host")
			testutil.HandleTestingErr(host3.Insert(), t, "error inserting host")
			testutil.HandleTestingErr(host4.Insert(), t, "error inserting host")
			testutil.HandleTestingErr(host5.Insert(), t, "error inserting host")

			// If encountered missing distros, we exit early before we ever check for hosts to flag as idle
			idle, err := flagIdleHosts(ctx, env)
			So(err.Error(), ShouldContainSubstring, "distro ids a,c,z not found")
			So(len(idle), ShouldEqual, 0)
		})
	})
}

////////////////////////////////////////////////////////////////////////
//
// Testing with non-zero values for Distro.PlannerSettings.MinimumHosts
//

func TestFlaggingIdleHostsWhenNonZeroMinimumHosts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testConfig := testutil.TestConfig()
	db.SetGlobalSessionProvider(testConfig.SessionFactory())

	env := evergreen.GetEnvironment()

	Convey("When flagging idle hosts to be terminated", t, func() {

		// reset the db
		testutil.HandleTestingErr(db.ClearCollections(distro.Collection),
			t, "error clearing distros collection")
		testutil.HandleTestingErr(db.ClearCollections(host.Collection),
			t, "error clearing hosts collection")

		// insert a reference distro.Distro
		distro1 := distro.Distro{
			Id: "distro1",
			PlannerSettings: distro.PlannerSettings{
				MinimumHosts: 2,
			},
		}

		testutil.HandleTestingErr(distro1.Insert(), t, "error inserting distro")

		Convey("Neither host should be flagged as idle as MinimumHosts is 2", func() {
			host1 := host.Host{
				Id:           "h1",
				Distro:       distro.Distro{Id: "distro1"},
				Provider:     evergreen.ProviderNameMock,
				CreationTime: time.Now().Add(-30 * time.Minute),
				Status:       evergreen.HostRunning,
				StartedBy:    evergreen.User,
			}
			host2 := host.Host{
				Id:           "h2",
				Distro:       distro.Distro{Id: "distro1"},
				Provider:     evergreen.ProviderNameMock,
				CreationTime: time.Now().Add(-20 * time.Minute),
				Status:       evergreen.HostRunning,
				StartedBy:    evergreen.User,
			}
			testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")
			testutil.HandleTestingErr(host2.Insert(), t, "error inserting host")

			// finding idle hosts should not return either host
			idle, err := flagIdleHosts(ctx, env)
			So(err, ShouldBeNil)
			So(len(idle), ShouldEqual, 0)
		})

		Convey("MinimumHosts is 2; 1 host is running a task and 2 hosts are idle", func() {
			host1 := host.Host{
				Id:           "h1",
				Distro:       distro.Distro{Id: "distro1"},
				Provider:     evergreen.ProviderNameMock,
				CreationTime: time.Now().Add(-30 * time.Minute),
				Status:       evergreen.HostRunning,
				StartedBy:    evergreen.User,
			}
			host2 := host.Host{
				Id:           "h2",
				Distro:       distro.Distro{Id: "distro1"},
				Provider:     evergreen.ProviderNameMock,
				CreationTime: time.Now().Add(-20 * time.Minute),
				Status:       evergreen.HostRunning,
				StartedBy:    evergreen.User,
			}
			host3 := host.Host{
				Id:           "h3",
				Distro:       distro.Distro{Id: "distro1"},
				Provider:     evergreen.ProviderNameMock,
				CreationTime: time.Now().Add(-10 * time.Minute),
				Status:       evergreen.HostRunning,
				StartedBy:    evergreen.User,
				RunningTask:  "t1",
			}
			testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")
			testutil.HandleTestingErr(host2.Insert(), t, "error inserting host")
			testutil.HandleTestingErr(host3.Insert(), t, "error inserting host")

			// Only the oldest host not running a task should be flagged as idle - leaving 2 running hosts.
			idle, err := flagIdleHosts(ctx, env)
			So(err, ShouldBeNil)
			So(len(idle), ShouldEqual, 1)
			So(idle[0], ShouldEqual, "h1")
		})
	})
}
