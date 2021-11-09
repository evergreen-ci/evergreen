package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/model"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	mgobson "gopkg.in/mgo.v2/bson"
)

func TestAPIPatch(t *testing.T) {
	assert.NoError(t, db.ClearCollections(model.ProjectRefCollection))
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
			patch.VariantTasks{
				Variant: "bv1",
				Tasks:   []string{"t1"},
			},
			patch.VariantTasks{
				Variant: "bv2",
				Tasks:   []string{"t2"},
			},
		},
		Patches: []patch.ModulePatch{
			patch.ModulePatch{},
		},
		Activated:     true,
		PatchedConfig: "config",
		Alias:         "__github",
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
	err := a.BuildFromService(p)
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
	assert.Equal("__github", utility.FromStringPtr(a.Alias))
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
	err := a.BuildFromService(p)
	assert.NoError(err)
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
	}

	childPatch := patch.Patch{
		Id:          mgobson.ObjectIdHex(childPatchId),
		Description: "test",
		Project:     "mci",
		Tasks:       []string{"child_task_1", "child_task_2"},
		Activated:   true,
	}
	assert.NoError(childPatch.Insert())

	a := APIPatch{}
	err := a.BuildFromService(p)
	assert.NoError(err)
	assert.Equal(*a.DownstreamTasks[0].Project, childPatch.Project)
	assert.Len(a.DownstreamTasks, 1)
}
