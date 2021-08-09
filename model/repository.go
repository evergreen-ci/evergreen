package model

import (
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"go.mongodb.org/mongo-driver/bson"
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
	AuthorID        string
	AuthorGithubUID int
	AuthorEmail     string
	RevisionMessage string
	Revision        string
	CreateTime      time.Time
}

type GitTag struct {
	Tag    string
	Pusher string
}

type GitTags []GitTag

func (tags GitTags) String() string {
	tagNames := []string{}
	for _, t := range tags {
		tagNames = append(tagNames, t.Tag)
	}
	return strings.Join(tagNames, ", ")
}

// FindRepository gets the repository object of a project.
func FindRepository(projectId string) (*Repository, error) {
	repository := &Repository{}
	q := db.Query(bson.M{RepoProjectKey: projectId})
	err := db.FindOneQ(RepositoriesCollection, q, repository)
	if adb.ResultsNotFound(err) {
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
		adb.Change{
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
