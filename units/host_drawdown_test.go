package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
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
			require.NoError(t, tsk1.Insert(ctx))
			require.NoError(t, tsk2.Insert(ctx))

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
			require.NoError(t, tsk1.Insert(ctx))
			require.NoError(t, tsk2.Insert(ctx))

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
			require.NoError(t, tsk1.Insert(ctx))

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
			require.NoError(t, tsk1.Insert(ctx))

			drawdownInfo := DrawdownInfo{
				DistroID:     d.Id,
				NewCapTarget: 0,
			}
			num, hosts := numHostsDecommissionedForDrawdown(ctx, t, env, drawdownInfo)
			assert.Zero(t, num, "should not draw down host that was recently running task group")
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
			require.NoError(t, tsk1.Insert(ctx))

			drawdownInfo := DrawdownInfo{
				DistroID:     d.Id,
				NewCapTarget: 0,
			}
			num, hosts := numHostsDecommissionedForDrawdown(ctx, t, env, drawdownInfo)
			assert.Equal(t, 1, num, "should draw down long idle hosts")
			assert.Contains(t, hosts, host1.Id)
		},
		"HandlesHostInTeardown": func(ctx context.Context, t *testing.T, env *mock.Environment, d distro.Distro) {
			// Host in recent teardown - should be ignored
			recentTeardownHost := host.Host{
				Id:                         "recent",
				Distro:                     d,
				Provider:                   evergreen.ProviderNameMock,
				CreationTime:               time.Now().Add(-30 * time.Minute),
				Status:                     evergreen.HostRunning,
				StartedBy:                  evergreen.User,
				LastCommunicationTime:      time.Now().Add(-time.Minute),
				TaskGroupTeardownStartTime: time.Now(),
			}
			require.NoError(t, recentTeardownHost.Insert(ctx))

			// Host with expired teardown - should be decommissioned
			oldTeardownHost := host.Host{
				Id:                         "old",
				Distro:                     d,
				Provider:                   evergreen.ProviderNameMock,
				CreationTime:               time.Now().Add(-30 * time.Minute),
				Status:                     evergreen.HostRunning,
				StartedBy:                  evergreen.User,
				LastCommunicationTime:      time.Now().Add(-time.Minute),
				TaskGroupTeardownStartTime: time.Now().Add(-30 * time.Minute),
			}
			require.NoError(t, oldTeardownHost.Insert(ctx))

			drawdownInfo := DrawdownInfo{
				DistroID:     d.Id,
				NewCapTarget: 0,
			}

			num, hosts := numHostsDecommissionedForDrawdown(ctx, t, env, drawdownInfo)
			assert.Equal(t, 1, num, "should only decommission host with expired teardown")
			assert.NotContains(t, hosts, recentTeardownHost.Id,
				"should not decommission host in recent teardown")
			assert.Contains(t, hosts, oldTeardownHost.Id,
				"should decommission host with expired teardown")
		},
		"HandlesIdleHostsWithTaskQueue": func(ctx context.Context, t *testing.T, env *mock.Environment, d distro.Distro) {
			d.HostAllocatorSettings.AcceptableHostIdleTime = 90 * time.Second

			hostWithLastTask := host.Host{
				Id:                    "active",
				Distro:                d,
				Provider:              evergreen.ProviderNameMock,
				CreationTime:          time.Now().Add(-30 * time.Minute),
				Status:                evergreen.HostRunning,
				StartedBy:             evergreen.User,
				LastCommunicationTime: time.Now().Add(-time.Minute),
				LastTaskCompletedTime: time.Now().Add(-5 * time.Second),
				LastTask:              "dummy_task_name1",
			}
			require.NoError(t, hostWithLastTask.Insert(ctx))

			hostWithoutLastTask := host.Host{
				Id:                    "stale",
				Distro:                d,
				Provider:              evergreen.ProviderNameMock,
				CreationTime:          time.Now().Add(-30 * time.Minute),
				Status:                evergreen.HostRunning,
				StartedBy:             evergreen.User,
				LastCommunicationTime: time.Now().Add(-time.Minute),
				LastTaskCompletedTime: time.Time{}, // zero time
			}
			require.NoError(t, hostWithoutLastTask.Insert(ctx))

			// Add task to queue
			taskQueue := model.TaskQueue{
				Distro: d.Id,
				Queue: []model.TaskQueueItem{
					{Id: "task1"},
				},
				DistroQueueInfo: model.DistroQueueInfo{
					Length:                    1,
					LengthWithDependenciesMet: 1,
				},
			}
			require.NoError(t, taskQueue.Save(ctx))

			drawdownInfo := DrawdownInfo{
				DistroID:     d.Id,
				NewCapTarget: 0,
			}

			num, hosts := numHostsDecommissionedForDrawdown(ctx, t, env, drawdownInfo)
			assert.Equal(t, 1, num, "should only decommission hostWithoutLastTask")
			assert.NotContains(t, hosts, hostWithLastTask.Id,
				"should not decommission recently active host with tasks in queue")
			assert.Contains(t, hosts, hostWithoutLastTask.Id,
				"should not decommission stale host within idle threshold")

		},
		"HandlesIdleHostsWithTaskQueueWithNoDependenceMet": func(ctx context.Context, t *testing.T, env *mock.Environment, d distro.Distro) {
			d.HostAllocatorSettings.AcceptableHostIdleTime = 90 * time.Second

			hostWithLastTask := host.Host{
				Id:                    "active",
				Distro:                d,
				Provider:              evergreen.ProviderNameMock,
				CreationTime:          time.Now().Add(-30 * time.Minute),
				Status:                evergreen.HostRunning,
				StartedBy:             evergreen.User,
				LastCommunicationTime: time.Now().Add(-time.Minute),
				LastTaskCompletedTime: time.Now().Add(-5 * time.Second),
				LastTask:              "dummy_task_name1",
			}
			require.NoError(t, hostWithLastTask.Insert(ctx))

			hostWithoutLastTask := host.Host{
				Id:                    "stale",
				Distro:                d,
				Provider:              evergreen.ProviderNameMock,
				CreationTime:          time.Now().Add(-30 * time.Minute),
				Status:                evergreen.HostRunning,
				StartedBy:             evergreen.User,
				LastCommunicationTime: time.Now().Add(-time.Minute),
				LastTaskCompletedTime: time.Time{}, // zero time
			}
			require.NoError(t, hostWithoutLastTask.Insert(ctx))

			// Add task to queue
			taskQueue := model.TaskQueue{
				Distro: d.Id,
				Queue: []model.TaskQueueItem{
					{Id: "task1"},
				},
				DistroQueueInfo: model.DistroQueueInfo{
					Length:                    1,
					LengthWithDependenciesMet: 0,
				},
			}
			require.NoError(t, taskQueue.Save(ctx))

			drawdownInfo := DrawdownInfo{
				DistroID:     d.Id,
				NewCapTarget: 0,
			}

			num, hosts := numHostsDecommissionedForDrawdown(ctx, t, env, drawdownInfo)
			assert.Equal(t, 2, num, "should decommission hostWithoutLastTask and hostWithLastTask when there are no tasks in the task queue with dependencies met")
			assert.Contains(t, hosts, hostWithLastTask.Id,
				"should decommission recently active host when with tasks in queue but no tasks with dependencies met")
			assert.Contains(t, hosts, hostWithoutLastTask.Id,
				"should not decommission stale host within idle threshold")

		},
		"HandlesIdleHostsWithNoQueue": func(ctx context.Context, t *testing.T, env *mock.Environment, d distro.Distro) {
			d.HostAllocatorSettings.AcceptableHostIdleTime = 90 * time.Second

			hostWithLastTask := host.Host{
				Id:                    "active",
				Distro:                d,
				Provider:              evergreen.ProviderNameMock,
				CreationTime:          time.Now().Add(-30 * time.Minute),
				Status:                evergreen.HostRunning,
				StartedBy:             evergreen.User,
				LastCommunicationTime: time.Now().Add(-time.Minute),
				LastTaskCompletedTime: time.Now().Add(-5 * time.Second),
				LastTask:              "dummy_task_name1",
			}
			require.NoError(t, hostWithLastTask.Insert(ctx))

			hostWithoutLastTask := host.Host{
				Id:                    "stale",
				Distro:                d,
				Provider:              evergreen.ProviderNameMock,
				CreationTime:          time.Now().Add(-30 * time.Minute),
				Status:                evergreen.HostRunning,
				StartedBy:             evergreen.User,
				LastCommunicationTime: time.Now().Add(-time.Minute),
				LastTaskCompletedTime: time.Time{}, // zero time
			}
			require.NoError(t, hostWithoutLastTask.Insert(ctx))

			drawdownInfo := DrawdownInfo{
				DistroID:     d.Id,
				NewCapTarget: 0,
			}

			// Clear task queue and verify hosts are decommissioned with default threshold
			require.NoError(t, model.ClearTaskQueue(ctx, d.Id))

			num, hosts := numHostsDecommissionedForDrawdown(ctx, t, env, drawdownInfo)
			assert.Equal(t, 2, num, "should decommission both hosts when queue is empty")
			assert.Contains(t, hosts, hostWithLastTask.Id)
			assert.Contains(t, hosts, hostWithoutLastTask.Id)
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
			taskQueue := model.TaskQueue{
				Distro: d.Id,
				Queue:  []model.TaskQueueItem{},
			}
			require.NoError(t, taskQueue.Save(ctx))
			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))

			tCase(ctx, t, env, d)
		})
	}
}
