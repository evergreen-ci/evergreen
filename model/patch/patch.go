package patch

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// SizeLimit is a hard limit on patch size.
const SizeLimit = 1024 * 1024 * 100

// VariantTasks contains the variant name and the set of tasks to be scheduled for that variant
type VariantTasks struct {
	Variant      string        `bson:"variant"`
	Tasks        []string      `bson:"tasks"`
	DisplayTasks []DisplayTask `bson:"displaytasks"`
}

// MergeVariantsTasks merges two slices of VariantsTasks into a single set.
func MergeVariantsTasks(vts1, vts2 []VariantTasks) []VariantTasks {
	bvToVT := map[string]VariantTasks{}
	for _, vt := range vts1 {
		if _, ok := bvToVT[vt.Variant]; !ok {
			bvToVT[vt.Variant] = VariantTasks{Variant: vt.Variant}
		}
		bvToVT[vt.Variant] = mergeVariantTasks(bvToVT[vt.Variant], vt)
	}
	for _, vt := range vts2 {
		if _, ok := bvToVT[vt.Variant]; !ok {
			bvToVT[vt.Variant] = VariantTasks{Variant: vt.Variant}
		}
		bvToVT[vt.Variant] = mergeVariantTasks(bvToVT[vt.Variant], vt)
	}

	var merged []VariantTasks
	for _, vt := range bvToVT {
		merged = append(merged, vt)
	}
	return merged
}

// mergeVariantTasks merges the current VariantTask for a specific variant with
// toMerge, whichs has the same variant.  The merged VariantTask contains all
// unique task names from current and toMerge. All display tasks merged such
// that, for each display task name, execution tasks are merged into a unique
// set for that display task.
func mergeVariantTasks(current VariantTasks, toMerge VariantTasks) VariantTasks {
	for _, t := range toMerge.Tasks {
		if !utility.StringSliceContains(current.Tasks, t) {
			current.Tasks = append(current.Tasks, t)
		}
	}
	for _, dt := range toMerge.DisplayTasks {
		var found bool
		for i := range current.DisplayTasks {
			if current.DisplayTasks[i].Name != dt.Name {
				continue
			}
			current.DisplayTasks[i] = mergeDisplayTasks(current.DisplayTasks[i], dt)
			found = true
			break
		}
		if !found {
			current.DisplayTasks = append(current.DisplayTasks, dt)
		}
	}
	return current
}

// mergeDisplayTasks merges two display tasks such that the resulting
// DisplayTask's execution tasks are the unique set of execution tasks from
// current and toMerge.
func mergeDisplayTasks(current DisplayTask, toMerge DisplayTask) DisplayTask {
	for _, et := range toMerge.ExecTasks {
		if !utility.StringSliceContains(current.ExecTasks, et) {
			current.ExecTasks = append(current.ExecTasks, et)
		}
	}
	return current
}

type DisplayTask struct {
	Name      string   `yaml:"name,omitempty" bson:"name,omitempty"`
	ExecTasks []string `yaml:"execution_tasks,omitempty" bson:"execution_tasks,omitempty"`
}

// Parameter defines a key/value pair to be used as an expansion.
type Parameter struct {
	Key   string `yaml:"key" bson:"key"`
	Value string `yaml:"value" bson:"value"`
}

type GitMetadata struct {
	Username    string `bson:"username" json:"username"`
	Email       string `bson:"email" json:"email"`
	GitVersion  string `bson:"git_version,omitempty" json:"git_version,omitempty"`
	LocalBranch string `bson:"local_branch,omitempty" json:"local_branch,omitempty"`
}

type LocalModuleInclude struct {
	FileName string `yaml:"filename,omitempty" bson:"filename,omitempty" json:"filename,omitempty"`
	Module   string `yaml:"module,omitempty" bson:"module,omitempty" json:"module,omitempty"`

	// FileContent is only used for local module includes for CLI patches
	FileContent []byte `yaml:"file_content,omitempty" bson:"file_content,omitempty" json:"file_content,omitempty"`
}

// Patch stores all details related to a patch request
type Patch struct {
	Id                                      mgobson.ObjectId `bson:"_id,omitempty"`
	Description                             string           `bson:"desc"`
	Path                                    string           `bson:"path,omitempty"`
	Githash                                 string           `bson:"githash"`
	Hidden                                  bool             `bson:"hidden"`
	PatchNumber                             int              `bson:"patch_number"`
	Author                                  string           `bson:"author"`
	Version                                 string           `bson:"version"`
	Status                                  string           `bson:"status"`
	CreateTime                              time.Time        `bson:"create_time"`
	StartTime                               time.Time        `bson:"start_time"`
	FinishTime                              time.Time        `bson:"finish_time"`
	BuildVariants                           []string         `bson:"build_variants"`
	RegexBuildVariants                      []string         `bson:"regex_build_variants"`
	RegexTestSelectionBuildVariants         []string         `bson:"regex_test_selection_build_variants,omitempty"`
	RegexTestSelectionExcludedBuildVariants []string         `bson:"regex_test_selection_excluded_build_variants,omitempty"`
	Tasks                                   []string         `bson:"tasks"`
	RegexTestSelectionTasks                 []string         `bson:"regex_test_selection_tasks,omitempty"`
	RegexTestSelectionExcludedTasks         []string         `bson:"regex_test_selection_excluded_tasks,omitempty"`
	RegexTasks                              []string         `bson:"regex_tasks"`
	VariantsTasks                           []VariantTasks   `bson:"variants_tasks"`
	Patches                                 []ModulePatch    `bson:"patches"`
	Parameters                              []Parameter      `bson:"parameters,omitempty"`
	// Activated indicates whether or not the patch is finalized (i.e.
	// tasks/variants are now scheduled to run). If true, the patch has been
	// finalized.
	Activated bool `bson:"activated"`
	// IsReconfigured indicates whether this patch was finalized with an initial
	// set of tasks, then later reconfigured to add more tasks to run.
	// This doesn't take into account if existing tasks were
	// scheduled/unscheduled, it's only considered reconfigured if new tasks
	// were created after the patch was finalized.
	IsReconfigured bool `bson:"is_reconfigured,omitempty"`
	// Project contains the project ID for the patch. The bson tag here is `branch` due to legacy usage.
	Project string `bson:"branch"`
	// Branch contains the branch that the project tracks. The tag `branch_name` is
	// used to avoid conflict with legacy usage of the Project field.
	Branch string `bson:"branch_name" json:"branch_name,omitempty"`
	// ProjectStorageMethod describes how the parser project is stored for this
	// patch before it's finalized. This field is only set while the patch is
	// unfinalized and is cleared once the patch has been finalized.
	ProjectStorageMethod evergreen.ParserProjectStorageMethod `bson:"project_storage_method,omitempty"`
	PatchedProjectConfig string                               `bson:"patched_project_config"`
	Alias                string                               `bson:"alias"`
	Triggers             TriggerInfo                          `bson:"triggers"`
	MergePatch           string                               `bson:"merge_patch"`
	GithubPatchData      thirdparty.GithubPatch               `bson:"github_patch_data,omitempty"`
	GithubMergeData      thirdparty.GithubMergeGroup          `bson:"github_merge_data,omitempty"`
	GitInfo              *GitMetadata                         `bson:"git_info,omitempty"`
	// DisplayNewUI is only used when roundtripping the patch via the CLI
	DisplayNewUI bool `bson:"display_new_ui,omitempty"`
	// MergeStatus is only used in gitServePatch to send the status of this
	// patch on the commit queue to the agent
	MergeStatus string `json:"merge_status"`
	// MergedFrom is populated with the patch id of the existing patch
	// the merged patch is based off of, if applicable.
	MergedFrom string `bson:"merged_from,omitempty"`
	// LocalModuleIncludes is only used for CLI patches to store local module changes.
	LocalModuleIncludes []LocalModuleInclude `bson:"local_module_includes,omitempty"`
	// ReferenceManifestID stores the ID of the manifest that this patch is based on.
	// It is used to determine the module revisions for this patch during creation.
	// This could potentially reference an invalid manifest, and should not error
	// when the manifest is not found.
	// Not stored in the database since it is only needed during patch creation.
	ReferenceManifestID string `bson:"-"`
}

func (p *Patch) MarshalBSON() ([]byte, error)  { return mgobson.Marshal(p) }
func (p *Patch) UnmarshalBSON(in []byte) error { return mgobson.Unmarshal(in, p) }

// ModulePatch stores request details for a patch
type ModulePatch struct {
	ModuleName string   `bson:"name"`
	Githash    string   `bson:"githash"`
	PatchSet   PatchSet `bson:"patch_set"`
}

// PatchSet stores information about the actual patch
type PatchSet struct {
	Patch          string               `bson:"patch,omitempty"`
	PatchFileId    string               `bson:"patch_file_id,omitempty"`
	CommitMessages []string             `bson:"commit_messages,omitempty"`
	Summary        []thirdparty.Summary `bson:"summary"`
}

type TriggerInfo struct {
	Aliases              []string    `bson:"aliases,omitempty"`
	ParentPatch          string      `bson:"parent_patch,omitempty"`
	ParentProjectID      string      `bson:"parent_project_id,omitempty"`
	DownstreamRevision   string      `bson:"downstream_revision,omitempty"`
	SameBranchAsParent   bool        `bson:"same_branch_as_parent"`
	ChildPatches         []string    `bson:"child_patches,omitempty"`
	DownstreamParameters []Parameter `bson:"downstream_parameters,omitempty"`
}

type PatchTriggerDefinition struct {
	Alias          string          `bson:"alias" json:"alias"`
	ChildProject   string          `bson:"child_project" json:"child_project"`
	TaskSpecifiers []TaskSpecifier `bson:"task_specifiers" json:"task_specifiers"`
	// The parent status that the child patch should run on: failure, success, or *
	Status         string `bson:"status,omitempty" json:"status,omitempty"`
	ParentAsModule string `bson:"parent_as_module,omitempty" json:"parent_as_module,omitempty"`
	// The revision to base the downstream patch off of
	DownstreamRevision string `bson:"downstream_revision,omitempty" json:"downstream_revision,omitempty"`
}

type TaskSpecifier struct {
	PatchAlias   string `bson:"patch_alias,omitempty" json:"patch_alias,omitempty"`
	TaskRegex    string `bson:"task_regex,omitempty" json:"task_regex,omitempty"`
	VariantRegex string `bson:"variant_regex,omitempty" json:"variant_regex,omitempty"`
}

// IsFinished returns whether or not the patch has finished based on its
// status.
func (p *Patch) IsFinished() bool {
	return evergreen.IsFinishedVersionStatus(p.Status)
}

// SetDescription sets a patch's description in the database
func (p *Patch) SetDescription(ctx context.Context, desc string) error {
	if p.Description == desc {
		return nil
	}
	p.Description = desc
	return UpdateOne(
		ctx,
		bson.M{IdKey: p.Id},
		bson.M{
			"$set": bson.M{
				DescriptionKey: desc,
			},
		},
	)
}

func (p *Patch) GetURL(uiHost string) string {
	var url string
	if p.Activated {
		url = uiHost + "/version/" + p.Id.Hex()
		if p.IsChild() {
			url += "/downstream-projects"
		}
	} else {
		url = uiHost + "/patch/" + p.Id.Hex()
	}

	return url
}

// ClearPatchData removes any inline patch data stored in this patch object for patches that have
// an associated id in gridfs, so that it can be stored properly.
func (p *Patch) ClearPatchData() {
	for i, patchPart := range p.Patches {
		// If the patch isn't stored externally, no need to do anything.
		if patchPart.PatchSet.PatchFileId != "" {
			p.Patches[i].PatchSet.Patch = ""
		}
	}
}

// FetchPatchFiles dereferences externally-stored patch diffs by fetching them from gridfs
// and placing their contents into the patch object.
func (p *Patch) FetchPatchFiles(ctx context.Context) error {
	for i, patchPart := range p.Patches {
		// If the patch isn't stored externally, no need to do anything.
		if patchPart.PatchSet.PatchFileId == "" {
			continue
		}

		rawStr, err := FetchPatchContents(ctx, patchPart.PatchSet.PatchFileId)
		if err != nil {
			return errors.Wrapf(err, "getting patch contents for patchfile '%s'", patchPart.PatchSet.PatchFileId)
		}
		p.Patches[i].PatchSet.Patch = rawStr
	}

	return nil
}

func FetchPatchContents(ctx context.Context, patchfileID string) (string, error) {
	fileReader, err := db.GetGridFile(ctx, GridFSPrefix, patchfileID)
	if err != nil {
		return "", errors.Wrap(err, "getting grid file")
	}
	defer fileReader.Close()
	patchContents, err := io.ReadAll(fileReader)
	if err != nil {
		return "", errors.Wrap(err, "reading patch contents")
	}

	return string(patchContents), nil
}

// UpdateVariantsTasks updates the patch's Tasks and BuildVariants fields to match with the set
// in the given list of VariantTasks. This is to ensure schema backwards compatibility for T shaped
// patches. This mutates the patch in memory but does not update it in the database; for that, use
// SetVariantsTasks.
func (p *Patch) UpdateVariantsTasks(variantsTasks []VariantTasks) {
	bvs, tasks := ResolveVariantTasks(variantsTasks)
	p.BuildVariants = bvs
	p.Tasks = tasks
	p.VariantsTasks = variantsTasks
}

func (p *Patch) SetParameters(ctx context.Context, parameters []Parameter) error {
	p.Parameters = parameters
	return UpdateOne(
		ctx,
		bson.M{IdKey: p.Id},
		bson.M{
			"$set": bson.M{
				ParametersKey: parameters,
			},
		},
	)
}

func (p *Patch) SetDownstreamParameters(ctx context.Context, parameters []Parameter) error {
	p.Triggers.DownstreamParameters = append(p.Triggers.DownstreamParameters, parameters...)

	triggersKey := bsonutil.GetDottedKeyName(TriggersKey, TriggerInfoDownstreamParametersKey)
	return UpdateOne(
		ctx,
		bson.M{IdKey: p.Id},
		bson.M{
			"$push": bson.M{triggersKey: bson.M{"$each": parameters}},
		},
	)
}

// ResolveVariantTasks returns a set of all build variants and a set of all
// tasks that will run based on the given VariantTasks, filtering out any
// duplicates.
func ResolveVariantTasks(vts []VariantTasks) (bvs []string, tasks []string) {
	taskSet := map[string]bool{}
	bvSet := map[string]bool{}

	// TODO after fully switching over to new schema, remove support for standalone
	// Variants and Tasks field
	for _, vt := range vts {
		bvSet[vt.Variant] = true
		for _, t := range vt.Tasks {
			taskSet[t] = true
		}
	}

	for k := range bvSet {
		bvs = append(bvs, k)
	}

	for k := range taskSet {
		tasks = append(tasks, k)
	}

	return bvs, tasks
}

// SetVariantsTasks updates the variant/tasks pairs in the database.
// Also updates the Tasks and Variants fields to maintain backwards compatibility between
// the old and new fields.
func (p *Patch) SetVariantsTasks(ctx context.Context, variantsTasks []VariantTasks) error {
	p.UpdateVariantsTasks(variantsTasks)
	return UpdateOne(
		ctx,
		bson.M{IdKey: p.Id},
		bson.M{
			"$set": bson.M{
				VariantsTasksKey: variantsTasks,
				BuildVariantsKey: p.BuildVariants,
				TasksKey:         p.Tasks,
			},
		},
	)
}

// UpdateRepeatPatchId updates the repeat patch Id value to be used for subsequent pr patches
func (p *Patch) UpdateRepeatPatchId(ctx context.Context, patchId string) error {
	repeatKey := bsonutil.GetDottedKeyName(githubPatchDataKey, thirdparty.RepeatPatchIdNextPatchKey)
	return UpdateOne(
		ctx,
		bson.M{IdKey: p.Id},
		bson.M{
			"$set": bson.M{
				repeatKey: patchId,
			},
		},
	)
}

func (p *Patch) FindModule(moduleName string) *ModulePatch {
	for _, module := range p.Patches {
		if module.ModuleName == moduleName {
			return &module
		}
	}
	return nil
}

// TryMarkStarted attempts to mark a patch as started if it
// isn't already marked as such
func TryMarkStarted(ctx context.Context, versionId string, startTime time.Time) error {
	filter := bson.M{
		VersionKey: versionId,
		StatusKey:  evergreen.VersionCreated,
	}
	update := bson.M{
		"$set": bson.M{
			StartTimeKey: startTime,
			StatusKey:    evergreen.VersionStarted,
		},
	}
	return UpdateOne(ctx, filter, update)
}

// Insert inserts the patch into the db, returning any errors that occur
func (p *Patch) Insert(ctx context.Context) error {
	return db.Insert(ctx, Collection, p)
}

func (p *Patch) UpdateStatus(ctx context.Context, newStatus string) (modified bool, err error) {
	if p.Status == newStatus {
		return false, nil
	}

	setFields := bson.M{
		StatusKey: newStatus,
	}
	finishTime := time.Now()
	isFinished := evergreen.IsFinishedVersionStatus(newStatus)
	if isFinished {
		setFields[FinishTimeKey] = finishTime
	}
	res, err := evergreen.GetEnvironment().DB().Collection(Collection).UpdateOne(ctx, bson.M{
		IdKey:     p.Id,
		StatusKey: bson.M{"$ne": newStatus},
	}, bson.M{
		"$set": setFields,
	})
	if err != nil {
		return false, err
	}

	p.Status = newStatus
	if isFinished {
		p.FinishTime = finishTime
	}

	return res.ModifiedCount > 0, nil
}

// ConfigChanged looks through the parts of the patch and returns true if the
// passed in remotePath is in the the name of the changed files that are part
// of the patch
func (p *Patch) ConfigChanged(remotePath string) bool {
	for _, patchPart := range p.Patches {
		if patchPart.ModuleName == "" {
			for _, summary := range patchPart.PatchSet.Summary {
				if summary.Name == remotePath {
					return true
				}
			}
			return false
		}
	}
	return false
}

func (p *Patch) FilesChanged() []string {
	var filenames []string
	for _, patchPart := range p.Patches {
		for _, summary := range patchPart.PatchSet.Summary {
			filenames = append(filenames, summary.Name)
		}
	}
	return filenames
}

// SetFinalized marks the patch as finalized.
func (p *Patch) SetFinalized(ctx context.Context, versionId string) error {
	if _, err := evergreen.GetEnvironment().DB().Collection(Collection).UpdateOne(ctx,
		bson.M{IdKey: p.Id},
		bson.M{
			"$set": bson.M{
				ActivatedKey: true,
				VersionKey:   versionId,
			},
			"$unset": bson.M{
				ProjectStorageMethodKey: 1,
				PatchedProjectConfigKey: 1,
			},
		},
	); err != nil {
		return err
	}

	p.Version = versionId
	p.Activated = true
	p.ProjectStorageMethod = ""
	p.PatchedProjectConfig = ""

	return nil
}

// SetTriggerAliases appends the names of invoked trigger aliases to the DB
func (p *Patch) SetTriggerAliases(ctx context.Context) error {
	triggersKey := bsonutil.GetDottedKeyName(TriggersKey, TriggerInfoAliasesKey)
	return UpdateOne(
		ctx,
		bson.M{IdKey: p.Id},
		bson.M{
			"$addToSet": bson.M{triggersKey: bson.M{"$each": p.Triggers.Aliases}},
		},
	)
}

// SetChildPatches appends the IDs of downstream patches to the db
func (p *Patch) SetChildPatches(ctx context.Context) error {
	triggersKey := bsonutil.GetDottedKeyName(TriggersKey, TriggerInfoChildPatchesKey)
	return UpdateOne(
		ctx,
		bson.M{IdKey: p.Id},
		bson.M{
			"$addToSet": bson.M{triggersKey: bson.M{"$each": p.Triggers.ChildPatches}},
		},
	)
}

// SetActivation sets the patch to the desired activation state without
// modifying the activation status of the possibly corresponding version.
func (p *Patch) SetActivation(ctx context.Context, activated bool) error {
	p.Activated = activated
	return UpdateOne(
		ctx,
		bson.M{IdKey: p.Id},
		bson.M{
			"$set": bson.M{
				ActivatedKey: activated,
			},
		},
	)
}

// SetPatchVisibility set the patch visibility to the desired state.
// This is used to hide patches that the user does not need to see.
func (p *Patch) SetPatchVisibility(ctx context.Context, hidden bool) error {
	if p.Hidden == hidden {
		return nil
	}
	p.Hidden = hidden
	return UpdateOne(
		ctx,
		bson.M{IdKey: p.Id},
		bson.M{
			"$set": bson.M{
				HiddenKey: hidden,
			},
		},
	)
}

// UpdateModulePatch adds or updates a module within a patch.
func (p *Patch) UpdateModulePatch(ctx context.Context, modulePatch ModulePatch) error {
	// update the in-memory patch
	patchFound := false
	for i, patch := range p.Patches {
		if patch.ModuleName == modulePatch.ModuleName {
			p.Patches[i] = modulePatch
			patchFound = true
			break
		}
	}
	if !patchFound {
		p.Patches = append(p.Patches, modulePatch)
	}

	// check that a patch for this module exists
	query := bson.M{
		IdKey:                                 p.Id,
		PatchesKey + "." + ModulePatchNameKey: modulePatch.ModuleName,
	}
	update := bson.M{PatchesKey + ".$": modulePatch}
	result, err := UpdateAll(ctx, query, bson.M{"$set": update})
	if err != nil {
		return err
	}
	// The patch already existed in the array, and it's been updated.
	if result.Updated > 0 {
		return nil
	}

	//it wasn't in the array, we need to add it.
	query = bson.M{IdKey: p.Id}
	update = bson.M{
		"$push": bson.M{PatchesKey: modulePatch},
	}
	return UpdateOne(ctx, query, update)
}

// RemoveModulePatch removes a module that's part of a patch request
func (p *Patch) RemoveModulePatch(ctx context.Context, moduleName string) error {
	// check that a patch for this module exists
	query := bson.M{
		IdKey: p.Id,
	}
	update := bson.M{
		"$pull": bson.M{
			PatchesKey: bson.M{ModulePatchNameKey: moduleName},
		},
	}
	return UpdateOne(ctx, query, update)
}

func (p *Patch) UpdateGithashProjectAndTasks(ctx context.Context) error {
	query := bson.M{
		IdKey: p.Id,
	}
	update := bson.M{
		"$set": bson.M{
			GithashKey:              p.Githash,
			PatchesKey:              p.Patches,
			ProjectStorageMethodKey: p.ProjectStorageMethod,
			PatchedProjectConfigKey: p.PatchedProjectConfig,
			VariantsTasksKey:        p.VariantsTasks,
			BuildVariantsKey:        p.BuildVariants,
			TasksKey:                p.Tasks,
		},
	}

	return UpdateOne(ctx, query, update)
}

func (p *Patch) IsGithubPRPatch() bool {
	return p.GithubPatchData.HeadOwner != ""
}

// IsMergeQueuePatch returns true if the the patch is part of GitHub's merge queue.
func (p *Patch) IsMergeQueuePatch() bool {
	return p.Alias == evergreen.CommitQueueAlias || p.GithubMergeData.HeadSHA != ""
}

func (p *Patch) IsChild() bool {
	return p.Triggers.ParentPatch != ""
}

// CollectiveStatus returns the aggregate display status of all tasks and child patches.
// If this is meant for display on the UI, we should also consider the display status aborted.
// NOTE that the result of this should not be compared against version statuses, because this can return display statuses.
func (p *Patch) CollectiveStatus(ctx context.Context) (string, error) {
	parentPatch := p
	if p.IsChild() {
		var err error
		parentPatch, err = FindOneId(ctx, p.Triggers.ParentPatch)
		if err != nil {
			return "", errors.Wrap(err, "getting parent patch")
		}
		if parentPatch == nil {
			return "", errors.Errorf("parent patch '%s' does not exist", p.Triggers.ParentPatch)
		}
	}
	allStatuses := []string{parentPatch.Status}
	for _, childPatchId := range parentPatch.Triggers.ChildPatches {
		cp, err := FindOneId(ctx, childPatchId)
		if err != nil {
			return "", errors.Wrapf(err, "getting child patch '%s' ", childPatchId)
		}
		if cp == nil {
			return "", errors.Wrapf(err, "child patch '%s' not found", childPatchId)
		}
		allStatuses = append(allStatuses, cp.Status)
	}

	return GetCollectiveStatusFromPatchStatuses(allStatuses), nil
}

func (p *Patch) IsParent() bool {
	return len(p.Triggers.ChildPatches) > 0
}

// ShouldPatchFileWithDiff returns true if the patch should read with diff
// (i.e. is not a PR patch) and the config has changed.
func (p *Patch) ShouldPatchFileWithDiff(path string) bool {
	return !p.IsGithubPRPatch() && p.ConfigChanged(path)
}

func (p *Patch) GetPatchIndex(parentPatch *Patch) (int, error) {
	if !p.IsChild() {
		return -1, nil
	}
	if parentPatch == nil {
		return -1, errors.New("parent patch does not exist")
	}
	siblings := parentPatch.Triggers.ChildPatches

	for index, patch := range siblings {
		if p.Id.Hex() == patch {
			return index, nil
		}
	}
	return -1, nil
}

// GetGithubContextForChildPatch returns the github context for the given child patch, to be used in github statuses.
func GetGithubContextForChildPatch(projectIdentifier string, parentPatch, childPatch *Patch) (string, error) {
	patchIndex, err := childPatch.GetPatchIndex(parentPatch)
	if err != nil {
		return "", errors.Wrap(err, "getting child patch index")
	}
	githubContext := fmt.Sprintf("evergreen/%s", projectIdentifier)
	// If there are multiple child patches, add the index to ensure these don't overlap,
	// since there can be multiple for the same child project.
	if patchIndex > 0 {
		githubContext = fmt.Sprintf("evergreen/%s/%d", projectIdentifier, patchIndex)
	}
	return githubContext, nil
}

func (p *Patch) GetFamilyInformation(ctx context.Context) (bool, *Patch, error) {
	if !p.IsChild() && !p.IsParent() {
		return evergreen.IsFinishedVersionStatus(p.Status), nil, nil
	}

	isDone := false
	childrenOrSiblings, parentPatch, err := p.GetPatchFamily(ctx)
	if err != nil {
		return isDone, parentPatch, errors.Wrap(err, "getting child or sibling patches")
	}

	// make sure the parent is done, if not, wait for the parent
	if p.IsChild() && !evergreen.IsFinishedVersionStatus(parentPatch.Status) {
		return isDone, parentPatch, nil
	}
	childrenStatus, err := GetChildrenOrSiblingsReadiness(ctx, childrenOrSiblings)
	if err != nil {
		return isDone, parentPatch, errors.Wrap(err, "getting child or sibling information")
	}
	if !evergreen.IsFinishedVersionStatus(childrenStatus) {
		return isDone, parentPatch, nil
	} else {
		isDone = true
	}

	return isDone, parentPatch, err
}

func GetChildrenOrSiblingsReadiness(ctx context.Context, childrenOrSiblings []string) (string, error) {
	if len(childrenOrSiblings) == 0 {
		return "", nil
	}
	childrenStatus := evergreen.VersionSucceeded
	for _, childPatch := range childrenOrSiblings {
		childPatchDoc, err := FindOneId(ctx, childPatch)
		if err != nil {
			return "", errors.Wrapf(err, "getting tasks for child patch '%s'", childPatch)
		}

		if childPatchDoc == nil {
			return "", errors.Errorf("child patch '%s' not found", childPatch)
		}
		if childPatchDoc.Status == evergreen.VersionFailed {
			childrenStatus = evergreen.VersionFailed
		}
		if !evergreen.IsFinishedVersionStatus(childPatchDoc.Status) {
			return childPatchDoc.Status, nil
		}
	}

	return childrenStatus, nil

}
func (p *Patch) GetPatchFamily(ctx context.Context) ([]string, *Patch, error) {
	var childrenOrSiblings []string
	var parentPatch *Patch
	var err error
	if p.IsParent() {
		parentPatch = p
		childrenOrSiblings = p.Triggers.ChildPatches
	}
	if p.IsChild() {
		parentPatchId := p.Triggers.ParentPatch
		parentPatch, err = FindOneId(ctx, parentPatchId)
		if err != nil {
			return nil, nil, errors.Wrap(err, "getting parent patch")
		}
		if parentPatch == nil {
			return nil, nil, errors.Errorf("parent patch '%s' does not exist", parentPatchId)
		}
		childrenOrSiblings = parentPatch.Triggers.ChildPatches
	}

	return childrenOrSiblings, parentPatch, nil
}

func (p *Patch) SetParametersFromParent(ctx context.Context) (*Patch, error) {
	parentPatchId := p.Triggers.ParentPatch
	parentPatch, err := FindOneId(ctx, parentPatchId)
	if err != nil {
		return nil, errors.Wrap(err, "getting parent patch")
	}
	if parentPatch == nil {
		return nil, errors.Errorf("parent patch '%s' does not exist", parentPatchId)
	}

	if downstreamParams := parentPatch.Triggers.DownstreamParameters; len(downstreamParams) > 0 {
		err = p.SetParameters(ctx, downstreamParams)
		if err != nil {
			return nil, errors.Wrap(err, "setting downstream parameters")
		}
	}
	return parentPatch, nil
}

// SetIsReconfigured sets whether the patch was reconfigured after it was
// already finalized.
func (p *Patch) SetIsReconfigured(ctx context.Context, isReconfigured bool) error {
	if p.IsReconfigured == isReconfigured {
		return nil
	}

	if err := UpdateOne(ctx, bson.M{IdKey: p.Id}, bson.M{
		"$set": bson.M{
			IsReconfiguredKey: isReconfigured,
		},
	}); err != nil {
		return err
	}

	p.IsReconfigured = isReconfigured
	return nil
}

func (p *Patch) GetRequester() string {
	if p.IsGithubPRPatch() {
		return evergreen.GithubPRRequester
	}
	if p.IsMergeQueuePatch() {
		return evergreen.GithubMergeRequester
	}
	return evergreen.PatchVersionRequester
}

func (p *Patch) HasValidGitInfo() bool {
	return p.GitInfo != nil && p.GitInfo.Email != "" && p.GitInfo.Username != ""
}

type PatchesByCreateTime []Patch

func (p PatchesByCreateTime) Len() int {
	return len(p)
}

func (p PatchesByCreateTime) Less(i, j int) bool {
	return p[i].CreateTime.Before(p[j].CreateTime)
}

func (p PatchesByCreateTime) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

// GetCollectiveStatusFromPatchStatuses answers the question of what the patch status should be
// when the patch status and the status of its children are different, given a list of statuses.
func GetCollectiveStatusFromPatchStatuses(statuses []string) string {
	hasCreated := false
	hasFailure := false
	hasSuccess := false
	hasAborted := false

	for _, s := range statuses {
		switch s {
		case evergreen.VersionStarted:
			return evergreen.VersionStarted
		case evergreen.VersionCreated:
			hasCreated = true
		case evergreen.VersionFailed:
			hasFailure = true
		case evergreen.VersionSucceeded:
			hasSuccess = true
		case evergreen.VersionAborted:
			// Note that we only consider this if the passed in statuses considered display status handling.
			hasAborted = true
		}
	}

	if !(hasCreated || hasFailure || hasSuccess || hasAborted) {
		grip.Critical(message.Fields{
			"message":  "An unknown patch status was found",
			"cause":    "Programmer error: new statuses should be added to GetCollectiveStatusFromPatchStatuses().",
			"statuses": statuses,
		})
	}

	if hasCreated && (hasFailure || hasSuccess) {
		return evergreen.VersionStarted
	} else if hasCreated {
		return evergreen.VersionCreated
	} else if hasFailure {
		return evergreen.VersionFailed
	} else if hasAborted {
		return evergreen.VersionAborted
	} else if hasSuccess {
		return evergreen.VersionSucceeded
	}
	return evergreen.VersionCreated
}
