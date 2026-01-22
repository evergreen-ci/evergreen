package data

import (
	"context"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetManifestByTask(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(manifest.Collection, task.Collection))
	}()
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, tsk *task.Task, mfest *manifest.Manifest){
		"Succeeds": func(ctx context.Context, t *testing.T, tsk *task.Task, mfest *manifest.Manifest) {
			require.NoError(t, tsk.Insert(t.Context()))
			require.NoError(t, mfest.InsertWithContext(ctx))
			dbManifest, err := GetManifestByTask(ctx, tsk.Id)
			require.NoError(t, err)
			require.NotZero(t, dbManifest)
			assert.Equal(t, mfest.Id, dbManifest.Id)
		},
		"FailsWithNonexistentTask": func(ctx context.Context, t *testing.T, tsk *task.Task, mfest *manifest.Manifest) {
			require.NoError(t, mfest.InsertWithContext(ctx))
			dbManifest, err := GetManifestByTask(ctx, tsk.Id)
			assert.Error(t, err)
			gimErr, ok := err.(gimlet.ErrorResponse)
			require.True(t, ok)
			assert.Equal(t, http.StatusNotFound, gimErr.StatusCode)
			assert.Zero(t, dbManifest)
		},
		"FailsWithNonexistentManifest": func(ctx context.Context, t *testing.T, tsk *task.Task, mfest *manifest.Manifest) {
			require.NoError(t, tsk.Insert(t.Context()))
			dbManifest, err := GetManifestByTask(ctx, tsk.Id)
			assert.Error(t, err)
			gimErr, ok := err.(gimlet.ErrorResponse)
			require.True(t, ok)
			assert.Equal(t, http.StatusNotFound, gimErr.StatusCode)
			assert.Zero(t, dbManifest)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			require.NoError(t, db.ClearCollections(manifest.Collection, task.Collection))

			tsk := &task.Task{
				Id:      "task_id",
				Version: "version_id",
			}
			mfest := &manifest.Manifest{
				Id: tsk.Version,
			}
			tCase(ctx, t, tsk, mfest)
		})
	}
}
