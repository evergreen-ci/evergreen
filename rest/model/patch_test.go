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
	assert := assert.New(t) //nolint
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
		GithubPatchData: patch.GithubPatch{
			PRNumber:  123,
			BaseOwner: "evergreen-ci",
			BaseRepo:  "evergreen",
			HeadOwner: "octocat",
			HeadRepo:  "evergreen",
			HeadHash:  "hash",
			Author:    "octocat",
			DiffURL:   "https://github.com/evergreen-ci/evergreen/pull/123.diff",
		},
	}

	a := APIPatch{}
	err := a.BuildFromService(p)
	assert.NoError(err)

	assert.Equal(p.Id.Hex(), string(a.Id))
	assert.Equal(p.Description, string(a.Description))
	assert.Equal(p.Project, string(a.ProjectId))
	assert.Equal(p.Project, string(a.Branch))
	assert.Equal(p.Githash, string(a.Githash))
	assert.Equal(p.PatchNumber, a.PatchNumber)
	assert.Equal(p.Author, string(a.Author))
	assert.Equal(p.Version, string(a.Version))
	assert.Equal(p.Status, string(a.Status))
	assert.Zero(time.Time(a.CreateTime).Sub(p.CreateTime))
	assert.Equal(-time.Hour, time.Time(a.StartTime).Sub(p.StartTime))
	assert.Equal(-2*time.Hour, time.Time(a.FinishTime).Sub(p.FinishTime))
	for i, variant := range a.Variants {
		assert.Equal(p.BuildVariants[i], string(variant))
	}
	for i, task := range a.Tasks {
		assert.Equal(p.Tasks[i], string(task))
	}
	for i, vt := range a.VariantsTasks {
		assert.Equal(p.VariantsTasks[i].Variant, string(vt.Name))

		for i2, task := range vt.Tasks {
			assert.Equal(a.Tasks[i2], task)
		}
	}
	assert.NotZero(a.GithubPatchData)
}

func TestGithubPatch(t *testing.T) {
	assert := assert.New(t) //nolint
	p := patch.GithubPatch{
		PRNumber:  123,
		BaseOwner: "evergreen-ci",
		BaseRepo:  "evergreen",
		HeadOwner: "octocat",
		HeadRepo:  "evergreen",
		HeadHash:  "hash",
		Author:    "octocat",
		DiffURL:   "https://github.com/evergreen-ci/evergreen/pull/123.diff",
	}
	a := githubPatch{}
	err := a.BuildFromService(p)
	assert.NoError(err)
	assert.Equal(123, a.PRNumber)
	assert.Equal("evergreen-ci", string(a.BaseOwner))
	assert.Equal("evergreen", string(a.BaseRepo))
	assert.Equal("octocat", string(a.HeadOwner))
	assert.Equal("evergreen", string(a.HeadRepo))
	assert.Equal("hash", string(a.HeadHash))
	assert.Equal("octocat", string(a.Author))
	assert.Equal("https://github.com/evergreen-ci/evergreen/pull/123.diff", string(a.DiffURL))
}
