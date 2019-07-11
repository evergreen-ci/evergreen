package model

import (
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
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
	apiPatch.Id = ToAPIString(v.Id.Hex())
	apiPatch.Description = ToAPIString(v.Description)
	apiPatch.ProjectId = ToAPIString(v.Project)
	apiPatch.Branch = ToAPIString(v.Project)
	apiPatch.Githash = ToAPIString(v.Githash)
	apiPatch.PatchNumber = v.PatchNumber
	apiPatch.Author = ToAPIString(v.Author)
	apiPatch.Version = ToAPIString(v.Version)
	apiPatch.Status = ToAPIString(v.Status)
	apiPatch.CreateTime = NewTime(v.CreateTime)
	apiPatch.StartTime = NewTime(v.StartTime)
	apiPatch.FinishTime = NewTime(v.FinishTime)
	builds := make([]APIString, 0)
	for _, b := range v.BuildVariants {
		builds = append(builds, ToAPIString(b))
	}
	apiPatch.Variants = builds
	tasks := make([]APIString, 0)
	for _, t := range v.Tasks {
		tasks = append(tasks, ToAPIString(t))
	}
	apiPatch.Tasks = tasks
	variantTasks := []variantTask{}
	for _, vt := range v.VariantsTasks {
		vtasks := make([]APIString, 0)
		for _, task := range v.Tasks {
			vtasks = append(vtasks, ToAPIString(task))
		}
		variantTasks = append(variantTasks, variantTask{
			Name:  ToAPIString(vt.Variant),
			Tasks: vtasks,
		})
	}
	apiPatch.VariantsTasks = variantTasks
	apiPatch.Activated = v.Activated
	apiPatch.Alias = ToAPIString(v.Alias)
	apiPatch.GithubPatchData = githubPatch{}
	return errors.WithStack(apiPatch.GithubPatchData.BuildFromService(v.GithubPatchData))
}

// ToService converts a service layer patch using the data from APIPatch
func (apiPatch *APIPatch) ToService() (interface{}, error) {
	var err error
	res := patch.Patch{}
	catcher := grip.NewBasicCatcher()
	res.Id = bson.ObjectIdHex(FromAPIString(apiPatch.Id))
	res.Description = FromAPIString(apiPatch.Description)
	res.Project = FromAPIString(apiPatch.Description)
	res.Githash = FromAPIString(apiPatch.Githash)
	res.PatchNumber = apiPatch.PatchNumber
	res.Author = FromAPIString(apiPatch.Author)
	res.Version = FromAPIString(apiPatch.Version)
	res.Status = FromAPIString(apiPatch.Status)
	res.Alias = FromAPIString(apiPatch.Alias)
	res.Activated = apiPatch.Activated
	res.CreateTime, err = ParseTime(apiPatch.CreateTime.String())
	catcher.Add(err)
	res.StartTime, err = ParseTime(apiPatch.StartTime.String())
	catcher.Add(err)
	res.FinishTime, err = ParseTime(apiPatch.FinishTime.String())
	catcher.Add(err)

	builds := make([]string, len(apiPatch.Variants))
	for _, b := range apiPatch.Variants {
		builds = append(builds, FromAPIString(b))
	}

	res.BuildVariants = builds
	tasks := make([]string, len(apiPatch.Tasks))
	for i, t := range apiPatch.Tasks {
		tasks[i] = FromAPIString(t)
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
	g.BaseOwner = ToAPIString(v.BaseOwner)
	g.BaseRepo = ToAPIString(v.BaseRepo)
	g.HeadOwner = ToAPIString(v.HeadOwner)
	g.HeadRepo = ToAPIString(v.HeadRepo)
	g.HeadHash = ToAPIString(v.HeadHash)
	g.Author = ToAPIString(v.Author)
	return nil
}

// ToService converts a service layer patch using the data from APIPatch
func (g *githubPatch) ToService() (interface{}, error) {
	res := patch.GithubPatch{}
	res.PRNumber = g.PRNumber
	res.BaseOwner = FromAPIString(g.BaseOwner)
	res.BaseRepo = FromAPIString(g.BaseRepo)
	res.HeadOwner = FromAPIString(g.HeadOwner)
	res.HeadRepo = FromAPIString(g.HeadRepo)
	res.HeadHash = FromAPIString(g.HeadHash)
	res.Author = FromAPIString(g.Author)
	return res, nil
}
