package patch

import (
	"context"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/mongodb/anser/bsonutil"
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

	// RegexTestSelectionVariants is a list of regular expressions to match
	// build variants that should use test selection.
	RegexTestSelectionBuildVariants []string `bson:"regex_test_selection_build_variants,omitempty"`
	// RegexTestSelectionExcludedBuildVariants is a list of regular expressions to
	// match build variants that should not use test selection. Takes
	// precedence over RegexTestSelectionBuildVariants.
	RegexTestSelectionExcludedBuildVariants []string `bson:"regex_test_selection_excluded_build_variants,omitempty"`
	// RegexTestSelectionTasks is a list of regular expressions to match tasks
	// that should use test selection.
	RegexTestSelectionTasks []string `bson:"regex_test_selection_tasks,omitempty"`
	// RegexTestSelectionExcludedTasks is a list of regular expressions to
	// match tasks that should not use test selection. Takes precedence over
	// RegexTestSelectionTasks.
	RegexTestSelectionExcludedTasks []string `bson:"regex_test_selection_excluded_tasks,omitempty"`

	// Parameters is a list of parameters to use with the task.
	Parameters []Parameter `bson:"parameters,omitempty"`

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

	// GitInfo contains information about the author's git environment.
	GitInfo *GitMetadata `bson:"git_info,omitempty"`

	// RepeatDefinition reuses the latest patch's task/variants (if no patch ID is provided)
	RepeatDefinition bool `bson:"reuse_definition"`
	// RepeatFailed reuses the latest patch's failed tasks (if no patch ID is provided)
	RepeatFailed bool `bson:"repeat_failed"`
	// RepeatPatchId uses the given patch to reuse the task/variant definitions
	RepeatPatchId string `bson:"repeat_patch_id"`

	// LocalModuleIncludes is only used to include local module changes
	LocalModuleIncludes []LocalModuleInclude `bson:"local_module_includes,omitempty"`
}

// BSON fields for the patches
var (
	cliDocumentIDKey  = bsonutil.MustHaveTag(cliIntent{}, "DocumentID")
	cliProcessedKey   = bsonutil.MustHaveTag(cliIntent{}, "Processed")
	cliProcessedAtKey = bsonutil.MustHaveTag(cliIntent{}, "ProcessedAt")
	cliIntentTypeKey  = bsonutil.MustHaveTag(cliIntent{}, "IntentType")
)

func (c *cliIntent) Insert(ctx context.Context) error {
	if len(c.PatchContent) > 0 {
		patchFileID := mgobson.NewObjectId()
		if err := db.WriteGridFile(ctx, GridFSPrefix, patchFileID.Hex(), strings.NewReader(c.PatchContent)); err != nil {
			return err
		}

		c.PatchContent = ""
		c.PatchFileID = patchFileID
	}

	c.CreatedAt = time.Now().UTC().Round(time.Millisecond)

	if err := db.Insert(ctx, IntentCollection, c); err != nil {
		c.CreatedAt = time.Time{}
		return err
	}

	return nil
}

func (c *cliIntent) SetProcessed(ctx context.Context) error {
	c.Processed = true
	c.ProcessedAt = time.Now().UTC().Round(time.Millisecond)
	return updateOneIntent(
		ctx,
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
		Description:                             c.Description,
		Author:                                  c.User,
		Project:                                 c.ProjectID,
		Githash:                                 c.BaseHash,
		Path:                                    c.Path,
		Status:                                  evergreen.VersionCreated,
		BuildVariants:                           c.BuildVariants,
		RegexBuildVariants:                      c.RegexBuildVariants,
		RegexTestSelectionBuildVariants:         c.RegexTestSelectionBuildVariants,
		RegexTestSelectionExcludedBuildVariants: c.RegexTestSelectionExcludedBuildVariants,
		Parameters:                              c.Parameters,
		Alias:                                   c.Alias,
		Triggers:                                TriggerInfo{Aliases: c.TriggerAliases},
		Tasks:                                   c.Tasks,
		RegexTasks:                              c.RegexTasks,
		RegexTestSelectionTasks:                 c.RegexTestSelectionTasks,
		RegexTestSelectionExcludedTasks:         c.RegexTestSelectionExcludedTasks,
		Patches:                                 []ModulePatch{},
		GitInfo:                                 c.GitInfo,
		LocalModuleIncludes:                     c.LocalModuleIncludes,
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
	User                               string
	Path                               string
	Project                            string
	BaseGitHash                        string
	Module                             string
	PatchContent                       string
	Description                        string
	Finalize                           bool
	GitInfo                            *GitMetadata
	Parameters                         []Parameter
	Variants                           []string
	Tasks                              []string
	RegexVariants                      []string
	RegexTasks                         []string
	RegexTestSelectionVariants         []string
	RegexTestSelectionExcludedVariants []string
	RegexTestSelectionTasks            []string
	RegexTestSelectionExcludedTasks    []string
	Alias                              string
	TriggerAliases                     []string
	RepeatDefinition                   bool
	RepeatFailed                       bool
	RepeatPatchId                      string
	LocalModuleIncludes                []LocalModuleInclude
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

	return &cliIntent{
		DocumentID:                              mgobson.NewObjectId().Hex(),
		IntentType:                              CliIntentType,
		PatchContent:                            params.PatchContent,
		Path:                                    params.Path,
		Description:                             params.Description,
		BuildVariants:                           params.Variants,
		Tasks:                                   params.Tasks,
		RegexBuildVariants:                      params.RegexVariants,
		RegexTasks:                              params.RegexTasks,
		RegexTestSelectionBuildVariants:         params.RegexTestSelectionVariants,
		RegexTestSelectionExcludedBuildVariants: params.RegexTestSelectionExcludedVariants,
		RegexTestSelectionTasks:                 params.RegexTestSelectionTasks,
		RegexTestSelectionExcludedTasks:         params.RegexTestSelectionExcludedTasks,
		Parameters:                              params.Parameters,
		User:                                    params.User,
		ProjectID:                               params.Project,
		BaseHash:                                params.BaseGitHash,
		Finalize:                                params.Finalize,
		Module:                                  params.Module,
		Alias:                                   params.Alias,
		TriggerAliases:                          params.TriggerAliases,
		GitInfo:                                 params.GitInfo,
		RepeatDefinition:                        params.RepeatDefinition,
		RepeatFailed:                            params.RepeatFailed,
		RepeatPatchId:                           params.RepeatPatchId,
		LocalModuleIncludes:                     params.LocalModuleIncludes,
	}, nil
}

func (c *cliIntent) GetAlias() string {
	return c.Alias
}
