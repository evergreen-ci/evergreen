package patch

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/google/go-github/v70/github"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	// IntentCollection is the database collection that stores patch intents.
	IntentCollection = "patch_intents"

	// GithubIntentType represents patch intents created for GitHub.
	GithubIntentType = "github"
)

// CalledBy can be either auto or manual.
const (
	AutomatedCaller = "auto"
	ManualCaller    = "manual"
	AllCallers      = ""
)

// githubIntent represents an intent to create a patch build as a result of a
// PullRequestEvent webhook. These intents are processed asynchronously by an
// amboy queue.
type githubIntent struct {
	// TODO: migrate/remove all documents to use the MsgID as the _id

	// ID is created by the driver and has no special meaning to the application.
	DocumentID string `bson:"_id"`

	// MsgId is a GUID provided by Github (X-Github-Delivery) for the event.
	MsgID string `bson:"msg_id"`

	// BaseRepoName is the full repository name, ex: mongodb/mongo, that
	// this PR will be merged into
	BaseRepoName string `bson:"base_repo_name"`

	// BaseBranch is the branch that this pull request was opened against
	BaseBranch string `bson:"base_branch"`

	// HeadRepoName is the full repository name that contains the changes
	// to be merged
	HeadRepoName string `bson:"head_repo_name"`

	// HeadBranch is the branch name that contains the changes for this PR
	HeadBranch string `bson:"head_branch"`

	// PRNumber is the pull request number in GitHub.
	PRNumber int `bson:"pr_number"`

	// User is the login username of the Github user that created the pull request
	User string `bson:"user"`

	// UID is the PR author's Github UID
	UID int `bson:"author_uid"`

	// HeadHash is the head hash of the diff, i.e. hash of the most recent
	// commit.
	HeadHash string `bson:"head_hash"`

	// BaseHash is the base hash of the commit.
	BaseHash string `bson:"base_hash"`

	// MergeBase is merge base of the pull request, or the common ancestor commit
	// between the feature branch and the target branch.
	MergeBase string `bson:"merge_base"`

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

	// CalledBy indicates whether the intent was created automatically by Evergreen or by a user
	CalledBy string `bson:"called_by"`

	// RepeatPatchId uses the given patch to reuse the task/variant definitions
	RepeatPatchId string `bson:"repeat_patch_id"`

	// Alias defines the variants and tasks to run this patch on. It will default to __github if not set.
	Alias string `bson:"alias"`
}

// BSON fields for the patches
var (
	documentIDKey  = bsonutil.MustHaveTag(githubIntent{}, "DocumentID")
	headHashKey    = bsonutil.MustHaveTag(githubIntent{}, "HeadHash")
	processedKey   = bsonutil.MustHaveTag(githubIntent{}, "Processed")
	processedAtKey = bsonutil.MustHaveTag(githubIntent{}, "ProcessedAt")
	intentTypeKey  = bsonutil.MustHaveTag(githubIntent{}, "IntentType")
)

// NewGithubIntent creates an Intent from a google/go-github PullRequestEvent,
// or returns an error if the some part of the struct is invalid
func NewGithubIntent(ctx context.Context, msgDeliveryID, patchOwner, calledBy, alias, mergeBase string, pr *github.PullRequest) (Intent, error) {
	if pr == nil ||
		pr.Base == nil || pr.Base.Repo == nil ||
		pr.Head == nil || pr.Head.Repo == nil ||
		pr.User == nil {
		return nil, errors.New("incomplete PR")
	}
	if msgDeliveryID == "" {
		return nil, errors.New("unique msg ID cannot be empty")
	}
	if len(strings.Split(pr.Base.Repo.GetFullName(), "/")) != 2 {
		return nil, errors.New("base repo name is invalid (shouldPatchFileWithDiff [owner]/[repo])")
	}
	if pr.Base.GetRef() == "" {
		return nil, errors.New("base ref is empty")
	}
	if pr.Head.GetRef() == "" {
		return nil, errors.New("head ref is empty")
	}
	if len(strings.Split(pr.Head.Repo.GetFullName(), "/")) != 2 {
		return nil, errors.New("head repo name is invalid (shouldPatchFileWithDiff [owner]/[repo])")
	}
	if pr.GetNumber() == 0 {
		return nil, errors.New("PR number must not be 0")
	}
	if pr.User.GetLogin() == "" || pr.User.GetID() == 0 {
		return nil, errors.New("GitHub sender missing login name or UID")
	}
	if pr.Head.GetSHA() == "" {
		return nil, errors.New("head hash must not be empty")
	}
	if pr.Base.GetSHA() == "" {
		return nil, errors.New("base hash must not be empty")
	}
	if pr.GetTitle() == "" {
		return nil, errors.New("PR title must not be empty")
	}
	if patchOwner == "" {
		patchOwner = pr.User.GetLogin()
	}
	if alias == "" {
		alias = evergreen.GithubPRAlias
	}

	// get the patchId to repeat the definitions from
	repeat, err := getRepeatPatchId(ctx, pr.Base.Repo.Owner.GetLogin(), pr.Base.Repo.GetName(), pr.GetNumber())
	if err != nil {
		return nil, errors.Wrap(err, "getting patch to repeat definitions from")
	}

	return &githubIntent{
		DocumentID:    msgDeliveryID,
		MsgID:         msgDeliveryID,
		BaseRepoName:  pr.Base.Repo.GetFullName(),
		BaseBranch:    pr.Base.GetRef(),
		HeadRepoName:  pr.Head.Repo.GetFullName(),
		HeadBranch:    pr.Head.GetRef(),
		PRNumber:      pr.GetNumber(),
		User:          patchOwner,
		UID:           int(pr.User.GetID()),
		HeadHash:      pr.Head.GetSHA(),
		BaseHash:      pr.Base.GetSHA(),
		MergeBase:     mergeBase,
		Title:         pr.GetTitle(),
		IntentType:    GithubIntentType,
		CalledBy:      calledBy,
		RepeatPatchId: repeat,
		Alias:         alias,
	}, nil
}

// getRepeatPatchId returns the patch id to repeat the definitions from
// this information is found on the most recent pr patch
func getRepeatPatchId(ctx context.Context, owner, repo string, prNumber int) (string, error) {
	p, err := FindLatestGithubPRPatch(ctx, owner, repo, prNumber)
	if err != nil {
		return "", errors.Errorf("finding latest patch for PR '%s/%s:%d'", owner, repo, prNumber)
	}
	if p == nil {
		// do not error, it may be the first patch for the PR
		return "", nil
	}
	return p.GithubPatchData.RepeatPatchIdNextPatch, nil
}

// SetProcessed should be called by an amboy queue after creating a patch from an intent.
func (g *githubIntent) SetProcessed(ctx context.Context) error {
	g.Processed = true
	g.ProcessedAt = time.Now().UTC().Round(time.Millisecond)
	return updateOneIntent(
		ctx,
		bson.M{documentIDKey: g.DocumentID},
		bson.M{"$set": bson.M{
			processedKey:   g.Processed,
			processedAtKey: g.ProcessedAt,
		}},
	)
}

// updateOne updates one patch intent.
func updateOneIntent(ctx context.Context, query any, update any) error {
	return db.Update(
		ctx,
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
func (g *githubIntent) Insert(ctx context.Context) error {
	g.CreatedAt = time.Now().UTC().Round(time.Millisecond)
	err := db.Insert(ctx, IntentCollection, g)
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

func (g *githubIntent) RepeatPreviousPatchDefinition() (string, bool) {
	return g.RepeatPatchId, g.RepeatPatchId != ""
}

func (g *githubIntent) RepeatFailedTasksAndVariants() (string, bool) {
	return "", false
}

func (g *githubIntent) RequesterIdentity() string {
	return evergreen.GithubPRRequester
}

func (g *githubIntent) GetCalledBy() string {
	return g.CalledBy
}

// FindUnprocessedGithubIntents finds all patch intents that have not yet been processed.
func FindUnprocessedGithubIntents(ctx context.Context) ([]*githubIntent, error) {
	var intents []*githubIntent
	err := db.FindAllQ(ctx, IntentCollection, db.Query(bson.M{processedKey: false, intentTypeKey: GithubIntentType}), &intents)
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
		Id:          mgobson.NewObjectId(),
		Alias:       g.Alias,
		Description: fmt.Sprintf("'%s' pull request #%d by %s: %s (%s)", g.BaseRepoName, g.PRNumber, g.User, g.Title, pullURL),
		Author:      evergreen.GithubPatchUser,
		Status:      evergreen.VersionCreated,
		CreateTime:  g.CreatedAt,
		Githash:     g.MergeBase,
		GithubPatchData: thirdparty.GithubPatch{
			PRNumber:   g.PRNumber,
			BaseOwner:  baseRepo[0],
			BaseRepo:   baseRepo[1],
			BaseBranch: g.BaseBranch,
			HeadOwner:  headRepo[0],
			HeadRepo:   headRepo[1],
			HeadBranch: g.HeadBranch,
			HeadHash:   g.HeadHash,
			BaseHash:   g.BaseHash,
			MergeBase:  g.MergeBase,
			Author:     g.User,
			AuthorUID:  g.UID,
		},
	}
	return patchDoc
}

func (g *githubIntent) GetAlias() string {
	return g.Alias
}
