package patch

import (
	"fmt"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/google/go-github/github"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

const (
	// IntentCollection is the database collection that stores patch intents.
	IntentCollection = "patch_intents"

	// GithubIntentType represents patch intents created for GitHub.
	GithubIntentType = "github"

	// GithubAlias is a special alias to specify default variants and tasks for GitHub pull requests.
	GithubAlias = "__github"
)

// Intent represents an intent to create a patch build and is processed by an amboy queue.
type Intent interface {
	// ID returns an identifier such that the tuple
	// (intent type, ID()) is unique in the collection.
	ID() string

	// Insert inserts a patch intent in the database.
	Insert() error

	// SetProcessed should be called by an amboy queue after creating a patch from an intent.
	SetProcessed() error

	// IsProcessed returns whether a patch exists for this intent.
	IsProcessed() bool

	// GetType returns the patch intent, e.g., GithubIntentType.
	GetType() string

	// NewPatch creates a patch from the intent
	NewPatch() *Patch

	// Finalize indicates whether or not the patch created from this
	// intent should be finalized
	ShouldFinalizePatch() bool

	// GetAlias defines the variants and tasks this intent should run on.
	GetAlias() string

	// RequesterIdentity supplies a valid requester type, that is recorded
	// in patches, versions, builds, and tasks to denote the origin of the
	// patch
	RequesterIdentity() string
}

// githubIntent represents an intent to create a patch build as a result of a
// PullRequestEvent webhook. These intents are processed asynchronously by an
// amboy queue.
type githubIntent struct {
	// ID is created by the driver and has no special meaning to the application.
	DocumentID bson.ObjectId `bson:"_id"`

	// MsgId is the unique message id as provided by Github (X-Github-Delivery)
	MsgID string `bson:"msg_id"`

	// BaseRepoName is the full repository name, ex: mongodb/mongo, that
	// this PR will be merged into
	BaseRepoName string `bson:"base_repo_name"`

	// BaseBranch is the branch that this pull request was opened against
	BaseBranch string `bson:"base_branch"`

	// HeadRepoName is the full repository name that contains the changes
	// to be merged
	HeadRepoName string `bson:"head_repo_name"`

	// PRNumber is the pull request number in GitHub.
	PRNumber int `bson:"pr_number"`

	// User is the login username of the Github user that created the pull request
	User string `bson:"user"`

	// HeadHash is the head hash of the diff, i.e. hash of the most recent
	// commit.
	HeadHash string `bson:"head_hash"`

	// Title is the title of the Github PR
	Title string `bson:"Title"`

	// CreatedAt is the time that this intent was stored in the database
	CreatedAt time.Time `bson:"created_at"`

	// Processed indicates whether a patch intent has been processed by the amboy queue.
	Processed bool `bson:"processed"`

	// ProcessedAt is the time that this intent was processed
	ProcessedAt time.Time `bson:"processed_at"`

	// IntentType indicates the type of the patch intent, e.g. GithubIntentType
	IntentType string `bson:"intent_type"`
}

// BSON fields for the patches
// nolint
var (
	documentIDKey   = bsonutil.MustHaveTag(githubIntent{}, "DocumentID")
	msgIDKey        = bsonutil.MustHaveTag(githubIntent{}, "MsgID")
	createdAtKey    = bsonutil.MustHaveTag(githubIntent{}, "CreatedAt")
	baseRepoNameKey = bsonutil.MustHaveTag(githubIntent{}, "BaseRepoName")
	baseBranchKey   = bsonutil.MustHaveTag(githubIntent{}, "BaseBranch")
	headRepoNameKey = bsonutil.MustHaveTag(githubIntent{}, "HeadRepoName")
	prNumberKey     = bsonutil.MustHaveTag(githubIntent{}, "PRNumber")
	userKey         = bsonutil.MustHaveTag(githubIntent{}, "User")
	headHashKey     = bsonutil.MustHaveTag(githubIntent{}, "HeadHash")
	processedKey    = bsonutil.MustHaveTag(githubIntent{}, "Processed")
	processedAtKey  = bsonutil.MustHaveTag(githubIntent{}, "ProcessedAt")
	intentTypeKey   = bsonutil.MustHaveTag(githubIntent{}, "IntentType")
)

// NewGithubIntent creates an Intent from a google/go-github PullRequestEvent,
// or returns an error if the some part of the struct is invalid
func NewGithubIntent(msgDeliveryID string, event *github.PullRequestEvent) (Intent, error) {
	if event.Action == nil || event.Number == nil ||
		event.Repo == nil || event.Repo.FullName == nil ||
		event.Sender == nil || event.Sender.Login == nil ||
		event.PullRequest == nil ||
		event.PullRequest.Head == nil || event.PullRequest.Head.SHA == nil ||
		event.PullRequest.Head.Repo == nil || event.PullRequest.Head.Repo.FullName == nil ||
		event.PullRequest.Title == nil || event.PullRequest.Base == nil ||
		event.PullRequest.Base.Ref == nil {
		return nil, errors.New("pull request document is malformed/missing data")
	}
	if msgDeliveryID == "" {
		return nil, errors.New("Unique msg id cannot be empty")
	}
	if len(strings.Split(*event.Repo.FullName, "/")) != 2 {
		return nil, errors.New("Base repo name is invalid (expected [owner]/[repo])")
	}
	if len(strings.Split(*event.PullRequest.Head.Repo.FullName, "/")) != 2 {
		return nil, errors.New("Head repo name is invalid (expected [owner]/[repo])")
	}
	if *event.PullRequest.Base.Ref == "" {
		return nil, errors.New("Base ref is empty")
	}
	if *event.Number == 0 {
		return nil, errors.New("PR number must not be 0")
	}
	if *event.Sender.Login == "" {
		return nil, errors.New("Github *event.Sender.Login name must not be empty string")
	}
	if len(*event.PullRequest.Head.SHA) == 0 {
		return nil, errors.New("Head hash must not be empty")
	}

	return &githubIntent{
		DocumentID:   bson.NewObjectId(),
		MsgID:        msgDeliveryID,
		BaseRepoName: *event.Repo.FullName,
		BaseBranch:   *event.PullRequest.Base.Ref,
		HeadRepoName: *event.PullRequest.Head.Repo.FullName,
		PRNumber:     *event.Number,
		User:         *event.Sender.Login,
		HeadHash:     *event.PullRequest.Head.SHA,
		Title:        *event.PullRequest.Title,
		IntentType:   GithubIntentType,
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

func (g *githubIntent) RequesterIdentity() string {
	return evergreen.GithubPRRequester
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
	baseRepo := strings.Split(g.BaseRepoName, "/")
	headRepo := strings.Split(g.HeadRepoName, "/")
	pullURL := fmt.Sprintf("https://github.com/%s/pull/%d", g.BaseRepoName, g.PRNumber)
	patchDoc := &Patch{
		Id:          bson.NewObjectId(),
		Alias:       GithubAlias,
		Description: fmt.Sprintf("'%s' pull request #%d by %s: %s (%s)", g.BaseRepoName, g.PRNumber, g.User, g.Title, pullURL),
		Author:      evergreen.GithubPatchUser,
		Status:      evergreen.PatchCreated,
		GithubPatchData: GithubPatch{
			PRNumber:   g.PRNumber,
			BaseOwner:  baseRepo[0],
			BaseRepo:   baseRepo[1],
			BaseBranch: g.BaseBranch,
			HeadOwner:  headRepo[0],
			HeadRepo:   headRepo[1],
			HeadHash:   g.HeadHash,
			Author:     g.User,
		},
	}
	return patchDoc
}

func (g *githubIntent) GetAlias() string {
	return GithubAlias
}
