package patch

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	mgobson "gopkg.in/mgo.v2/bson"
)

const TriggerIntentType = "trigger"

type TriggerIntent struct {
	Id             string                   `bson:"_id"`
	Requester      string                   `bson:"requester"`
	Author         string                   `bson:"author"`
	ProjectID      string                   `bson:"project_id"`
	ParentID       string                   `bson:"parent_id"`
	ParentAsModule string                   `bson:"parent_as_module"`
	ParentStatus   string                   `bson:"parent_status"`
	Definitions    []PatchTriggerDefinition `bson:"definitions"`

	Processed bool `bson:"processed"`
}

var (
	triggerIDKey        = bsonutil.MustHaveTag(TriggerIntent{}, "Id")
	triggerProcessedKey = bsonutil.MustHaveTag(TriggerIntent{}, "Processed")
)

func (t *TriggerIntent) ID() string {
	return t.Id
}

func (t *TriggerIntent) Insert() error {
	return errors.Wrap(db.Insert(IntentCollection, t), "problem inserting trigger intent")
}

func (t *TriggerIntent) SetProcessed() error {
	t.Processed = true
	return updateOneIntent(
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
		Id:       mgobson.ObjectIdHex(t.Id),
		Author:   t.Author,
		Triggers: TriggerInfo{ParentPatch: t.ParentID},
		Status:   evergreen.PatchCreated,
		Project:  t.ProjectID,
	}
}

func (t *TriggerIntent) ShouldFinalizePatch() bool {
	// finalize child patches that aren't waiting on a parent to complete
	return t.ParentStatus == ""
}

func (t *TriggerIntent) ReusePreviousPatchDefinition() bool {
	return false
}

func (t *TriggerIntent) GetAlias() string {
	// triggers have no alias
	return ""
}

func (t *TriggerIntent) RequesterIdentity() string {
	return t.Requester
}

type TriggerIntentOptions struct {
	Requester      string
	Author         string
	ProjectID      string
	ParentID       string
	ParentAsModule string
	ParentStatus   string
	Definitions    []PatchTriggerDefinition
}

func NewTriggerIntent(opts TriggerIntentOptions) Intent {
	return &TriggerIntent{
		Id:             mgobson.NewObjectId().Hex(),
		Requester:      opts.Requester,
		Author:         opts.Author,
		ProjectID:      opts.ProjectID,
		ParentID:       opts.ParentID,
		ParentAsModule: opts.ParentAsModule,
		ParentStatus:   opts.ParentStatus,
		Definitions:    opts.Definitions,
	}
}
