package model

import (
	"github.com/evergreen-ci/evergreen/model"
	"github.com/pkg/errors"
)

// APIVersion is the model to be returned by the API whenever versions are fetched.
type APIVersion struct {
	Id            *string     `json:"version_id"`
	CreateTime    APITime       `json:"create_time"`
	StartTime     APITime       `json:"start_time"`
	FinishTime    APITime       `json:"finish_time"`
	Revision      *string     `json:"revision"`
	Order         int           `json:"order"`
	Project       *string     `json:"project"`
	Author        *string     `json:"author"`
	AuthorEmail   *string     `json:"author_email"`
	Message       *string     `json:"message"`
	Status        *string     `json:"status"`
	Repo          *string     `json:"repo"`
	Branch        *string     `json:"branch"`
	BuildVariants []buildDetail `json:"build_variants_status"`
	Requester     *string     `json:"requester"`
}

type buildDetail struct {
	BuildVariant *string `json:"build_variant"`
	BuildId      *string `json:"build_id"`
}

// BuildFromService converts from service level structs to an APIVersion.
func (apiVersion *APIVersion) BuildFromService(h interface{}) error {
	v, ok := h.(*model.Version)
	if !ok {
		return errors.Errorf("incorrect type when fetching converting version type")
	}

	apiVersion.Id = ToStringPtr(v.Id)
	apiVersion.CreateTime = NewTime(v.CreateTime)
	apiVersion.StartTime = NewTime(v.StartTime)
	apiVersion.FinishTime = NewTime(v.FinishTime)
	apiVersion.Revision = ToStringPtr(v.Revision)
	apiVersion.Author = ToStringPtr(v.Author)
	apiVersion.AuthorEmail = ToStringPtr(v.AuthorEmail)
	apiVersion.Message = ToStringPtr(v.Message)
	apiVersion.Status = ToStringPtr(v.Status)
	apiVersion.Repo = ToStringPtr(v.Repo)
	apiVersion.Branch = ToStringPtr(v.Branch)
	apiVersion.Order = v.RevisionOrderNumber
	apiVersion.Project = ToStringPtr(v.Identifier)
	apiVersion.Requester = ToStringPtr(v.Requester)

	var bd buildDetail
	for _, t := range v.BuildVariants {
		bd = buildDetail{
			BuildVariant: ToStringPtr(t.BuildVariant),
			BuildId:      ToStringPtr(t.BuildId),
		}
		apiVersion.BuildVariants = append(apiVersion.BuildVariants, bd)
	}

	return nil
}

// ToService returns a service layer build using the data from the APIVersion.
func (apiVersion *APIVersion) ToService() (interface{}, error) {
	return nil, errors.New("not implemented for read-only route")
}
