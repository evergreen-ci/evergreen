package pod

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func init() {
	testutil.Setup()
}

func TestInsertAndFindOneByID(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T){
		"FindOneByIDReturnsNoResultForNonexistentPod": func(t *testing.T) {
			p, err := FindOneByID("foo")
			assert.NoError(t, err)
			assert.Zero(t, p)
		},
		"InsertSucceedsAndIsFoundByID": func(t *testing.T) {
			p := Pod{
				ID: "id",
				Secret: Secret{
					Name:   "name",
					Value:  "value",
					Exists: utility.FalsePtr(),
					Owned:  utility.TruePtr(),
				},
				Status: StatusRunning,
				TaskContainerCreationOpts: TaskContainerCreationOptions{
					Image:          "alpine",
					MemoryMB:       128,
					CPU:            128,
					OS:             OSWindows,
					Arch:           ArchAMD64,
					WindowsVersion: WindowsVersionServer2019,
				},
				TimeInfo: TimeInfo{
					Initializing:     utility.BSONTime(time.Now()),
					Starting:         utility.BSONTime(time.Now()),
					LastCommunicated: utility.BSONTime(time.Now()),
				},
				Resources: ResourceInfo{
					ExternalID:   "external_id",
					DefinitionID: "definition_id",
					Cluster:      "cluster",
					Containers: []ContainerResourceInfo{
						{
							ExternalID: "container_id",
							Name:       "container_name",
							SecretIDs:  []string{"secret0", "secret1"},
						},
					},
				},
			}
			require.NoError(t, p.Insert())

			dbPod, err := FindOneByID(p.ID)
			require.NoError(t, err)
			require.NotNil(t, dbPod)

			assert.Equal(t, p.ID, dbPod.ID)
			assert.Equal(t, p.Status, dbPod.Status)
			assert.Equal(t, p.Secret, dbPod.Secret)
			assert.Equal(t, p.Resources, dbPod.Resources)
			assert.Equal(t, p.TimeInfo, dbPod.TimeInfo)
			assert.Equal(t, p.TaskContainerCreationOpts, dbPod.TaskContainerCreationOpts)
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

func TestRemove(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T){
		"NoopsWithNonexistentPod": func(t *testing.T) {
			p := Pod{
				ID: "nonexistent",
			}
			dbPod, err := FindOneByID(p.ID)
			assert.NoError(t, err)
			assert.Zero(t, dbPod)

			assert.NoError(t, p.Remove())
		},
		"SucceedsWithExistingPod": func(t *testing.T) {
			p := Pod{
				ID: "id",
			}
			require.NoError(t, p.Insert())

			dbPod, err := FindOneByID(p.ID)
			require.NoError(t, err)
			require.NotNil(t, dbPod)

			require.NoError(t, p.Remove())

			dbPod, err = FindOneByID(p.ID)
			assert.NoError(t, err)
			assert.Zero(t, dbPod)
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

func TestUpdateStatus(t *testing.T) {
	checkStatus := func(t *testing.T, p Pod) {
		dbPod, err := FindOneByID(p.ID)
		require.NoError(t, err)
		require.NotZero(t, dbPod)
		assert.Equal(t, p.Status, dbPod.Status)
	}

	checkStatusAndTimeInfo := func(t *testing.T, p Pod) {
		dbPod, err := FindOneByID(p.ID)
		require.NoError(t, err)
		require.NotZero(t, dbPod)
		assert.Equal(t, p.Status, dbPod.Status)
		switch p.Status {
		case StatusInitializing:
			assert.NotZero(t, p.TimeInfo.Initializing)
			assert.NotZero(t, dbPod.TimeInfo.Initializing)
			assert.Equal(t, p.TimeInfo.Initializing, dbPod.TimeInfo.Initializing)
		case StatusStarting:
			assert.NotZero(t, p.TimeInfo.Starting)
			assert.NotZero(t, dbPod.TimeInfo.Starting)
			assert.Equal(t, p.TimeInfo.Starting, dbPod.TimeInfo.Starting)
		}
	}

	checkEventLog := func(t *testing.T, p Pod) {
		events, err := event.Find(event.AllLogCollection, event.MostRecentPodEvents(p.ID, 10))
		require.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, p.ID, events[0].ResourceId)
		assert.Equal(t, event.ResourceTypePod, events[0].ResourceType)
		assert.Equal(t, string(event.EventPodStatusChange), events[0].EventType)
	}

	for tName, tCase := range map[string]func(t *testing.T, p Pod){
		"SucceedsWithInitializingStatus": func(t *testing.T, p Pod) {
			require.NoError(t, p.Insert())

			require.NoError(t, p.UpdateStatus(StatusInitializing))
			assert.Equal(t, StatusInitializing, p.Status)

			checkStatusAndTimeInfo(t, p)
			checkEventLog(t, p)
		},
		"SucceedsWithStartingStatus": func(t *testing.T, p Pod) {
			require.NoError(t, p.Insert())

			require.NoError(t, p.UpdateStatus(StatusStarting))
			assert.Equal(t, StatusStarting, p.Status)

			checkStatusAndTimeInfo(t, p)
			checkEventLog(t, p)
		},
		"SucceedsWithTerminatedStatus": func(t *testing.T, p Pod) {
			require.NoError(t, p.Insert())

			require.NoError(t, p.UpdateStatus(StatusTerminated))
			assert.Equal(t, StatusTerminated, p.Status)

			checkStatus(t, p)
			checkEventLog(t, p)
		},
		"NoopsWithIdenticalStatus": func(t *testing.T, p Pod) {
			require.NoError(t, p.Insert())

			require.NoError(t, p.UpdateStatus(p.Status))
			checkStatus(t, p)
		},
		"FailsWithNonexistentPod": func(t *testing.T, p Pod) {
			assert.Error(t, p.UpdateStatus(StatusTerminated))

			dbPod, err := FindOneByID(p.ID)
			assert.NoError(t, err)
			assert.Zero(t, dbPod)
		},
		"FailsWithChangedPodStatus": func(t *testing.T, p Pod) {
			require.NoError(t, p.Insert())

			require.NoError(t, UpdateOne(ByID(p.ID), bson.M{
				"$set": bson.M{StatusKey: StatusInitializing},
			}))

			assert.Error(t, p.UpdateStatus(StatusTerminated))

			dbPod, err := FindOneByID(p.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, StatusInitializing, dbPod.Status)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(Collection, event.AllLogCollection))
			defer func() {
				assert.NoError(t, db.ClearCollections(Collection, event.AllLogCollection))
			}()
			tCase(t, Pod{
				ID:     "id",
				Status: StatusRunning,
			})
		})
	}
}

func TestUpdateResources(t *testing.T) {
	checkResources := func(t *testing.T, p Pod) {
		dbPod, err := FindOneByID(p.ID)
		require.NoError(t, err)
		require.NotZero(t, dbPod)
		assert.Equal(t, p.Resources, dbPod.Resources)
	}

	for tName, tCase := range map[string]func(t *testing.T, p Pod){
		"SucceedsForEmptyResources": func(t *testing.T, p Pod) {
			require.NoError(t, p.Insert())

			require.NoError(t, p.UpdateResources(p.Resources))
			checkResources(t, p)
		},
		"SuccessWithCluster": func(t *testing.T, p Pod) {
			require.NoError(t, p.Insert())

			info := ResourceInfo{
				Cluster: "cluster",
			}
			require.NoError(t, p.UpdateResources(info))
			assert.Equal(t, "cluster", p.Resources.Cluster)

			checkResources(t, p)
		},
		"SuccessWithExternalID": func(t *testing.T, p Pod) {
			require.NoError(t, p.Insert())

			info := ResourceInfo{
				ExternalID: "external",
			}
			require.NoError(t, p.UpdateResources(info))
			assert.Equal(t, "external", p.Resources.ExternalID)

			checkResources(t, p)
		},
		"SuccessWithDefinitionID": func(t *testing.T, p Pod) {
			require.NoError(t, p.Insert())

			info := ResourceInfo{
				DefinitionID: "definition",
			}
			require.NoError(t, p.UpdateResources(info))
			assert.Equal(t, "definition", p.Resources.DefinitionID)

			checkResources(t, p)
		},
		"SuccessWithContainers": func(t *testing.T, p Pod) {
			require.NoError(t, p.Insert())

			containerInfo := ContainerResourceInfo{
				ExternalID: "container_id",
				Name:       "container_name",
				SecretIDs:  []string{"secret"},
			}
			info := ResourceInfo{
				Containers: []ContainerResourceInfo{containerInfo},
			}
			require.NoError(t, p.UpdateResources(info))
			require.Len(t, p.Resources.Containers, 1)
			assert.Equal(t, containerInfo.ExternalID, p.Resources.Containers[0].ExternalID)
			assert.Equal(t, containerInfo.Name, p.Resources.Containers[0].Name)
			assert.Equal(t, containerInfo.SecretIDs, p.Resources.Containers[0].SecretIDs)

			checkResources(t, p)
		},
		"FailsWithNonexistentPod": func(t *testing.T, p Pod) {
			assert.Error(t, p.UpdateResources(ResourceInfo{}))

			dbPod, err := FindOneByID(p.ID)
			assert.NoError(t, err)
			assert.Zero(t, dbPod)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(Collection, event.AllLogCollection))
			defer func() {
				assert.NoError(t, db.ClearCollections(Collection, event.AllLogCollection))
			}()
			tCase(t, Pod{
				ID:     "id",
				Status: StatusRunning,
				Resources: ResourceInfo{
					ExternalID:   utility.RandomString(),
					DefinitionID: utility.RandomString(),
					Cluster:      utility.RandomString(),
					Containers: []ContainerResourceInfo{
						{
							ExternalID: utility.RandomString(),
							Name:       utility.RandomString(),
							SecretIDs:  []string{utility.RandomString()},
						},
					},
				},
			})
		})
	}

}
