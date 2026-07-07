package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const injectTasksConfig = `
buildvariants:
  - name: bv1
    display_name: BV One
    run_on:
      - ubuntu1604-test
    tasks:
      - name: task_existing
      - name: task_new

tasks:
  - name: task_existing
    commands:
      - command: shell.exec
        params:
          script: "echo hi"
  - name: task_new
    commands:
      - command: shell.exec
        params:
          script: "echo hi"
`

func TestInjectTasksIntoVersionAddsDeltaAndUpdatesPatch(t *testing.T) {
	ctx := testutil.TestSpan(t.Context(), t)
	settings := testutil.TestConfig()

	require.NoError(t, db.ClearCollections(ProjectRefCollection, VersionCollection, build.Collection, task.Collection, patch.Collection, ParserProjectCollection))

	const (
		projectID = "myproj"
		variant   = "bv1"
		existing  = "task_existing"
		injected  = "task_new"
	)

	oid := mgobson.NewObjectId()
	versionID := oid.Hex()

	v := &Version{
		Id:         versionID,
		Identifier: projectID,
		Requester:  evergreen.GithubPRRequester,
		CreateTime: time.Now(),
		BuildIds:   []string{"b1"},
		BuildVariants: []VersionBuildStatus{{
			BuildVariant: variant,
			BuildId:      "b1",
		}},
	}
	require.NoError(t, v.Insert(t.Context()))

	b := build.Build{
		Id:           "b1",
		BuildVariant: variant,
		Version:      versionID,
		Activated:    true,
	}
	require.NoError(t, b.Insert(t.Context()))

	projectRef := ProjectRef{Id: projectID, Identifier: projectID}
	require.NoError(t, projectRef.Insert(t.Context()))

	pp := ParserProject{}
	require.NoError(t, util.UnmarshalYAMLWithFallback([]byte(injectTasksConfig), &pp))
	pp.Id = versionID
	pp.Identifier = utility.ToStringPtr(projectID)
	require.NoError(t, pp.Insert(t.Context()))

	p, err := TranslateProject(ctx, &pp)
	require.NoError(t, err)
	require.NotNil(t, p)
	p.Identifier = projectID

	existingTask := task.Task{
		Id:           "existing_task_id",
		Version:      versionID,
		BuildId:      "b1",
		Project:      projectID,
		DisplayName:  existing,
		BuildVariant: variant,
		Activated:    true,
	}
	require.NoError(t, existingTask.Insert(t.Context()))

	patchDoc := patch.Patch{
		Id:      oid,
		Project: projectID,
		Version: versionID,
		VariantsTasks: []patch.VariantTasks{{
			Variant: variant,
			Tasks:   []string{existing},
		}},
	}
	require.NoError(t, patchDoc.Insert(t.Context()))

	pairs := TaskVariantPairs{ExecTasks: TVPairSet{{Variant: variant, TaskName: injected}}}

	createdIDs, err := InjectTasksIntoVersion(ctx, settings, v, p, pairs)
	require.NoError(t, err)
	require.Len(t, createdIDs, 1)

	tasks, err := task.FindAll(ctx, db.Query(task.ByVersion(versionID)))
	require.NoError(t, err)
	names := map[string]bool{}
	for _, tsk := range tasks {
		names[tsk.DisplayName] = true
	}
	assert.True(t, names[existing])
	assert.True(t, names[injected])
	assert.Len(t, tasks, 2)

	// A second call with the same pairs is idempotent: no new tasks, empty result.
	createdIDsAgain, err := InjectTasksIntoVersion(ctx, settings, v, p, pairs)
	require.NoError(t, err)
	assert.Empty(t, createdIDsAgain)

	tasks, err = task.FindAll(ctx, db.Query(task.ByVersion(versionID)))
	require.NoError(t, err)
	assert.Len(t, tasks, 2)

	// The patch's VariantsTasks reflects the injected pair so it survives a restart.
	reloaded, err := patch.FindOneId(ctx, versionID)
	require.NoError(t, err)
	require.NotNil(t, reloaded)
	found := false
	for _, vt := range reloaded.VariantsTasks {
		if vt.Variant == variant {
			assert.Contains(t, vt.Tasks, existing)
			if utility.StringSliceContains(vt.Tasks, injected) {
				found = true
			}
		}
	}
	assert.True(t, found, "injected task should be persisted in patch VariantsTasks")
}
