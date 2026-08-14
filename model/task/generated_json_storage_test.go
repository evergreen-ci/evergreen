package task

import (
	"context"
	"fmt"
	"path"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratedJSONS3Storage(t *testing.T) {
	ctx := t.Context()

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

	testutil.ConfigureIntegrationTest(t, env.Settings())

	c := utility.GetHTTPClient()
	defer utility.PutHTTPClient(c)

	ppConf := env.Settings().Providers.AWS.ParserProject
	bucket, err := pail.NewS3BucketWithHTTPClient(ctx, c, pail.S3Options{
		Name:   ppConf.Bucket,
		Region: evergreen.DefaultEC2Region,
	})
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, bucket.RemovePrefix(ctx, ppConf.Prefix))
	}()

	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()

	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment, tsk *Task){
		"FindReturnsNilErrorAndResultForTaskWithNoGeneratedJSON": func(ctx context.Context, t *testing.T, env *mock.Environment, tsk *Task) {
			fileStorage, err := newGeneratedJSONS3Storage(ctx, env.Settings().Providers.AWS.ParserProject)
			require.NoError(t, err)

			files, err := fileStorage.Find(ctx, tsk)
			assert.NoError(t, err)
			assert.Empty(t, files)
		},
		"InsertStoresGeneratedJSONFiles": func(ctx context.Context, t *testing.T, env *mock.Environment, tsk *Task) {
			fileStorage, err := newGeneratedJSONS3Storage(ctx, env.Settings().Providers.AWS.ParserProject)
			require.NoError(t, err)

			files := GeneratedJSONFiles{`{"key0": "value0"}`, `{"key1": "value1"}`}
			assert.NoError(t, fileStorage.Insert(ctx, tsk, files))

			storedFiles, err := fileStorage.Find(ctx, tsk)
			assert.NoError(t, err)
			require.NotNil(t, storedFiles)
			assert.Equal(t, files, storedFiles)
		},
		"InsertNoopsForExistingGeneratedJSONFiles": func(ctx context.Context, t *testing.T, env *mock.Environment, tsk *Task) {
			fileStorage, err := newGeneratedJSONS3Storage(ctx, env.Settings().Providers.AWS.ParserProject)
			require.NoError(t, err)

			files := GeneratedJSONFiles{`{"key": "value"}`}
			assert.NoError(t, fileStorage.Insert(ctx, tsk, files))

			storedFiles, err := fileStorage.Find(ctx, tsk)
			assert.NoError(t, err)
			require.NotNil(t, files)
			assert.Equal(t, files, storedFiles)

			newFiles := GeneratedJSONFiles{`{"new_key": "new_value"}`}
			assert.NoError(t, fileStorage.Insert(ctx, tsk, newFiles))

			storedFiles, err = fileStorage.Find(ctx, tsk)
			assert.NoError(t, err)
			require.NotNil(t, storedFiles)
			assert.Equal(t, files, storedFiles, "inserting new files should not overwrite existing files")
		},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(Collection))
			require.NoError(t, bucket.RemovePrefix(ctx, ppConf.Prefix))

			tsk := &Task{
				Id: fmt.Sprintf("%s-%s", path.Base(t.Name()), utility.RandomString()),
			}
			require.NoError(t, tsk.Insert(t.Context()))

			testCase(ctx, t, env, tsk)
		})
	}
}

// TestGeneratedJSONFind covers retrieval for every storage method. generate.tasks
// only writes to S3 now, but tasks that ran before that switch still store their
// generated JSON in the task document.
func TestGeneratedJSONFind(t *testing.T) {
	ctx := t.Context()

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

	testutil.ConfigureIntegrationTest(t, env.Settings())

	c := utility.GetHTTPClient()
	defer utility.PutHTTPClient(c)

	ppConf := env.Settings().Providers.AWS.ParserProject
	bucket, err := pail.NewS3BucketWithHTTPClient(ctx, c, pail.S3Options{
		Name:   ppConf.Bucket,
		Region: evergreen.DefaultEC2Region,
	})
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, bucket.RemovePrefix(ctx, ppConf.GeneratedJSONPrefix))
	}()

	files := GeneratedJSONFiles{`{"key0": "value0"}`, `{"key1": "value1"}`}

	for testName, storeFiles := range map[string]func(t *testing.T, tsk *Task){
		"UnsetStorageMethodReadsFromTheTaskDocument": func(t *testing.T, tsk *Task) {
			tsk.GeneratedJSONAsString = files
		},
		"DBStorageMethodReadsFromTheTaskDocument": func(t *testing.T, tsk *Task) {
			tsk.GeneratedJSONAsString = files
			tsk.GeneratedJSONStorageMethod = evergreen.ProjectStorageMethodDB
		},
		"S3StorageMethodReadsFromS3": func(t *testing.T, tsk *Task) {
			require.NoError(t, GeneratedJSONInsert(ctx, env.Settings(), tsk, files))
		},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(Collection))
			t.Cleanup(func() {
				assert.NoError(t, db.ClearCollections(Collection))
			})

			tsk := &Task{
				Id: fmt.Sprintf("%s-%s", path.Base(t.Name()), utility.RandomString()),
			}
			require.NoError(t, tsk.Insert(ctx))

			storeFiles(t, tsk)

			storedFiles, err := GeneratedJSONFind(ctx, env.Settings(), tsk)
			require.NoError(t, err)
			assert.Equal(t, files, storedFiles)
		})
	}
}
