package model

import (
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/pkg/errors"
)

// APIPatch is the model to be returned by the API whenever patches are fetched.
type APIPatch struct {
	Id              APIString     `json:"patch_id"`
	Description     APIString     `json:"description"`
	ProjectId       APIString     `json:"project_id"`
	Branch          APIString     `json:"branch"`
	Githash         APIString     `json:"git_hash"`
	PatchNumber     int           `json:"patch_number"`
	Author          APIString     `json:"author"`
	Version         APIString     `json:"version"`
	Status          APIString     `json:"status"`
	CreateTime      APITime       `json:"create_time"`
	StartTime       APITime       `json:"start_time"`
	FinishTime      APITime       `json:"finish_time"`
	Variants        []APIString   `json:"builds"`
	Tasks           []APIString   `json:"tasks"`
	VariantsTasks   []variantTask `json:"variants_tasks"`
	Activated       bool          `json:"activated"`
	Alias           APIString     `json:"alias,omitempty"`
	GithubPatchData githubPatch   `json:"github_patch_data,omitempty"`
}
type variantTask struct {
	Name  APIString   `json:"name"`
	Tasks []APIString `json:"tasks"`
}

// BuildFromService converts from service level structs to an APIPatch
func (apiPatch *APIPatch) BuildFromService(h interface{}) error {
	v, ok := h.(patch.Patch)
	if !ok {
		return errors.New("incorrect type when fetching converting patch type")
	}
	apiPatch.Id = ToApiString(v.Id.Hex())
	apiPatch.Description = ToApiString(v.Description)
	apiPatch.ProjectId = ToApiString(v.Project)
	apiPatch.Branch = ToApiString(v.Project)
	apiPatch.Githash = ToApiString(v.Githash)
	apiPatch.PatchNumber = v.PatchNumber
	apiPatch.Author = ToApiString(v.Author)
	apiPatch.Version = ToApiString(v.Version)
	apiPatch.Status = ToApiString(v.Status)
	apiPatch.CreateTime = NewTime(v.CreateTime)
	apiPatch.StartTime = NewTime(v.CreateTime)
	apiPatch.FinishTime = NewTime(v.CreateTime)
	builds := make([]APIString, 0)
	for _, b := range v.BuildVariants {
		builds = append(builds, ToApiString(b))
	}
	apiPatch.Variants = builds
	tasks := make([]APIString, 0)
	for _, t := range v.Tasks {
		tasks = append(tasks, ToApiString(t))
	}
	apiPatch.Tasks = tasks
	variantTasks := []variantTask{}
	for _, vt := range v.VariantsTasks {
		vtasks := make([]APIString, 0)
		for _, task := range v.Tasks {
			vtasks = append(vtasks, ToApiString(task))
		}
		variantTasks = append(variantTasks, variantTask{
			Name:  ToApiString(vt.Variant),
			Tasks: vtasks,
		})
	}
	apiPatch.VariantsTasks = variantTasks
	apiPatch.Activated = v.Activated
	apiPatch.Alias = ToApiString(v.Alias)
	apiPatch.GithubPatchData = githubPatch{}
	return errors.WithStack(apiPatch.GithubPatchData.BuildFromService(v.GithubPatchData))
}

// ToService converts a service layer patch using the data from APIPatch
func (apiPatch *APIPatch) ToService() (interface{}, error) {
	return nil, errors.New("not implemented for read-only route")
}

type githubPatch struct {
	PRNumber  int       `json:"pr_number"`
	BaseOwner APIString `json:"base_owner"`
	BaseRepo  APIString `json:"base_repo"`
	HeadOwner APIString `json:"head_owner"`
	HeadRepo  APIString `json:"head_repo"`
	HeadHash  APIString `json:"head_hash"`
	Author    APIString `json:"author"`
}

// BuildFromService converts from service level structs to an APIPatch
func (g *githubPatch) BuildFromService(h interface{}) error {
	v, ok := h.(patch.GithubPatch)
	if !ok {
		return errors.New("incorrect type when fetching converting github patch type")
	}
	g.PRNumber = v.PRNumber
	g.BaseOwner = ToApiString(v.BaseOwner)
	g.BaseRepo = ToApiString(v.BaseRepo)
	g.HeadOwner = ToApiString(v.HeadOwner)
	g.HeadRepo = ToApiString(v.HeadRepo)
	g.HeadHash = ToApiString(v.HeadHash)
	g.Author = ToApiString(v.Author)
	return nil
}

// ToService converts a service layer patch using the data from APIPatch
func (g *githubPatch) ToService() (interface{}, error) {
	return nil, errors.New("(*githubPatch) ToService not implemented for read-only route")
}
