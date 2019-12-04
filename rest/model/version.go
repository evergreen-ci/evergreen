package model

import (
	"github.com/evergreen-ci/evergreen/model"
	"github.com/pkg/errors"
)

// APIVersion is the model to be returned by the API whenever versions are fetched.
type APIVersion struct {
	Id            APIString     `json:"version_id"`
	CreateTime    APITime       `json:"create_time"`
	StartTime     APITime       `json:"start_time"`
	FinishTime    APITime       `json:"finish_time"`
	Revision      APIString     `json:"revision"`
	Order         int           `json:"order"`
	Project       APIString     `json:"project"`
	Author        APIString     `json:"author"`
	AuthorEmail   APIString     `json:"author_email"`
	Message       APIString     `json:"message"`
	Status        APIString     `json:"status"`
	Repo          APIString     `json:"repo"`
	Branch        APIString     `json:"branch"`
	BuildVariants []buildDetail `json:"build_variants_status"`
	Requester     APIString     `json:"requester"`
}

type buildDetail struct {
	BuildVariant APIString `json:"build_variant"`
	BuildId      APIString `json:"build_id"`
}

// BuildFromService converts from service level structs to an APIVersion.
func (apiVersion *APIVersion) BuildFromService(h interface{}) error {
	v, ok := h.(*model.Version)
	if !ok {
		return errors.Errorf("incorrect type when fetching converting version type")
	}

	apiVersion.Id = ToAPIString(v.Id)
	apiVersion.CreateTime = NewTime(v.CreateTime)
	apiVersion.StartTime = NewTime(v.StartTime)
	apiVersion.FinishTime = NewTime(v.FinishTime)
	apiVersion.Revision = ToAPIString(v.Revision)
	apiVersion.Author = ToAPIString(v.Author)
	apiVersion.AuthorEmail = ToAPIString(v.AuthorEmail)
	apiVersion.Message = ToAPIString(v.Message)
	apiVersion.Status = ToAPIString(v.Status)
	apiVersion.Repo = ToAPIString(v.Repo)
	apiVersion.Branch = ToAPIString(v.Branch)
	apiVersion.Order = v.RevisionOrderNumber
	apiVersion.Project = ToAPIString(v.Identifier)
	apiVersion.Requester = ToAPIString(v.Requester)

	var bd buildDetail
	for _, t := range v.BuildVariants {
		bd = buildDetail{
			BuildVariant: ToAPIString(t.BuildVariant),
			BuildId:      ToAPIString(t.BuildId),
		}
		apiVersion.BuildVariants = append(apiVersion.BuildVariants, bd)
	}

	return nil
}

// ToService returns a service layer build using the data from the APIVersion.
func (apiVersion *APIVersion) ToService() (interface{}, error) {
	return nil, errors.New("not implemented for read-only route")
}
