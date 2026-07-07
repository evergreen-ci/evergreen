package units

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

const githubPRLabelInjectConfig = `
buildvariants:
  - name: bv1
    display_name: BV One
    run_on:
      - ubuntu1604-test
    tasks:
      - name: task_existing
      - name: task_gated
  - name: bv2
    display_name: BV Two
    run_on:
      - ubuntu1604-test
    tasks:
      - name: task_gated

tasks:
  - name: task_existing
    commands:
      - command: shell.exec
        params:
          script: "echo hi"
  - name: task_gated
    commands:
      - command: shell.exec
        params:
          script: "echo hi"
`

func TestGithubPRLabelInjectAddsGatedTasks(t *testing.T) {
	ctx := testutil.TestSpan(t.Context(), t)

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

	require.NoError(t, db.ClearCollections(model.ProjectRefCollection, model.VersionCollection, build.Collection,
		task.Collection, patch.Collection, model.ParserProjectCollection, model.ProjectAliasCollection))
	t.Cleanup(func() {
		assert.NoError(t, db.ClearCollections(model.ProjectRefCollection, model.VersionCollection, build.Collection,
			task.Collection, patch.Collection, model.ParserProjectCollection, model.ProjectAliasCollection))
	})

	const (
		projectID = "myproj"
		owner     = "evergreen-ci"
		repo      = "evergreen"
		prNumber  = 42
		headSHA   = "abc123"
		variant   = "bv1"
		existing  = "task_existing"
		gated     = "task_gated"
		label     = "run-gated"
	)

	oid := mgobson.NewObjectId()
	versionID := oid.Hex()

	v := &model.Version{
		Id:         versionID,
		Identifier: projectID,
		Requester:  evergreen.GithubPRRequester,
		CreateTime: time.Now(),
		Activated:  utility.TruePtr(),
		BuildIds:   []string{"b1"},
		BuildVariants: []model.VersionBuildStatus{{
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

	projectRef := model.ProjectRef{Id: projectID, Identifier: projectID}
	require.NoError(t, projectRef.Insert(t.Context()))

	pp := model.ParserProject{}
	require.NoError(t, util.UnmarshalYAMLWithFallback([]byte(githubPRLabelInjectConfig), &pp))
	pp.Id = versionID
	pp.Identifier = utility.ToStringPtr(projectID)
	require.NoError(t, pp.Insert(t.Context()))

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

	// The label-gated github_pr_aliases entry lives in the project YAML, which a
	// finalized PR patch stores as its patched project config. This is the only
	// supported configuration for the feature, so the test must exercise the
	// config path (not a DB alias).
	patchedProjectConfig := `
github_pr_aliases:
  - variant: "^bv1$"
    task: "^task_gated$"
    required_labels: ["` + label + `"]
`

	patchDoc := patch.Patch{
		Id:                   oid,
		Project:              projectID,
		Version:              versionID,
		PatchedProjectConfig: patchedProjectConfig,
		GithubPatchData: thirdparty.GithubPatch{
			BaseOwner: owner,
			BaseRepo:  repo,
			PRNumber:  prNumber,
			HeadHash:  headSHA,
		},
		VariantsTasks: []patch.VariantTasks{{
			Variant: variant,
			Tasks:   []string{existing},
		}},
	}
	require.NoError(t, patchDoc.Insert(t.Context()))

	j := NewGithubPRLabelInjectJob(env, owner, repo, prNumber, headSHA, []string{label}, "ts")
	j.Run(ctx)
	require.NoError(t, j.Error())

	tasks, err := task.FindAll(ctx, db.Query(task.ByVersion(versionID)))
	require.NoError(t, err)
	names := map[string]bool{}
	for _, tsk := range tasks {
		names[tsk.DisplayName] = true
	}
	assert.True(t, names[existing], "pre-existing task should remain")
	assert.True(t, names[gated], "label-gated task should be injected")
}

func TestGithubPRLabelInjectNoVersionIsNoOp(t *testing.T) {
	ctx := testutil.TestSpan(t.Context(), t)

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

	require.NoError(t, db.ClearCollections(model.VersionCollection, task.Collection, patch.Collection))
	t.Cleanup(func() {
		assert.NoError(t, db.ClearCollections(model.VersionCollection, task.Collection, patch.Collection))
	})

	j := NewGithubPRLabelInjectJob(env, "evergreen-ci", "evergreen", 999, "deadbeef", []string{"run-gated"}, "ts")
	j.Run(ctx)
	require.NoError(t, j.Error())

	versions, err := model.VersionFind(ctx, db.Query(bson.M{}))
	assert.NoError(t, err)
	assert.Empty(t, versions)

	tasks, err := task.FindAll(ctx, db.Query(bson.M{}))
	assert.NoError(t, err)
	assert.Empty(t, tasks)
}
