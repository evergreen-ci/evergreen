package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func numHostsDecommissionedForDrawdown(ctx context.Context, t *testing.T, env evergreen.Environment, drawdownInfo DrawdownInfo) (int, []string) {
	j, ok := NewHostDrawdownJob(env, drawdownInfo, utility.RoundPartOfHour(1).Format(TSFormat)).(*hostDrawdownJob)
	require.True(t, ok)

	j.Run(ctx)
	assert.NoError(t, j.Error())

	assert.Equal(t, len(j.DecommissionedHosts), j.Decommissioned)

	return j.Decommissioned, j.DecommissionedHosts
}

func TestHostDrawdown(t *testing.T) {
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment, d distro.Distro){
		"DecommissionsHostsDownToCap": func(ctx context.Context, t *testing.T, env *mock.Environment, d distro.Distro) {
			// insert a gaggle of hosts, some of which reference a host.Distro that doesn't exist in the distro collection
			host1 := host.Host{
				Id:           "h1",
				Distro:       d,
				Provider:     evergreen.ProviderNameMock,
				CreationTime: time.Now().Add(-10 * time.Minute),
				Status:       evergreen.HostRunning,
				RunningTask:  "dummy_task_name1",
				StartedBy:    evergreen.User,
			}
			host2 := host.Host{
				Id:           "h2",
				Distro:       d,
				Provider:     evergreen.ProviderNameMock,
				CreationTime: time.Now().Add(-20 * time.Minute),
				Status:       evergreen.HostRunning,
				RunningTask:  "dummy_task_name2",
				StartedBy:    evergreen.User,
			}
			host3 := host.Host{
				Id:           "h3",
				Distro:       d,
				Provider:     evergreen.ProviderNameMock,
				CreationTime: time.Now().Add(-30 * time.Minute),
				Status:       evergreen.HostRunning,
				StartedBy:    evergreen.User,
			}
			host4 := host.Host{
				Id:           "h4",
				Distro:       d,
				Provider:     evergreen.ProviderNameMock,
				CreationTime: time.Now().Add(-30 * time.Minute),
				Status:       evergreen.HostRunning,
				StartedBy:    evergreen.User,
			}
			host5 := host.Host{
				Id:           "h5",
				Distro:       d,
				Provider:     evergreen.ProviderNameMock,
				CreationTime: time.Now().Add(-20 * time.Minute),
				Status:       evergreen.HostRunning,
				StartedBy:    evergreen.User,
			}
			require.NoError(t, host1.Insert(ctx))
			require.NoError(t, host2.Insert(ctx))
			require.NoError(t, host3.Insert(ctx))
			require.NoError(t, host4.Insert(ctx))
			require.NoError(t, host5.Insert(ctx))
			tsk1 := task.Task{
				Id: "dummy_task_name1",
			}
			tsk2 := task.Task{
				Id: "dummy_task_name2",
			}
			require.NoError(t, tsk1.Insert())
			require.NoError(t, tsk2.Insert())

			// If we encounter missing distros, we decommission hosts from those
			// distros.

			drawdownInfo := DrawdownInfo{
				DistroID:     d.Id,
				NewCapTarget: 3,
			}
			// 3 idle hosts, 2 need to be decommissioned to reach NewCapTarget
			num, hosts := numHostsDecommissionedForDrawdown(ctx, t, env, drawdownInfo)
			assert.Equal(t, 2, num)

			assert.Contains(t, hosts, host3.Id)
			assert.Contains(t, hosts, host4.Id)
		},
		"IgnoresSingleHostTaskGroupHosts": func(ctx context.Context, t *testing.T, env *mock.Environment, d distro.Distro) {
			host1 := host.Host{
				Id:               "h1",
				Distro:           d,
				Provider:         evergreen.ProviderNameMock,
				CreationTime:     time.Now().Add(-30 * time.Minute),
				Status:           evergreen.HostRunning,
				StartedBy:        evergreen.User,
				RunningTaskGroup: "dummy_task_group1",
				RunningTask:      "dummy_task_name1",
			}
			host2 := host.Host{
				Id:           "h2",
				Distro:       d,
				Provider:     evergreen.ProviderNameMock,
				CreationTime: time.Now().Add(-30 * time.Minute),
				Status:       evergreen.HostRunning,
				StartedBy:    evergreen.User,
				LastGroup:    "dummy_task_group2",
				LastTask:     "dummy_task_name2",
			}
			require.NoError(t, host1.Insert(ctx))
			require.NoError(t, host2.Insert(ctx))
			tsk1 := task.Task{
				Id:                "dummy_task_name1",
				TaskGroup:         "dummy_task_group1",
				TaskGroupMaxHosts: 1,
			}
			tsk2 := task.Task{
				Id:                "dummy_task_name2",
				Status:            evergreen.TaskSucceeded,
				TaskGroup:         "dummy_task_group2",
				TaskGroupMaxHosts: 1,
			}
			require.NoError(t, tsk1.Insert())
			require.NoError(t, tsk2.Insert())

			drawdownInfo := DrawdownInfo{
				DistroID:     d.Id,
				NewCapTarget: 0,
			}
			num, _ := numHostsDecommissionedForDrawdown(ctx, t, env, drawdownInfo)
			assert.Zero(t, num, "should not draw down any hosts running single host task groups")
		},
		"IgnoresHostRunningTask": func(ctx context.Context, t *testing.T, env *mock.Environment, d distro.Distro) {
			host1 := host.Host{
				Id:           "h1",
				Distro:       d,
				Provider:     evergreen.ProviderNameMock,
				CreationTime: time.Now().Add(-30 * time.Minute),
				Status:       evergreen.HostRunning,
				StartedBy:    evergreen.User,
				RunningTask:  "dummy_task_name1",
			}
			require.NoError(t, host1.Insert(ctx))
			tsk1 := task.Task{
				Id: "dummy_task_name1",
			}
			require.NoError(t, tsk1.Insert())

			drawdownInfo := DrawdownInfo{
				DistroID:     d.Id,
				NewCapTarget: 0,
			}
			num, hosts := numHostsDecommissionedForDrawdown(ctx, t, env, drawdownInfo)
			assert.Zero(t, num, "should not draw down host running task")
			assert.Empty(t, hosts)
		},
		"IgnoresHostThatRecentlyRanTaskGroup": func(ctx context.Context, t *testing.T, env *mock.Environment, d distro.Distro) {
			host1 := host.Host{
				Id:                    "h1",
				Distro:                d,
				Provider:              evergreen.ProviderNameMock,
				CreationTime:          time.Now().Add(-30 * time.Minute),
				Status:                evergreen.HostRunning,
				StartedBy:             evergreen.User,
				LastTask:              "dummy_task_group1",
				LastGroup:             "dummy_task_group1",
				LastTaskCompletedTime: time.Now().Add(-time.Minute),
			}
			require.NoError(t, host1.Insert(ctx))
			tsk1 := task.Task{
				Id:                "dummy_task_name1",
				TaskGroup:         "dummy_task_group1",
				TaskGroupMaxHosts: 5,
			}
			require.NoError(t, tsk1.Insert())

			drawdownInfo := DrawdownInfo{
				DistroID:     d.Id,
				NewCapTarget: 0,
			}
			num, hosts := numHostsDecommissionedForDrawdown(ctx, t, env, drawdownInfo)
			assert.Zero(t, 0, num, "should not draw down host that was recently running task group")
			assert.Empty(t, hosts)
		},
		"DecommissionsIdleMultiHostTaskGroupHost": func(ctx context.Context, t *testing.T, env *mock.Environment, d distro.Distro) {

			host1 := host.Host{
				Id:                    "h1",
				Distro:                d,
				Provider:              evergreen.ProviderNameMock,
				CreationTime:          time.Now().Add(-30 * time.Minute),
				Status:                evergreen.HostRunning,
				StartedBy:             evergreen.User,
				LastTask:              "dummy_task_name1",
				LastTaskCompletedTime: time.Now().Add(-20 * time.Minute),
			}
			require.NoError(t, host1.Insert(ctx))
			tsk1 := task.Task{
				Id: "dummy_task_name1",
			}
			require.NoError(t, tsk1.Insert())

			drawdownInfo := DrawdownInfo{
				DistroID:     d.Id,
				NewCapTarget: 0,
			}
			num, hosts := numHostsDecommissionedForDrawdown(ctx, t, env, drawdownInfo)
			assert.Equal(t, 1, num, "should draw down long idle hosts")
			assert.Contains(t, hosts, host1.Id)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx = testutil.TestSpan(ctx, t)
			testFlaggingIdleHostsSetupTest(t)

			d := distro.Distro{
				Id:       "d",
				Provider: evergreen.ProviderNameMock,
				HostAllocatorSettings: distro.HostAllocatorSettings{
					MinimumHosts: 0,
				},
			}
			require.NoError(t, d.Insert(ctx))

			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))

			tCase(ctx, t, env, d)
		})
	}
}
