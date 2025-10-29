package model

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPIPatch(t *testing.T) {
	assert.NoError(t, db.ClearCollections(model.ProjectRefCollection))
	assert := assert.New(t)
	baseTime := time.Now()
	pRef := model.ProjectRef{
		Id:         "mci",
		Identifier: "evergreen",
		Branch:     "main",
	}
	assert.NoError(pRef.Insert(t.Context()))
	p := patch.Patch{
		Id:            mgobson.NewObjectId(),
		Description:   "test",
		Project:       pRef.Id,
		Branch:        pRef.Branch,
		Githash:       "hash",
		PatchNumber:   9000,
		Author:        "root",
		Version:       "version_1",
		Status:        evergreen.VersionCreated,
		CreateTime:    baseTime,
		StartTime:     baseTime.Add(time.Hour),
		FinishTime:    baseTime.Add(2 * time.Hour),
		BuildVariants: []string{"bv1", "bv2"},
		Tasks:         []string{"t1", "t2"},
		VariantsTasks: []patch.VariantTasks{
			{
				Variant: "bv1",
				Tasks:   []string{"t1"},
			},
			{
				Variant: "bv2",
				Tasks:   []string{"t2"},
			},
		},
		Patches: []patch.ModulePatch{
			{},
		},
		Activated: true,
		Alias:     evergreen.CommitQueueAlias,
		GithubPatchData: thirdparty.GithubPatch{
			PRNumber:  123,
			BaseOwner: "evergreen-ci",
			BaseRepo:  "evergreen",
			HeadOwner: "octocat",
			HeadRepo:  "evergreen",
			HeadHash:  "hash",
			Author:    "octocat",
		},
	}

	a := APIPatch{}
	err := a.BuildFromService(t.Context(), p, &APIPatchArgs{
		IncludeProjectIdentifier: true,
	})
	assert.NoError(err)

	assert.Equal(p.Id.Hex(), utility.FromStringPtr(a.Id))
	assert.Equal(p.Description, utility.FromStringPtr(a.Description))
	assert.Equal(p.Project, utility.FromStringPtr(a.ProjectId))
	assert.Equal(p.Project, utility.FromStringPtr(a.LegacyProjectId))
	assert.Equal(pRef.Identifier, utility.FromStringPtr(a.ProjectIdentifier))
	assert.Equal(p.Branch, utility.FromStringPtr(a.Branch))
	assert.Equal(p.Githash, utility.FromStringPtr(a.Githash))
	assert.Equal(p.PatchNumber, a.PatchNumber)
	assert.Equal(p.Author, utility.FromStringPtr(a.Author))
	assert.Equal(p.Version, utility.FromStringPtr(a.Version))
	assert.Equal(p.Status, utility.FromStringPtr(a.Status))
	assert.Zero(a.CreateTime.Sub(p.CreateTime))
	assert.Equal(-time.Hour, a.CreateTime.Sub(p.StartTime))
	assert.Equal(-2*time.Hour, a.CreateTime.Sub(p.FinishTime))
	for i, variant := range a.Variants {
		assert.Equal(p.BuildVariants[i], utility.FromStringPtr(variant))
	}
	for i, task := range a.Tasks {
		assert.Equal(p.Tasks[i], utility.FromStringPtr(task))
	}
	for i, vt := range a.VariantsTasks {
		assert.Equal(p.VariantsTasks[i].Variant, utility.FromStringPtr(vt.Name))
	}
	assert.Equal(evergreen.CommitQueueAlias, utility.FromStringPtr(a.Alias))
	assert.NotZero(a.GithubPatchData)
	assert.NotEqual(a.VariantsTasks[0].Tasks, a.VariantsTasks[1].Tasks)
	assert.Len(a.VariantsTasks[0].Tasks, 1)
}

func TestAPIPatchBuildModuleChanges(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	originalEnv := evergreen.GetEnvironment()
	env := testutil.NewEnvironment(ctx, t)
	evergreen.SetEnvironment(env)
	defer func() {
		evergreen.SetEnvironment(originalEnv)
	}()
	p := patch.Patch{
		Patches: []patch.ModulePatch{
			{
				PatchSet: patch.PatchSet{
					Summary: []thirdparty.Summary{
						{
							Description: "test",
						},
						{
							Description: "test2",
						},
						{
							Description: "test3",
						},
						{
							Description: "test3",
						},
					},
				},
			},
		},
	}
	a := APIPatch{Id: utility.ToStringPtr("patch_id")}
	a.buildModuleChanges(ctx, p, "")
	require.Len(t, a.ModuleCodeChanges, 1)
	assert.Len(t, a.ModuleCodeChanges[0].FileDiffs, 4)
	assert.NotEqual(t, strings.Index(utility.FromStringPtr(a.ModuleCodeChanges[0].FileDiffs[0].DiffLink), "commit_number=0"), -1)
	assert.NotEqual(t, strings.Index(utility.FromStringPtr(a.ModuleCodeChanges[0].FileDiffs[1].DiffLink), "commit_number=1"), -1)
	assert.NotEqual(t, strings.Index(utility.FromStringPtr(a.ModuleCodeChanges[0].FileDiffs[2].DiffLink), "commit_number=2"), -1)
	assert.NotEqual(t, strings.Index(utility.FromStringPtr(a.ModuleCodeChanges[0].FileDiffs[3].DiffLink), "commit_number=3"), -1)

}

func TestAPIPatchBuildModuleChangesWithDiff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	originalEnv := evergreen.GetEnvironment()
	env := testutil.NewEnvironment(ctx, t)
	evergreen.SetEnvironment(env)
	defer func() {
		evergreen.SetEnvironment(originalEnv)
	}()

	testDiffContent := "diff --git a/file.txt b/file.txt\nindex 1234..5678 100644\n--- a/file.txt\n+++ b/file.txt\n@@ -1,1 +1,1 @@\n-old line\n+new line"

	p := patch.Patch{
		Id: mgobson.NewObjectId(),
		Patches: []patch.ModulePatch{
			{
				ModuleName: "test-module",
				PatchSet: patch.PatchSet{
					Patch: testDiffContent,
					Summary: []thirdparty.Summary{
						{
							Name:        "file1.txt",
							Description: "Modified file1",
							Additions:   10,
							Deletions:   5,
						},
						{
							Name:        "file2.go",
							Description: "Modified file2",
							Additions:   20,
							Deletions:   3,
						},
					},
				},
			},
		},
	}

	a := APIPatch{Id: utility.ToStringPtr(p.Id.Hex())}
	a.buildModuleChanges(ctx, p, "test-project")

	require.Len(t, a.ModuleCodeChanges, 1)
	assert.Equal(t, "test-module", utility.FromStringPtr(a.ModuleCodeChanges[0].BranchName))

	require.Len(t, a.ModuleCodeChanges[0].FileDiffs, 2)

	// Verify first file diff
	fileDiff1 := a.ModuleCodeChanges[0].FileDiffs[0]
	assert.Equal(t, "file1.txt", utility.FromStringPtr(fileDiff1.FileName))
	assert.Equal(t, "Modified file1", fileDiff1.Description)
	assert.Equal(t, 10, fileDiff1.Additions)
	assert.Equal(t, 5, fileDiff1.Deletions)
	assert.Equal(t, testDiffContent, fileDiff1.Diff)
	assert.NotNil(t, fileDiff1.DiffLink)

	// Verify second file diff
	fileDiff2 := a.ModuleCodeChanges[0].FileDiffs[1]
	assert.Equal(t, "file2.go", utility.FromStringPtr(fileDiff2.FileName))
	assert.Equal(t, "Modified file2", fileDiff2.Description)
	assert.Equal(t, 20, fileDiff2.Additions)
	assert.Equal(t, 3, fileDiff2.Deletions)
	assert.Equal(t, testDiffContent, fileDiff2.Diff)
	assert.NotNil(t, fileDiff2.DiffLink)
}

func TestAPIPatchBuildModuleChangesWithEmptyDiff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	originalEnv := evergreen.GetEnvironment()
	env := testutil.NewEnvironment(ctx, t)
	evergreen.SetEnvironment(env)
	defer func() {
		evergreen.SetEnvironment(originalEnv)
	}()

	p := patch.Patch{
		Id: mgobson.NewObjectId(),
		Patches: []patch.ModulePatch{
			{
				PatchSet: patch.PatchSet{
					Patch: "", // Empty diff
					Summary: []thirdparty.Summary{
						{
							Name:        "file.txt",
							Description: "Test file",
							Additions:   1,
							Deletions:   0,
						},
					},
				},
			},
		},
	}

	a := APIPatch{Id: utility.ToStringPtr(p.Id.Hex())}
	a.buildModuleChanges(ctx, p, "test-project")

	require.Len(t, a.ModuleCodeChanges, 1)
	require.Len(t, a.ModuleCodeChanges[0].FileDiffs, 1)

	// Verify that Diff field is populated even when empty
	fileDiff := a.ModuleCodeChanges[0].FileDiffs[0]
	assert.Equal(t, "", fileDiff.Diff)
}

func TestAPIPatchBuildModuleChangesMultipleModules(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	originalEnv := evergreen.GetEnvironment()
	env := testutil.NewEnvironment(ctx, t)
	evergreen.SetEnvironment(env)
	defer func() {
		evergreen.SetEnvironment(originalEnv)
	}()

	diff1 := "diff for module 1"
	diff2 := "diff for module 2"

	p := patch.Patch{
		Id: mgobson.NewObjectId(),
		Patches: []patch.ModulePatch{
			{
				ModuleName: "module1",
				PatchSet: patch.PatchSet{
					Patch: diff1,
					Summary: []thirdparty.Summary{
						{
							Name:        "module1_file.txt",
							Description: "Module 1 file",
							Additions:   5,
							Deletions:   2,
						},
					},
				},
			},
			{
				ModuleName: "module2",
				PatchSet: patch.PatchSet{
					Patch: diff2,
					Summary: []thirdparty.Summary{
						{
							Name:        "module2_file.go",
							Description: "Module 2 file",
							Additions:   8,
							Deletions:   1,
						},
					},
				},
			},
		},
	}

	a := APIPatch{Id: utility.ToStringPtr(p.Id.Hex())}
	a.buildModuleChanges(ctx, p, "test-project")

	require.Len(t, a.ModuleCodeChanges, 2)

	// Verify first module
	assert.Equal(t, "module1", utility.FromStringPtr(a.ModuleCodeChanges[0].BranchName))
	require.Len(t, a.ModuleCodeChanges[0].FileDiffs, 1)
	assert.Equal(t, diff1, a.ModuleCodeChanges[0].FileDiffs[0].Diff)

	// Verify second module
	assert.Equal(t, "module2", utility.FromStringPtr(a.ModuleCodeChanges[1].BranchName))
	require.Len(t, a.ModuleCodeChanges[1].FileDiffs, 1)
	assert.Equal(t, diff2, a.ModuleCodeChanges[1].FileDiffs[0].Diff)
}

func TestGithubPatch(t *testing.T) {
	assert := assert.New(t)
	p := thirdparty.GithubPatch{
		PRNumber:  123,
		BaseOwner: "evergreen-ci",
		BaseRepo:  "evergreen",
		HeadOwner: "octocat",
		HeadRepo:  "evergreen",
		HeadHash:  "hash",
		Author:    "octocat",
	}
	a := APIGithubPatch{}
	a.BuildFromService(p)
	assert.Equal(123, a.PRNumber)
	assert.Equal("evergreen-ci", utility.FromStringPtr(a.BaseOwner))
	assert.Equal("evergreen", utility.FromStringPtr(a.BaseRepo))
	assert.Equal("octocat", utility.FromStringPtr(a.HeadOwner))
	assert.Equal("evergreen", utility.FromStringPtr(a.HeadRepo))
	assert.Equal("hash", utility.FromStringPtr(a.HeadHash))
	assert.Equal("octocat", utility.FromStringPtr(a.Author))
}

func TestDownstreamTasks(t *testing.T) {
	assert := assert.New(t)
	require.NoError(t, db.ClearCollections(patch.Collection, model.ProjectRefCollection))
	childPatchId := "6047a08d93572d8dd14fb5eb"

	projectRef := model.ProjectRef{
		Id:         "mci",
		Identifier: "evergreen",
	}
	assert.NoError(projectRef.Insert(t.Context()))
	p := patch.Patch{
		Id:          mgobson.NewObjectId(),
		Description: "test",
		Project:     "mci",
		Tasks:       []string{"t1", "t2"},
		Activated:   true,
		Triggers: patch.TriggerInfo{
			ChildPatches: []string{childPatchId},
		},
		Status: evergreen.VersionCreated,
	}

	childPatch := patch.Patch{
		Id:          mgobson.ObjectIdHex(childPatchId),
		Description: "test",
		Project:     "mci",
		Tasks:       []string{"child_task_1", "child_task_2"},
		VariantsTasks: []patch.VariantTasks{
			{
				Variant: "coverage",
				Tasks:   []string{"variant_task_1", "variant_task_2"},
			},
		},
		Activated: true,
		Status:    evergreen.VersionCreated,
	}
	assert.NoError(childPatch.Insert(t.Context()))

	a := APIPatch{}
	err := a.BuildFromService(t.Context(), p, &APIPatchArgs{
		IncludeChildPatches: true,
	})
	assert.NoError(err)
	require.Len(t, a.DownstreamTasks, 1)
	assert.Equal(*a.DownstreamTasks[0].Project, childPatch.Project)
	assert.Len(a.DownstreamTasks[0].Tasks, 2)
	assert.Len(a.DownstreamTasks[0].VariantTasks, 1)
}

func TestPreselectedDisplayTasks(t *testing.T) {
	require.NoError(t, db.ClearCollections(patch.Collection, model.ProjectRefCollection))

	p := patch.Patch{
		Id:          mgobson.NewObjectId(),
		Description: "test",
		Project:     "mci",
		Tasks:       []string{"variant_task_1", "variant_task_2", "exec1", "exec2"},
		VariantsTasks: []patch.VariantTasks{
			{
				Variant: "coverage",
				Tasks:   []string{"variant_task_1", "variant_task_2", "exec1", "exec2"},
				DisplayTasks: []patch.DisplayTask{
					{
						Name:      "display_task",
						ExecTasks: []string{"exec1", "exec2"},
					},
				},
			},
		},
	}
	require.NoError(t, p.Insert(t.Context()))

	a := APIPatch{}
	err := a.BuildFromService(t.Context(), p, nil)
	require.NoError(t, err)

	// We expect the tasks from the patch to be only non-execution tasks + display tasks.
	require.Len(t, a.VariantsTasks, 1)
	assert.Len(t, a.VariantsTasks[0].Tasks, 3)

	tasks := []string{}
	for _, task := range a.VariantsTasks[0].Tasks {
		tasks = append(tasks, utility.FromStringPtr(task))
	}

	assert.Contains(t, tasks, "variant_task_1")
	assert.Contains(t, tasks, "variant_task_2")
	assert.NotContains(t, tasks, "exec1")
	assert.NotContains(t, tasks, "exec2")
	assert.Contains(t, tasks, "display_task")
}
