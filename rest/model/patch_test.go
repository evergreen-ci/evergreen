package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/stretchr/testify/assert"
	mgobson "gopkg.in/mgo.v2/bson"
)

func TestAPIPatch(t *testing.T) {
	assert := assert.New(t)
	baseTime := time.Now()
	p := patch.Patch{
		Id:            mgobson.NewObjectId(),
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

	assert.Equal(p.Id.Hex(), FromAPIString(a.Id))
	assert.Equal(p.Description, FromAPIString(a.Description))
	assert.Equal(p.Project, FromAPIString(a.ProjectId))
	assert.Equal(p.Project, FromAPIString(a.Branch))
	assert.Equal(p.Githash, FromAPIString(a.Githash))
	assert.Equal(p.PatchNumber, a.PatchNumber)
	assert.Equal(p.Author, FromAPIString(a.Author))
	assert.Equal(p.Version, FromAPIString(a.Version))
	assert.Equal(p.Status, FromAPIString(a.Status))
	assert.Zero(time.Time(a.CreateTime).Sub(p.CreateTime))
	assert.Equal(-time.Hour, time.Time(a.CreateTime).Sub(p.StartTime))
	assert.Equal(-2*time.Hour, time.Time(a.CreateTime).Sub(p.FinishTime))
	for i, variant := range a.Variants {
		assert.Equal(p.BuildVariants[i], FromAPIString(variant))
	}
	for i, task := range a.Tasks {
		assert.Equal(p.Tasks[i], FromAPIString(task))
	}
	for i, vt := range a.VariantsTasks {
		assert.Equal(p.VariantsTasks[i].Variant, FromAPIString(vt.Name))

		for i2, task := range vt.Tasks {
			assert.Equal(a.Tasks[i2], task)
		}
	}
	assert.Equal("__github", FromAPIString(a.Alias))
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
	assert.Equal("evergreen-ci", FromAPIString(a.BaseOwner))
	assert.Equal("evergreen", FromAPIString(a.BaseRepo))
	assert.Equal("octocat", FromAPIString(a.HeadOwner))
	assert.Equal("evergreen", FromAPIString(a.HeadRepo))
	assert.Equal("hash", FromAPIString(a.HeadHash))
	assert.Equal("octocat", FromAPIString(a.Author))
}
