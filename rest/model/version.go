package model

import (
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/pkg/errors"
)

// APIVersion is the model to be returned by the API whenever versions are fetched.
type APIVersion struct {
	Id         APIString `json:"version_id"`
	CreateTime APITime   `json:"create_time"`
	StartTime  APITime   `json:"start_time"`
	FinishTime APITime   `json:"finish_time"`
	Revision   APIString `json:"revision"`

	Author        APIString     `json:"author"`
	AuthorEmail   APIString     `json:"author_email"`
	Message       APIString     `json:"message"`
	Status        APIString     `json:"status"`
	Repo          APIString     `json:"repo"`
	Branch        APIString     `json:"branch"`
	BuildVariants []buildDetail `json:"build_variants_status"`
}

type buildDetail struct {
	BuildVariant APIString `json:"build_variant"`
	BuildId      APIString `json:"build_id"`
}

// BuildFromService converts from service level structs to an APIVersion.
func (apiVersion *APIVersion) BuildFromService(h interface{}) error {
	v, ok := h.(*version.Version)
	if !ok {
		return errors.Errorf("incorrect type when fetching converting version type")
	}

	apiVersion.Id = APIString(v.Id)
	apiVersion.CreateTime = APITime(v.CreateTime)
	apiVersion.StartTime = APITime(v.StartTime)
	apiVersion.FinishTime = APITime(v.FinishTime)
	apiVersion.Revision = APIString(v.Revision)
	apiVersion.Author = APIString(v.Author)
	apiVersion.AuthorEmail = APIString(v.AuthorEmail)
	apiVersion.Message = APIString(v.Message)
	apiVersion.Status = APIString(v.Status)
	apiVersion.Repo = APIString(v.Repo)
	apiVersion.Branch = APIString(v.Branch)

	var bd buildDetail
	for _, t := range v.BuildVariants {
		bd = buildDetail{
			BuildVariant: APIString(t.BuildVariant),
			BuildId:      APIString(t.BuildId),
		}
		apiVersion.BuildVariants = append(apiVersion.BuildVariants, bd)
	}

	return nil
}

// ToService returns a service layer build using the data from the APIVersion.
func (apiVersion *APIVersion) ToService() (interface{}, error) {
	return nil, errors.New("not implemented for read-only route")
}
