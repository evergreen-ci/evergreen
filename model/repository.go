package model

import (
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

// Repository contains fields used to track projects.
type Repository struct {
	Project             string `bson:"_id"`
	LastRevision        string `bson:"last_revision"`
	RevisionOrderNumber int    `bson:"last_commit_number"`
}

var (
	// BSON fields for the Repository struct
	RepoProjectKey = bsonutil.MustHaveTag(Repository{},
		"Project")
	RepoLastRevisionKey = bsonutil.MustHaveTag(Repository{},
		"LastRevision")
	RepositoryOrderNumberKey = bsonutil.MustHaveTag(Repository{},
		"RevisionOrderNumber")
)

const (
	RepositoriesCollection = "repo_revisions"
)

const (
	GithubRepoType = "github"
)

// valid repositories - currently only github supported
var (
	ValidRepoTypes = []string{GithubRepoType}
)

type Revision struct {
	Author          string
	AuthorEmail     string
	RevisionMessage string
	Revision        string
	CreateTime      time.Time
}

// FindRepository gets the repository object of a project.
func FindRepository(projectId string) (*Repository, error) {
	repository := &Repository{}
	err := db.FindOne(
		RepositoriesCollection,
		bson.M{
			RepoProjectKey: projectId,
		},
		db.NoProjection,
		db.NoSort,
		repository,
	)
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	return repository, err
}

// UpdateLastRevision updates the last created revision of a project.
func UpdateLastRevision(projectId, revision string) error {
	return db.Update(
		RepositoriesCollection,
		bson.M{
			RepoProjectKey: projectId,
		},
		bson.M{
			"$set": bson.M{
				RepoLastRevisionKey: revision,
			},
		},
	)
}

// GetNewRevisionOrderNumber gets a new revision order number for a project.
func GetNewRevisionOrderNumber(projectId string) (int, error) {
	repo := &Repository{}
	_, err := db.FindAndModify(
		RepositoriesCollection,
		bson.M{
			RepoProjectKey: projectId,
		},
		nil,
		mgo.Change{
			Update: bson.M{
				"$inc": bson.M{
					RepositoryOrderNumberKey: 1,
				},
			},
			Upsert:    true,
			ReturnNew: true,
		},
		repo,
	)
	if err != nil {
		return 0, err
	}
	return repo.RevisionOrderNumber, nil
}
