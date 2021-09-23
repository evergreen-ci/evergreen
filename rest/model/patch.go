package model

import (
	"fmt"
	"net/url"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
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
	PatchedConfig           *string              `json:"patched_config"`
	CanEnqueueToCommitQueue bool                 `json:"can_enqueue_to_commit_queue"`
	ChildPatches            []APIPatch           `json:"child_patches"`
	ChildPatchAliases       []APIChildPatchAlias `json:"child_patch_aliases,omitempty"`
	Requester               *string              `json:"requester"`
}

type DownstreamTasks struct {
	Project *string   `json:"project"`
	Tasks   []*string `json:"tasks"`
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
func (apiPatch *APIPatch) BuildFromService(h interface{}) error {
	v, ok := h.(patch.Patch)
	if !ok {
		return errors.New("incorrect type when fetching converting patch type")
	}
	apiPatch.Id = utility.ToStringPtr(v.Id.Hex())
	apiPatch.Description = utility.ToStringPtr(v.Description)
	apiPatch.ProjectId = utility.ToStringPtr(v.Project)
	apiPatch.Branch = utility.ToStringPtr(v.Project)
	apiPatch.Githash = utility.ToStringPtr(v.Githash)
	apiPatch.PatchNumber = v.PatchNumber
	apiPatch.Author = utility.ToStringPtr(v.Author)
	apiPatch.Version = utility.ToStringPtr(v.Version)
	apiPatch.Status = utility.ToStringPtr(v.Status)
	apiPatch.CreateTime = ToTimePtr(v.CreateTime)
	apiPatch.StartTime = ToTimePtr(v.StartTime)
	apiPatch.FinishTime = ToTimePtr(v.FinishTime)
	builds := make([]*string, 0)
	for _, b := range v.BuildVariants {
		builds = append(builds, utility.ToStringPtr(b))
	}
	apiPatch.Variants = builds
	tasks := make([]*string, 0)
	for _, t := range v.Tasks {
		tasks = append(tasks, utility.ToStringPtr(t))
	}
	apiPatch.Tasks = tasks
	variantTasks := []VariantTask{}
	for _, vt := range v.VariantsTasks {
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
	apiPatch.Activated = v.Activated
	apiPatch.Alias = utility.ToStringPtr(v.Alias)
	apiPatch.GithubPatchData = githubPatch{}
	apiPatch.Requester = utility.ToStringPtr(v.GetRequester())

	if v.Parameters != nil {
		apiPatch.Parameters = []APIParameter{}
		for _, param := range v.Parameters {
			apiPatch.Parameters = append(apiPatch.Parameters, APIParameter{
				Key:   utility.ToStringPtr(param.Key),
				Value: utility.ToStringPtr(param.Value),
			})
		}
	}

	if env := evergreen.GetEnvironment(); env != nil {
		codeChanges := []APIModulePatch{}
		apiURL := env.Settings().ApiUrl

		for patchNumber, modPatch := range v.Patches {
			branchName := modPatch.ModuleName
			if branchName == "" {
				branchName = v.Project
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

	apiPatch.PatchedConfig = utility.ToStringPtr(v.PatchedConfig)
	apiPatch.CanEnqueueToCommitQueue = v.HasValidGitInfo()

	downstreamTasks, childPatches, err := getChildPatchesData(v)
	if err != nil {
		return errors.Wrap(err, "error getting downstream tasks")
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

			if i < len(v.Triggers.Aliases) {
				childPatchAlias := APIChildPatchAlias{
					Alias:   utility.ToStringPtr(v.Triggers.Aliases[i]),
					PatchID: utility.ToStringPtr(*cp.Id),
				}
				childPatchAliases = append(childPatchAliases, childPatchAlias)
			}
		}
		apiPatch.Status = utility.ToStringPtr(patch.GetCollectiveStatus(allStatuses))
		apiPatch.ChildPatchAliases = childPatchAliases

	}

	if v.Project != "" {
		identifier, err := model.GetIdentifierForProject(v.Project)
		if err != nil {
			return errors.Wrapf(err, "error getting project '%s'", v.Project)
		}
		apiPatch.ProjectIdentifier = utility.ToStringPtr(identifier)
	}

	return errors.WithStack(apiPatch.GithubPatchData.BuildFromService(v.GithubPatchData))
}

func getChildPatchesData(p patch.Patch) ([]DownstreamTasks, []APIPatch, error) {
	if len(p.Triggers.ChildPatches) <= 0 {
		return nil, nil, nil
	}
	downstreamTasks := []DownstreamTasks{}
	childPatches := []APIPatch{}
	for _, childPatch := range p.Triggers.ChildPatches {
		childPatchDoc, err := patch.FindOneId(childPatch)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "error getting child patch '%s'", childPatch)
		}
		if childPatchDoc == nil {
			continue
		}

		tasks := utility.ToStringPtrSlice(childPatchDoc.Tasks)

		dt := DownstreamTasks{
			Project: utility.ToStringPtr(childPatchDoc.Project),
			Tasks:   tasks,
		}
		apiPatch := APIPatch{}
		err = apiPatch.BuildFromService(*childPatchDoc)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "error building child patch from service '%s'", childPatch)
		}
		downstreamTasks = append(downstreamTasks, dt)
		childPatches = append(childPatches, apiPatch)
	}
	return downstreamTasks, childPatches, nil
}

// ToService converts a service layer patch using the data from APIPatch
func (apiPatch *APIPatch) ToService() (interface{}, error) {
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

	i, err := apiPatch.GithubPatchData.ToService()
	catcher.Add(err)
	data, ok := i.(thirdparty.GithubPatch)
	if !ok {
		catcher.Add(errors.New("cannot resolve patch data"))
	}
	res.GithubPatchData = data
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
func (g *githubPatch) BuildFromService(h interface{}) error {
	v, ok := h.(thirdparty.GithubPatch)
	if !ok {
		return errors.New("incorrect type when fetching converting github patch type")
	}
	g.PRNumber = v.PRNumber
	g.BaseOwner = utility.ToStringPtr(v.BaseOwner)
	g.BaseRepo = utility.ToStringPtr(v.BaseRepo)
	g.HeadOwner = utility.ToStringPtr(v.HeadOwner)
	g.HeadRepo = utility.ToStringPtr(v.HeadRepo)
	g.HeadHash = utility.ToStringPtr(v.HeadHash)
	g.Author = utility.ToStringPtr(v.Author)
	return nil
}

// ToService converts a service layer patch using the data from APIPatch
func (g *githubPatch) ToService() (interface{}, error) {
	res := thirdparty.GithubPatch{}
	res.PRNumber = g.PRNumber
	res.BaseOwner = utility.FromStringPtr(g.BaseOwner)
	res.BaseRepo = utility.FromStringPtr(g.BaseRepo)
	res.HeadOwner = utility.FromStringPtr(g.HeadOwner)
	res.HeadRepo = utility.FromStringPtr(g.HeadRepo)
	res.HeadHash = utility.FromStringPtr(g.HeadHash)
	res.Author = utility.FromStringPtr(g.Author)
	return res, nil
}
