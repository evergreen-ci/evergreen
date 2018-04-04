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
	Order      int       `json:"order"`

	Author        APIString     `json:"author"`
	AuthorEmail   APIString     `json:"author_email"`
	Message       APIString     `json:"message"`
	Status        APIString     `json:"status"`
	Repo          APIString     `json:"repo"`
	Branch        APIString     `json:"branch"`
	BuildVariants []buildDetail `json:"build_variants_status"`

	Errors   []APIString `json:"errors"`
	Warnings []APIString `json:"warnings"`
	Ignored  bool        `json:"ignored"`
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

	apiVersion.Id = ToApiString(v.Id)
	apiVersion.CreateTime = NewTime(v.CreateTime)
	apiVersion.StartTime = NewTime(v.StartTime)
	apiVersion.FinishTime = NewTime(v.FinishTime)
	apiVersion.Revision = ToApiString(v.Revision)
	apiVersion.Author = ToApiString(v.Author)
	apiVersion.AuthorEmail = ToApiString(v.AuthorEmail)
	apiVersion.Message = ToApiString(v.Message)
	apiVersion.Status = ToApiString(v.Status)
	apiVersion.Repo = ToApiString(v.Repo)
	apiVersion.Branch = ToApiString(v.Branch)
	apiVersion.Order = v.RevisionOrderNumber

	var bd buildDetail
	for _, t := range v.BuildVariants {
		bd = buildDetail{
			BuildVariant: ToApiString(t.BuildVariant),
			BuildId:      ToApiString(t.BuildId),
		}
		apiVersion.BuildVariants = append(apiVersion.BuildVariants, bd)
	}

	return nil
}

// ToService returns a service layer build using the data from the APIVersion.
func (apiVersion *APIVersion) ToService() (interface{}, error) {
	return nil, errors.New("not implemented for read-only route")
}
