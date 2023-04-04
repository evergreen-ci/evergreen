package pod

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindByNeedsTermination(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T){
		"ReturnsEmptyForNoMatches": func(t *testing.T) {
			pods, err := FindByNeedsTermination()
			assert.NoError(t, err)
			assert.Empty(t, pods)
		},
		"ReturnsMatchingStaleStartingJob": func(t *testing.T) {
			stalePod := Pod{
				ID:     "pod_id0",
				Status: StatusStarting,
				TimeInfo: TimeInfo{
					Starting: time.Now().Add(-time.Hour),
				},
			}
			require.NoError(t, stalePod.Insert())
			runningPod := Pod{
				ID:     "pod_id1",
				Status: StatusRunning,
				TimeInfo: TimeInfo{
					Starting: time.Now().Add(-time.Hour),
				},
			}
			require.NoError(t, runningPod.Insert())
			startingPod := Pod{
				ID:     "pod_id2",
				Status: StatusRunning,
				TimeInfo: TimeInfo{
					Starting: time.Now(),
				},
			}
			require.NoError(t, startingPod.Insert())

			pods, err := FindByNeedsTermination()
			require.NoError(t, err)
			require.Len(t, pods, 1)
			assert.Equal(t, stalePod.ID, pods[0].ID)
		},
		"ReturnsMatchingDecommissionedPod": func(t *testing.T) {
			decommissionedPod := Pod{
				ID:     "pod_id",
				Status: StatusDecommissioned,
			}
			// this pod should not be returned because it has a running task
			decommissionedPodWithTask := Pod{
				ID:     "pod_id_with_task",
				Status: StatusDecommissioned,
				TaskRuntimeInfo: TaskRuntimeInfo{
					RunningTaskID: "task_id",
				},
			}
			require.NoError(t, decommissionedPod.Insert())
			require.NoError(t, decommissionedPodWithTask.Insert())

			pods, err := FindByNeedsTermination()
			require.NoError(t, err)
			require.Len(t, pods, 1)
			assert.Equal(t, decommissionedPod.ID, pods[0].ID)
		},
		"ReturnsMatchingStaleInitializingPod": func(t *testing.T) {
			stalePod := Pod{
				ID:     "pod_id",
				Status: StatusInitializing,
				TimeInfo: TimeInfo{
					Initializing: time.Now().Add(-time.Hour),
				},
			}
			require.NoError(t, stalePod.Insert())

			pods, err := FindByNeedsTermination()
			require.NoError(t, err)
			require.Len(t, pods, 1)
			assert.Equal(t, stalePod.ID, pods[0].ID)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			defer func() {
				assert.NoError(t, db.Clear(Collection))
			}()

			tCase(t)
		})
	}
}

func TestFindByInitializing(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T){
		"ReturnsEmptyForNoMatches": func(t *testing.T) {
			pods, err := FindByInitializing()
			assert.NoError(t, err)
			assert.Empty(t, pods)
		},
		"ReturnsMatchingPods": func(t *testing.T) {
			p1 := &Pod{
				ID:     utility.RandomString(),
				Status: StatusInitializing,
			}
			require.NoError(t, p1.Insert())

			p2 := &Pod{
				ID:     utility.RandomString(),
				Status: StatusStarting,
			}
			require.NoError(t, p2.Insert())

			p3 := &Pod{
				ID:     utility.RandomString(),
				Status: StatusInitializing,
			}
			require.NoError(t, p3.Insert())

			pods, err := FindByInitializing()
			require.NoError(t, err)
			require.Len(t, pods, 2)
			assert.Equal(t, StatusInitializing, pods[0].Status)
			assert.Equal(t, StatusInitializing, pods[1].Status)

			ids := map[string]struct{}{p1.ID: {}, p3.ID: {}}
			assert.Contains(t, ids, pods[0].ID)
			assert.Contains(t, ids, pods[1].ID)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			defer func() {
				assert.NoError(t, db.Clear(Collection))
			}()

			tCase(t)
		})
	}
}

func TestCountByInitializing(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T){
		"ReturnsZeroForNoMatches": func(t *testing.T) {
			count, err := CountByInitializing()
			assert.NoError(t, err)
			assert.Empty(t, count)
		},
		"CountsInitializingPods": func(t *testing.T) {
			p1 := &Pod{
				ID:     utility.RandomString(),
				Status: StatusInitializing,
			}
			require.NoError(t, p1.Insert())

			p2 := &Pod{
				ID:     utility.RandomString(),
				Status: StatusStarting,
			}
			require.NoError(t, p2.Insert())

			p3 := &Pod{
				ID:     utility.RandomString(),
				Status: StatusInitializing,
			}
			require.NoError(t, p3.Insert())

			count, err := CountByInitializing()
			require.NoError(t, err)
			assert.Equal(t, 2, count)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			defer func() {
				assert.NoError(t, db.Clear(Collection))
			}()

			tCase(t)
		})
	}
}

func TestFindOneByID(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T){
		"Succeeds": func(t *testing.T) {
			p := Pod{
				ID:     "id",
				Status: StatusInitializing,
			}
			require.NoError(t, p.Insert())

			dbPod, err := FindOneByID(p.ID)
			require.NoError(t, err)
			assert.Equal(t, p.ID, dbPod.ID)
			assert.Equal(t, p.Status, dbPod.Status)
		},
		"ReturnsNilWithNonexistentPod": func(t *testing.T) {
			p, err := FindOneByID("nonexistent")
			assert.NoError(t, err)
			assert.Zero(t, p)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			defer func() {
				assert.NoError(t, db.Clear(Collection))
			}()

			tCase(t)
		})
	}
}

func TestFindOneByExternalID(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T){
		"Succeeds": func(t *testing.T) {
			p := Pod{
				ID:     "id",
				Status: StatusStarting,
				Resources: ResourceInfo{
					ExternalID: "external_id",
				},
			}
			require.NoError(t, p.Insert())

			dbPod, err := FindOneByExternalID(p.Resources.ExternalID)
			require.NoError(t, err)
			assert.Equal(t, p.ID, dbPod.ID)
			assert.Equal(t, p.Status, dbPod.Status)
		},
		"ReturnsNilWithNonexistentPod": func(t *testing.T) {
			p, err := FindOneByExternalID("nonexistent")
			assert.NoError(t, err)
			assert.Zero(t, p)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			defer func() {
				assert.NoError(t, db.Clear(Collection))
			}()

			tCase(t)
		})
	}
}

func TestFindByFamily(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T){
		"FindsMatchingSubsetOfPods": func(t *testing.T) {
			const family = "cool_fam"
			pods := []Pod{
				{
					ID:     "p0",
					Status: StatusInitializing,
					Family: family,
				},
				{
					ID:     "p1",
					Status: StatusRunning,
					Family: family,
				},
				{
					ID:     "p2",
					Status: StatusInitializing,
					Family: family,
				},
				{
					ID:     "p3",
					Status: StatusInitializing,
					Family: "not_as_cool_fam",
				},
			}
			for _, p := range pods {
				require.NoError(t, p.Insert())
			}

			dbPods, err := FindIntentByFamily(family)
			require.NoError(t, err)
			var numMatches int
			for _, p := range dbPods {
				switch p.ID {
				case pods[0].ID, pods[2].ID:
					numMatches++
				default:
					assert.FailNow(t, "found unexpected pod '%s'", p.ID)
				}
			}
			assert.Equal(t, 2, numMatches)
		},
		"IgnoresNonIntentPods": func(t *testing.T) {
			p := Pod{
				ID:     "pod",
				Status: StatusStarting,
				Family: "family",
			}
			require.NoError(t, p.Insert())
			dbPods, err := FindIntentByFamily(p.Family)
			assert.NoError(t, err)
			assert.Empty(t, dbPods)
		},
		"IgnoresPodsWithoutMatchingFamily": func(t *testing.T) {
			p := Pod{
				ID:     "pod",
				Status: StatusStarting,
				Family: "family",
			}
			require.NoError(t, p.Insert())
			dbPods, err := FindIntentByFamily("foo")
			assert.NoError(t, err)
			assert.Empty(t, dbPods)
		},
		"ReturnsNoErrorForNoMatchingPods": func(t *testing.T) {
			dbPods, err := FindIntentByFamily("nonexistent")
			assert.NoError(t, err)
			assert.Empty(t, dbPods)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			defer func() {
				assert.NoError(t, db.Clear(Collection))
			}()

			tCase(t)
		})
	}
}

func TestUpdateOneStatus(t *testing.T) {
	checkStatusAndTimeInfo := func(t *testing.T, p *Pod, s Status) {
		assert.Equal(t, s, p.Status)
		switch p.Status {
		case StatusInitializing:
			assert.NotZero(t, p.TimeInfo.Initializing)
		case StatusStarting:
			assert.NotZero(t, p.TimeInfo.Starting)
		}
	}

	checkEventLog := func(t *testing.T, p Pod) {
		events, err := event.Find(event.MostRecentPodEvents(p.ID, 10))
		require.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, p.ID, events[0].ResourceId)
		assert.Equal(t, event.ResourceTypePod, events[0].ResourceType)
		assert.Equal(t, string(event.EventPodStatusChange), events[0].EventType)
	}

	for tName, tCase := range map[string]func(t *testing.T, p Pod){
		"NoopsWithIdenticalStatus": func(t *testing.T, p Pod) {
			p.Status = StatusInitializing
			require.NoError(t, p.Insert())

			require.NoError(t, p.UpdateStatus(p.Status, ""))
			assert.Equal(t, StatusInitializing, p.Status)

			dbPod, err := FindOneByID(p.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, p.Status, dbPod.Status)
			assert.Zero(t, dbPod.TimeInfo.Initializing)
		},
		"SucceedsWithInitializingStatus": func(t *testing.T, p Pod) {
			require.NoError(t, p.Insert())

			updated := StatusInitializing
			require.NoError(t, UpdateOneStatus(p.ID, p.Status, updated, time.Now(), ""))

			dbPod, err := FindOneByID(p.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			checkStatusAndTimeInfo(t, dbPod, updated)
			checkEventLog(t, p)
		},
		"SucceedsWithStartingStatus": func(t *testing.T, p Pod) {
			require.NoError(t, p.Insert())

			updated := StatusStarting
			require.NoError(t, UpdateOneStatus(p.ID, p.Status, updated, time.Now(), ""))

			dbPod, err := FindOneByID(p.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			checkStatusAndTimeInfo(t, dbPod, updated)
			checkEventLog(t, p)
		},
		"FailsWithMismatchedCurrentStatus": func(t *testing.T, p Pod) {
			require.NoError(t, p.Insert())

			assert.Error(t, UpdateOneStatus(p.ID, StatusInitializing, StatusTerminated, time.Now(), ""))
		},
		"SucceedsWithTerminatedStatus": func(t *testing.T, p Pod) {
			require.NoError(t, p.Insert())

			updated := StatusTerminated
			require.NoError(t, UpdateOneStatus(p.ID, p.Status, updated, time.Now(), ""))

			dbPod, err := FindOneByID(p.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)

			assert.Equal(t, updated, dbPod.Status)
			checkEventLog(t, p)
		},
		"FailsWithNonexistentPod": func(t *testing.T, p Pod) {
			require.NoError(t, p.Insert())

			assert.Error(t, UpdateOneStatus("nonexistent", StatusStarting, StatusRunning, time.Now(), ""))
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(Collection, event.EventCollection))
			defer func() {
				assert.NoError(t, db.ClearCollections(Collection, event.EventCollection))
			}()

			p := Pod{
				ID:     "id",
				Status: StatusRunning,
			}

			tCase(t, p)
		})
	}
}

func TestFindByLastCommunicatedBefore(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()
	for tName, tCase := range map[string]func(t *testing.T){
		"FindsStartingStalePod": func(t *testing.T) {
			p := Pod{
				ID:     "id",
				Status: StatusStarting,
				TimeInfo: TimeInfo{
					LastCommunicated: time.Now().Add(-time.Hour),
				},
			}
			require.NoError(t, p.Insert())

			found, err := FindByLastCommunicatedBefore(time.Now().Add(-10 * time.Minute))
			require.NoError(t, err)
			require.Len(t, found, 1)
			assert.Equal(t, p.ID, found[0].ID)
		},
		"FindsRunningStalePod": func(t *testing.T) {
			p := Pod{
				ID:     "id",
				Status: StatusRunning,
				TimeInfo: TimeInfo{
					LastCommunicated: time.Now().Add(-time.Hour),
				},
			}
			require.NoError(t, p.Insert())

			found, err := FindByLastCommunicatedBefore(time.Now().Add(-10 * time.Minute))
			require.NoError(t, err)
			require.Len(t, found, 1)
			assert.Equal(t, p.ID, found[0].ID)
		},
		"FindsMultipleStalePods": func(t *testing.T) {
			pods := []Pod{
				{
					ID:     "pod0",
					Status: StatusDecommissioned,
					TimeInfo: TimeInfo{
						LastCommunicated: time.Now().Add(-time.Hour),
					},
				},
				{
					ID:     "pod1",
					Status: StatusStarting,
					TimeInfo: TimeInfo{
						LastCommunicated: time.Now().Add(-time.Minute),
					},
				},
				{
					ID:     "pod2",
					Status: StatusStarting,
					TimeInfo: TimeInfo{
						LastCommunicated: time.Now().Add(-time.Hour),
					},
				},
				{
					ID:     "pod3",
					Status: StatusRunning,
					TimeInfo: TimeInfo{
						LastCommunicated: time.Now().Add(-time.Minute),
					},
				},
				{
					ID:     "pod4",
					Status: StatusRunning,
					TimeInfo: TimeInfo{
						LastCommunicated: time.Now().Add(-time.Hour),
					},
				},
				{
					ID:     "pod5",
					Status: StatusTerminated,
					TimeInfo: TimeInfo{
						LastCommunicated: time.Now().Add(-time.Minute),
					},
				},
			}
			for _, p := range pods {
				require.NoError(t, p.Insert())
			}

			found, err := FindByLastCommunicatedBefore(time.Now().Add(-10 * time.Minute))
			require.NoError(t, err)
			require.Len(t, found, 2)
			assert.ElementsMatch(t, []string{pods[2].ID, pods[4].ID}, []string{found[0].ID, found[1].ID})
		},
		"IgnoresPodWithRecentCommunication": func(t *testing.T) {
			p := Pod{
				ID:     "id",
				Status: StatusRunning,
				TimeInfo: TimeInfo{
					LastCommunicated: time.Now().Add(-time.Minute),
				},
			}
			require.NoError(t, p.Insert())

			found, err := FindByLastCommunicatedBefore(time.Now().Add(-10 * time.Minute))
			assert.NoError(t, err)
			assert.Empty(t, found)
		},
		"IgnoresAlreadyTerminatedPod": func(t *testing.T) {
			p := Pod{
				ID:     "id",
				Status: StatusTerminated,
				TimeInfo: TimeInfo{
					LastCommunicated: time.Now().Add(-time.Hour),
				},
			}
			require.NoError(t, p.Insert())

			found, err := FindByLastCommunicatedBefore(time.Now().Add(-10 * time.Minute))
			assert.NoError(t, err)
			assert.Empty(t, found)
		},
		"IgnoresIntentPod": func(t *testing.T) {
			p := Pod{
				ID:     "id",
				Status: StatusInitializing,
				TimeInfo: TimeInfo{
					LastCommunicated: time.Now().Add(-time.Hour),
				},
			}
			require.NoError(t, p.Insert())

			found, err := FindByLastCommunicatedBefore(time.Now().Add(-10 * time.Minute))
			assert.NoError(t, err)
			assert.Empty(t, found)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(Collection))

			tCase(t)
		})
	}
}

func TestGetStatsByStatus(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()
	for tName, tCase := range map[string]func(t *testing.T){
		"ReturnsEmptyForNoMatchingPods": func(t *testing.T) {
			stats, err := GetStatsByStatus()
			assert.NoError(t, err)
			assert.Empty(t, stats)
		},
		"ReturnsStatisticsForMatchingStatusAndType": func(t *testing.T) {
			for _, p := range []Pod{
				{
					ID:     "p0",
					Status: StatusRunning,
					Type:   TypeAgent,
					TaskRuntimeInfo: TaskRuntimeInfo{
						RunningTaskID:        "t0",
						RunningTaskExecution: 5,
					},
				},
				{
					ID:     "p1",
					Status: StatusTerminated,
					Type:   TypeAgent,
				},
				{
					ID:     "p2",
					Status: StatusRunning,
					Type:   TypeAgent,
				},
				{
					ID:     "p3",
					Status: StatusRunning,
				},
			} {
				require.NoError(t, p.Insert())
			}

			stats, err := GetStatsByStatus(StatusRunning)
			require.NoError(t, err)
			require.Len(t, stats, 1)
			assert.Equal(t, StatusRunning, stats[0].Status)
			assert.Equal(t, 2, stats[0].Count)
			assert.Equal(t, 1, stats[0].NumRunningTasks)
		},
		"ReturnsStatisticsForMultipleMatchingStatuses": func(t *testing.T) {
			for _, p := range []Pod{
				{
					ID:     "p0",
					Status: StatusRunning,
					Type:   TypeAgent,
					TaskRuntimeInfo: TaskRuntimeInfo{
						RunningTaskID:        "t0",
						RunningTaskExecution: 2,
					},
				},
				{
					ID:     "p1",
					Status: StatusTerminated,
					Type:   TypeAgent,
				},
				{
					ID:     "p2",
					Status: StatusInitializing,
					Type:   TypeAgent,
				},
				{
					ID:     "p3",
					Status: StatusStarting,
					Type:   TypeAgent,
				},
				{
					ID:     "p4",
					Status: StatusInitializing,
				},
				{
					ID:     "p5",
					Status: StatusDecommissioned,
					Type:   TypeAgent,
					TaskRuntimeInfo: TaskRuntimeInfo{
						RunningTaskID:        "t1",
						RunningTaskExecution: 0,
					},
				},
				{
					ID:     "p6",
					Status: StatusRunning,
					Type:   TypeAgent,
				},
			} {
				require.NoError(t, p.Insert())
			}

			stats, err := GetStatsByStatus(StatusInitializing, StatusStarting, StatusRunning)
			require.NoError(t, err)
			require.Len(t, stats, 3)
			for _, s := range stats {
				switch s.Status {
				case StatusInitializing:
					assert.Equal(t, 1, s.Count)
					assert.Zero(t, s.NumRunningTasks)
				case StatusStarting:
					assert.Equal(t, 1, s.Count)
					assert.Zero(t, s.NumRunningTasks)
				case StatusRunning:
					assert.Equal(t, 2, s.Count)
					assert.Equal(t, 1, s.NumRunningTasks)
				default:
					assert.Fail(t, "unexpected pod status '%s'", s.Status)
				}
			}
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(Collection))

			tCase(t)
		})
	}
}
