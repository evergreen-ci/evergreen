package model

import (
	"fmt"
	"net/url"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// APIPatch is the model to be returned by the API whenever patches are fetched.
type APIPatch struct {
	Id                      *string              `json:"patch_id"`
	Description             *string              `json:"description"`
	ProjectId               *string              `json:"project_id"`
	ProjectIdentifier       *string              `json:"project_identifier"`
	Branch                  *string              `json:"branch"`
	Githash                 *string              `json:"git_hash"`
	PatchNumber             int                  `json:"patch_number"`
	Author                  *string              `json:"author"`
	Version                 *string              `json:"version"`
	Status                  *string              `json:"status"`
	CreateTime              *time.Time           `json:"create_time"`
	StartTime               *time.Time           `json:"start_time"`
	FinishTime              *time.Time           `json:"finish_time"`
	Variants                []*string            `json:"builds"`
	Tasks                   []*string            `json:"tasks"`
	DownstreamTasks         []DownstreamTasks    `json:"downstream_tasks"`
	VariantsTasks           []VariantTask        `json:"variants_tasks"`
	Activated               bool                 `json:"activated"`
	Alias                   *string              `json:"alias,omitempty"`
	GithubPatchData         githubPatch          `json:"github_patch_data,omitempty"`
	ModuleCodeChanges       []APIModulePatch     `json:"module_code_changes"`
	Parameters              []APIParameter       `json:"parameters"`
	PatchedParserProject    *string              `json:"patched_config"`
	CanEnqueueToCommitQueue bool                 `json:"can_enqueue_to_commit_queue"`
	ChildPatches            []APIPatch           `json:"child_patches"`
	ChildPatchAliases       []APIChildPatchAlias `json:"child_patch_aliases,omitempty"`
	Requester               *string              `json:"requester"`
	MergedFrom              *string              `json:"merged_from"`
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
	Name  *string   `json:"name"`
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

// ToService converts a service layer parameter using the data from APIParameter
func (p *APIParameter) ToService() patch.Parameter {
	res := patch.Parameter{}
	res.Key = utility.FromStringPtr(p.Key)
	res.Value = utility.FromStringPtr(p.Value)
	return res
}

// BuildFromService converts from service level structs to an APIPatch
func (apiPatch *APIPatch) BuildFromService(p patch.Patch) error {
	apiPatch.Id = utility.ToStringPtr(p.Id.Hex())
	apiPatch.Description = utility.ToStringPtr(p.Description)
	apiPatch.ProjectId = utility.ToStringPtr(p.Project)
	apiPatch.Branch = utility.ToStringPtr(p.Project)
	apiPatch.Githash = utility.ToStringPtr(p.Githash)
	apiPatch.PatchNumber = p.PatchNumber
	apiPatch.Author = utility.ToStringPtr(p.Author)
	apiPatch.Version = utility.ToStringPtr(p.Version)
	apiPatch.Status = utility.ToStringPtr(p.Status)
	apiPatch.CreateTime = ToTimePtr(p.CreateTime)
	apiPatch.StartTime = ToTimePtr(p.StartTime)
	apiPatch.FinishTime = ToTimePtr(p.FinishTime)
	apiPatch.MergedFrom = utility.ToStringPtr(p.MergedFrom)
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
	for _, vt := range p.VariantsTasks {
		vtasks := make([]*string, 0)
		for _, task := range vt.Tasks {
			vtasks = append(vtasks, utility.ToStringPtr(task))
		}
		variantTasks = append(variantTasks, VariantTask{
			Name:  utility.ToStringPtr(vt.Variant),
			Tasks: vtasks,
		})
	}
	apiPatch.VariantsTasks = variantTasks
	apiPatch.Activated = p.Activated
	apiPatch.Alias = utility.ToStringPtr(p.Alias)
	apiPatch.GithubPatchData = githubPatch{}
	apiPatch.Requester = utility.ToStringPtr(p.GetRequester())

	if p.Parameters != nil {
		apiPatch.Parameters = []APIParameter{}
		for _, param := range p.Parameters {
			apiPatch.Parameters = append(apiPatch.Parameters, APIParameter{
				Key:   utility.ToStringPtr(param.Key),
				Value: utility.ToStringPtr(param.Value),
			})
		}
	}

	projectIdentifier := p.Project
	if p.Project != "" {
		identifier, err := model.GetIdentifierForProject(p.Project)
		if err != nil {
			return errors.Wrapf(err, "getting project '%s'", p.Project)
		}
		apiPatch.ProjectIdentifier = utility.ToStringPtr(identifier)
		projectIdentifier = identifier
	}

	if env := evergreen.GetEnvironment(); env != nil {
		codeChanges := []APIModulePatch{}
		apiURL := env.Settings().ApiUrl

		for patchNumber, modPatch := range p.Patches {
			branchName := modPatch.ModuleName
			if branchName == "" {
				branchName = projectIdentifier
			}
			htmlLink := fmt.Sprintf("%s/filediff/%s?patch_number=%d", apiURL, *apiPatch.Id, patchNumber)
			rawLink := fmt.Sprintf("%s/rawdiff/%s?patch_number=%d", apiURL, *apiPatch.Id, patchNumber)
			fileDiffs := []FileDiff{}
			for _, file := range modPatch.PatchSet.Summary {
				diffLink := fmt.Sprintf("%s/filediff/%s?file_name=%s&patch_number=%d", apiURL, *apiPatch.Id, url.QueryEscape(file.Name), patchNumber)
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

	apiPatch.PatchedParserProject = utility.ToStringPtr(p.PatchedParserProject)
	apiPatch.CanEnqueueToCommitQueue = p.HasValidGitInfo()

	downstreamTasks, childPatches, err := getChildPatchesData(p)
	if err != nil {
		return errors.Wrap(err, "getting downstream tasks")
	}
	apiPatch.DownstreamTasks = downstreamTasks
	apiPatch.ChildPatches = childPatches

	// set the patch status to the collective status between the parent and child patches
	// Also correlate each child patch ID with the alias that invoked it
	if len(childPatches) > 0 {
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
		apiPatch.Status = utility.ToStringPtr(patch.GetCollectiveStatus(allStatuses))
		apiPatch.ChildPatchAliases = childPatchAliases

	}
	apiPatch.GithubPatchData.BuildFromService(p.GithubPatchData)
	return nil
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
		err = apiPatch.BuildFromService(childPatch)
		if err != nil {
			return nil, nil, errors.Wrap(err, "converting child patch to API model")
		}
		downstreamTasks = append(downstreamTasks, dt)
		apiChildPatches = append(apiChildPatches, apiPatch)
	}
	return downstreamTasks, apiChildPatches, nil
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

	res.GithubPatchData = apiPatch.GithubPatchData.ToService()
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
