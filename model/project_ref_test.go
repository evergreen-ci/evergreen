package model

import (
	"math"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
)

func TestFindOneProjectRef(t *testing.T) {
	assert := assert.New(t)
	testutil.HandleTestingErr(db.Clear(ProjectRefCollection), t,
		"Error clearing collection")
	projectRef := &ProjectRef{
		Owner:      "mongodb",
		Repo:       "mci",
		Branch:     "master",
		RepoKind:   "github",
		Enabled:    true,
		BatchTime:  10,
		Identifier: "ident",
	}
	assert.Nil(projectRef.Insert())

	projectRefFromDB, err := FindOneProjectRef("ident")
	assert.Nil(err)
	assert.NotNil(projectRefFromDB)

	assert.Equal(projectRef.Owner, "mongodb")
	assert.Equal(projectRef.Repo, "mci")
	assert.Equal(projectRef.Branch, "master")
	assert.Equal(projectRef.RepoKind, "github")
	assert.Equal(projectRef.Enabled, true)
	assert.Equal(projectRef.BatchTime, 10)
	assert.Equal(projectRef.Identifier, "ident")
}

func TestGetBatchTimeDoesNotExceedMaxInt32(t *testing.T) {
	assert := assert.New(t)

	projectRef := &ProjectRef{
		Owner:      "mongodb",
		Repo:       "mci",
		Branch:     "master",
		RepoKind:   "github",
		Enabled:    true,
		BatchTime:  math.MaxInt64,
		Identifier: "ident",
	}

	emptyVariant := &BuildVariant{}

	assert.Equal(projectRef.GetBatchTime(emptyVariant), math.MaxInt32,
		"ProjectRef.GetBatchTime() is not capping BatchTime to MaxInt32")

	projectRef.BatchTime = 55
	assert.Equal(projectRef.GetBatchTime(emptyVariant), 55,
		"ProjectRef.GetBatchTime() is not returning the correct BatchTime")

}
