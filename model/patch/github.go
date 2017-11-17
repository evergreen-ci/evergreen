package patch

import (
	"strings"

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
}

// githubIntent represents an intent to create a patch build as a result of a
// PullRequestEvent webhook. These intents are processed asynchronously by an
// amboy queue.
type githubIntent struct {
	// ID is created by the driver and has no special meaning to the application.
	ID bson.ObjectId `bson:"_id"`

	// PRNumber is the PR number for the project in GitHub.
	PRNumber int `bson:"pr_number"`

	// HeadHash is the base SHA of the patch.
	HeadHash string `bson:"head_hash"`

	// URL is the URL of the patch in GitHub.
	URL string `bson:"url"`

	// Processed indicates whether a patch intent has been processed by the amboy queue.
	Processed bool `bson:"processed"`

	// IntentType indicates the type of the patch intent, i.e., GithubIntentType
	IntentType string `bson:"intent_type"`
}

// BSON fields for the patches
// nolint
var (
	idKey         = bsonutil.MustHaveTag(githubIntent{}, "ID")
	prNumberKey   = bsonutil.MustHaveTag(githubIntent{}, "PRNumber")
	headHashKey   = bsonutil.MustHaveTag(githubIntent{}, "HeadHash")
	urlKey        = bsonutil.MustHaveTag(githubIntent{}, "URL")
	processedKey  = bsonutil.MustHaveTag(githubIntent{}, "Processed")
	intentTypeKey = bsonutil.MustHaveTag(githubIntent{}, "IntentType")
)

// NewGithubIntent return a new github patch intent.
func NewGithubIntent(pr int, sha string, url string) (Intent, error) {
	g := &githubIntent{}
	if pr == 0 {
		return nil, errors.New("PR number must not be 0")
	}
	if len(sha) == 0 {
		return nil, errors.New("Base hash must not be empty")
	}
	if !strings.HasPrefix(url, "http") {
		return nil, errors.Errorf("URL does not appear valid (%s)", g.URL)
	}

	g.PRNumber = pr
	g.HeadHash = sha
	g.URL = url
	g.IntentType = GithubIntentType
	g.ID = bson.NewObjectId()

	return g, nil
}

// SetProcessed should be called by an amboy queue after creating a patch from an intent.
func (g *githubIntent) SetProcessed() error {
	g.Processed = true
	return updateOneIntent(
		bson.M{idKey: g.ID},
		bson.M{"$set": bson.M{processedKey: g.Processed}},
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
	return db.Insert(IntentCollection, g)
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
