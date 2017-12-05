package patch

import (
	"fmt"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

const (
	// IntentCollection is the database collection that stores patch intents.
	IntentCollection = "patch_intents"

	// GithubIntentType represents patch intents created for GitHub.
	GithubIntentType = "github"
)

// Intent represents an intent to create a patch build and is processed by an amboy queue.
type Intent interface {
	// Insert inserts a patch intent in the database.
	Insert() error

	// SetProcessed should be called by an amboy queue after creating a patch from an intent.
	SetProcessed() error

	// IsProcessed returns whether a patch exists for this intent.
	IsProcessed() bool

	// GetType returns the patch intent, e.g., GithubType.
	GetType() string

	// ID returns an identifier such that the tuple
	// (intent type, ID()) is unique in the collection.
	ID() string

	// NewPatch creates a patch from the intent
	NewPatch() *Patch

	// Finalize indicates whether or not the patch created from this
	// intent should be finalized
	ShouldFinalizePatch() bool
}

// githubIntent represents an intent to create a patch build as a result of a
// PullRequestEvent webhook. These intents are processed asynchronously by an
// amboy queue.
type githubIntent struct {
	// ID is created by the driver and has no special meaning to the application.
	DocumentID bson.ObjectId `bson:"_id"`

	// MsgId is the unique message id as provided by Github (X-Github-Delivery)
	MsgID string `bson:"msg_id"`

	// CreatedAt is the time that this intent was stored in the database
	CreatedAt time.Time `bson:"created_at"`

	// RepoName is the full repository name, ex: mongodb/mongo
	RepoName string `bson:"repo_name"`

	// PRNumber is the pull request number in GitHub.
	PRNumber int `bson:"pr_number"`

	// User is the login username of the Github user that created the pull request
	User string `bson:"user"`

	// BaseHash is the base hash of the patch.
	BaseHash string `bson:"base_hash"`

	// URL is the URL of the patch in GitHub.
	URL string `bson:"url"`

	// Processed indicates whether a patch intent has been processed by the amboy queue.
	Processed bool `bson:"processed"`

	// ProcessedAt is the time that this intent was processed
	ProcessedAt time.Time `bson:"processed_at"`

	// IntentType indicates the type of the patch intent, i.e., GithubIntentType
	IntentType string `bson:"intent_type"`
}

// BSON fields for the patches
// nolint
var (
	documentIDKey  = bsonutil.MustHaveTag(githubIntent{}, "DocumentID")
	msgIDKey       = bsonutil.MustHaveTag(githubIntent{}, "MsgID")
	createdAtKey   = bsonutil.MustHaveTag(githubIntent{}, "CreatedAt")
	repoNameKey    = bsonutil.MustHaveTag(githubIntent{}, "RepoName")
	prNumberKey    = bsonutil.MustHaveTag(githubIntent{}, "PRNumber")
	userKey        = bsonutil.MustHaveTag(githubIntent{}, "User")
	baseHashKey    = bsonutil.MustHaveTag(githubIntent{}, "BaseHash")
	urlKey         = bsonutil.MustHaveTag(githubIntent{}, "URL")
	processedKey   = bsonutil.MustHaveTag(githubIntent{}, "Processed")
	processedAtKey = bsonutil.MustHaveTag(githubIntent{}, "ProcessedAt")
	intentTypeKey  = bsonutil.MustHaveTag(githubIntent{}, "IntentType")
)

// NewGithubIntent return a new github patch intent.
func NewGithubIntent(msgDeliveryID, repoName string, prNumber int, user, baseHash, url string) (Intent, error) {
	if msgDeliveryID == "" {
		return nil, errors.New("Unique msg id cannot be empty")
	}
	if repoName == "" || len(strings.Split(repoName, "/")) != 2 {
		return nil, errors.New("Repo name is invalid")
	}
	if prNumber == 0 {
		return nil, errors.New("PR number must not be 0")
	}
	if user == "" {
		return nil, errors.New("Github user name must not be empty string")
	}
	if len(baseHash) == 0 {
		return nil, errors.New("Base hash must not be empty")
	}
	if !strings.HasPrefix(url, "http") {
		return nil, errors.Errorf("URL does not appear valid (%s)", url)
	}

	return &githubIntent{
		DocumentID: bson.NewObjectId(),
		MsgID:      msgDeliveryID,
		RepoName:   repoName,
		PRNumber:   prNumber,
		User:       user,
		BaseHash:   baseHash,
		URL:        url,
		IntentType: GithubIntentType,
	}, nil
}

// SetProcessed should be called by an amboy queue after creating a patch from an intent.
func (g *githubIntent) SetProcessed() error {
	g.Processed = true
	g.ProcessedAt = time.Now()
	return updateOneIntent(
		bson.M{documentIDKey: g.DocumentID},
		bson.M{"$set": bson.M{
			processedKey:   g.Processed,
			processedAtKey: g.ProcessedAt,
		}},
	)
}

// updateOne updates one patch intent.
func updateOneIntent(query interface{}, update interface{}) error {
	return db.Update(
		IntentCollection,
		query,
		update,
	)
}

// IsProcessed returns whether a patch exists for this intent.
func (g *githubIntent) IsProcessed() bool {
	return g.Processed
}

// GetType returns the patch intent, e.g., GithubIntentType.
func (g *githubIntent) GetType() string {
	return g.IntentType
}

// Insert inserts a patch intent in the database.
func (g *githubIntent) Insert() error {
	g.CreatedAt = time.Now()
	err := db.Insert(IntentCollection, g)
	if err != nil {
		g.CreatedAt = time.Time{}
		return err
	}

	return nil
}

func (g *githubIntent) ID() string {
	return g.MsgID
}

func (g *githubIntent) ShouldFinalizePatch() bool {
	return true
}

// FindUnprocessedGithubIntents finds all patch intents that have not yet been processed.
func FindUnprocessedGithubIntents() ([]*githubIntent, error) {
	var intents []*githubIntent
	err := db.FindAllQ(IntentCollection, db.Query(bson.M{processedKey: false, intentTypeKey: GithubIntentType}), &intents)
	if err != nil {
		return []*githubIntent{}, err
	}
	return intents, nil
}

func (g *githubIntent) NewPatch() *Patch {
	repo := strings.Split(g.RepoName, "/")
	patchDoc := &Patch{
		Id:          bson.NewObjectId(),
		Description: fmt.Sprintf("%s pull request #%d", g.RepoName, g.PRNumber),
		//Author:      "",
		Githash: g.BaseHash,
		Status:  evergreen.PatchCreated,
		GithubPatchData: GithubPatch{
			PRNumber:   g.PRNumber,
			Owner:      repo[0],
			Repository: repo[1],
			Author:     g.User,
		},
	}
	return patchDoc
}
