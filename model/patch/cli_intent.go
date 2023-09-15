package patch

import (
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

const CliIntentType = "cli"

type cliIntent struct {
	// ID is created by the driver and has no special meaning to the application.
	DocumentID string `bson:"_id"`

	// PatchFileID is the object id of the patch file created in gridfs.
	PatchFileID mgobson.ObjectId `bson:"patch_file_id,omitempty"`

	// PatchContent is the patch as supplied by the client. It is saved
	// separately from the patch intent.
	PatchContent string

	// Description is the optional description of the patch.
	Description string `bson:"description,omitempty"`

	// BuildVariants is a list of build variants associated with the patch.
	BuildVariants []string `bson:"variants,omitempty"`

	// Tasks is a list of tasks associated with the patch.
	Tasks []string `bson:"tasks"`

	// RegexBuildVariants is a list of regular expressions to match build variants associated with the patch.
	RegexBuildVariants []string `bson:"regex_variants,omitempty"`

	// RegexTasks is a list of regular expressions to match tasks associated with the patch.
	RegexTasks []string `bson:"regex_tasks,omitempty"`

	// Parameters is a list of parameters to use with the task.
	Parameters []Parameter `bson:"parameters,omitempty"`

	// SyncAtEndOpts describe behavior for task sync at the end of the task.
	SyncAtEndOpts SyncAtEndOptions `bson:"sync_at_end_opts,omitempty"`

	// Finalize is whether or not the patch should finalized.
	Finalize bool `bson:"finalize"`

	// Module is the name of the module id as represented in the project's
	// YAML configuration.
	Module string `bson:"module"`

	// User is the username of the patch creator.
	User string `bson:"user"`

	// ProjectID is the identifier of project this patch is associated with.
	ProjectID string `bson:"project"`

	// BaseHash is the base hash of the patch.
	BaseHash string `bson:"base_hash"`

	// CreatedAt is the time that this intent was stored in the database.
	CreatedAt time.Time `bson:"created_at"`

	// Processed indicates whether a patch intent has been processed by the amboy queue.
	Processed bool `bson:"processed"`

	// ProcessedAt is the time that this intent was processed.
	ProcessedAt time.Time `bson:"processed_at"`

	// IntentType indicates the type of the patch intent, i.e., GithubIntentType.
	IntentType string `bson:"intent_type"`

	// alias defines the variants and tasks to run this patch on.
	Alias string `bson:"alias"`

	// path defines the path to an evergreen project configuration file.
	Path string `bson:"path"`

	// TriggerAliases alias sets of tasks to include in child patches.
	TriggerAliases []string `bson:"trigger_aliases"`

	// BackportOf specifies what to backport.
	BackportOf BackportInfo `bson:"backport_of,omitempty"`

	// GitInfo contains information about the author's git environment.
	GitInfo *GitMetadata `bson:"git_info,omitempty"`

	// RepeatDefinition reuses the latest patch's task/variants (if no patch ID is provided)
	RepeatDefinition bool `bson:"reuse_definition"`
	// RepeatFailed reuses the latest patch's failed tasks (if no patch ID is provided)
	RepeatFailed bool `bson:"repeat_failed"`
	// RepeatPatchId uses the given patch to reuse the task/variant definitions
	RepeatPatchId string `bson:"repeat_patch_id"`
}

// BSON fields for the patches
//
//nolint:unused
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
	if len(c.PatchContent) > 0 {
		patchFileID := mgobson.NewObjectId()
		if err := db.WriteGridFile(GridFSPrefix, patchFileID.Hex(), strings.NewReader(c.PatchContent)); err != nil {
			return err
		}

		c.PatchContent = ""
		c.PatchFileID = patchFileID
	}

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

func (c *cliIntent) RepeatPreviousPatchDefinition() (string, bool) {
	return c.RepeatPatchId, c.RepeatDefinition
}

func (c *cliIntent) RepeatFailedTasksAndVariants() (string, bool) {
	return c.RepeatPatchId, c.RepeatFailed
}

func (g *cliIntent) RequesterIdentity() string {
	return evergreen.PatchVersionRequester
}

func (g *cliIntent) GetCalledBy() string {
	// not relevant to CLI intents
	return AllCallers
}

// NewPatch creates a patch from the intent
func (c *cliIntent) NewPatch() *Patch {
	p := Patch{
		Description:        c.Description,
		Author:             c.User,
		Project:            c.ProjectID,
		Githash:            c.BaseHash,
		Path:               c.Path,
		Status:             evergreen.VersionCreated,
		BuildVariants:      c.BuildVariants,
		RegexBuildVariants: c.RegexBuildVariants,
		Parameters:         c.Parameters,
		Alias:              c.Alias,
		Triggers:           TriggerInfo{Aliases: c.TriggerAliases},
		Tasks:              c.Tasks,
		RegexTasks:         c.RegexTasks,
		SyncAtEndOpts:      c.SyncAtEndOpts,
		BackportOf:         c.BackportOf,
		Patches:            []ModulePatch{},
		GitInfo:            c.GitInfo,
	}
	if len(c.PatchFileID) > 0 {
		p.Patches = append(p.Patches,
			ModulePatch{
				ModuleName: c.Module,
				Githash:    c.BaseHash,
				PatchSet: PatchSet{
					PatchFileId: c.PatchFileID.Hex(),
				},
			})
	}
	return &p
}

type CLIIntentParams struct {
	User             string
	Path             string
	Project          string
	BaseGitHash      string
	Module           string
	PatchContent     string
	Description      string
	Finalize         bool
	BackportOf       BackportInfo
	GitInfo          *GitMetadata
	Parameters       []Parameter
	Variants         []string
	Tasks            []string
	RegexVariants    []string
	RegexTasks       []string
	Alias            string
	TriggerAliases   []string
	RepeatDefinition bool
	RepeatFailed     bool
	RepeatPatchId    string
	SyncParams       SyncAtEndOptions
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
	if params.Finalize && params.Alias == "" && !params.RepeatFailed && !params.RepeatDefinition {
		if len(params.Variants)+len(params.RegexVariants)+len(params.Tasks)+len(params.RegexTasks) == 0 {
			return nil, errors.New("no tasks or variants provided")
		}
		if len(params.Variants)+len(params.RegexVariants) == 0 {
			return nil, errors.New("no variants provided")
		}
		if len(params.Tasks)+len(params.RegexTasks) == 0 {
			return nil, errors.New("no tasks provided")
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
		DocumentID:         mgobson.NewObjectId().Hex(),
		IntentType:         CliIntentType,
		PatchContent:       params.PatchContent,
		Path:               params.Path,
		Description:        params.Description,
		BuildVariants:      params.Variants,
		Tasks:              params.Tasks,
		RegexBuildVariants: params.RegexVariants,
		RegexTasks:         params.RegexTasks,
		Parameters:         params.Parameters,
		SyncAtEndOpts:      params.SyncParams,
		User:               params.User,
		ProjectID:          params.Project,
		BaseHash:           params.BaseGitHash,
		Finalize:           params.Finalize,
		Module:             params.Module,
		Alias:              params.Alias,
		TriggerAliases:     params.TriggerAliases,
		BackportOf:         params.BackportOf,
		GitInfo:            params.GitInfo,
		RepeatDefinition:   params.RepeatDefinition,
		RepeatFailed:       params.RepeatFailed,
		RepeatPatchId:      params.RepeatPatchId,
	}, nil
}

func (c *cliIntent) GetAlias() string {
	return c.Alias
}
