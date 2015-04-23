package model

import (
	"10gen.com/mci/db"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"time"
)

type Repository struct {
	RepositoryProject   string `bson:"_id"`
	LastRevision        string `bson:"last_revision"`
	RevisionOrderNumber int    `bson:"last_commit_number"`
}

var (
	// bson fields for the Repository struct
	RepoProjectKey = MustHaveBsonTag(Repository{},
		"RepositoryProject")
	RepoLastRevisionKey = MustHaveBsonTag(Repository{},
		"LastRevision")
	RepositoryOrderNumberKey = MustHaveBsonTag(Repository{},
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

// Repository contains fields representing basic information
// that is expected from every repository
type Revision struct {
	Author          string
	AuthorEmail     string
	RevisionMessage string
	Revision        string
	CreateTime      time.Time
}

// FindRepository gets the repository identified by the passed in repositoryName
func FindRepository(repositoryName string) (*Repository, error) {
	repository := &Repository{}
	err := db.FindOne(
		RepositoriesCollection,
		bson.M{
			RepoProjectKey: repositoryName,
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

// GetNewRevisionOrderNumber gets a new commit order number for the specified
// repository
func GetNewRevisionOrderNumber(repository string) (int, error) {
	repo := &Repository{}
	_, err := db.FindAndModify(
		RepositoriesCollection,
		bson.M{
			RepoProjectKey: repository,
		},
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
