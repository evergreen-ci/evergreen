package model

import (
	"fmt"

	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/pkg/errors"
)

// APIPatch is the model to be returned by the API whenever patches are fetched.
type APIPatch struct {
	Id            APIString     `json:"patch_id"`
	Description   APIString     `json:"description"`
	ProjectId     APIString     `json:"project_id"`
	Branch        APIString     `json:"branch"`
	Githash       APIString     `json:"git_hash"`
	PatchNumber   int           `json:"patch_number"`
	Author        APIString     `json:"author"`
	Status        APIString     `json:"status"`
	CreateTime    APITime       `json:"create_time"`
	StartTime     APITime       `json:"start_time"`
	FinishTime    APITime       `json:"finish_time"`
	Variants      []APIString   `json:"builds"`
	Tasks         []APIString   `json:"tasks"`
	VariantsTasks []variantTask `json:"variants_tasks"`
	Activated     bool          `json:"activated"`
}

type variantTask struct {
	Name  APIString   `json:"name"`
	Tasks []APIString `json:"tasks"`
}

// BuildFromService converts from service level structs to an APIPatch
func (apiPatch *APIPatch) BuildFromService(h interface{}) error {
	v, ok := h.(patch.Patch)
	if !ok {
		return fmt.Errorf("incorrect type when fetching converting patch type")
	}
	apiPatch.Id = APIString(v.Id.Hex())
	apiPatch.Description = APIString(v.Description)
	apiPatch.ProjectId = APIString(v.Project)
	apiPatch.Branch = APIString(v.Project)
	apiPatch.Githash = APIString(v.Githash)
	apiPatch.PatchNumber = v.PatchNumber
	apiPatch.Author = APIString(v.Author)
	apiPatch.Status = APIString(v.Status)
	apiPatch.CreateTime = NewTime(v.CreateTime)
	apiPatch.StartTime = NewTime(v.CreateTime)
	apiPatch.FinishTime = NewTime(v.CreateTime)
	builds := make([]APIString, 0)
	for _, b := range v.BuildVariants {
		builds = append(builds, APIString(b))
	}
	apiPatch.Variants = builds
	tasks := make([]APIString, 0)
	for _, t := range v.Tasks {
		tasks = append(tasks, APIString(t))
	}
	apiPatch.Tasks = tasks
	variantTasks := []variantTask{}
	for _, vt := range v.VariantsTasks {
		vtasks := make([]APIString, 0)
		for _, task := range v.Tasks {
			vtasks = append(vtasks, APIString(task))
		}
		variantTasks = append(variantTasks, variantTask{
			Name:  APIString(vt.Variant),
			Tasks: vtasks,
		})
	}
	apiPatch.VariantsTasks = variantTasks
	apiPatch.Activated = v.Activated
	return nil
}

// ToService converts a service layer patch using the data from APIPatch
func (apiPatch *APIPatch) ToService() (interface{}, error) {
	return nil, errors.New("not implemented for read-only route")
}
