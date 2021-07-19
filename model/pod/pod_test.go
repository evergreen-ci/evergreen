package pod

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
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
				ID:     "id",
				Secret: "secret",
				Status: RunningStatus,
				TaskContainerCreationOpts: TaskContainerCreationOptions{
					Image:    "alpine",
					MemoryMB: 128,
					CPU:      128,
				},
				Resources: ResourceInfo{
					ID:           "task_id",
					DefinitionID: "task_definition_id",
					Cluster:      "cluster",
					SecretIDs:    []string{"secret0", "secret1"},
				},
				TimeInfo: TimeInfo{
					Initialized: utility.BSONTime(time.Now()),
					Started:     utility.BSONTime(time.Now()),
					Provisioned: utility.BSONTime(time.Now()),
					Terminated:  utility.BSONTime(time.Now()),
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
			assert.Equal(t, p.TaskContainerCreationOpts, dbPod.TaskContainerCreationOpts)
			assert.Equal(t, p.TimeInfo, dbPod.TimeInfo)
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
	for tName, tCase := range map[string]func(t *testing.T, p Pod){
		"Succeeds": func(t *testing.T, p Pod) {
			require.NoError(t, p.Insert())

			dbPod, err := FindOneByID(p.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, p.Status, dbPod.Status)

			require.NoError(t, p.UpdateStatus(TerminatedStatus))
			assert.Equal(t, TerminatedStatus, p.Status)

			dbPod, err = FindOneByID(p.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, p.Status, dbPod.Status)
		},
		"FailsWithNonexistentPod": func(t *testing.T, p Pod) {
			assert.Error(t, p.UpdateStatus(TerminatedStatus))

			dbPod, err := FindOneByID(p.ID)
			assert.NoError(t, err)
			assert.Zero(t, dbPod)
		},
		"FailsWithChangedPodStatus": func(t *testing.T, p Pod) {
			require.NoError(t, p.Insert())

			require.NoError(t, UpdateOne(ByID(p.ID), bson.M{
				"$set": bson.M{StatusKey: InitializingStatus},
			}))

			assert.Error(t, p.UpdateStatus(TerminatedStatus))

			dbPod, err := FindOneByID(p.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, InitializingStatus, dbPod.Status)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			defer func() {
				assert.NoError(t, db.Clear(Collection))
			}()
			tCase(t, Pod{
				ID:     "id",
				Status: RunningStatus,
			})
		})
	}
}
