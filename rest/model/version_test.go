package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/model/version"
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

	buildVariants := []version.BuildStatus{
		{
			BuildVariant: bv1,
			BuildId:      bi1,
		},
		{
			BuildVariant: bv2,
			BuildId:      bi2,
		},
	}
	v := &version.Version{
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
	assert.Equal(apiVersion.Id, APIString(versionId))
	assert.Equal(apiVersion.CreateTime, APITime(time))
	assert.Equal(apiVersion.StartTime, APITime(time))
	assert.Equal(apiVersion.FinishTime, APITime(time))
	assert.Equal(apiVersion.Revision, APIString(revision))
	assert.Equal(apiVersion.Author, APIString(author))
	assert.Equal(apiVersion.AuthorEmail, APIString(authorEmail))
	assert.Equal(apiVersion.Message, APIString(msg))
	assert.Equal(apiVersion.Status, APIString(status))
	assert.Equal(apiVersion.Repo, APIString(repo))
	assert.Equal(apiVersion.Branch, APIString(branch))

	bvs := apiVersion.BuildVariants
	assert.Equal(bvs[0].BuildVariant, APIString(bv1))
	assert.Equal(bvs[0].BuildId, APIString(bi1))
	assert.Equal(bvs[1].BuildVariant, APIString(bv2))
	assert.Equal(bvs[1].BuildId, APIString(bi2))
}

func TestVersionToService(t *testing.T) {
	assert := assert.New(t)
	apiVersion := &APIVersion{}
	v, err := apiVersion.ToService()
	assert.Nil(v)
	assert.Error(err)
}
