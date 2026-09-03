package route

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// POST /rest/v2/task/{task_id}/source_cache/credentials
//
// Returns source cache credentials scoped server-side to the task's own repo
// prefix. The caller supplies no role ARN and no policy.
type sourceCacheCredentials struct {
	taskID string
	hostID string

	settings   *evergreen.Settings
	stsManager cloud.STSManager
}

func makeSourceCacheCredentials(settings *evergreen.Settings, stsManager cloud.STSManager) gimlet.RouteHandler {
	return &sourceCacheCredentials{settings: settings, stsManager: stsManager}
}

func (h *sourceCacheCredentials) Factory() gimlet.RouteHandler {
	return &sourceCacheCredentials{settings: h.settings, stsManager: h.stsManager}
}

func (h *sourceCacheCredentials) Parse(ctx context.Context, r *http.Request) error {
	if h.taskID = gimlet.GetVars(r)["task_id"]; h.taskID == "" {
		return errors.New("missing task ID")
	}
	h.hostID = r.Header.Get(evergreen.HostHeader)
	return nil
}

func (h *sourceCacheCredentials) Run(ctx context.Context) gimlet.Responder {
	t, err := task.FindOneId(ctx, h.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", h.taskID))
	}
	if t == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", h.taskID),
		})
	}

	bucket := h.settings.Buckets.GetSourceCacheBucket(t.Project)
	if bucket.Name == "" {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusConflict,
			Message:    fmt.Sprintf("no source cache bucket is configured for project '%s'", t.Project),
		})
	}
	if bucket.RoleARN == "" {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusConflict,
			Message:    "no role is configured for the source cache bucket",
		})
	}

	pRef, err := model.GetProjectRefForTask(ctx, h.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "getting project for task '%s'", h.taskID))
	}
	if pRef == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("project '%s' not found for task '%s'", t.Project, h.taskID),
		})
	}
	if pRef.Owner == "" || pRef.Repo == "" {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusConflict,
			Message:    fmt.Sprintf("project '%s' has no owner and repo to scope source cache credentials to", t.Project),
		})
	}
	if err := validateSourceCacheRepoComponents(pRef.Owner, pRef.Repo); err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusConflict,
			Message:    fmt.Sprintf("project '%s' has an owner or repo that cannot appear in a source cache policy: %s", t.Project, err),
		})
	}

	namespace, err := sourceCacheNamespaceForTask(ctx, t)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "resolving the source cache namespace for task '%s'", h.taskID))
	}

	policy, err := sourceCacheSessionPolicy(bucket.Name, pRef.Owner, pRef.Repo, namespace)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "building the source cache session policy"))
	}

	creds, err := h.stsManager.AssumeRole(ctx, h.taskID, h.hostID, cloud.AssumeRoleOptions{
		RoleARN:          bucket.RoleARN,
		Policy:           &policy,
		ExternalIDPrefix: evergreen.SourceCacheExternalIDPrefix,
	})
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "assuming the source cache role for task '%s'", h.taskID))
	}

	return gimlet.NewJSONResponse(apimodels.SourceCacheCredentialsResponse{
		AWSCredentials: apimodels.AWSCredentials{
			AccessKeyID:     creds.AccessKeyID,
			SecretAccessKey: creds.SecretAccessKey,
			SessionToken:    creds.SessionToken,
			Expiration:      creds.Expiration.Format(time.RFC3339),
			ExternalID:      creds.ExternalID,
		},
		// The namespace the policy scopes writes to, so the agent does not re-derive it.
		Namespaces: []string{namespace},
	})
}

// sourceCacheRepoComponentRegexp matches the characters safe to interpolate into an IAM resource ARN.
var sourceCacheRepoComponentRegexp = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// validateSourceCacheRepoComponents rejects an owner or repo that could widen or escape the IAM resource pattern.
func validateSourceCacheRepoComponents(owner, repo string) error {
	for _, component := range []struct{ name, value string }{
		{name: "owner", value: owner},
		{name: "repo", value: repo},
	} {
		if component.value == "." || component.value == ".." {
			return errors.Errorf("%s '%s' is not a valid GitHub path component", component.name, component.value)
		}
		if !sourceCacheRepoComponentRegexp.MatchString(component.value) {
			return errors.Errorf("%s '%s' must match %s", component.name, component.value, sourceCacheRepoComponentRegexp.String())
		}
	}
	return nil
}

// sourceCacheNamespaceForTask returns the namespace the task may write to. It must
// agree with the agent's namespace or the task's uploads are denied.
func sourceCacheNamespaceForTask(ctx context.Context, t *task.Task) (string, error) {
	if t.Requester == evergreen.GithubPRRequester || t.Requester == evergreen.GithubMergeRequester {
		return evergreen.SourceCachePRNamespace, nil
	}
	if !evergreen.IsPatchRequester(t.Requester) {
		return evergreen.SourceCacheBaseNamespace, nil
	}

	// A parent PR checkout leaves the working tree at unreviewed PR code.
	p, err := patch.FindOneId(ctx, t.Version)
	if err != nil {
		return "", errors.Wrapf(err, "finding patch '%s'", t.Version)
	}
	if p != nil && p.GitHubParentPRCheckout != nil && p.GitHubParentPRCheckout.ForSource {
		return evergreen.SourceCachePRNamespace, nil
	}
	return evergreen.SourceCacheBaseNamespace, nil
}

// sourceCacheSessionPolicy allows reads under the task's whole repo prefix, so a PR
// task can restore the base artifact, but writes only under its own namespace.
func sourceCacheSessionPolicy(bucketName, owner, repo, namespace string) (string, error) {
	type statement struct {
		Sid      string
		Effect   string
		Action   []string
		Resource string
	}
	bucketARN := fmt.Sprintf("arn:aws:s3:::%s", bucketName)
	policy, err := json.Marshal(struct {
		Version   string
		Statement []statement
	}{
		Version: "2012-10-17",
		Statement: []statement{
			{
				Sid:      "SourceCacheRead",
				Effect:   "Allow",
				Action:   []string{"s3:GetObject"},
				Resource: fmt.Sprintf("%s/%s/*", bucketARN, evergreen.SourceCacheRepoPrefix(owner, repo)),
			},
			{
				Sid:      "SourceCacheWrite",
				Effect:   "Allow",
				Action:   []string{"s3:PutObject", "s3:AbortMultipartUpload"},
				Resource: fmt.Sprintf("%s/%s/*", bucketARN, evergreen.SourceCacheNamespacePrefix(owner, repo, namespace)),
			},
		},
	})
	return string(policy), errors.Wrap(err, "marshalling the policy")
}
