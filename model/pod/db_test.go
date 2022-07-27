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
			require.NoError(t, decommissionedPod.Insert())

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

func TestFindByIntentDigest(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T){
		"FindsMatchingSubsetOfPods": func(t *testing.T) {
			const intentDigest = "abc123"
			pods := []Pod{
				{
					ID:           "p0",
					Status:       StatusInitializing,
					IntentDigest: intentDigest,
				},
				{
					ID:           "p1",
					Status:       StatusRunning,
					IntentDigest: intentDigest,
				},
				{
					ID:           "p2",
					Status:       StatusInitializing,
					IntentDigest: intentDigest,
				},
				{
					ID:           "p3",
					Status:       StatusInitializing,
					IntentDigest: "something_else",
				},
			}
			for _, p := range pods {
				require.NoError(t, p.Insert())
			}

			dbPods, err := FindByIntentDigest(intentDigest)
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
				ID:           "pod",
				Status:       StatusStarting,
				IntentDigest: "intent_digest",
			}
			require.NoError(t, p.Insert())
			dbPods, err := FindByIntentDigest(p.IntentDigest)
			assert.NoError(t, err)
			assert.Empty(t, dbPods)
		},
		"IgnoresPodsWithoutMatchingDigest": func(t *testing.T) {
			p := Pod{
				ID:           "pod",
				Status:       StatusStarting,
				IntentDigest: "intent_digest",
			}
			require.NoError(t, p.Insert())
			dbPods, err := FindByIntentDigest("foo")
			assert.NoError(t, err)
			assert.Empty(t, dbPods)
		},
		"ReturnsNoErrorForNoMatchingPods": func(t *testing.T) {
			dbPods, err := FindByIntentDigest("nonexistent")
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

			require.NoError(t, p.UpdateStatus(p.Status))
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
			require.NoError(t, UpdateOneStatus(p.ID, p.Status, updated, time.Now()))

			dbPod, err := FindOneByID(p.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			checkStatusAndTimeInfo(t, dbPod, updated)
			checkEventLog(t, p)
		},
		"SucceedsWithStartingStatus": func(t *testing.T, p Pod) {
			require.NoError(t, p.Insert())

			updated := StatusStarting
			require.NoError(t, UpdateOneStatus(p.ID, p.Status, updated, time.Now()))

			dbPod, err := FindOneByID(p.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			checkStatusAndTimeInfo(t, dbPod, updated)
			checkEventLog(t, p)
		},
		"FailsWithMismatchedCurrentStatus": func(t *testing.T, p Pod) {
			require.NoError(t, p.Insert())

			assert.Error(t, UpdateOneStatus(p.ID, StatusInitializing, StatusTerminated, time.Now()))
		},
		"SucceedsWithTerminatedStatus": func(t *testing.T, p Pod) {
			require.NoError(t, p.Insert())

			updated := StatusTerminated
			require.NoError(t, UpdateOneStatus(p.ID, p.Status, updated, time.Now()))

			dbPod, err := FindOneByID(p.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)

			assert.Equal(t, updated, dbPod.Status)
			checkEventLog(t, p)
		},
		"FailsWithNonexistentPod": func(t *testing.T, p Pod) {
			require.NoError(t, p.Insert())

			assert.Error(t, UpdateOneStatus("nonexistent", StatusStarting, StatusRunning, time.Now()))
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(Collection, event.LegacyEventLogCollection))
			defer func() {
				assert.NoError(t, db.ClearCollections(Collection, event.LegacyEventLogCollection))
			}()

			p := Pod{
				ID:     "id",
				Status: StatusRunning,
			}

			tCase(t, p)
		})
	}
}
