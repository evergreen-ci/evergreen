package model

import (
	"math"
	"net/url"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func TestFindOneProjectRef(t *testing.T) {
	assert := assert.New(t)
	require.NoError(t, db.Clear(ProjectRefCollection),
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

func TestProjectRefHTTPLocation(t *testing.T) {
	assert := assert.New(t)

	projectRef := &ProjectRef{
		Owner: "mongodb",
		Repo:  "mci",
	}

	urlStr, err := projectRef.HTTPLocation()
	assert.NoError(err)
	parsedURL, err := url.Parse(urlStr)
	assert.NoError(err)
	assert.NotNil(parsedURL)
	assert.Equal("https", parsedURL.Scheme)
	assert.Equal("github.com", parsedURL.Host)
	assert.Equal("/mongodb/mci.git", parsedURL.Path)
	assert.Nil(parsedURL.User)

	projectRef.Owner = ""
	urlStr, err = projectRef.HTTPLocation()
	assert.Error(err)
	assert.Empty(urlStr)

	projectRef.Owner = "mongodb"
	projectRef.Repo = ""
	urlStr, err = projectRef.HTTPLocation()
	assert.Error(err)
	assert.Empty(urlStr)
}

func TestProjectRefLocation(t *testing.T) {
	assert := assert.New(t)

	projectRef := &ProjectRef{
		Owner: "mongodb",
		Repo:  "mci",
	}

	location, err := projectRef.Location()
	assert.NoError(err)
	assert.NotEmpty(location)
	assert.Equal("git@github.com:mongodb/mci.git", location)

	projectRef.Owner = ""
	location, err = projectRef.Location()
	assert.Error(err)
	assert.Empty(location)

	projectRef.Owner = "mongodb"
	projectRef.Repo = ""
	location, err = projectRef.Location()
	assert.Error(err)
	assert.Empty(location)
}

func TestFindProjectRefsByRepoAndBranch(t *testing.T) {
	assert := assert.New(t)

	assert.NoError(db.Clear(ProjectRefCollection))

	projectRefs, err := FindProjectRefsByRepoAndBranch("mongodb", "mci", "master")
	assert.NoError(err)
	assert.Empty(projectRefs)

	projectRef := &ProjectRef{
		Owner:            "mongodb",
		Repo:             "mci",
		Branch:           "master",
		RepoKind:         "github",
		Enabled:          false,
		BatchTime:        10,
		Identifier:       "iden_",
		PRTestingEnabled: true,
	}
	assert.NoError(projectRef.Insert())
	projectRefs, err = FindProjectRefsByRepoAndBranch("mongodb", "mci", "master")
	assert.NoError(err)
	assert.Empty(projectRefs)

	projectRef.Identifier = "ident"
	projectRef.Enabled = true
	assert.NoError(projectRef.Insert())

	projectRefs, err = FindProjectRefsByRepoAndBranch("mongodb", "mci", "master")
	assert.NoError(err)
	assert.Len(projectRefs, 1)

	projectRef.Identifier = "ident2"
	assert.NoError(projectRef.Insert())
	projectRefs, err = FindProjectRefsByRepoAndBranch("mongodb", "mci", "master")
	assert.NoError(err)
	assert.Len(projectRefs, 2)
}

func TestFindOneProjectRefByRepoAndBranchWithPRTesting(t *testing.T) {
	assert := assert.New(t)   //nolint
	require := require.New(t) //nolint

	require.NoError(db.Clear(ProjectRefCollection))

	projectRef, err := FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "master")
	assert.NoError(err)
	assert.Nil(projectRef)

	doc := &ProjectRef{
		Owner:            "mongodb",
		Repo:             "mci",
		Branch:           "master",
		RepoKind:         "github",
		Enabled:          false,
		BatchTime:        10,
		Identifier:       "ident0",
		PRTestingEnabled: false,
	}
	require.NoError(doc.Insert())

	// 1 disabled document = no match
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "master")
	assert.NoError(err)
	assert.Nil(projectRef)

	// 2 docs, 1 enabled, but the enabled one has pr testing disabled = no match
	doc.Identifier = "ident_"
	doc.PRTestingEnabled = false
	doc.Enabled = true
	require.NoError(doc.Insert())
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "master")
	assert.NoError(err)
	require.Nil(projectRef)

	// 3 docs, 2 enabled, but only 1 has pr testing enabled = match
	doc.Identifier = "ident1"
	doc.PRTestingEnabled = true
	require.NoError(doc.Insert())
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "master")
	assert.NoError(err)
	require.NotNil(projectRef)
	assert.Equal("ident1", projectRef.Identifier)

	// 2 matching documents, error!
	doc.Identifier = "ident2"
	require.NoError(doc.Insert())
	projectRef, err = FindOneProjectRefByRepoAndBranchWithPRTesting("mongodb", "mci", "master")
	assert.Error(err)
	assert.Contains(err.Error(), "found 2 project refs, when 1 was expected")
	require.Nil(projectRef)
}

func TestFindOneProjectRefWithCommitQueueByOwnerRepoAndBranch(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	require.NoError(db.Clear(ProjectRefCollection))

	projectRef, err := FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch("mongodb", "mci", "master")
	assert.Error(err)
	assert.Nil(projectRef)

	doc := &ProjectRef{
		Owner:      "mongodb",
		Repo:       "mci",
		Branch:     "master",
		RepoKind:   "github",
		Identifier: "mci",
		CommitQueue: CommitQueueParams{
			Enabled: false,
		},
	}
	require.NoError(doc.Insert())

	projectRef, err = FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch("mongodb", "mci", "master")
	assert.Error(err)
	assert.Nil(projectRef)

	doc.CommitQueue.Enabled = true
	require.NoError(db.Update(ProjectRefCollection, bson.M{ProjectRefIdentifierKey: "mci"}, doc))

	projectRef, err = FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch("mongodb", "mci", "master")
	assert.NoError(err)
	assert.NotNil(projectRef)
}

func TestFindProjectRefsWithCommitQueueEnabled(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	require.NoError(db.Clear(ProjectRefCollection))
	doc := &ProjectRef{
		Enabled:    true,
		Owner:      "mongodb",
		Repo:       "mci",
		Branch:     "master",
		RepoKind:   "github",
		Identifier: "mci",
		CommitQueue: CommitQueueParams{
			Enabled: true,
		},
	}
	require.NoError(doc.Insert())

	doc.Branch = "fix"
	require.NoError(doc.Insert())

	doc.Identifier = "grip"
	doc.Repo = "grip"
	doc.CommitQueue.Enabled = false
	require.NoError(doc.Insert())

	projectRefs, err := FindProjectRefsWithCommitQueueEnabled()
	assert.NoError(err)
	require.Len(projectRefs, 2)
	assert.Equal("mci", projectRefs[0].Identifier)
	assert.Equal("mci", projectRefs[1].Identifier)
}
