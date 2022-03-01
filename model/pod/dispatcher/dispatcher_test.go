package dispatcher

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestUpsertAtomically(t *testing.T) {
	defer func() {
		assert.NoError(t, db.DropCollections(Collection))
	}()
	require.NoError(t, testutil.AddTestIndexes(Collection, true, false, GroupIDKey))

	for tName, tCase := range map[string]func(t *testing.T, pd PodDispatcher){
		"InsertsNewPodDispatcher": func(t *testing.T, pd PodDispatcher) {
			change, err := pd.UpsertAtomically()
			require.NoError(t, err)
			require.Equal(t, change.Updated, 1)

			dbDispatcher, err := FindOneByID(pd.ID)
			require.NoError(t, err)
			require.NotZero(t, dbDispatcher)
			assert.Equal(t, pd.GroupID, dbDispatcher.GroupID)
		},
		"UpdatesExistingPodDispatcher": func(t *testing.T, pd PodDispatcher) {
			require.NoError(t, pd.Insert())

			change, err := pd.UpsertAtomically()
			require.NoError(t, err)
			require.Equal(t, change.Updated, 1)

			dbDispatcher, err := FindOneByID(pd.ID)
			require.NoError(t, err)
			require.NotZero(t, dbDispatcher)
			assert.Equal(t, pd.GroupID, dbDispatcher.GroupID)
			assert.Equal(t, pd.PodIDs, dbDispatcher.PodIDs)
			assert.Equal(t, pd.TaskIDs, dbDispatcher.TaskIDs)
			assert.NotZero(t, pd.ModificationCount)
			assert.Equal(t, pd.ModificationCount, dbDispatcher.ModificationCount)
		},
		"FailsWithMatchingGroupIDButDifferentDispatcherID": func(t *testing.T, pd PodDispatcher) {
			require.NoError(t, pd.Insert())

			modified := pd
			modified.ID = primitive.NewObjectID().Hex()
			modified.PodIDs = []string{"modified-pod0"}
			modified.TaskIDs = []string{"modified-task0"}

			change, err := modified.UpsertAtomically()
			assert.Error(t, err)
			assert.Zero(t, change)

			dbDispatcher, err := FindOneByID(modified.ID)
			assert.NoError(t, err)
			assert.Zero(t, dbDispatcher)

			dbDispatcher, err = FindOneByID(pd.ID)
			require.NoError(t, err)
			require.NotZero(t, dbDispatcher)
			assert.Equal(t, pd.GroupID, dbDispatcher.GroupID)
			assert.Equal(t, pd.PodIDs, dbDispatcher.PodIDs)
			assert.Equal(t, pd.TaskIDs, dbDispatcher.TaskIDs)
		},
		"FailsWithMatchingDispatcherIDButDifferentGroupID": func(t *testing.T, pd PodDispatcher) {
			require.NoError(t, pd.Insert())

			modified := pd
			modified.GroupID = utility.RandomString()
			modified.PodIDs = []string{"modified-pod0"}
			modified.TaskIDs = []string{"modified-task0"}

			change, err := modified.UpsertAtomically()
			assert.Error(t, err)
			assert.Zero(t, change)

			dbDispatcher, err := FindOneByID(pd.ID)
			require.NoError(t, err)
			require.NotZero(t, dbDispatcher)
			assert.Equal(t, pd.GroupID, dbDispatcher.GroupID)
			assert.Equal(t, pd.PodIDs, dbDispatcher.PodIDs)
			assert.Equal(t, pd.TaskIDs, dbDispatcher.TaskIDs)
		},
		"FailsWithDifferentModificationCount": func(t *testing.T, pd PodDispatcher) {
			require.NoError(t, pd.Insert())

			modified := pd
			modified.ModificationCount = 12345
			modified.PodIDs = []string{"modified-pod0"}
			modified.TaskIDs = []string{"modified-task0"}

			change, err := modified.UpsertAtomically()
			assert.Error(t, err)
			assert.Zero(t, change)

			dbDispatcher, err := FindOneByID(pd.ID)
			require.NoError(t, err)
			require.NotZero(t, dbDispatcher)
			assert.Equal(t, pd.GroupID, dbDispatcher.GroupID)
			assert.Equal(t, pd.PodIDs, dbDispatcher.PodIDs)
			assert.Equal(t, pd.TaskIDs, dbDispatcher.TaskIDs)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(Collection))
			tCase(t, NewPodDispatcher("group0", []string{"task0"}, []string{"pod0"}))
		})
	}
}
