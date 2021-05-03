package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func numHostsTerminated(ctx context.Context, env evergreen.Environment, drawdownInfo DrawdownInfo, t *testing.T) (int, []string) {
	queue := queue.NewAdaptiveOrderedLocalQueue(3, 1024)
	require.NoError(t, queue.Start(ctx))
	defer queue.Runner().Close(ctx)

	require.NoError(t, queue.Put(ctx, NewHostDrawdownJob(env, drawdownInfo, utility.RoundPartOfHour(1).Format(TSFormat))))

	amboy.WaitInterval(ctx, queue, 50*time.Millisecond)
	out := []string{}
	num := 0
	for j := range queue.Results(ctx) {
		if ij, ok := j.(*hostDrawdownJob); ok {
			num += ij.Terminated
			out = append(out, ij.TerminatedHosts...)
		}
	}

	assert.Equal(t, num, len(out))

	return num, out
}

////////////////////////////////////////////////////////////////////////
// Testing with reference distro ids that are not present in the 'distro' collection in the database
//

func TestTerminatingHosts(t *testing.T) {
	ctx := context.Background()
	env := evergreen.GetEnvironment()

	t.Run("SimpleTerminationTest", func(t *testing.T) {
		// clear the distro and hosts collections; add an index on the host collection
		testFlaggingIdleHostsSetupTest(t)
		// insert two reference distro.Distro

		distro1 := distro.Distro{
			Id:       "distro1",
			Provider: evergreen.ProviderNameMock,
			HostAllocatorSettings: distro.HostAllocatorSettings{
				MinimumHosts: 2,
			},
		}
		require.NoError(t, distro1.Insert(), "error inserting distro '%s'", distro1.Id)

		// insert a gaggle of hosts, some of which reference a host.Distro that doesn't exist in the distro collection
		host1 := host.Host{
			Id:           "h1",
			Distro:       distro1,
			Provider:     evergreen.ProviderNameMock,
			CreationTime: time.Now().Add(-10 * time.Minute),
			Status:       evergreen.HostRunning,
			RunningTask:  "dummy_task_name",
			StartedBy:    evergreen.User,
		}
		host2 := host.Host{
			Id:           "h2",
			Distro:       distro1,
			Provider:     evergreen.ProviderNameMock,
			CreationTime: time.Now().Add(-20 * time.Minute),
			Status:       evergreen.HostRunning,
			RunningTask:  "dummy_task_name2",
			StartedBy:    evergreen.User,
		}
		host3 := host.Host{
			Id:           "h3",
			Distro:       distro1,
			Provider:     evergreen.ProviderNameMock,
			CreationTime: time.Now().Add(-30 * time.Minute),
			Status:       evergreen.HostRunning,
			StartedBy:    evergreen.User,
		}
		host4 := host.Host{
			Id:           "h4",
			Distro:       distro1,
			Provider:     evergreen.ProviderNameMock,
			CreationTime: time.Now().Add(-30 * time.Minute),
			Status:       evergreen.HostRunning,
			StartedBy:    evergreen.User,
		}
		host5 := host.Host{
			Id:           "h5",
			Distro:       distro1,
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

		// If we encounter missing distros, we decommission hosts from those
		// distros.

		drawdownInfo := DrawdownInfo{
			DistroID:     "distro1",
			NewCapTarget: 3,
		}
		// 3 idle hosts, 2 need to be terminated to reach NewCapTarget
		num, hosts := numHostsTerminated(ctx, env, drawdownInfo, t)
		assert.Equal(t, 2, num)

		assert.Contains(t, hosts, "h3")
		assert.Contains(t, hosts, "h4")
	})
}
