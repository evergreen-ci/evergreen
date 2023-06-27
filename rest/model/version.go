package model

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/utility"
)

// APIVersion is the model to be returned by the API whenever versions are fetched.
type APIVersion struct {
	Id                 *string        `json:"version_id"`
	CreateTime         *time.Time     `json:"create_time"`
	StartTime          *time.Time     `json:"start_time"`
	FinishTime         *time.Time     `json:"finish_time"`
	Revision           *string        `json:"revision"`
	Order              int            `json:"order"`
	Project            *string        `json:"project"`
	ProjectIdentifier  *string        `json:"project_identifier"`
	Author             *string        `json:"author"`
	AuthorEmail        *string        `json:"author_email"`
	Message            *string        `json:"message"`
	Status             *string        `json:"status"`
	Repo               *string        `json:"repo"`
	Branch             *string        `json:"branch"`
	Parameters         []APIParameter `json:"parameters"`
	BuildVariantStatus []buildDetail  `json:"build_variants_status"`
	Builds             []APIBuild     `json:"builds,omitempty"`
	Requester          *string        `json:"requester"`
	Errors             []*string      `json:"errors"`
	Activated          *bool          `json:"activated"`
	Aborted            *bool          `json:"aborted"`
	GitTags            []APIGitTag    `json:"git_tags"`
}

type APIGitTag struct {
	Tag    *string `json:"tag"`
	Pusher *string `json:"pusher"`
}

type buildDetail struct {
	BuildVariant *string `json:"build_variant"`
	BuildId      *string `json:"build_id"`
}

// BuildFromService converts from service level structs to an APIVersion.
func (apiVersion *APIVersion) BuildFromService(v model.Version) {
	apiVersion.Id = utility.ToStringPtr(v.Id)
	apiVersion.CreateTime = ToTimePtr(v.CreateTime)
	apiVersion.StartTime = ToTimePtr(v.StartTime)
	apiVersion.FinishTime = ToTimePtr(v.FinishTime)
	apiVersion.Revision = utility.ToStringPtr(v.Revision)
	apiVersion.Author = utility.ToStringPtr(v.Author)
	apiVersion.AuthorEmail = utility.ToStringPtr(v.AuthorEmail)
	apiVersion.Message = utility.ToStringPtr(v.Message)
	apiVersion.Status = utility.ToStringPtr(v.Status)
	apiVersion.Repo = utility.ToStringPtr(v.Repo)
	apiVersion.Branch = utility.ToStringPtr(v.Branch)
	apiVersion.Order = v.RevisionOrderNumber
	apiVersion.Project = utility.ToStringPtr(v.Identifier)
	apiVersion.Requester = utility.ToStringPtr(v.Requester)
	apiVersion.Errors = utility.ToStringPtrSlice(v.Errors)
	apiVersion.Activated = v.Activated
	apiVersion.Aborted = utility.ToBoolPtr(v.Aborted)

	var bd buildDetail
	for _, t := range v.BuildVariants {
		bd = buildDetail{
			BuildVariant: utility.ToStringPtr(t.BuildVariant),
			BuildId:      utility.ToStringPtr(t.BuildId),
		}
		apiVersion.BuildVariantStatus = append(apiVersion.BuildVariantStatus, bd)
	}
	for _, bv := range v.Builds {
		apiBuild := APIBuild{}
		apiBuild.BuildFromService(bv, nil)
		apiVersion.Builds = append(apiVersion.Builds, apiBuild)
	}

	for _, param := range v.Parameters {
		apiVersion.Parameters = append(apiVersion.Parameters, APIParameter{
			Key:   utility.ToStringPtr(param.Key),
			Value: utility.ToStringPtr(param.Value),
		})
	}

	for _, gt := range v.GitTags {
		apiVersion.GitTags = append(apiVersion.GitTags, APIGitTag{
			Pusher: utility.ToStringPtr(gt.Pusher),
			Tag:    utility.ToStringPtr(gt.Tag),
		})
	}

	if v.Identifier != "" {
		identifier, err := model.GetIdentifierForProject(v.Identifier)
		if err == nil {
			apiVersion.ProjectIdentifier = utility.ToStringPtr(identifier)
		}
	}
}

func (apiVersion *APIVersion) IsPatchRequester() bool {
	return evergreen.IsPatchRequester(utility.FromStringPtr(apiVersion.Requester))
}
