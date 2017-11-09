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
	IntentCollection = "patch_intent"

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

// GithubIntent represents an intent to create a patch build as a result of a
// PullRequestEvent webhook. These intents are processed asynchronously by an
// amboy queue.
type GithubIntent struct {
	// ID is created by the driver and has no special meaning to the application.
	ID bson.ObjectId `bson:"_id"`

	// PRNumber is the PR number for the project in GitHub.
	PRNumber int `bson:"pr_number"`

	// HeadSHA is the base SHA of the patch.
	HeadSHA string `bson:"head_sha"`

	// URL is the URL of the patch in GitHub.
	URL string `bson:"url"`

	// Processed indicates whether a patch intent has been processed by the amboy queue.
	Processed bool `bson:"processed"`

	// IntentType indicates the type of the patch intent, i.e., GithubIntentType
	IntentType string `bson:"intent_type"`
}

// BSON fields for the patches
var (
	idKey         = bsonutil.MustHaveTag(GithubIntent{}, "ID")
	prNumberKey   = bsonutil.MustHaveTag(GithubIntent{}, "PRNumber")
	headSHAKey    = bsonutil.MustHaveTag(GithubIntent{}, "HeadSHA")
	urlKey        = bsonutil.MustHaveTag(GithubIntent{}, "URL")
	processedKey  = bsonutil.MustHaveTag(GithubIntent{}, "Processed")
	intentTypeKey = bsonutil.MustHaveTag(GithubIntent{}, "IntentType")
)

// NewGithubIntent return a new github patch intent.
func NewGithubIntent(pr int, sha string, url string) (*GithubIntent, error) {
	g := &GithubIntent{}
	if pr == 0 {
		return g, errors.New("PR number must not be 0")
	}
	if len(sha) != 40 {
		return g, errors.New("Base SHA must be 40 characters long")
	}
	if !strings.HasPrefix(url, "http") {
		return g, errors.Errorf("URL does not appear valid (%s)", g.URL)
	}

	g.PRNumber = pr
	g.HeadSHA = sha
	g.URL = url
	g.IntentType = GithubIntentType
	g.ID = bson.NewObjectId()

	return g, nil
}

// SetProcessed should be called by an amboy queue after creating a patch from an intent.
func (g *GithubIntent) SetProcessed() error {
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
func (g *GithubIntent) IsProcessed() bool {
	return g.Processed
}

// GetType returns the patch intent, e.g., GithubIntentType.
func (g *GithubIntent) GetType() string {
	return g.IntentType
}

// Insert inserts a patch intent in the database.
func (g *GithubIntent) Insert() error {
	return db.Insert(IntentCollection, g)
}

// FindUnprocessedGithubIntents finds all patch intents that have not yet been processed.
func FindUnprocessedGithubIntents() ([]GithubIntent, error) {
	var intents []GithubIntent
	err := db.FindAllQ(IntentCollection, db.Query(bson.M{processedKey: false}), &intents)
	if err != nil {
		return []GithubIntent{}, err
	}
	return intents, nil
}
