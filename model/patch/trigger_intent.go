package patch

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

const TriggerIntentType = "trigger"

type TriggerIntent struct {
	Id              string `bson:"_id"`
	Requester       string `bson:"requester"`
	Author          string `bson:"author"`
	ProjectID       string `bson:"project_id"`
	ParentID        string `bson:"parent_id"`
	ParentProjectID string `bson:"parent_project"`
	ParentAsModule  string `bson:"parent_as_module"`
	// The parent status that the child patch should run on
	ParentStatus string                   `bson:"parent_status"`
	Definitions  []PatchTriggerDefinition `bson:"definitions"`
	// The revision to base the downstream patch off of
	DownstreamRevision string `bson:"downstream_revision"`

	Processed bool `bson:"processed"`
}

var (
	triggerIDKey        = bsonutil.MustHaveTag(TriggerIntent{}, "Id")
	triggerProcessedKey = bsonutil.MustHaveTag(TriggerIntent{}, "Processed")
)

func (t *TriggerIntent) ID() string {
	return t.Id
}

func (t *TriggerIntent) Insert(ctx context.Context) error {
	return errors.Wrap(db.Insert(ctx, IntentCollection, t), "inserting trigger intent")
}

func (t *TriggerIntent) SetProcessed(ctx context.Context) error {
	t.Processed = true
	return updateOneIntent(
		ctx,
		bson.M{triggerIDKey: t.Id},
		bson.M{"$set": bson.M{
			triggerProcessedKey: true,
		}},
	)
}

func (t *TriggerIntent) IsProcessed() bool {
	return t.Processed
}

// GetType returns the patch intent, e.g., GithubIntentType.
func (t *TriggerIntent) GetType() string {
	return TriggerIntentType
}

func (t *TriggerIntent) NewPatch() *Patch {
	return &Patch{
		Id:     mgobson.ObjectIdHex(t.Id),
		Author: evergreen.ParentPatchUser,
		Triggers: TriggerInfo{
			ParentPatch:        t.ParentID,
			ParentProjectID:    t.ParentProjectID,
			DownstreamRevision: t.DownstreamRevision,
			ParentAsModule:     t.ParentAsModule,
		},
		Status:  evergreen.VersionCreated,
		Project: t.ProjectID,
	}
}

func (t *TriggerIntent) ShouldFinalizePatch() bool {
	// trigger intents are finalized in one of two ways:
	// if parentStatus = "": in patch_lifecycle when the parent is finalized
	// if the parentStatus is set: they are scheduled based on the parent patches's outcome
	return false
}

func (t *TriggerIntent) RepeatPreviousPatchDefinition() (string, bool) {
	return "", false
}

func (g *TriggerIntent) RepeatFailedTasksAndVariants() (string, bool) {
	return "", false
}

func (t *TriggerIntent) GetAlias() string {
	// triggers have no alias
	return ""
}

func (t *TriggerIntent) RequesterIdentity() string {
	return t.Requester
}

func (t *TriggerIntent) GetCalledBy() string {
	// not relevant to trigger intents
	return AllCallers
}

type TriggerIntentOptions struct {
	Requester          string
	Author             string
	ProjectID          string
	ParentID           string
	ParentProjectID    string
	ParentAsModule     string
	ParentStatus       string
	DownstreamRevision string
	Definitions        []PatchTriggerDefinition
}

func NewTriggerIntent(opts TriggerIntentOptions) Intent {
	return &TriggerIntent{
		Id:                 mgobson.NewObjectId().Hex(),
		Requester:          opts.Requester,
		Author:             opts.Author,
		ProjectID:          opts.ProjectID,
		ParentID:           opts.ParentID,
		ParentProjectID:    opts.ParentProjectID,
		ParentAsModule:     opts.ParentAsModule,
		ParentStatus:       opts.ParentStatus,
		Definitions:        opts.Definitions,
		DownstreamRevision: opts.DownstreamRevision,
	}
}
