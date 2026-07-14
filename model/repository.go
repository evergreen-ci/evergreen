package model

import (
	"context"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"go.mongodb.org/mongo-driver/bson"
)

// Repository has 1 document per project and it tracks the last revision
// that Evergreen ingested for the project.
type Repository struct {
	Project             string `bson:"_id"`
	LastRevision        string `bson:"last_revision"`
	RevisionOrderNumber int    `bson:"last_commit_number"`
}

// RepositoryRevision records when Evergreen ingested a project revision.
type RepositoryRevision struct {
	Owner    string `bson:"owner"`
	Repo     string `bson:"repo"`
	Branch   string `bson:"branch"`
	Revision string `bson:"revision"`

	IngestTime time.Time `bson:"ingest_time"`
}

var (
	// BSON fields for the Repository struct
	RepoProjectKey           = bsonutil.MustHaveTag(Repository{}, "Project")
	RepoLastRevisionKey      = bsonutil.MustHaveTag(Repository{}, "LastRevision")
	RepositoryOrderNumberKey = bsonutil.MustHaveTag(Repository{}, "RevisionOrderNumber")

	RepositoryRevisionOwnerKey      = bsonutil.MustHaveTag(RepositoryRevision{}, "Owner")
	RepositoryRevisionRepoKey       = bsonutil.MustHaveTag(RepositoryRevision{}, "Repo")
	RepositoryRevisionBranchKey     = bsonutil.MustHaveTag(RepositoryRevision{}, "Branch")
	RepositoryRevisionRevisionKey   = bsonutil.MustHaveTag(RepositoryRevision{}, "Revision")
	RepositoryRevisionIngestTimeKey = bsonutil.MustHaveTag(RepositoryRevision{}, "IngestTime")
)

const (
	RepositoriesCollection               = "repo_revisions"
	RepositoryRevisionsHistoryCollection = "repo_revision_history"
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
	Tag    string `bson:"tag"`
	Pusher string `bson:"pusher"`
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
func FindRepository(ctx context.Context, projectId string) (*Repository, error) {
	repository := &Repository{}
	q := db.Query(bson.M{RepoProjectKey: projectId})
	err := db.FindOneQ(ctx, RepositoriesCollection, q, repository)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return repository, err
}

// UpdateLastRevision updates the last created revision of a project.
func UpdateLastRevision(ctx context.Context, projectId, revision string) error {
	return db.Update(
		ctx,
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

// UpsertRepositoryRevision records that Evergreen ingested a project revision.
func UpsertRepositoryRevision(ctx context.Context, owner, repo, branch, revision string, ingestTime time.Time) error {
	if utility.IsZeroTime(ingestTime) {
		ingestTime = time.Now()
	}
	_, err := db.Upsert(
		ctx,
		RepositoryRevisionsHistoryCollection,
		bson.M{
			RepositoryRevisionOwnerKey:    owner,
			RepositoryRevisionRepoKey:     repo,
			RepositoryRevisionBranchKey:   branch,
			RepositoryRevisionRevisionKey: revision,
		},
		bson.M{
			"$setOnInsert": bson.M{
				RepositoryRevisionOwnerKey:      owner,
				RepositoryRevisionRepoKey:       repo,
				RepositoryRevisionBranchKey:     branch,
				RepositoryRevisionRevisionKey:   revision,
				RepositoryRevisionIngestTimeKey: ingestTime,
			},
		},
	)
	return err
}

// FindLatestRepositoryRevisionByIngestTime finds the latest project revision Evergreen ingested at or before ingestTime.
func FindLatestRepositoryRevisionByIngestTime(ctx context.Context, owner, repo, branch string, ingestTime time.Time) (*RepositoryRevision, error) {
	revision := &RepositoryRevision{}
	q := db.Query(bson.M{
		RepositoryRevisionOwnerKey:  owner,
		RepositoryRevisionRepoKey:   repo,
		RepositoryRevisionBranchKey: branch,
		RepositoryRevisionIngestTimeKey: bson.M{
			"$lte": ingestTime,
		},
	}).Sort([]string{"-" + RepositoryRevisionIngestTimeKey})
	err := db.FindOneQ(ctx, RepositoryRevisionsHistoryCollection, q, revision)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return revision, err
}

// GetNewRevisionOrderNumber gets a new revision order number for a project.
func GetNewRevisionOrderNumber(ctx context.Context, projectId string) (int, error) {
	repo := &Repository{}
	_, err := db.FindAndModify(ctx,
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
