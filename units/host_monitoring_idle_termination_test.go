package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/host"
	modelUtil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	. "github.com/smartystreets/goconvey/convey"
)

func flagIdleHosts(ctx context.Context, env evergreen.Environment) ([]string, error) {
	queue := queue.NewAdaptiveOrderedLocalQueue(3, 1024)
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
