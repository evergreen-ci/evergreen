package patch

import (
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	mgobson "gopkg.in/mgo.v2/bson"
)

const CliIntentType = "cli"

type cliIntent struct {
	// ID is created by the driver and has no special meaning to the application.
	DocumentID string `bson:"_id"`

	// PatchFileID is the object id of the patch file created in gridfs
	PatchFileID mgobson.ObjectId `bson:"patch_file_id"`

	// PatchContent is the patch as supplied by the client. It is saved
	// separately from the patch intent
	PatchContent string

	// Description is the optional description of the patch
	Description string `bson:"description,omitempty"`

	// BuildVariants is a list of build variants associated with the patch
	BuildVariants []string `bson:"variants,omitempty"`

	// Tasks is a list of tasks associated with the patch
	Tasks []string `bson:"tasks"`

	// SyncAtEndOpts describe behavior for task sync at the end of the task.
	SyncAtEndOpts SyncAtEndOptions `bson:"sync_at_end_opts,omitempty"`

	// Finalize is whether or not the patch should finalized
	Finalize bool `bson:"finalize"`

	// Module is the name of the module id as represented in the project's
	// YAML configuration
	Module string `bson:"module"`

	// User is the username of the patch creator
	User string `bson:"user"`

	// ProjectID is the identifier of project this patch is associated with
	ProjectID string `bson:"project"`

	// BaseHash is the base hash of the patch.
	BaseHash string `bson:"base_hash"`

	// CreatedAt is the time that this intent was stored in the database
	CreatedAt time.Time `bson:"created_at"`

	// Processed indicates whether a patch intent has been processed by the amboy queue.
	Processed bool `bson:"processed"`

	// ProcessedAt is the time that this intent was processed
	ProcessedAt time.Time `bson:"processed_at"`

	// IntentType indicates the type of the patch intent, i.e., GithubIntentType
	IntentType string `bson:"intent_type"`

	// alias defines the variants and tasks to run this patch on.
	Alias string `bson:"alias"`

	// IsMbox indicates if the patch content is in MBox format
	IsMbox bool `bson:"is_mbox"`
}

// BSON fields for the patches
// nolint
var (
	cliDocumentIDKey    = bsonutil.MustHaveTag(cliIntent{}, "DocumentID")
	cliPatchFileIDKey   = bsonutil.MustHaveTag(cliIntent{}, "PatchFileID")
	cliDescriptionKey   = bsonutil.MustHaveTag(cliIntent{}, "Description")
	cliBuildVariantsKey = bsonutil.MustHaveTag(cliIntent{}, "BuildVariants")
	cliTasksKey         = bsonutil.MustHaveTag(cliIntent{}, "Tasks")
	cliFinalizeKey      = bsonutil.MustHaveTag(cliIntent{}, "Finalize")
	cliModuleKey        = bsonutil.MustHaveTag(cliIntent{}, "Module")
	cliUserKey          = bsonutil.MustHaveTag(cliIntent{}, "User")
	cliProjectIDKey     = bsonutil.MustHaveTag(cliIntent{}, "ProjectID")
	cliBaseHashKey      = bsonutil.MustHaveTag(cliIntent{}, "BaseHash")
	cliCreatedAtKey     = bsonutil.MustHaveTag(cliIntent{}, "CreatedAt")
	cliProcessedKey     = bsonutil.MustHaveTag(cliIntent{}, "Processed")
	cliProcessedAtKey   = bsonutil.MustHaveTag(cliIntent{}, "ProcessedAt")
	cliIntentTypeKey    = bsonutil.MustHaveTag(cliIntent{}, "IntentType")
	cliAliasKey         = bsonutil.MustHaveTag(cliIntent{}, "Alias")
)

func (c *cliIntent) Insert() error {
	patchFileID := mgobson.NewObjectId()
	if err := db.WriteGridFile(GridFSPrefix, patchFileID.Hex(), strings.NewReader(c.PatchContent)); err != nil {
		return err
	}

	c.PatchContent = ""
	c.PatchFileID = patchFileID
	c.CreatedAt = time.Now().UTC().Round(time.Millisecond)

	if err := db.Insert(IntentCollection, c); err != nil {
		c.CreatedAt = time.Time{}
		return err
	}

	return nil
}

func (c *cliIntent) SetProcessed() error {
	c.Processed = true
	c.ProcessedAt = time.Now().UTC().Round(time.Millisecond)
	return updateOneIntent(
		bson.M{cliDocumentIDKey: c.DocumentID},
		bson.M{"$set": bson.M{
			cliProcessedKey:   c.Processed,
			cliProcessedAtKey: c.ProcessedAt,
		}},
	)
}

func (c *cliIntent) IsProcessed() bool {
	return c.Processed
}

func (c *cliIntent) GetType() string {
	return CliIntentType
}

func (c *cliIntent) ID() string {
	return c.DocumentID
}

func (c *cliIntent) ShouldFinalizePatch() bool {
	return c.Finalize
}

func (g *cliIntent) RequesterIdentity() string {
	return evergreen.PatchVersionRequester
}

// NewPatch creates a patch from the intent
func (c *cliIntent) NewPatch() *Patch {
	return &Patch{
		Description:   c.Description,
		Author:        c.User,
		Project:       c.ProjectID,
		Githash:       c.BaseHash,
		Status:        evergreen.PatchCreated,
		BuildVariants: c.BuildVariants,
		Alias:         c.Alias,
		Tasks:         c.Tasks,
		SyncAtEndOpts: c.SyncAtEndOpts,
		Patches: []ModulePatch{
			{
				ModuleName: c.Module,
				Githash:    c.BaseHash,
				IsMbox:     c.IsMbox,
				PatchSet: PatchSet{
					PatchFileId: c.PatchFileID.Hex(),
				},
			},
		},
	}
}

type CLIIntentParams struct {
	User         string
	Project      string
	BaseGitHash  string
	Module       string
	PatchContent string
	Description  string
	Finalize     bool
	Variants     []string
	Tasks        []string
	Alias        string
	SyncParams   SyncAtEndOptions
}

func NewCliIntent(params CLIIntentParams) (Intent, error) {
	if params.User == "" {
		return nil, errors.New("no user provided")
	}
	if params.Project == "" {
		return nil, errors.New("no project provided")
	}
	if params.BaseGitHash == "" {
		return nil, errors.New("no base hash provided")
	}
	if params.Finalize {
		if params.Alias == "" {
			if len(params.Variants) == 0 {
				return nil, errors.New("no variants provided")
			}
			if len(params.Tasks) == 0 {
				return nil, errors.New("no tasks provided")
			}
		}
	}
	if len(params.SyncParams.BuildVariants) != 0 && len(params.SyncParams.Tasks) == 0 {
		return nil, errors.New("build variants provided for task sync but task names missing")
	}
	if len(params.SyncParams.Tasks) != 0 && len(params.SyncParams.BuildVariants) == 0 {
		return nil, errors.New("task names provided for sync but build variants missing")
	}
	for _, status := range params.SyncParams.Statuses {
		catcher := grip.NewBasicCatcher()
		if !utility.StringSliceContains(evergreen.SyncStatuses, status) {
			catcher.Errorf("invalid sync status '%s'", status)
		}
		if catcher.HasErrors() {
			return nil, catcher.Resolve()
		}
	}

	return &cliIntent{
		DocumentID:    mgobson.NewObjectId().Hex(),
		IntentType:    CliIntentType,
		PatchContent:  params.PatchContent,
		Description:   params.Description,
		BuildVariants: params.Variants,
		Tasks:         params.Tasks,
		SyncAtEndOpts: params.SyncParams,
		User:          params.User,
		ProjectID:     params.Project,
		BaseHash:      params.BaseGitHash,
		Finalize:      params.Finalize,
		Module:        params.Module,
		Alias:         params.Alias,
		IsMbox:        len(params.PatchContent) == 0 || IsMailboxDiff(params.PatchContent),
	}, nil
}

func (c *cliIntent) GetAlias() string {
	return c.Alias
}
