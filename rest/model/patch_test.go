package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/stretchr/testify/assert"
	"gopkg.in/mgo.v2/bson"
)

func TestAPIPatch(t *testing.T) {
	assert := assert.New(t)
	baseTime := time.Now()
	p := patch.Patch{
		Id:            bson.NewObjectId(),
		Description:   "test",
		Project:       "mci",
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
		GithubPatchData: patch.GithubPatch{
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

	assert.Equal(p.Id.Hex(), FromApiString(a.Id))
	assert.Equal(p.Description, FromApiString(a.Description))
	assert.Equal(p.Project, FromApiString(a.ProjectId))
	assert.Equal(p.Project, FromApiString(a.Branch))
	assert.Equal(p.Githash, FromApiString(a.Githash))
	assert.Equal(p.PatchNumber, a.PatchNumber)
	assert.Equal(p.Author, FromApiString(a.Author))
	assert.Equal(p.Version, FromApiString(a.Version))
	assert.Equal(p.Status, FromApiString(a.Status))
	assert.Zero(time.Time(a.CreateTime).Sub(p.CreateTime))
	assert.Equal(-time.Hour, time.Time(a.StartTime).Sub(p.StartTime))
	assert.Equal(-2*time.Hour, time.Time(a.FinishTime).Sub(p.FinishTime))
	for i, variant := range a.Variants {
		assert.Equal(p.BuildVariants[i], FromApiString(variant))
	}
	for i, task := range a.Tasks {
		assert.Equal(p.Tasks[i], FromApiString(task))
	}
	for i, vt := range a.VariantsTasks {
		assert.Equal(p.VariantsTasks[i].Variant, FromApiString(vt.Name))

		for i2, task := range vt.Tasks {
			assert.Equal(a.Tasks[i2], task)
		}
	}
	assert.Equal("__github", FromApiString(a.Alias))
	assert.NotZero(a.GithubPatchData)
}

func TestGithubPatch(t *testing.T) {
	assert := assert.New(t)
	p := patch.GithubPatch{
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
	assert.Equal("evergreen-ci", FromApiString(a.BaseOwner))
	assert.Equal("evergreen", FromApiString(a.BaseRepo))
	assert.Equal("octocat", FromApiString(a.HeadOwner))
	assert.Equal("evergreen", FromApiString(a.HeadRepo))
	assert.Equal("hash", FromApiString(a.HeadHash))
	assert.Equal("octocat", FromApiString(a.Author))
}
