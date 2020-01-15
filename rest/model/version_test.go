package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/stretchr/testify/assert"
)

// TestVersionBuildFromService tests that BuildFromService function completes
// correctly and without error.
func TestVersionBuildFromService(t *testing.T) {
	assert := assert.New(t)

	time := time.Now()
	versionId := "versionId"
	revision := "revision"
	author := "author"
	authorEmail := "author_email"
	msg := "message"
	status := "status"
	repo := "repo"
	branch := "branch"

	bv1 := "buildvariant1"
	bv2 := "buildvariant2"
	bi1 := "buildId1"
	bi2 := "buildId2"

	buildVariants := []model.VersionBuildStatus{
		{
			BuildVariant: bv1,
			BuildId:      bi1,
		},
		{
			BuildVariant: bv2,
			BuildId:      bi2,
		},
	}
	v := &model.Version{
		Id:            versionId,
		CreateTime:    time,
		StartTime:     time,
		FinishTime:    time,
		Revision:      revision,
		Author:        author,
		AuthorEmail:   authorEmail,
		Message:       msg,
		Status:        status,
		Repo:          repo,
		Branch:        branch,
		BuildVariants: buildVariants,
	}

	apiVersion := &APIVersion{}
	// BuildFromService should complete without error
	err := apiVersion.BuildFromService(v)
	assert.Nil(err)
	// Each field should be as expected
	assert.Equal(apiVersion.Id, ToStringPtr(versionId))
	assert.Equal(apiVersion.CreateTime, NewTime(time))
	assert.Equal(apiVersion.StartTime, NewTime(time))
	assert.Equal(apiVersion.FinishTime, NewTime(time))
	assert.Equal(apiVersion.Revision, ToStringPtr(revision))
	assert.Equal(apiVersion.Author, ToStringPtr(author))
	assert.Equal(apiVersion.AuthorEmail, ToStringPtr(authorEmail))
	assert.Equal(apiVersion.Message, ToStringPtr(msg))
	assert.Equal(apiVersion.Status, ToStringPtr(status))
	assert.Equal(apiVersion.Repo, ToStringPtr(repo))
	assert.Equal(apiVersion.Branch, ToStringPtr(branch))

	bvs := apiVersion.BuildVariants
	assert.Equal(bvs[0].BuildVariant, ToStringPtr(bv1))
	assert.Equal(bvs[0].BuildId, ToStringPtr(bi1))
	assert.Equal(bvs[1].BuildVariant, ToStringPtr(bv2))
	assert.Equal(bvs[1].BuildId, ToStringPtr(bi2))
}

func TestVersionToService(t *testing.T) {
	assert := assert.New(t)
	apiVersion := &APIVersion{}
	v, err := apiVersion.ToService()
	assert.Nil(v)
	assert.Error(err)
}
