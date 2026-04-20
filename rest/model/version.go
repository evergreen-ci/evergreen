package model

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/cost"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/s3usage"
	"github.com/evergreen-ci/utility"
)

// APIVersion is the model to be returned by the API whenever versions are fetched.
type APIVersion struct {
	Id *string `json:"version_id"`
	// Time that the version was first created
	CreateTime *time.Time `json:"create_time"`
	// Time at which the version document was persisted in Evergreen. Will be null for versions created before this field was added.
	IngestTime *time.Time `json:"ingest_time,omitempty"`
	// Time at which tasks associated with this version started running
	StartTime *time.Time `json:"start_time"`
	// Time at which tasks associated with this version finished running
	FinishTime *time.Time `json:"finish_time"`
	// The version control identifier
	Revision          *string `json:"revision"`
	Order             int     `json:"order"`
	Project           *string `json:"project"`
	ProjectIdentifier *string `json:"project_identifier"`
	// Author of the version
	Author *string `json:"author"`
	// Author ID is the Evergreen user ID associated with the version.
	AuthorID *string `json:"author_id"`
	// Email of the author of the version
	AuthorEmail *string `json:"author_email"`
	// Message left with the commit
	Message *string `json:"message"`
	// The status of the version (possible values are "created", "started", "success", or "failed")
	Status *string `json:"status"`
	// The github repository where the commit was made
	Repo *string `json:"repo"`
	// The version control branch where the commit was made
	Branch     *string        `json:"branch"`
	Parameters []APIParameter `json:"parameters"`
	// List of documents of the associated build variant and the build id
	BuildVariantStatus []buildDetail `json:"build_variants_status"`
	// Version created by one of "patch_request", "github_pull_request",
	// "gitter_request" (caused by git commit, aka the repotracker requester),
	// "trigger_request" (Project Trigger versions) , "github_merge_request" (GitHub merge queue), "ad_hoc" (periodic builds)
	Requester *string   `json:"requester"`
	Errors    []*string `json:"errors"`
	// Will be null for versions created before this field was added.
	Activated *bool `json:"activated"`
	Aborted   *bool `json:"aborted"`
	// The git tag that triggered this version, if any.
	TriggeredGitTag *APIGitTag `json:"triggered_by_git_tag"`
	// Git tags that were pushed to this version.
	GitTags []APIGitTag `json:"git_tags"`
	// Indicates if the version was ignored due to only making changes to ignored files.
	Ignored *bool `json:"ignored"`
	// Aggregated actual cost of all tasks in the version
	Cost *cost.Cost `json:"cost,omitempty"`
	// CostTotal is the sum of all adjusted cost components in Cost (convenience for clients).
	CostTotal *float64 `json:"cost_total,omitempty"`
	// Aggregated predicted cost of all tasks in the version
	PredictedCost *cost.Cost `json:"predicted_cost,omitempty"`
	// PredictedCostTotal is the sum of all adjusted cost components in PredictedCost.
	PredictedCostTotal *float64 `json:"predicted_cost_total,omitempty"`
	// Aggregated S3 upload metrics across all tasks in the version
	S3Usage *APIVersionS3Usage `json:"s3_usage,omitempty"`
}

// APIVersionS3Usage holds aggregated S3 upload metrics for a version.
// Logs only exposes puts and bytes since per-type breakdown is not aggregated at version level.
type APIVersionS3Usage struct {
	Artifacts s3usage.ArtifactMetrics `json:"artifacts,omitempty"`
	Logs      APIVersionS3LogUsage    `json:"logs,omitempty"`
}

// APIVersionS3LogUsage holds aggregated S3 log upload metrics for a version.
type APIVersionS3LogUsage struct {
	PutRequests int   `json:"put_requests,omitempty"`
	UploadBytes int64 `json:"upload_bytes,omitempty"`
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
func (apiVersion *APIVersion) BuildFromService(ctx context.Context, v model.Version) {
	apiVersion.Id = utility.ToStringPtr(v.Id)
	apiVersion.CreateTime = ToTimePtr(v.CreateTime)
	apiVersion.IngestTime = ToTimePtr(v.IngestTime)
	apiVersion.StartTime = ToTimePtr(v.StartTime)
	apiVersion.FinishTime = ToTimePtr(v.FinishTime)
	apiVersion.Revision = utility.ToStringPtr(v.Revision)
	apiVersion.Author = utility.ToStringPtr(v.Author)
	apiVersion.AuthorID = utility.ToStringPtr(v.AuthorID)
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
	apiVersion.Ignored = utility.ToBoolPtr(v.Ignored)

	var bd buildDetail
	for _, t := range v.BuildVariants {
		bd = buildDetail{
			BuildVariant: utility.ToStringPtr(t.BuildVariant),
			BuildId:      utility.ToStringPtr(t.BuildId),
		}
		apiVersion.BuildVariantStatus = append(apiVersion.BuildVariantStatus, bd)
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

	if v.TriggeredByGitTag.Tag != "" {
		apiVersion.TriggeredGitTag = &APIGitTag{
			Tag:    utility.ToStringPtr(v.TriggeredByGitTag.Tag),
			Pusher: utility.ToStringPtr(v.TriggeredByGitTag.Pusher),
		}
	}

	if v.Identifier != "" {
		identifier, err := model.GetIdentifierForProject(ctx, v.Identifier)
		if err == nil {
			apiVersion.ProjectIdentifier = utility.ToStringPtr(identifier)
		}
	}

	if !v.Cost.IsZero() {
		versionCost := v.Cost
		apiVersion.Cost = &versionCost
		apiVersion.CostTotal = utility.ToFloat64Ptr(TotalAdjustedCost(versionCost))
	}
	if !v.PredictedCost.IsZero() {
		predictedCost := v.PredictedCost
		apiVersion.PredictedCost = &predictedCost
		apiVersion.PredictedCostTotal = utility.ToFloat64Ptr(TotalAdjustedCost(predictedCost))
	}
	if !v.S3Usage.IsZero() {
		apiVersion.S3Usage = &APIVersionS3Usage{
			Artifacts: v.S3Usage.Artifacts,
			Logs: APIVersionS3LogUsage{
				PutRequests: v.S3Usage.Logs.PutRequests,
				UploadBytes: v.S3Usage.Logs.UploadBytes,
			},
		}
	}
	if apiVersion.IsPatchRequester() {
		p, err := patch.FindOneId(ctx, v.Id)
		if err != nil || p == nil {
			return
		}
		if p.IsParent() && len(p.Triggers.ChildPatches) > 0 {
			childPatches, err := patch.Find(ctx, patch.ByStringIds(p.Triggers.ChildPatches))
			if err != nil {
				return
			}
			for _, childPatch := range childPatches {
				if !childPatch.StartTime.IsZero() && childPatch.StartTime.Before(utility.FromTimePtr(apiVersion.StartTime)) {
					apiVersion.StartTime = ToTimePtr(childPatch.StartTime)
				}
				if !childPatch.FinishTime.IsZero() && childPatch.FinishTime.After(utility.FromTimePtr(apiVersion.FinishTime)) {
					apiVersion.FinishTime = ToTimePtr(childPatch.FinishTime)
				}
			}
		}
	}
}

func (apiVersion *APIVersion) IsPatchRequester() bool {
	return evergreen.IsPatchRequester(utility.FromStringPtr(apiVersion.Requester))
}
