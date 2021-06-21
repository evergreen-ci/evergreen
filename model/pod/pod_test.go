package pod

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	testutil.Setup()
}

func TestInsertAndFindOneByID(t *testing.T) {
	for testName, testCase := range map[string]func(t *testing.T){
		"FindOneByIDReturnsNoResultForNonexistentPod": func(t *testing.T) {
			p, err := FindOneByID("foo")
			assert.NoError(t, err)
			assert.Zero(t, p)
		},
		"InsertSucceedsAndIsFoundByID": func(t *testing.T) {
			p := Pod{
				ID:    "id",
				PodID: "pod_id",
				TaskContainerCreationOpts: TaskContainerCreationOptions{
					Image:    "alpine",
					MemoryMB: 128,
					CPU:      128,
				},
				TimeInfo: TimeInfo{
					Initialized: utility.BSONTime(time.Now()),
				},
			}
			require.NoError(t, p.Insert())

			dbPod, err := FindOneByID(p.ID)
			require.NoError(t, err)
			require.NotNil(t, dbPod)

			assert.Equal(t, p.ID, dbPod.ID)
			assert.Equal(t, p.PodID, dbPod.PodID)
			assert.Equal(t, p.TaskContainerCreationOpts, dbPod.TaskContainerCreationOpts)
			assert.Equal(t, p.TimeInfo, dbPod.TimeInfo)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			defer func() {
				assert.NoError(t, db.Clear(Collection))
			}()
			testCase(t)
		})
	}
}

func TestDelete(t *testing.T) {
	for testName, testCase := range map[string]func(t *testing.T){
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
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			defer func() {
				assert.NoError(t, db.Clear(Collection))
			}()
			testCase(t)
		})
	}
}
