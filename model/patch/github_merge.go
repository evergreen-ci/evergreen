package patch

import (
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/google/go-github/v52/github"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	// GithubMergeIntentType is an intent to create a version for a GitHub merge group.
	GithubMergeIntentType = "github_merge"
)

// githubMergeIntent represents an intent to create a patch build as a result of a
// MergeGroupEvent webhook. These intents are processed asynchronously by an
// amboy queue.
type githubMergeIntent struct {
	// DocumentID is created by the driver and has no special meaning to the application
	DocumentID string `bson:"_id"`

	// MsgId is a GUID provided by Github (X-Github-Delivery) for the event
	MsgID string `bson:"msg_id"`

	// CreatedAt is the time that this intent was stored in the database
	CreatedAt time.Time `bson:"created_at"`

	// Processed indicates whether a patch intent has been processed by the amboy queue.
	Processed bool `bson:"processed"`

	// ProcessedAt is the time that this intent was processed
	ProcessedAt time.Time `bson:"processed_at"`

	// IntentType indicates the type of the patch intent, i.e., GithubMergeIntentType
	IntentType string `bson:"intent_type"`

	// CalledBy indicates whether the intent was created automatically by Evergreen or by a user.
	CalledBy string `bson:"called_by"`

	// HeadRef is the head ref of the merge group. Evergreen clones this.
	HeadRef string `bson:"base_branch"`

	// HeadHash is the SHA of the head of the merge group. Evergreen checks this out.
	HeadSHA string `bson:"head_hash"`

	// Owner is the GitHub repository organization name.
	Org string `bson:"org"`

	// Repo is the GitHub repository name
	Repo string `bson:"repo"`
}

// NewGithubIntent creates an Intent from a google/go-github MergeGroup.
func NewGithubMergeIntent(msgDeliveryID string, caller string, mg *github.MergeGroupEvent) (Intent, error) {
	if msgDeliveryID == "" {
		return nil, errors.New("message ID cannot be empty")
	}
	if caller == "" {
		return nil, errors.New("empty caller errors")
	}
	if mg.GetOrg().GetName() == "" {
		return nil, errors.New("merge group org name cannot be empty")
	}
	if mg.GetRepo().GetName() == "" {
		return nil, errors.New("merge group repo name cannot be empty")
	}
	if headRef := mg.GetMergeGroup().GetHeadRef(); headRef == "" {
		return nil, errors.New("merge group head ref cannot be empty")
	}
	if mg.GetMergeGroup().GetHeadSHA() == "" {
		return nil, errors.New("head ref cannot be empty")
	}
	return &githubMergeIntent{
		DocumentID: msgDeliveryID,
		MsgID:      msgDeliveryID,
		IntentType: GithubMergeIntentType,
		Org:        mg.GetOrg().GetName(),
		Repo:       mg.GetRepo().GetName(),
		HeadRef:    mg.GetMergeGroup().GetHeadRef(),
		HeadSHA:    mg.GetMergeGroup().GetHeadSHA(),
		CalledBy:   caller,
	}, nil
}

// SetProcessed should be called by an amboy queue after creating a patch from an intent.
func (g *githubMergeIntent) SetProcessed() error {
	g.Processed = true
	g.ProcessedAt = time.Now().UTC().Round(time.Millisecond)
	return updateOneIntent(
		bson.M{documentIDKey: g.DocumentID},
		bson.M{"$set": bson.M{
			processedKey:   g.Processed,
			processedAtKey: g.ProcessedAt,
		}},
	)
}

// IsProcessed returns whether the intent has been processed.
func (g *githubMergeIntent) IsProcessed() bool {
	return g.Processed
}

// GetType returns the patch intent, i.e., GithubMergeIntentType
func (g *githubMergeIntent) GetType() string {
	return g.IntentType
}

// Insert inserts a patch intent in the database.
func (g *githubMergeIntent) Insert() error {
	g.CreatedAt = time.Now().UTC().Round(time.Millisecond)
	err := db.Insert(IntentCollection, g)
	if err != nil {
		g.CreatedAt = time.Time{}
		return err
	}

	return nil
}

// ID returns the GitHub message GUID, which is also the document ID.
func (g *githubMergeIntent) ID() string {
	return g.MsgID
}

// ShouldFinalizePatch returns true, since merge group patches should always be scheduled.
func (g *githubMergeIntent) ShouldFinalizePatch() bool {
	return true
}

// RepeatPreviousPatchDefinition does not apply to GitHub merge groups.
func (g *githubMergeIntent) RepeatPreviousPatchDefinition() (string, bool) {
	return "", false
}

// RepeatFailedTasksAndVariants does not apply to GitHub merge groups.
func (g *githubMergeIntent) RepeatFailedTasksAndVariants() (string, bool) {
	return "", false
}

// RequesterIdentity returns the requester, i.e., GithubMergeRequester.
func (g *githubMergeIntent) RequesterIdentity() string {
	return evergreen.GithubMergeRequester
}

// GetCalledBy returns the caller of the merge group, e.g., patch.AutomatedCaller
func (g *githubMergeIntent) GetCalledBy() string {
	return g.CalledBy
}

// NewPatch creates a patch document from a merge intent.
func (g *githubMergeIntent) NewPatch() *Patch {
	// merge_group.head_ref looks like this:
	// refs/heads/gh-readonly-queue/main/pr-515-9cd8a2532bcddf58369aa82eb66ba88e2323c056
	split := strings.Split(g.HeadRef, "/")

	// handle cases where base branch has a slash in it
	baseBranchSlice := []string{}
	for i := 3; i < len(split)-1; i++ {
		baseBranchSlice = append(baseBranchSlice, split[i])
	}
	baseBranch := strings.Join(baseBranchSlice, "/")

	// produce a branch name like gh-readonly-queue/main/pr-515-9cd8a2532bcddf58369aa82eb66ba88e2323c056
	headBranch := strings.Join([]string{split[2], baseBranch, split[len(split)-1]}, "/")

	patchDoc := &Patch{
		Id:      mgobson.NewObjectId(),
		Alias:   g.GetAlias(),
		Status:  evergreen.PatchCreated,
		Author:  evergreen.GithubMergeUser,
		Githash: g.HeadSHA,
		GithubMergeData: thirdparty.GithubMergeGroup{
			Org:        g.Org,
			Repo:       g.Repo,
			BaseBranch: baseBranch,
			HeadBranch: headBranch,
			HeadSHA:    g.HeadSHA,
		},
	}
	return patchDoc
}

// GetAlias defines the variants and tasks this intent should run on
func (g *githubMergeIntent) GetAlias() string {
	return evergreen.CommitQueueAlias
}
