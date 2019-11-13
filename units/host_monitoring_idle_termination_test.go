package units

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	modelUtil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func flagIdleHosts(ctx context.Context, env evergreen.Environment) ([]string, error) {
	queue := queue.NewAdaptiveOrderedLocalQueue(3, 1024)
	if err := queue.Start(ctx); err != nil {
		return nil, err
	}
	defer queue.Runner().Close(ctx)

	if err := PopulateIdleHostJobs(env)(ctx, queue); err != nil {
		return nil, err
	}

	amboy.WaitInterval(ctx, queue, 50*time.Millisecond)

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

// testFlaggingIdleHostsSetupTest resets the relevant db collections prior to a test
func testFlaggingIdleHostsSetupTest(t *testing.T) {
	require.NoError(t, db.ClearCollections(distro.Collection), "error clearing distro collection")
	require.NoError(t, db.ClearCollections(host.Collection), "error clearing hosts collection")
	require.NoError(t, modelUtil.AddTestIndexes(host.Collection, true, true, host.RunningTaskKey), "error adding host index")
}

////////////////////////////////////////////////////////////////////////
//
// legacy test case

func TestFlaggingIdleHosts(t *testing.T) {
	ctx := context.Background()
	env := evergreen.GetEnvironment()

	t.Run("HostsCurrentlyRunningTasksShouldNeverBeFlagged", func(t *testing.T) {
		// clear the distro and hosts collections; add an index on the host collection
		testFlaggingIdleHostsSetupTest(t)

		// insert a reference distro.Distro
		distro1 := distro.Distro{
			Id: "distro1",
		}
		require.NoError(t, distro1.Insert(), "error inserting distro '%s'", distro1.Id)
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
		require.NoError(t, host1.Insert(), "error inserting host '%s'", host1.Id)

		// finding idle hosts should not return the host
		idle, err := flagIdleHosts(ctx, env)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(idle))
	})

	t.Run("EvenWithLastCommunicationTimeGreaterThanTenMinutes", func(t *testing.T) {
		// clear the distro and hosts collections; add an index on the host collection
		testFlaggingIdleHostsSetupTest(t)

		// insert a reference distro.Distro
		distro1 := distro.Distro{
			Id: "distro1",
		}
		require.NoError(t, distro1.Insert(), "error inserting distro '%s'", distro1.Id)

		host1 := host.Host{
			Id:                    "h1",
			Distro:                distro.Distro{Id: "distro1"},
			Provider:              evergreen.ProviderNameMock,
			CreationTime:          time.Now().Add(-30 * time.Minute),
			RunningTask:           "t3",
			Status:                evergreen.HostRunning,
			LastCommunicationTime: time.Now().Add(-30 * time.Minute),
			StartedBy:             evergreen.User,
		}
		require.NoError(t, host1.Insert(), "error inserting host '%s'", host1.Id)

		// finding idle hosts should not return the host
		idle, err := flagIdleHosts(ctx, env)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(idle))
	})

	t.Run("HostsNotRunningTasksShouldBeFlaggedIfTheyHaveBeenIdleAtLeastFifteenMinutesAndWillIncurPaymentInLessThanTenMinutes", func(t *testing.T) {
		// clear the distro and hosts collections; add an index on the host collection
		testFlaggingIdleHostsSetupTest(t)

		// insert a reference distro.Distro
		distro1 := distro.Distro{
			Id: "distro1",
		}
		require.NoError(t, distro1.Insert(), "error inserting distro '%s'", distro1.Id)

		// insert two hosts - one whose last task was more than 15 minutes
		// ago, one whose last task was less than 15 minutes ago
		host1 := host.Host{
			Id:                    "h1",
			Distro:                distro.Distro{Id: "distro1"},
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
			Distro:                distro.Distro{Id: "distro1"},
			Provider:              evergreen.ProviderNameMock,
			LastTask:              "t2",
			LastTaskCompletedTime: time.Now().Add(-time.Minute * 2),
			LastCommunicationTime: time.Now(),
			Status:                evergreen.HostRunning,
			StartedBy:             evergreen.User,
			Provisioned:           true,
		}
		require.NoError(t, host1.Insert(), "error inserting host '%s'", host1.Id)
		require.NoError(t, host2.Insert(), "error inserting host '%s'", host2.Id)

		// finding idle hosts should only return the first host
		idle, err := flagIdleHosts(ctx, env)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(idle))
		assert.Equal(t, "h1", idle[0])
	})

	t.Run("HostsNotCurrentlyRunningTaskWithLastCommunicationTimeGreaterThanTenMinsShouldBeMarkedAsIdle", func(t *testing.T) {
		// clear the distro and hosts collections; add an index on the host collection
		testFlaggingIdleHostsSetupTest(t)
		// insert a reference distro.Distro
		distro1 := distro.Distro{
			Id: "distro1",
		}
		require.NoError(t, distro1.Insert(), "error inserting distro '%s'", distro1.Id)

		// insert two hosts - one whose last task was more than 15 minutes
		// ago, one whose last task was less than 15 minutes ago
		host1 := host.Host{
			Id:                    "h1",
			Distro:                distro.Distro{Id: "distro1"},
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
			Distro:                distro.Distro{Id: "distro1"},
			Provider:              evergreen.ProviderNameMock,
			LastTask:              "t2",
			LastTaskCompletedTime: time.Now().Add(-time.Minute * 2),
			LastCommunicationTime: time.Now(),
			Status:                evergreen.HostRunning,
			StartedBy:             evergreen.User,
			Provisioned:           true,
		}
		require.NoError(t, host1.Insert(), "error inserting host '%s'", host1.Id)
		require.NoError(t, host2.Insert(), "error inserting host '%s'", host2.Id)

		// finding idle hosts should only return the first host 'h1'
		idle, err := flagIdleHosts(ctx, env)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(idle))
		assert.Equal(t, "h1", idle[0])
	})

	t.Run("HostsThatHaveBeenProvisionedShouldHaveTheTimerReset", func(t *testing.T) {
		// clear the distro and hosts collections; add an index on the host collection
		testFlaggingIdleHostsSetupTest(t)
		// insert our reference distro.Distro
		distro1 := distro.Distro{
			Id: "distro1",
		}
		require.NoError(t, distro1.Insert(), "error inserting distro '%s'", distro1.Id)

		h5 := host.Host{
			Id:                    "h5",
			Distro:                distro.Distro{Id: "distro1"},
			Provider:              evergreen.ProviderNameMock,
			LastCommunicationTime: time.Now(),
			Status:                evergreen.HostRunning,
			StartedBy:             evergreen.User,
			CreationTime:          time.Now().Add(-10 * time.Minute), // created before the cutoff
			ProvisionTime:         time.Now().Add(-2 * time.Minute),  // provisioned after the cutoff
		}
		require.NoError(t, h5.Insert(), "error inserting host '%s'", h5.Id)

		// 'h5' should not be flagged as idle
		idle, err := flagIdleHosts(ctx, env)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(idle))
	})

	t.Run("LegacyHostsThatNeedNewAgentsShouldNotBeMarkedIdle", func(t *testing.T) {
		testFlaggingIdleHostsSetupTest(t)

		// insert a reference distro.Distro
		distro1 := distro.Distro{
			Id: "distro1",
			BootstrapSettings: distro.BootstrapSettings{
				Method:        distro.BootstrapMethodLegacySSH,
				Communication: distro.CommunicationMethodLegacySSH,
			},
		}
		require.NoError(t, distro1.Insert(), "error inserting distro '%s'", distro1.Id)

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
		require.NoError(t, host1.Insert(), "error inserting host '%s'", host1.Id)

		// finding idle hosts should not return the host
		idle, err := flagIdleHosts(ctx, env)
		require.NoError(t, err)
		assert.Equal(t, 0, len(idle))
	})

	t.Run("NonLegacyHostsThatNeedNewAgentMonitorsShouldNotBeMarkedIdle", func(t *testing.T) {
		testFlaggingIdleHostsSetupTest(t)

		// insert a reference distro.Distro
		distro1 := distro.Distro{
			Id: "distro1",
			BootstrapSettings: distro.BootstrapSettings{
				Method:        distro.BootstrapMethodSSH,
				Communication: distro.CommunicationMethodSSH,
			},
		}
		require.NoError(t, distro1.Insert(), "error inserting distro '%s'", distro1.Id)

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
		require.NoError(t, host1.Insert(), "error inserting host '%s'", host1.Id)

		// finding idle hosts should not return the host
		idle, err := flagIdleHosts(ctx, env)
		require.NoError(t, err)
		assert.Equal(t, 0, len(idle))
	})

	t.Run("NonLegacyHostsThatDoNotNeedNewAgentMonitorsShouldBeMarkedIdle", func(t *testing.T) {
		testFlaggingIdleHostsSetupTest(t)

		// insert a reference distro.Distro
		distro1 := distro.Distro{
			Id: "distro1",
			BootstrapSettings: distro.BootstrapSettings{
				Method:        distro.BootstrapMethodSSH,
				Communication: distro.CommunicationMethodSSH,
			},
		}
		require.NoError(t, distro1.Insert(), "error inserting distro '%s'", distro1.Id)

		host1 := host.Host{
			Id:                    "h1",
			Distro:                distro1,
			Provider:              evergreen.ProviderNameMock,
			Status:                evergreen.HostRunning,
			CreationTime:          time.Now().Add(-24 * time.Hour),
			LastCommunicationTime: time.Now(),
			StartedBy:             evergreen.User,
			NeedsNewAgent:         true,
		}
		require.NoError(t, host1.Insert(), "error inserting host '%s'", host1.Id)

		// finding idle hosts should not return the host
		idle, err := flagIdleHosts(ctx, env)
		require.NoError(t, err)
		assert.Equal(t, 1, len(idle))
	})
}

////////////////////////////////////////////////////////////////////////
//
// Testing with reference distro ids that are not present in the 'distro' collection in the database
//

func TestFlaggingIdleHostsWithMissingDistroIDs(t *testing.T) {
	ctx := context.Background()
	env := evergreen.GetEnvironment()

	t.Run("AddSomeHostsWithReferencedDistrosThatDoNotExistInTheDistroCollection", func(t *testing.T) {
		// clear the distro and hosts collections; add an index on the host collection
		testFlaggingIdleHostsSetupTest(t)
		// insert two reference distro.Distro

		distro1 := distro.Distro{
			Id: "distro1",
			HostAllocatorSettings: distro.HostAllocatorSettings{
				MinimumHosts: 2,
			},
		}
		distro2 := distro.Distro{
			Id: "distro2",
			HostAllocatorSettings: distro.HostAllocatorSettings{
				MinimumHosts: 1,
			},
		}
		require.NoError(t, distro1.Insert(), "error inserting distro '%s'", distro1.Id)
		require.NoError(t, distro2.Insert(), "error inserting distro '%s'", distro2.Id)

		// insert a gaggle of hosts, some of which reference a host.Distro that doesn't exist in the distro collection
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
			Distro:       distro.Distro{Id: "distroZ"},
			Provider:     evergreen.ProviderNameMock,
			CreationTime: time.Now().Add(-30 * time.Minute),
			Status:       evergreen.HostRunning,
			StartedBy:    evergreen.User,
		}
		host4 := host.Host{
			Id:           "h4",
			Distro:       distro.Distro{Id: "distroA"},
			Provider:     evergreen.ProviderNameMock,
			CreationTime: time.Now().Add(-30 * time.Minute),
			Status:       evergreen.HostRunning,
			StartedBy:    evergreen.User,
		}
		host5 := host.Host{
			Id:           "h5",
			Distro:       distro.Distro{Id: "distroC"},
			Provider:     evergreen.ProviderNameMock,
			CreationTime: time.Now().Add(-20 * time.Minute),
			Status:       evergreen.HostRunning,
			StartedBy:    evergreen.User,
		}
		require.NoError(t, host1.Insert(), "error inserting host '%s'", host1.Id)
		require.NoError(t, host2.Insert(), "error inserting host '%s'", host2.Id)
		require.NoError(t, host3.Insert(), "error inserting host '%s'", host3.Id)
		require.NoError(t, host4.Insert(), "error inserting host '%s'", host4.Id)
		require.NoError(t, host5.Insert(), "error inserting host '%s'", host5.Id)

		// If we encounter missing distros, we exit early before we ever evaluate if any hosts are idle
		idle, err := flagIdleHosts(ctx, env)
		assert.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "distroZ"))
		assert.True(t, strings.Contains(err.Error(), "distroA"))
		assert.True(t, strings.Contains(err.Error(), "distroC"))
		assert.False(t, strings.Contains(err.Error(), "distro1"))
		assert.False(t, strings.Contains(err.Error(), "distro2"))
		assert.Equal(t, 0, len(idle))
	})
}

////////////////////////////////////////////////////////////////////////
//
// Testing with non-zero values for Distro.HostAllocatorSettings.MinimumHosts
//

func TestFlaggingIdleHostsWhenNonZeroMinimumHosts(t *testing.T) {
	ctx := context.Background()
	env := evergreen.GetEnvironment()

	t.Run("NeitherHostShouldBeFlaggedAsIdleAsMinimumHostsIsTwo", func(t *testing.T) {
		// clear the distro and hosts collections; add an index on the host collection
		testFlaggingIdleHostsSetupTest(t)

		// insert a reference distro.Distro
		distro1 := distro.Distro{
			Id: "distro1",
			HostAllocatorSettings: distro.HostAllocatorSettings{
				MinimumHosts: 2,
			},
		}
		require.NoError(t, distro1.Insert(), "error inserting distro '%s'", distro1.Id)

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
		require.NoError(t, host1.Insert(), "error inserting host '%s'", host1.Id)
		require.NoError(t, host2.Insert(), "error inserting host '%s'", host2.Id)

		// Nither host should be returned
		idle, err := flagIdleHosts(ctx, env)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(idle))
	})

	t.Run("MinimumHostsIsTwo;OneHostIsRunningItsTaskAndTwoHostsAreIdle", func(t *testing.T) {
		// clear the distro and hosts collections; add an index on the host collection
		testFlaggingIdleHostsSetupTest(t)

		// insert a reference distro.Distro (which has a non-zero value for its HostAllocatorSettings.MinimumHosts field)
		distro1 := distro.Distro{
			Id: "distro1",
			HostAllocatorSettings: distro.HostAllocatorSettings{
				MinimumHosts: 2,
			},
		}
		require.NoError(t, distro1.Insert(), "error inserting distro '%s'", distro1.Id)

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
		require.NoError(t, host1.Insert(), "error inserting host '%s'", host1.Id)
		require.NoError(t, host2.Insert(), "error inserting host '%s'", host2.Id)
		require.NoError(t, host3.Insert(), "error inserting host '%s'", host3.Id)

		// Only the oldest host not running a task should be flagged as idle - leaving 2 running hosts.
		idle, err := flagIdleHosts(ctx, env)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(idle))
		assert.Equal(t, "h1", idle[0])
	})
}

func TestPopulateIdleHostJobsCalculations(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(host.Collection))
	assert.NoError(db.ClearCollections(distro.Collection))

	distro1 := distro.Distro{
		Id: "distro1",
		HostAllocatorSettings: distro.HostAllocatorSettings{
			MinimumHosts: 3,
		},
	}

	distro2 := distro.Distro{
		Id: "distro2",
		HostAllocatorSettings: distro.HostAllocatorSettings{
			MinimumHosts: 0,
		},
	}
	assert.NoError(distro1.Insert())
	assert.NoError(distro2.Insert())

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
	assert.NoError(host1.Insert())
	assert.NoError(host2.Insert())
	assert.NoError(host3.Insert())
	assert.NoError(host4.Insert())
	assert.NoError(host5.Insert())
	assert.NoError(host6.Insert())

	distroHosts, err := host.IdleEphemeralGroupedByDistroID()
	assert.NoError(err)
	assert.Equal(2, len(distroHosts))

	distroIDsToFind := make([]string, 0, len(distroHosts))
	for _, info := range distroHosts {
		distroIDsToFind = append(distroIDsToFind, info.DistroID)
	}
	distrosFound, err := distro.Find(distro.ByIds(distroIDsToFind))
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
