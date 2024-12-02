package model

import (
	"fmt"
	"net/url"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// APIPatch is the model to be returned by the API whenever patches are fetched.
type APIPatch struct {
	// Unique identifier of a specific patch
	Id *string `json:"patch_id"`
	// Description of the patch
	Description *string `json:"description"`
	// Name of the project
	ProjectId         *string `json:"project_id"`
	ProjectIdentifier *string `json:"project_identifier"`
	// The branch on which the patch was initiated
	Branch *string `json:"branch"`
	// Hash of commit off which the patch was initiated
	Githash *string `json:"git_hash"`
	// Incrementing counter of user's patches
	PatchNumber int  `json:"patch_number"`
	Hidden      bool `json:"hidden"`
	// Author of the patch
	Author  *string `json:"author"`
	Version *string `json:"version"`
	// Status of patch (possible values are "created", "started", "success", or "failed")
	Status *string `json:"status"`
	// Time patch was created
	CreateTime *time.Time `json:"create_time"`
	// Time patch started to run
	StartTime *time.Time `json:"start_time"`
	// Time at patch completion
	FinishTime *time.Time `json:"finish_time"`
	// List of identifiers of builds to run for this patch
	Variants []*string `json:"builds"`
	// List of identifiers of tasks used in this patch
	Tasks           []*string         `json:"tasks"`
	DownstreamTasks []DownstreamTasks `json:"downstream_tasks"`
	// List of documents of available tasks and associated build variant
	VariantsTasks []VariantTask `json:"variants_tasks"`
	// Whether the patch has been finalized and activated
	Activated            bool                 `json:"activated"`
	Alias                *string              `json:"alias,omitempty"`
	GithubPatchData      githubPatch          `json:"github_patch_data,omitempty"`
	ModuleCodeChanges    []APIModulePatch     `json:"module_code_changes"`
	Parameters           []APIParameter       `json:"parameters"`
	ProjectStorageMethod *string              `json:"project_storage_method,omitempty"`
	ChildPatches         []APIPatch           `json:"child_patches"`
	ChildPatchAliases    []APIChildPatchAlias `json:"child_patch_aliases,omitempty"`
	Requester            *string              `json:"requester"`
	MergedFrom           *string              `json:"merged_from"`
	// Only populated for commit queue patches: returns the 0-indexed position of the patch on the queue, or -1 if not on the queue anymore
	CommitQueuePosition *int `json:"commit_queue_position,omitempty"`
}

type DownstreamTasks struct {
	Project      *string       `json:"project"`
	Tasks        []*string     `json:"tasks"`
	VariantTasks []VariantTask `json:"variant_tasks"`
}

type ChildPatch struct {
	Project *string `json:"project"`
	PatchID *string `json:"patch_id"`
	Status  *string `json:"status"`
}

type VariantTask struct {
	// Name of build variant
	Name *string `json:"name"`
	// All tasks available to run on this build variant
	Tasks []*string `json:"tasks"`
}

type FileDiff struct {
	FileName    *string `json:"file_name"`
	Additions   int     `json:"additions"`
	Deletions   int     `json:"deletions"`
	DiffLink    *string `json:"diff_link"`
	Description string  `json:"description"`
}

type APIChildPatchAlias struct {
	Alias   *string `json:"alias"`
	PatchID *string `json:"patch_id"`
}

type APIModulePatch struct {
	BranchName     *string    `json:"branch_name"`
	HTMLLink       *string    `json:"html_link"`
	RawLink        *string    `json:"raw_link"`
	CommitMessages []*string  `json:"commit_messages"`
	FileDiffs      []FileDiff `json:"file_diffs"`
}

type APIParameter struct {
	Key   *string `json:"key"`
	Value *string `json:"value"`
}

// APIRawPatch contains a patch diff along with its module diffs.
type APIRawPatch struct {
	// The main patch
	Patch APIRawModule `json:"patch"`
	// The list of module diffs
	RawModules []APIRawModule `json:"raw_modules"`
}

// APIRawModule contains a module diff.
type APIRawModule struct {
	// The module name
	Name string `json:"name"`
	// The module diff
	Diff string `json:"diff"`
	// The githash for the module
	Githash string `json:"githash"`
}

// ToService converts a service layer parameter using the data from APIParameter
func (p *APIParameter) ToService() patch.Parameter {
	res := patch.Parameter{}
	res.Key = utility.FromStringPtr(p.Key)
	res.Value = utility.FromStringPtr(p.Value)
	return res
}

// BuildFromService converts from service level parameter to an APIPatch.
func (p *APIParameter) BuildFromService(param *patch.Parameter) {
	p.Key = utility.ToStringPtr(param.Key)
	p.Value = utility.ToStringPtr(param.Value)
}

type APIPatchArgs struct {
	IncludeProjectIdentifier   bool
	IncludeCommitQueuePosition bool
	IncludeChildPatches        bool
}

// BuildFromService converts from service level structs to an APIPatch.
// An APIPatch expects the VariantTasks to be populated with only non-execution tasks and display tasks.
// If args are set, includes identifier, commit queue position, and/or child patches from the DB, if applicable.
func (apiPatch *APIPatch) BuildFromService(p patch.Patch, args *APIPatchArgs) error {
	apiPatch.buildBasePatch(p)

	projectIdentifier := p.Project
	if args != nil {
		if args.IncludeProjectIdentifier && p.Project != "" {
			apiPatch.GetIdentifier()
			if apiPatch.ProjectIdentifier != nil {
				projectIdentifier = utility.FromStringPtr(apiPatch.ProjectIdentifier)
			}

		}

		if args.IncludeCommitQueuePosition {
			if err := apiPatch.GetCommitQueuePosition(); err != nil {
				return errors.Wrap(err, "getting commit queue position")
			}
		}
	}
	apiPatch.buildModuleChanges(p, projectIdentifier)

	if args != nil && args.IncludeChildPatches {
		return apiPatch.buildChildPatches(p)
	}
	return nil
}

func (apiPatch *APIPatch) GetCommitQueuePosition() error {
	if apiPatch.CommitQueuePosition != nil {
		return nil
	}
	// GitHub merge patches use the commit queue alias but do not have commit queue positions.
	if utility.FromStringPtr(apiPatch.Requester) == evergreen.GithubMergeRequester {
		return nil
	}
	if utility.FromStringPtr(apiPatch.Alias) == evergreen.CommitQueueAlias {
		cq, err := commitqueue.FindOneId(utility.FromStringPtr(apiPatch.ProjectId))
		if err != nil {
			return errors.Wrap(err, "error getting commit queue position")
		}
		apiPatch.CommitQueuePosition = utility.ToIntPtr(-1)
		if cq != nil {
			apiPatch.CommitQueuePosition = utility.ToIntPtr(cq.FindItem(utility.FromStringPtr(apiPatch.Id)))
		}
	}
	return nil
}

func (apiPatch *APIPatch) GetIdentifier() {
	if utility.FromStringPtr(apiPatch.ProjectIdentifier) != "" {
		return
	}
	if utility.FromStringPtr(apiPatch.ProjectId) != "" {
		identifier, err := model.GetIdentifierForProject(utility.FromStringPtr(apiPatch.ProjectId))

		grip.Error(message.WrapError(err, message.Fields{
			"message": "could not get identifier for project",
			"project": apiPatch.ProjectId,
		}))

		if err == nil && identifier != "" {
			apiPatch.ProjectIdentifier = utility.ToStringPtr(identifier)
		}
	}
}

func (apiPatch *APIPatch) buildBasePatch(p patch.Patch) {
	apiPatch.Id = utility.ToStringPtr(p.Id.Hex())
	apiPatch.Description = utility.ToStringPtr(p.Description)
	apiPatch.ProjectId = utility.ToStringPtr(p.Project)
	apiPatch.Branch = utility.ToStringPtr(p.Project)
	apiPatch.Githash = utility.ToStringPtr(p.Githash)
	apiPatch.PatchNumber = p.PatchNumber
	apiPatch.Author = utility.ToStringPtr(p.Author)
	apiPatch.Version = utility.ToStringPtr(p.Version)
	apiPatch.Hidden = p.Hidden
	apiPatch.CreateTime = ToTimePtr(p.CreateTime)
	apiPatch.StartTime = ToTimePtr(p.StartTime)
	apiPatch.FinishTime = ToTimePtr(p.FinishTime)
	apiPatch.MergedFrom = utility.ToStringPtr(p.MergedFrom)
	apiPatch.Status = utility.ToStringPtr(p.Status)
	builds := make([]*string, 0)
	for _, b := range p.BuildVariants {
		builds = append(builds, utility.ToStringPtr(b))
	}
	apiPatch.Variants = builds
	tasks := make([]*string, 0)
	for _, t := range p.Tasks {
		tasks = append(tasks, utility.ToStringPtr(t))
	}
	apiPatch.Tasks = tasks
	variantTasks := []VariantTask{}

	// We remove the execution tasks from selected display tasks to avoid duplication.
	execTasksToRemove := []string{}
	for _, vt := range p.VariantsTasks {
		vtasks := make([]*string, 0)
		for _, task := range vt.Tasks {
			vtasks = append(vtasks, utility.ToStringPtr(task))
		}
		for _, task := range vt.DisplayTasks {
			vtasks = append(vtasks, utility.ToStringPtr(task.Name))
			execTasksToRemove = append(execTasksToRemove, task.ExecTasks...)
		}
		variantTasks = append(variantTasks, VariantTask{
			Name:  utility.ToStringPtr(vt.Variant),
			Tasks: vtasks,
		})
	}
	for i, vt := range variantTasks {
		tasks := []*string{}
		for j, t := range vt.Tasks {
			keepTask := true
			for _, task := range execTasksToRemove {
				if utility.FromStringPtr(t) == task {
					keepTask = false
					break
				}
			}
			if keepTask {
				tasks = append(tasks, vt.Tasks[j])
			}
		}
		variantTasks[i].Tasks = tasks
	}
	apiPatch.VariantsTasks = variantTasks
	apiPatch.Activated = p.Activated
	apiPatch.Alias = utility.ToStringPtr(p.Alias)
	apiPatch.GithubPatchData = githubPatch{}
	apiPatch.Requester = utility.ToStringPtr(p.GetRequester())

	if p.Parameters != nil {
		apiPatch.Parameters = []APIParameter{}
		for _, param := range p.Parameters {
			APIParam := APIParameter{}
			APIParam.BuildFromService(&param)
			apiPatch.Parameters = append(apiPatch.Parameters, APIParam)
		}
	}

	apiPatch.ProjectStorageMethod = utility.ToStringPtr(string(p.ProjectStorageMethod))
	apiPatch.GithubPatchData.BuildFromService(p.GithubPatchData)
}

func getChildPatchesData(p patch.Patch) ([]DownstreamTasks, []APIPatch, error) {
	if len(p.Triggers.ChildPatches) <= 0 {
		return nil, nil, nil
	}
	childPatches, err := patch.Find(patch.ByStringIds(p.Triggers.ChildPatches))
	if err != nil {
		return nil, nil, errors.Wrap(err, "getting child patches")
	}
	downstreamTasks := []DownstreamTasks{}
	apiChildPatches := []APIPatch{}
	for _, childPatch := range childPatches {
		tasks := utility.ToStringPtrSlice(childPatch.Tasks)
		variantTasks := []VariantTask{}
		for _, vt := range childPatch.VariantsTasks {
			vtasks := make([]*string, 0)
			for _, task := range vt.Tasks {
				vtasks = append(vtasks, utility.ToStringPtr(task))
			}
			variantTasks = append(variantTasks, VariantTask{
				Name:  utility.ToStringPtr(vt.Variant),
				Tasks: vtasks,
			})
		}

		dt := DownstreamTasks{
			Project:      utility.ToStringPtr(childPatch.Project),
			Tasks:        tasks,
			VariantTasks: variantTasks,
		}
		apiPatch := APIPatch{}
		err = apiPatch.BuildFromService(childPatch, &APIPatchArgs{IncludeProjectIdentifier: true})
		if err != nil {
			return nil, nil, errors.Wrap(err, "converting child patch to API model")
		}
		downstreamTasks = append(downstreamTasks, dt)
		apiChildPatches = append(apiChildPatches, apiPatch)
	}
	return downstreamTasks, apiChildPatches, nil
}

func (apiPatch *APIPatch) buildChildPatches(p patch.Patch) error {
	downstreamTasks, childPatches, err := getChildPatchesData(p)
	if err != nil {
		return errors.Wrap(err, "getting downstream tasks")
	}
	apiPatch.DownstreamTasks = downstreamTasks
	apiPatch.ChildPatches = childPatches
	if len(childPatches) == 0 {
		return nil
	}
	// set the patch status to the collective status between the parent and child patches
	// Also correlate each child patch ID with the alias that invoked it
	allStatuses := []string{*apiPatch.Status}
	childPatchAliases := []APIChildPatchAlias{}
	for i, cp := range childPatches {
		allStatuses = append(allStatuses, *cp.Status)

		if i < len(p.Triggers.Aliases) {
			childPatchAlias := APIChildPatchAlias{
				Alias:   utility.ToStringPtr(p.Triggers.Aliases[i]),
				PatchID: utility.ToStringPtr(*cp.Id),
			}
			childPatchAliases = append(childPatchAliases, childPatchAlias)
		}
	}
	apiPatch.Status = utility.ToStringPtr(patch.GetCollectiveStatusFromPatchStatuses(allStatuses))
	apiPatch.ChildPatchAliases = childPatchAliases

	return nil
}

func (apiPatch *APIPatch) buildModuleChanges(p patch.Patch, identifier string) {
	env := evergreen.GetEnvironment()
	if env == nil {
		return
	}
	codeChanges := []APIModulePatch{}
	apiURL := env.Settings().Api.URL

	for patchNumber, modPatch := range p.Patches {
		branchName := modPatch.ModuleName
		if branchName == "" {
			branchName = identifier
		}
		htmlLink := fmt.Sprintf("%s/filediff/%s?patch_number=%d", apiURL, *apiPatch.Id, patchNumber)
		rawLink := fmt.Sprintf("%s/rawdiff/%s?patch_number=%d", apiURL, *apiPatch.Id, patchNumber)
		fileDiffs := []FileDiff{}
		for i, file := range modPatch.PatchSet.Summary {
			diffLink := fmt.Sprintf("%s/filediff/%s?file_name=%s&patch_number=%d&commit_number=%d", apiURL, *apiPatch.Id, url.QueryEscape(file.Name), patchNumber, i)
			fileName := file.Name
			fileDiff := FileDiff{
				FileName:    &fileName,
				Additions:   file.Additions,
				Deletions:   file.Deletions,
				DiffLink:    &diffLink,
				Description: file.Description,
			}
			fileDiffs = append(fileDiffs, fileDiff)
		}
		apiModPatch := APIModulePatch{
			BranchName:     &branchName,
			HTMLLink:       &htmlLink,
			RawLink:        &rawLink,
			FileDiffs:      fileDiffs,
			CommitMessages: utility.ToStringPtrSlice(modPatch.PatchSet.CommitMessages),
		}
		codeChanges = append(codeChanges, apiModPatch)
	}

	apiPatch.ModuleCodeChanges = codeChanges
}

// ToService converts a service layer patch using the data from APIPatch
func (apiPatch *APIPatch) ToService() (patch.Patch, error) {
	var err error
	res := patch.Patch{}
	catcher := grip.NewBasicCatcher()
	res.Id = bson.ObjectIdHex(utility.FromStringPtr(apiPatch.Id))
	res.Description = utility.FromStringPtr(apiPatch.Description)
	res.Project = utility.FromStringPtr(apiPatch.ProjectId)
	res.Githash = utility.FromStringPtr(apiPatch.Githash)
	res.PatchNumber = apiPatch.PatchNumber
	res.Hidden = apiPatch.Hidden
	res.Author = utility.FromStringPtr(apiPatch.Author)
	res.Version = utility.FromStringPtr(apiPatch.Version)
	res.Status = utility.FromStringPtr(apiPatch.Status)
	res.Alias = utility.FromStringPtr(apiPatch.Alias)
	res.Activated = apiPatch.Activated
	res.CreateTime, err = FromTimePtr(apiPatch.CreateTime)
	catcher.Add(err)
	res.StartTime, err = FromTimePtr(apiPatch.StartTime)
	catcher.Add(err)
	res.FinishTime, err = FromTimePtr(apiPatch.FinishTime)
	catcher.Add(err)

	builds := make([]string, len(apiPatch.Variants))
	for _, b := range apiPatch.Variants {
		builds = append(builds, utility.FromStringPtr(b))
	}

	res.BuildVariants = builds
	tasks := make([]string, len(apiPatch.Tasks))
	for i, t := range apiPatch.Tasks {
		tasks[i] = utility.FromStringPtr(t)
	}
	res.Tasks = tasks
	if apiPatch.Parameters != nil {
		res.Parameters = []patch.Parameter{}
		for _, param := range apiPatch.Parameters {
			res.Parameters = append(res.Parameters, patch.Parameter{
				Key:   utility.FromStringPtr(param.Key),
				Value: utility.FromStringPtr(param.Value),
			})
		}
	}
	variantTasks := []patch.VariantTasks{}
	for _, vt := range apiPatch.VariantsTasks {
		vtasks := make([]string, 0)
		for _, task := range vt.Tasks {
			vtasks = append(vtasks, utility.FromStringPtr(task))
		}
		variantTasks = append(variantTasks, patch.VariantTasks{
			Variant: utility.FromStringPtr(vt.Name),
			Tasks:   vtasks,
		})
	}
	res.VariantsTasks = variantTasks

	res.GithubPatchData = apiPatch.GithubPatchData.ToService()
	res.ProjectStorageMethod = evergreen.ParserProjectStorageMethod(utility.FromStringPtr(apiPatch.ProjectStorageMethod))
	return res, catcher.Resolve()
}

type githubPatch struct {
	PRNumber  int     `json:"pr_number"`
	BaseOwner *string `json:"base_owner"`
	BaseRepo  *string `json:"base_repo"`
	HeadOwner *string `json:"head_owner"`
	HeadRepo  *string `json:"head_repo"`
	HeadHash  *string `json:"head_hash"`
	Author    *string `json:"author"`
}

// BuildFromService converts from service level structs to an APIPatch
func (g *githubPatch) BuildFromService(p thirdparty.GithubPatch) {
	g.PRNumber = p.PRNumber
	g.BaseOwner = utility.ToStringPtr(p.BaseOwner)
	g.BaseRepo = utility.ToStringPtr(p.BaseRepo)
	g.HeadOwner = utility.ToStringPtr(p.HeadOwner)
	g.HeadRepo = utility.ToStringPtr(p.HeadRepo)
	g.HeadHash = utility.ToStringPtr(p.HeadHash)
	g.Author = utility.ToStringPtr(p.Author)
}

// ToService converts a service layer patch using the data from APIPatch
func (g *githubPatch) ToService() thirdparty.GithubPatch {
	res := thirdparty.GithubPatch{}
	res.PRNumber = g.PRNumber
	res.BaseOwner = utility.FromStringPtr(g.BaseOwner)
	res.BaseRepo = utility.FromStringPtr(g.BaseRepo)
	res.HeadOwner = utility.FromStringPtr(g.HeadOwner)
	res.HeadRepo = utility.FromStringPtr(g.HeadRepo)
	res.HeadHash = utility.FromStringPtr(g.HeadHash)
	res.Author = utility.FromStringPtr(g.Author)
	return res
}
