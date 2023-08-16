package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	modelUtil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
)

func numIdleHostsFound(ctx context.Context, env evergreen.Environment, t *testing.T) (int, []string) {
	queue := queue.NewLocalLimitedSize(3, 1024)
	require.NoError(t, queue.Start(ctx))
	defer queue.Runner().Close(ctx)

	jobs, err := idleHostJobs(ctx, time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.NoError(t, queue.PutMany(ctx, jobs))

	amboy.WaitInterval(ctx, queue, 50*time.Millisecond)
	out := []string{}
	num := 0
	for j := range queue.Results(ctx) {
		if ij, ok := j.(*idleHostJob); ok {
			num += ij.Terminated
			out = append(out, ij.TerminatedHosts...)
		}
	}

	assert.Equal(t, num, len(out))

	return num, out
}

// testFlaggingIdleHostsSetupTest resets the relevant db collections prior to a
// test.
func testFlaggingIdleHostsSetupTest(t *testing.T) {
	require.NoError(t, db.DropCollections(host.Collection, distro.Collection), "dropping collections")
	require.NoError(t, modelUtil.AddTestIndexes(host.Collection, true, true, host.RunningTaskKey), "adding running_task_1 index")
	require.NoError(t, modelUtil.AddTestIndexes(host.Collection, false, false, host.StartedByKey, host.StatusKey), "adding started_by_1_status_1 index")
}

// testFlaggingIdleHostsTeardownTest resets the relevant DB collections after a
// test.
func testFlaggingIdleHostsTeardownTest(t *testing.T) {
	assert.NoError(t, db.DropCollections(host.Collection, distro.Collection), "dropping collections")
}

////////////////////////////////////////////////////////////////////////
//
// legacy test case

func TestFlaggingIdleHosts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = testutil.TestSpan(ctx, t)

	env := evergreen.GetEnvironment()

	t.Run("HostsCurrentlyRunningTasksShouldNeverBeFlagged", func(t *testing.T) {
		tctx := testutil.TestSpan(ctx, t)
		testFlaggingIdleHostsSetupTest(t)
		defer testFlaggingIdleHostsTeardownTest(t)

		// insert a reference distro.Distro
		distro1 := distro.Distro{
			Id:       "distro1",
			Provider: evergreen.ProviderNameMock,
			HostAllocatorSettings: distro.HostAllocatorSettings{
				AcceptableHostIdleTime: 4 * time.Minute,
			},
		}
		require.NoError(t, distro1.Insert(tctx))
		// insert a host that is currently running a task - but whose
		// creation time would otherwise indicate it has been idle a while
		host1 := host.Host{
			Id:           "h1",
			Distro:       distro1,
			Provider:     evergreen.ProviderNameMock,
			CreationTime: time.Now().Add(-30 * time.Minute),
			RunningTask:  "t1",
			Status:       evergreen.HostRunning,
			StartedBy:    evergreen.User,
		}
		require.NoError(t, host1.Insert(tctx))

		// finding idle hosts should not return the host
		num, hosts := numIdleHostsFound(tctx, env, t)
		assert.Equal(t, 0, num)
		assert.Empty(t, hosts)
	})

	t.Run("EvenWithLastCommunicationTimeGreaterThanTenMinutes", func(t *testing.T) {
		tctx := testutil.TestSpan(ctx, t)
		testFlaggingIdleHostsSetupTest(t)
		defer testFlaggingIdleHostsTeardownTest(t)

		distro1 := distro.Distro{
			Id:       "distro1",
			Provider: evergreen.ProviderNameMock,
			HostAllocatorSettings: distro.HostAllocatorSettings{
				AcceptableHostIdleTime: 4 * time.Minute,
			},
		}
		require.NoError(t, distro1.Insert(tctx))

		host1 := host.Host{
			Id:                    "h1",
			Distro:                distro1,
			Provider:              evergreen.ProviderNameMock,
			CreationTime:          time.Now().Add(-30 * time.Minute),
			RunningTask:           "t3",
			Status:                evergreen.HostRunning,
			LastCommunicationTime: time.Now().Add(-30 * time.Minute),
			StartedBy:             evergreen.User,
		}
		require.NoError(t, host1.Insert(tctx))

		num, hosts := numIdleHostsFound(tctx, env, t)
		assert.Equal(t, 0, num)
		assert.Empty(t, hosts)
	})

	t.Run("HostsNotRunningTasksShouldBeFlaggedIfTheyHaveBeenIdleLongerThanIdleThreshold", func(t *testing.T) {
		tctx := testutil.TestSpan(ctx, t)
		testFlaggingIdleHostsSetupTest(t)
		defer testFlaggingIdleHostsTeardownTest(t)

		distro1 := distro.Distro{
			Id:       "distro1",
			Provider: evergreen.ProviderNameMock,
			HostAllocatorSettings: distro.HostAllocatorSettings{
				AcceptableHostIdleTime: 4 * time.Minute,
			},
		}
		require.NoError(t, distro1.Insert(tctx))

		host1 := host.Host{
			Id:                    "h1",
			Distro:                distro1,
			Provider:              evergreen.ProviderNameMock,
			LastTask:              "t1",
			LastTaskCompletedTime: time.Now().Add(-time.Minute * 20),
			LastCommunicationTime: time.Now(),
			Status:                evergreen.HostRunning,
			StartedBy:             evergreen.User,
			Provisioned:           true,
		}
		host2 := host.Host{
			Id:                    "h2",
			Distro:                distro1,
			Provider:              evergreen.ProviderNameMock,
			LastTask:              "t2",
			LastTaskCompletedTime: time.Now().Add(-time.Minute * 2),
			LastCommunicationTime: time.Now(),
			Status:                evergreen.HostRunning,
			StartedBy:             evergreen.User,
			Provisioned:           true,
		}
		require.NoError(t, host1.Insert(tctx))
		require.NoError(t, host2.Insert(tctx))

		num, hosts := numIdleHostsFound(tctx, env, t)
		assert.Equal(t, 1, num)
		assert.Equal(t, hosts[0], "h1")
	})

	t.Run("LegacyHostsThatNeedNewAgentsShouldNotBeMarkedIdle", func(t *testing.T) {
		tctx := testutil.TestSpan(ctx, t)
		testFlaggingIdleHostsSetupTest(t)
		defer testFlaggingIdleHostsTeardownTest(t)

		distro1 := distro.Distro{
			Id:       "distro1",
			Provider: evergreen.ProviderNameMock,
			HostAllocatorSettings: distro.HostAllocatorSettings{
				AcceptableHostIdleTime: 4 * time.Minute,
			},
			BootstrapSettings: distro.BootstrapSettings{
				Method:        distro.BootstrapMethodLegacySSH,
				Communication: distro.CommunicationMethodLegacySSH,
			},
		}
		require.NoError(t, distro1.Insert(tctx))

		host1 := host.Host{
			Id:                    "h1",
			Distro:                distro1,
			Provider:              evergreen.ProviderNameMock,
			Status:                evergreen.HostRunning,
			CreationTime:          time.Now().Add(-30 * time.Minute),
			LastCommunicationTime: time.Now(),
			StartedBy:             evergreen.User,
			NeedsNewAgent:         true,
		}
		require.NoError(t, host1.Insert(tctx))

		num, hosts := numIdleHostsFound(tctx, env, t)
		assert.Equal(t, 0, num)
		assert.Empty(t, hosts)
	})

	t.Run("NonLegacyHostsThatNeedNewAgentMonitorsShouldNotBeMarkedIdle", func(t *testing.T) {
		tctx := testutil.TestSpan(ctx, t)
		testFlaggingIdleHostsSetupTest(t)
		defer testFlaggingIdleHostsTeardownTest(t)

		distro1 := distro.Distro{
			Id:       "distro1",
			Provider: evergreen.ProviderNameMock,
			HostAllocatorSettings: distro.HostAllocatorSettings{
				AcceptableHostIdleTime: 4 * time.Minute,
			},
			BootstrapSettings: distro.BootstrapSettings{
				Method:        distro.BootstrapMethodSSH,
				Communication: distro.CommunicationMethodSSH,
			},
		}
		require.NoError(t, distro1.Insert(tctx))

		host1 := host.Host{
			Id:                    "h1",
			Distro:                distro1,
			Provider:              evergreen.ProviderNameMock,
			Status:                evergreen.HostRunning,
			CreationTime:          time.Now().Add(-30 * time.Minute),
			LastCommunicationTime: time.Now(),
			StartedBy:             evergreen.User,
			NeedsNewAgentMonitor:  true,
		}
		require.NoError(t, host1.Insert(tctx))

		num, hosts := numIdleHostsFound(tctx, env, t)
		assert.Equal(t, 0, num)
		assert.Empty(t, hosts)
	})

	t.Run("NonLegacyHostsThatDoNotNeedNewAgentMonitorsShouldBeMarkedIdle", func(t *testing.T) {
		tctx := testutil.TestSpan(ctx, t)
		testFlaggingIdleHostsSetupTest(t)
		defer testFlaggingIdleHostsTeardownTest(t)

		distro1 := distro.Distro{
			Id:       "distro1",
			Provider: evergreen.ProviderNameMock,
			HostAllocatorSettings: distro.HostAllocatorSettings{
				AcceptableHostIdleTime: 4 * time.Minute,
			},
			BootstrapSettings: distro.BootstrapSettings{
				Method:        distro.BootstrapMethodSSH,
				Communication: distro.CommunicationMethodSSH,
			},
		}
		require.NoError(t, distro1.Insert(tctx))

		host1 := host.Host{
			Id:                    "host1",
			Distro:                distro1,
			Provider:              evergreen.ProviderNameMock,
			Status:                evergreen.HostRunning,
			CreationTime:          time.Now().Add(-24 * time.Hour),
			LastCommunicationTime: time.Now(),
			StartedBy:             evergreen.User,
			NeedsNewAgent:         true,
		}
		require.NoError(t, host1.Insert(tctx))

		// finding idle hosts should not return the host
		num, hosts := numIdleHostsFound(tctx, env, t)
		assert.Equal(t, 1, num)
		assert.Equal(t, hosts[0], "host1")
	})
}

////////////////////////////////////////////////////////////////////////
//
// Testing with reference distro ids that are not present in the 'distro' collection in the database
//

func TestFlaggingIdleHostsWithMissingDistroIDs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = testutil.TestSpan(ctx, t)

	env := evergreen.GetEnvironment()

	t.Run("AddSomeHostsWithReferencedDistrosThatDoNotExistInTheDistroCollection", func(t *testing.T) {
		tctx := testutil.TestSpan(ctx, t)
		testFlaggingIdleHostsSetupTest(t)
		defer testFlaggingIdleHostsTeardownTest(t)

		distro1 := distro.Distro{
			Id:       "distro1",
			Provider: evergreen.ProviderNameMock,
			HostAllocatorSettings: distro.HostAllocatorSettings{
				MinimumHosts: 2,
			},
		}
		distro2 := distro.Distro{
			Id:       "distro2",
			Provider: evergreen.ProviderNameMock,
			HostAllocatorSettings: distro.HostAllocatorSettings{
				MinimumHosts: 1,
			},
		}
		require.NoError(t, distro1.Insert(tctx))
		require.NoError(t, distro2.Insert(tctx))

		host1 := host.Host{
			Id:           "h1",
			Distro:       distro2,
			Provider:     evergreen.ProviderNameMock,
			CreationTime: time.Now().Add(-10 * time.Minute),
			Status:       evergreen.HostRunning,
			StartedBy:    evergreen.User,
		}
		host2 := host.Host{
			Id:           "h2",
			Distro:       distro1,
			Provider:     evergreen.ProviderNameMock,
			CreationTime: time.Now().Add(-20 * time.Minute),
			Status:       evergreen.HostRunning,
			StartedBy:    evergreen.User,
		}
		host3 := host.Host{
			Id: "h3",
			Distro: distro.Distro{
				Id:       "distroZ",
				Provider: evergreen.ProviderNameMock,
			},
			Provider:     evergreen.ProviderNameMock,
			CreationTime: time.Now().Add(-30 * time.Minute),
			Status:       evergreen.HostRunning,
			StartedBy:    evergreen.User,
		}
		host4 := host.Host{
			Id: "h4",
			Distro: distro.Distro{
				Id:       "distroA",
				Provider: evergreen.ProviderNameMock,
			},
			Provider:     evergreen.ProviderNameMock,
			CreationTime: time.Now().Add(-30 * time.Minute),
			Status:       evergreen.HostRunning,
			StartedBy:    evergreen.User,
		}
		host5 := host.Host{
			Id: "h5",
			Distro: distro.Distro{
				Id:       "distroC",
				Provider: evergreen.ProviderNameMock,
			},
			Provider:     evergreen.ProviderNameMock,
			CreationTime: time.Now().Add(-20 * time.Minute),
			Status:       evergreen.HostRunning,
			StartedBy:    evergreen.User,
		}
		require.NoError(t, host1.Insert(tctx))
		require.NoError(t, host2.Insert(tctx))
		require.NoError(t, host3.Insert(tctx))
		require.NoError(t, host4.Insert(tctx))
		require.NoError(t, host5.Insert(tctx))

		// If we encounter missing distros, we decommission hosts from those
		// distros.
		num, hosts := numIdleHostsFound(tctx, env, t)
		assert.Equal(t, 3, num)

		assert.Contains(t, hosts, "h3")
		assert.Contains(t, hosts, "h4")
		assert.Contains(t, hosts, "h5")
	})
}

////////////////////////////////////////////////////////////////////////
//
// Testing with non-zero values for Distro.HostAllocatorSettings.MinimumHosts
//

func TestFlaggingIdleHostsWhenNonZeroMinimumHosts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = testutil.TestSpan(ctx, t)

	env := evergreen.GetEnvironment()

	t.Run("NeitherHostShouldBeFlaggedAsIdleAsMinimumHostsIsTwo", func(t *testing.T) {
		tctx := testutil.TestSpan(ctx, t)
		testFlaggingIdleHostsSetupTest(t)
		defer testFlaggingIdleHostsTeardownTest(t)

		distro1 := distro.Distro{
			Id:       "distro1",
			Provider: evergreen.ProviderNameMock,
			HostAllocatorSettings: distro.HostAllocatorSettings{
				MinimumHosts: 2,
			},
		}
		require.NoError(t, distro1.Insert(tctx))

		host1 := host.Host{
			Id:           "h1",
			Distro:       distro1,
			Provider:     evergreen.ProviderNameMock,
			CreationTime: time.Now().Add(-30 * time.Minute),
			Status:       evergreen.HostRunning,
			StartedBy:    evergreen.User,
		}
		host2 := host.Host{
			Id:           "h2",
			Distro:       distro1,
			Provider:     evergreen.ProviderNameMock,
			CreationTime: time.Now().Add(-20 * time.Minute),
			Status:       evergreen.HostRunning,
			StartedBy:    evergreen.User,
		}
		require.NoError(t, host1.Insert(tctx))
		require.NoError(t, host2.Insert(tctx))

		num, hosts := numIdleHostsFound(tctx, env, t)
		assert.Equal(t, 0, num)
		assert.Empty(t, hosts)
	})

	t.Run("MinimumHostsIsTwo;OneHostIsRunningItsTaskAndTwoHostsAreIdle", func(t *testing.T) {
		tctx := testutil.TestSpan(ctx, t)
		testFlaggingIdleHostsSetupTest(t)
		defer testFlaggingIdleHostsTeardownTest(t)

		distro1 := distro.Distro{
			Id:       "distro1",
			Provider: evergreen.ProviderNameMock,
			HostAllocatorSettings: distro.HostAllocatorSettings{
				MinimumHosts: 2,
			},
		}
		require.NoError(t, distro1.Insert(tctx))

		host1 := host.Host{
			Id:           "h1",
			Distro:       distro1,
			Provider:     evergreen.ProviderNameMock,
			CreationTime: time.Now().Add(-30 * time.Minute),
			Status:       evergreen.HostRunning,
			StartedBy:    evergreen.User,
		}
		host2 := host.Host{
			Id:           "h2",
			Distro:       distro1,
			Provider:     evergreen.ProviderNameMock,
			CreationTime: time.Now().Add(-20 * time.Minute),
			Status:       evergreen.HostRunning,
			StartedBy:    evergreen.User,
		}
		host3 := host.Host{
			Id:           "h3",
			Distro:       distro1,
			Provider:     evergreen.ProviderNameMock,
			CreationTime: time.Now().Add(-10 * time.Minute),
			Status:       evergreen.HostRunning,
			StartedBy:    evergreen.User,
			RunningTask:  "t1",
		}
		require.NoError(t, host1.Insert(tctx))
		require.NoError(t, host2.Insert(tctx))
		require.NoError(t, host3.Insert(tctx))

		// Only the oldest host not running a task should be flagged as idle.
		num, hosts := numIdleHostsFound(tctx, env, t)
		assert.Equal(t, 1, num)
		assert.Equal(t, "h1", hosts[0])
	})
}

func TestPopulateIdleHostJobsCalculations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = testutil.TestSpan(ctx, t)

	assert := assert.New(t)
	assert.NoError(db.DropCollections(host.Collection, distro.Collection))
	defer func() {
		assert.NoError(db.DropCollections(host.Collection, distro.Collection))
	}()
	env := mock.Environment{}
	assert.NoError(env.Configure(ctx))

	require.NoError(t, db.EnsureIndex(host.Collection, mongo.IndexModel{
		Keys: host.StartedByStatusIndex,
	}))

	distro1 := distro.Distro{
		Id:       "distro1",
		Provider: evergreen.ProviderNameMock,
		HostAllocatorSettings: distro.HostAllocatorSettings{
			MinimumHosts: 3,
		},
	}

	distro2 := distro.Distro{
		Id:       "distro2",
		Provider: evergreen.ProviderNameMock,
		HostAllocatorSettings: distro.HostAllocatorSettings{
			MinimumHosts: 0,
		},
	}
	assert.NoError(distro1.Insert(ctx))
	assert.NoError(distro2.Insert(ctx))

	host1 := &host.Host{
		Id:            "host1",
		Distro:        distro1,
		Status:        evergreen.HostRunning,
		StartedBy:     evergreen.User,
		Provider:      evergreen.ProviderNameMock,
		HasContainers: false,
		CreationTime:  time.Now().Add(-20 * time.Minute),
	}
	host2 := &host.Host{
		Id:            "host2",
		Distro:        distro1,
		Status:        evergreen.HostRunning,
		StartedBy:     evergreen.User,
		Provider:      evergreen.ProviderNameMock,
		HasContainers: false,
		CreationTime:  time.Now().Add(-10 * time.Minute),
	}
	host3 := &host.Host{
		Id:            "host3",
		Distro:        distro2,
		Status:        evergreen.HostRunning,
		StartedBy:     evergreen.User,
		Provider:      evergreen.ProviderNameMock,
		HasContainers: false,
		CreationTime:  time.Now().Add(-30 * time.Minute),
	}
	host4 := &host.Host{
		Id: "host4",

		Distro:        distro1,
		Status:        evergreen.HostRunning,
		StartedBy:     evergreen.User,
		Provider:      evergreen.ProviderNameMock,
		HasContainers: false,
		CreationTime:  time.Now().Add(-40 * time.Minute),
	}
	host5 := &host.Host{
		Id:            "host5",
		Distro:        distro2,
		Status:        evergreen.HostRunning,
		StartedBy:     evergreen.User,
		Provider:      evergreen.ProviderNameMock,
		HasContainers: false,
		CreationTime:  time.Now().Add(-50 * time.Minute),
	}
	host6 := &host.Host{
		Id:            "host6",
		Distro:        distro1,
		RunningTask:   "I'm running a task so I'm certainly not idle!",
		Status:        evergreen.HostRunning,
		StartedBy:     evergreen.User,
		Provider:      evergreen.ProviderNameMock,
		HasContainers: false,
		CreationTime:  time.Now().Add(-60 * time.Minute),
	}
	assert.NoError(host1.Insert(ctx))
	assert.NoError(host2.Insert(ctx))
	assert.NoError(host3.Insert(ctx))
	assert.NoError(host4.Insert(ctx))
	assert.NoError(host5.Insert(ctx))
	assert.NoError(host6.Insert(ctx))

	distroHosts, err := host.IdleEphemeralGroupedByDistroID(ctx, &env)
	assert.NoError(err)
	assert.Equal(2, len(distroHosts))

	distroIDsToFind := make([]string, 0, len(distroHosts))
	for _, info := range distroHosts {
		distroIDsToFind = append(distroIDsToFind, info.DistroID)
	}
	distrosFound, err := distro.Find(ctx, distro.ByIds(distroIDsToFind))
	assert.NoError(err)
	distrosMap := make(map[string]distro.Distro, len(distrosFound))
	for i := range distrosFound {
		d := distrosFound[i]
		distrosMap[d.Id] = d
	}
	assert.Equal(2, len(distrosMap))

	// The order of distroHosts is not guaranteed
	info1 := distroHosts[0] // "distro1"
	info2 := distroHosts[1] // "distro2"

	if info1.DistroID == "distro2" {
		info1 = distroHosts[1]
		info2 = distroHosts[0]
	}

	//////////////////////////////////////////////////////////////////////////////
	// distroID: "distro1"
	//
	// totalRunningHosts: 4
	// minimumHosts: 3
	// nIdleHosts: 3
	// maxHostsToTerminate: 1
	// nHostsToEvaluateForTermination: 1

	distroID := info1.DistroID
	assert.Equal("distro1", distroID)
	nIdleHosts := len(info1.IdleHosts)
	// Confirm the RunningHostsCount and the number of idle hosts for the given distro
	assert.Equal(4, info1.RunningHostsCount)
	assert.Equal(3, nIdleHosts)
	// Confirm the hosts are sorted from oldest to newest CreationTime
	assert.Equal("host4", info1.IdleHosts[0].Id)
	assert.Equal("host1", info1.IdleHosts[1].Id)
	assert.Equal("host2", info1.IdleHosts[2].Id)
	assert.True(info1.IdleHosts[0].CreationTime.Before(info1.IdleHosts[1].CreationTime))
	assert.True(info1.IdleHosts[1].CreationTime.Before(info1.IdleHosts[2].CreationTime))

	// Confirm the associated distro's HostAllocatorSettings.MinimumHosts value
	settings := distrosMap[info1.DistroID].HostAllocatorSettings
	minimumHosts := settings.MinimumHosts
	assert.Equal(3, minimumHosts)
	// Confirm the maxHostsToTerminate
	maxHostsToTerminate := info1.RunningHostsCount - minimumHosts
	assert.Equal(1, maxHostsToTerminate)
	// Confirm the nHostsToEvaluateForTermination
	nHostsToEvaluateForTermination := nIdleHosts
	if nIdleHosts > maxHostsToTerminate {
		nHostsToEvaluateForTermination = maxHostsToTerminate
	}
	assert.Equal(1, nHostsToEvaluateForTermination)

	////////////////////////////////////////////////////////////////////////////////
	// distroID: "distro2"
	//
	// totalRunningHosts: 2
	// minimumHosts: 0
	// nIdleHosts: 2
	// maxHostsToTerminate: 2
	// nHostsToEvaluateForTermination: 2

	distroID = info2.DistroID
	assert.Equal("distro2", distroID)
	nIdleHosts = len(info2.IdleHosts)
	// Confirm the RunningHostsCount and the number of idle hosts for the given distro
	assert.Equal(2, info2.RunningHostsCount)
	assert.Equal(2, nIdleHosts)
	// Confirm the hosts are sorted from oldest to newest CreationTime
	assert.Equal("host5", info2.IdleHosts[0].Id)
	assert.Equal("host3", info2.IdleHosts[1].Id)
	assert.True(info2.IdleHosts[0].CreationTime.Before(info2.IdleHosts[1].CreationTime))
	// Confirm the associated distro's HostAllocatorSettings.MinimumHosts value
	settings = distrosMap[info2.DistroID].HostAllocatorSettings
	minimumHosts = settings.MinimumHosts
	assert.Equal(0, minimumHosts)
	// Confirm the maxHostsToTerminate
	maxHostsToTerminate = info2.RunningHostsCount - minimumHosts
	assert.Equal(2, maxHostsToTerminate)
	// Confirm the nHostsToEvaluateForTermination
	nHostsToEvaluateForTermination = nIdleHosts
	if nIdleHosts > maxHostsToTerminate {
		nHostsToEvaluateForTermination = maxHostsToTerminate
	}
	assert.Equal(2, nHostsToEvaluateForTermination)
}

func TestGetNumHostsToEvaluate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testutil.TestSpan(ctx, t)

	info := host.IdleHostsByDistroID{
		DistroID: "d1",
		IdleHosts: []host.Host{
			{Id: "h1"},
			{Id: "h2"},
			{Id: "h3"},
		},
		RunningHostsCount: 5,
	}
	// If minimum hosts is 0, then we attempt to terminate all idle hosts.
	numHosts := getMinNumHostsToEvaluate(info, 0)
	assert.Equal(t, numHosts, 3)
	// If minimum hosts is 4, then we attempt to terminate just one.
	numHosts = getMinNumHostsToEvaluate(info, 4)
	assert.Equal(t, numHosts, 1)
	// If minimum hosts is 5, then we don't attempt to terminate.
	numHosts = getMinNumHostsToEvaluate(info, 5)
	assert.Equal(t, numHosts, 0)

}
