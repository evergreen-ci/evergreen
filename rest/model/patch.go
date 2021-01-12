package model

import (
	"fmt"
	"net/url"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

// APIPatch is the model to be returned by the API whenever patches are fetched.
type APIPatch struct {
	Id                      *string          `json:"patch_id"`
	Description             *string          `json:"description"`
	ProjectId               *string          `json:"project_id"`
	Branch                  *string          `json:"branch"`
	Githash                 *string          `json:"git_hash"`
	PatchNumber             int              `json:"patch_number"`
	Author                  *string          `json:"author"`
	Version                 *string          `json:"version"`
	Status                  *string          `json:"status"`
	CreateTime              *time.Time       `json:"create_time"`
	StartTime               *time.Time       `json:"start_time"`
	FinishTime              *time.Time       `json:"finish_time"`
	Variants                []*string        `json:"builds"`
	Tasks                   []*string        `json:"tasks"`
	VariantsTasks           []VariantTask    `json:"variants_tasks"`
	Activated               bool             `json:"activated"`
	Alias                   *string          `json:"alias,omitempty"`
	GithubPatchData         githubPatch      `json:"github_patch_data,omitempty"`
	ModuleCodeChanges       []APIModulePatch `json:"module_code_changes"`
	Parameters              []APIParameter   `json:"parameters"`
	PatchedConfig           *string          `json:"patched_config"`
	Project                 *string          `json:"project"`
	CanEnqueueToCommitQueue bool             `json:"can_enqueue_to_commit_queue"`
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

type APIModulePatch struct {
	BranchName *string    `json:"branch_name"`
	HTMLLink   *string    `json:"html_link"`
	RawLink    *string    `json:"raw_link"`
	FileDiffs  []FileDiff `json:"file_diffs"`
}

type APIParameter struct {
	Key   *string `json:"key"`
	Value *string `json:"value"`
}

// ToService converts a service layer parameter using the data from APIParameter
func (p *APIParameter) ToService() patch.Parameter {
	res := patch.Parameter{}
	res.Key = FromStringPtr(p.Key)
	res.Value = FromStringPtr(p.Value)
	return res
}

// BuildFromService converts from service level structs to an APIPatch
func (apiPatch *APIPatch) BuildFromService(h interface{}) error {
	v, ok := h.(patch.Patch)
	if !ok {
		return errors.New("incorrect type when fetching converting patch type")
	}
	apiPatch.Id = ToStringPtr(v.Id.Hex())
	apiPatch.Description = ToStringPtr(v.Description)
	apiPatch.ProjectId = ToStringPtr(v.Project)
	apiPatch.Branch = ToStringPtr(v.Project)
	apiPatch.Githash = ToStringPtr(v.Githash)
	apiPatch.PatchNumber = v.PatchNumber
	apiPatch.Author = ToStringPtr(v.Author)
	apiPatch.Version = ToStringPtr(v.Version)
	apiPatch.Status = ToStringPtr(v.Status)
	apiPatch.CreateTime = ToTimePtr(v.CreateTime)
	apiPatch.StartTime = ToTimePtr(v.StartTime)
	apiPatch.FinishTime = ToTimePtr(v.FinishTime)
	builds := make([]*string, 0)
	for _, b := range v.BuildVariants {
		builds = append(builds, ToStringPtr(b))
	}
	apiPatch.Variants = builds
	tasks := make([]*string, 0)
	for _, t := range v.Tasks {
		tasks = append(tasks, ToStringPtr(t))
	}
	apiPatch.Tasks = tasks
	variantTasks := []VariantTask{}
	for _, vt := range v.VariantsTasks {
		vtasks := make([]*string, 0)
		for _, task := range vt.Tasks {
			vtasks = append(vtasks, ToStringPtr(task))
		}
		variantTasks = append(variantTasks, VariantTask{
			Name:  ToStringPtr(vt.Variant),
			Tasks: vtasks,
		})
	}
	apiPatch.VariantsTasks = variantTasks
	apiPatch.Activated = v.Activated
	apiPatch.Alias = ToStringPtr(v.Alias)
	apiPatch.GithubPatchData = githubPatch{}

	if len(v.Parameters) > 0 {
		apiPatch.Parameters = []APIParameter{}
		for _, param := range v.Parameters {
			apiPatch.Parameters = append(apiPatch.Parameters, APIParameter{
				Key:   ToStringPtr(param.Key),
				Value: ToStringPtr(param.Value),
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
				BranchName: &branchName,
				HTMLLink:   &htmlLink,
				RawLink:    &rawLink,
				FileDiffs:  fileDiffs,
			}
			codeChanges = append(codeChanges, apiModPatch)
		}

		apiPatch.ModuleCodeChanges = codeChanges
	}

	apiPatch.PatchedConfig = ToStringPtr(v.PatchedConfig)
	apiPatch.Project = ToStringPtr(v.Project)
	apiPatch.CanEnqueueToCommitQueue = v.CanEnqueueToCommitQueue()

	return errors.WithStack(apiPatch.GithubPatchData.BuildFromService(v.GithubPatchData))
}

// ToService converts a service layer patch using the data from APIPatch
func (apiPatch *APIPatch) ToService() (interface{}, error) {
	var err error
	res := patch.Patch{}
	catcher := grip.NewBasicCatcher()
	res.Id = bson.ObjectIdHex(FromStringPtr(apiPatch.Id))
	res.Description = FromStringPtr(apiPatch.Description)
	res.Project = FromStringPtr(apiPatch.Project)
	res.Githash = FromStringPtr(apiPatch.Githash)
	res.PatchNumber = apiPatch.PatchNumber
	res.Author = FromStringPtr(apiPatch.Author)
	res.Version = FromStringPtr(apiPatch.Version)
	res.Status = FromStringPtr(apiPatch.Status)
	res.Alias = FromStringPtr(apiPatch.Alias)
	res.Activated = apiPatch.Activated
	res.CreateTime, err = FromTimePtr(apiPatch.CreateTime)
	catcher.Add(err)
	res.StartTime, err = FromTimePtr(apiPatch.StartTime)
	catcher.Add(err)
	res.FinishTime, err = FromTimePtr(apiPatch.FinishTime)
	catcher.Add(err)

	builds := make([]string, len(apiPatch.Variants))
	for _, b := range apiPatch.Variants {
		builds = append(builds, FromStringPtr(b))
	}

	res.BuildVariants = builds
	tasks := make([]string, len(apiPatch.Tasks))
	for i, t := range apiPatch.Tasks {
		tasks[i] = FromStringPtr(t)
	}
	res.Tasks = tasks
	params := []patch.Parameter{}
	for _, param := range apiPatch.Parameters {
		params = append(params, patch.Parameter{
			Key:   FromStringPtr(param.Key),
			Value: FromStringPtr(param.Value),
		})
	}
	res.Parameters = params
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
	g.BaseOwner = ToStringPtr(v.BaseOwner)
	g.BaseRepo = ToStringPtr(v.BaseRepo)
	g.HeadOwner = ToStringPtr(v.HeadOwner)
	g.HeadRepo = ToStringPtr(v.HeadRepo)
	g.HeadHash = ToStringPtr(v.HeadHash)
	g.Author = ToStringPtr(v.Author)
	return nil
}

// ToService converts a service layer patch using the data from APIPatch
func (g *githubPatch) ToService() (interface{}, error) {
	res := thirdparty.GithubPatch{}
	res.PRNumber = g.PRNumber
	res.BaseOwner = FromStringPtr(g.BaseOwner)
	res.BaseRepo = FromStringPtr(g.BaseRepo)
	res.HeadOwner = FromStringPtr(g.HeadOwner)
	res.HeadRepo = FromStringPtr(g.HeadRepo)
	res.HeadHash = FromStringPtr(g.HeadHash)
	res.Author = FromStringPtr(g.Author)
	return res, nil
}
