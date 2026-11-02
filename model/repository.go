package model

import (
	"context"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// Repository has 1 document per project and it tracks the last revision
// that Evergreen ingested for the project.
type Repository struct {
	Project             string         `bson:"_id"`
	LastRevision        string         `bson:"last_revision"`
	RevisionOrderNumber int            `bson:"last_commit_number"`
	WaterfallHead       *WaterfallHead `bson:"waterfall_head,omitempty"`
}

// WaterfallHead is a snapshot of the latest version in a project's waterfall
// chain, which is made up of its mainline and push-triggered versions. Each new
// version in the chain carries its module commits forward from the head, and
// replaces it when it gets its revision order number, so the versions in the
// chain never move a module backwards.
type WaterfallHead struct {
	// ID identifies the snapshot. It's empty if no version has replaced the
	// head yet.
	ID        string                      `bson:"id"`
	VersionID string                      `bson:"version_id"`
	Revision  string                      `bson:"revision"`
	Modules   map[string]*manifest.Module `bson:"modules,omitempty"`
}

// ErrWaterfallHeadMoved indicates that another version replaced the project's
// waterfall head while a version was being created from it.
var ErrWaterfallHeadMoved = errors.New("another version replaced the waterfall head")

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
	RepoWaterfallHeadKey     = bsonutil.MustHaveTag(Repository{}, "WaterfallHead")
	WaterfallHeadIDKey       = bsonutil.MustHaveTag(WaterfallHead{}, "ID")

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
// This function is idempotent with regard to storing the same revision multiple times. It is safe to call this function multiple times with the same revision.
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

// FindWaterfallHead returns the project's waterfall head. Until a version
// replaces it, the head starts from the latest mainline or trigger version's
// manifest and the latest mainline version's revision.
func FindWaterfallHead(ctx context.Context, projectID string) (*WaterfallHead, error) {
	repo, err := FindRepository(ctx, projectID)
	if err != nil {
		return nil, errors.Wrapf(err, "finding repository for project '%s'", projectID)
	}
	if repo != nil && repo.WaterfallHead != nil {
		return repo.WaterfallHead, nil
	}

	head := &WaterfallHead{}
	mfst, err := findPreviousWaterfallManifest(ctx, projectID, 0)
	if err != nil {
		return nil, errors.Wrap(err, "finding latest waterfall manifest")
	}
	if mfst != nil {
		head.VersionID = mfst.Id
		head.Modules = mfst.Modules
	}
	mainline, err := VersionFindOne(ctx, db.Query(byLatestProjectVersion(projectID)).
		Sort([]string{"-" + VersionRevisionOrderNumberKey}).WithFields(VersionRevisionKey))
	if err != nil {
		return nil, errors.Wrap(err, "finding latest mainline version")
	}
	if mainline != nil {
		head.Revision = mainline.Revision
	}
	return head, nil
}

// claimWaterfallHead gets a new revision order number for the project and makes
// next its waterfall head, as long as the head is still the one with the ID
// previousHeadID. It returns ErrWaterfallHeadMoved if another version replaced
// the head first.
func claimWaterfallHead(ctx context.Context, projectID, previousHeadID string, next WaterfallHead) (int, error) {
	q := bson.M{RepoProjectKey: projectID}
	if previousHeadID == "" {
		q[RepoWaterfallHeadKey] = bson.M{"$exists": false}
	} else {
		q[bsonutil.GetDottedKeyName(RepoWaterfallHeadKey, WaterfallHeadIDKey)] = previousHeadID
	}
	repo := &Repository{}
	_, err := db.FindAndModify(ctx, RepositoriesCollection, q, nil, adb.Change{
		Update: bson.M{
			"$inc": bson.M{RepositoryOrderNumberKey: 1},
			"$set": bson.M{RepoWaterfallHeadKey: next},
		},
		// Only a project without a head can be missing its repository
		// document. If the document exists, the upsert fails on its ID.
		Upsert:    previousHeadID == "",
		ReturnNew: true,
	}, repo)
	if adb.ResultsNotFound(err) || db.IsDuplicateKey(err) {
		return 0, ErrWaterfallHeadMoved
	}
	if err != nil {
		return 0, errors.Wrap(err, "claiming waterfall head")
	}
	return repo.RevisionOrderNumber, nil
}
