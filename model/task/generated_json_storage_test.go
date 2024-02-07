package task

import (
	"context"
	"fmt"
	"path"
	"testing"

	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratedJSONStorage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

	testutil.ConfigureIntegrationTest(t, env.Settings(), t.Name())

	c := utility.GetHTTPClient()
	defer utility.PutHTTPClient(c)

	ppConf := env.Settings().Providers.AWS.ParserProject
	bucket, err := pail.NewS3BucketWithHTTPClient(c, pail.S3Options{
		Name:   ppConf.Bucket,
		Region: endpoints.UsEast1RegionID,
	})
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, bucket.RemovePrefix(ctx, ppConf.Prefix))
	}()

	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()

	for methodName, storageMethod := range map[string]evergreen.ParserProjectStorageMethod{
		"DB": evergreen.ProjectStorageMethodDB,
		"S3": evergreen.ProjectStorageMethodS3,
	} {
		t.Run("StorageMethod"+methodName, func(t *testing.T) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment, tsk *Task){
				"FindReturnsNilErrorAndResultForTaskWithNoGeneratedJSON": func(ctx context.Context, t *testing.T, env *mock.Environment, tsk *Task) {
					filesStorage, err := GetGeneratedJSONFileStorage(env.Settings(), storageMethod)
					require.NoError(t, err)
					defer filesStorage.Close(ctx)

					files, err := filesStorage.Find(ctx, tsk)
					assert.NoError(t, err)
					assert.Empty(t, files)
				},
				"InsertStoresGeneratedJSONFiles": func(ctx context.Context, t *testing.T, env *mock.Environment, tsk *Task) {
					fileStorage, err := GetGeneratedJSONFileStorage(env.Settings(), storageMethod)
					require.NoError(t, err)
					defer fileStorage.Close(ctx)

					files := GeneratedJSONFiles{`{"key0": "value0"}`, `{"key1": "value1"}`}
					assert.NoError(t, fileStorage.Insert(ctx, tsk, files))

					storedFiles, err := fileStorage.Find(ctx, tsk)
					assert.NoError(t, err)
					require.NotNil(t, storedFiles)
					assert.Equal(t, files, storedFiles)
				},
				"InsertNoopsForExistingGeneratedJSONFiles": func(ctx context.Context, t *testing.T, env *mock.Environment, tsk *Task) {
					fileStorage, err := GetGeneratedJSONFileStorage(env.Settings(), storageMethod)
					require.NoError(t, err)
					defer fileStorage.Close(ctx)

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
					require.NoError(t, tsk.Insert())

					testCase(ctx, t, env, tsk)
				})
			}
		})
	}
}
