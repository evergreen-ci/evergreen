package model

import (
	"github.com/evergreen-ci/evergreen/testutil"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() { testutil.Setup() }

func TestAPIPatch(t *testing.T) {
	assert.NoError(t, db.ClearCollections(model.ProjectRefCollection, commitqueue.Collection))
	assert := assert.New(t)
	baseTime := time.Now()
	pRef := model.ProjectRef{
		Id:         "mci",
		Identifier: "evergreen",
	}
	assert.NoError(pRef.Insert())
	p := patch.Patch{
		Id:            mgobson.NewObjectId(),
		Description:   "test",
		Project:       pRef.Id,
		Githash:       "hash",
		PatchNumber:   9000,
		Author:        "root",
		Version:       "version_1",
		Status:        evergreen.PatchCreated,
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
		Activated:            true,
		PatchedParserProject: "config",
		Alias:                evergreen.CommitQueueAlias,
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
	cq := commitqueue.CommitQueue{
		ProjectID: p.Project,
		Queue: []commitqueue.CommitQueueItem{
			{PatchId: "something else"},
			{PatchId: p.Id.Hex()},
		},
	}
	assert.NoError(commitqueue.InsertQueue(&cq))

	a := APIPatch{}
	err := a.BuildFromService(p, &APIPatchArgs{
		IncludeProjectIdentifier:   true,
		IncludeCommitQueuePosition: true,
	})
	assert.NoError(err)

	assert.Equal(p.Id.Hex(), utility.FromStringPtr(a.Id))
	assert.Equal(p.Description, utility.FromStringPtr(a.Description))
	assert.Equal(p.Project, utility.FromStringPtr(a.ProjectId))
	assert.Equal(pRef.Identifier, utility.FromStringPtr(a.ProjectIdentifier))
	assert.Equal(p.Project, utility.FromStringPtr(a.Branch))
	assert.Equal(p.Githash, utility.FromStringPtr(a.Githash))
	assert.Equal(p.PatchNumber, a.PatchNumber)
	assert.Equal(p.Author, utility.FromStringPtr(a.Author))
	assert.Equal(p.Version, utility.FromStringPtr(a.Version))
	assert.Equal(p.Status, utility.FromStringPtr(a.Status))
	assert.Zero(a.CreateTime.Sub(p.CreateTime))
	assert.Equal(1, utility.FromIntPtr(a.CommitQueuePosition))
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
	a := githubPatch{}
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
	assert.NoError(projectRef.Insert())
	p := patch.Patch{
		Id:          mgobson.NewObjectId(),
		Description: "test",
		Project:     "mci",
		Tasks:       []string{"t1", "t2"},
		Activated:   true,
		Triggers: patch.TriggerInfo{
			ChildPatches: []string{childPatchId},
		},
		Status: evergreen.PatchCreated,
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
		Status:    evergreen.PatchCreated,
	}
	assert.NoError(childPatch.Insert())

	a := APIPatch{}
	err := a.BuildFromService(p, &APIPatchArgs{
		IncludeChildPatches: true,
	})
	assert.NoError(err)
	require.Len(t, a.DownstreamTasks, 1)
	assert.Equal(*a.DownstreamTasks[0].Project, childPatch.Project)
	assert.Len(a.DownstreamTasks[0].Tasks, 2)
	assert.Len(a.DownstreamTasks[0].VariantTasks, 1)
}
