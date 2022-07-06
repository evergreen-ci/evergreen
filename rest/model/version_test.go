package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/utility"
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
	errors := []string{"made a mistake"}

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
	v := model.Version{
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
		Errors:        errors,
	}

	apiVersion := &APIVersion{}
	// BuildFromService should complete without error
	apiVersion.BuildFromService(v)
	// Each field should be as expected
	assert.Equal(apiVersion.Id, utility.ToStringPtr(versionId))
	assert.Equal(*apiVersion.CreateTime, time)
	assert.Equal(*apiVersion.StartTime, time)
	assert.Equal(*apiVersion.FinishTime, time)
	assert.Equal(apiVersion.Revision, utility.ToStringPtr(revision))
	assert.Equal(apiVersion.Author, utility.ToStringPtr(author))
	assert.Equal(apiVersion.AuthorEmail, utility.ToStringPtr(authorEmail))
	assert.Equal(apiVersion.Message, utility.ToStringPtr(msg))
	assert.Equal(apiVersion.Status, utility.ToStringPtr(status))
	assert.Equal(apiVersion.Repo, utility.ToStringPtr(repo))
	assert.Equal(apiVersion.Branch, utility.ToStringPtr(branch))
	assert.Equal(apiVersion.Errors, utility.ToStringPtrSlice(errors))

	bvs := apiVersion.BuildVariantStatus
	assert.Equal(bvs[0].BuildVariant, utility.ToStringPtr(bv1))
	assert.Equal(bvs[0].BuildId, utility.ToStringPtr(bi1))
	assert.Equal(bvs[1].BuildVariant, utility.ToStringPtr(bv2))
	assert.Equal(bvs[1].BuildId, utility.ToStringPtr(bi2))
}
