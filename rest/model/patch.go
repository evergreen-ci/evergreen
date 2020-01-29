package model

import (
	"time"

	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

// APIPatch is the model to be returned by the API whenever patches are fetched.
type APIPatch struct {
	Id              *string       `json:"patch_id"`
	Description     *string       `json:"description"`
	ProjectId       *string       `json:"project_id"`
	Branch          *string       `json:"branch"`
	Githash         *string       `json:"git_hash"`
	PatchNumber     int           `json:"patch_number"`
	Author          *string       `json:"author"`
	Version         *string       `json:"version"`
	Status          *string       `json:"status"`
	CreateTime      *time.Time    `json:"create_time"`
	StartTime       *time.Time    `json:"start_time"`
	FinishTime      *time.Time    `json:"finish_time"`
	Variants        []*string     `json:"builds"`
	Tasks           []*string     `json:"tasks"`
	VariantsTasks   []VariantTask `json:"variants_tasks"`
	Activated       bool          `json:"activated"`
	Alias           *string       `json:"alias,omitempty"`
	GithubPatchData githubPatch   `json:"github_patch_data,omitempty"`
}
type VariantTask struct {
	Name  *string   `json:"name"`
	Tasks []*string `json:"tasks"`
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
	return errors.WithStack(apiPatch.GithubPatchData.BuildFromService(v.GithubPatchData))
}

// ToService converts a service layer patch using the data from APIPatch
func (apiPatch *APIPatch) ToService() (interface{}, error) {
	var err error
	res := patch.Patch{}
	catcher := grip.NewBasicCatcher()
	res.Id = bson.ObjectIdHex(FromStringPtr(apiPatch.Id))
	res.Description = FromStringPtr(apiPatch.Description)
	res.Project = FromStringPtr(apiPatch.Description)
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
	i, err := apiPatch.GithubPatchData.ToService()
	catcher.Add(err)
	data, ok := i.(patch.GithubPatch)
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
	v, ok := h.(patch.GithubPatch)
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
	res := patch.GithubPatch{}
	res.PRNumber = g.PRNumber
	res.BaseOwner = FromStringPtr(g.BaseOwner)
	res.BaseRepo = FromStringPtr(g.BaseRepo)
	res.HeadOwner = FromStringPtr(g.HeadOwner)
	res.HeadRepo = FromStringPtr(g.HeadRepo)
	res.HeadHash = FromStringPtr(g.HeadHash)
	res.Author = FromStringPtr(g.Author)
	return res, nil
}
