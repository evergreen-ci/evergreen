package route

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	req    apimodels.SourceCacheCredentialsRequest

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
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return errors.Wrap(err, "reading request body")
	}
	return errors.Wrap(json.Unmarshal(body, &h.req), "reading source cache credentials request")
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

	plan, err := buildSourceCachePlan(ctx, t, pRef, h.req)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "computing the source cache plan"))
	}

	policy, err := sourceCacheSessionPolicy(bucket.Name, plan.restoreKeys, plan.saveKey)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "building the source cache session policy"))
	}

	creds, err := h.stsManager.AssumeRole(ctx, h.taskID, h.hostID, cloud.AssumeRoleOptions{
		RoleARN:    bucket.RoleARN,
		Policy:     &policy,
		ExternalID: evergreen.SourceCacheExternalID,
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
		RestoreKeys: plan.restoreKeys,
		SaveKey:     plan.saveKey,
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

// sourceCacheNamespaceForTask returns the namespace the task's own artifact lives in.
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

// sourceCachePlan holds the exact origin keys a session may touch.
type sourceCachePlan struct {
	restoreKeys []apimodels.SourceCacheRestoreKey
	saveKey     apimodels.SourceCacheRestoreKey
}

// sourceCacheCandidate is one artifact the task may restore from.
type sourceCacheCandidate struct {
	revision  string
	namespace string
}

// buildSourceCachePlan resolves the ordered restore keys and the save key from the
// task, the patch, and the clone shape the agent resolved.
func buildSourceCachePlan(ctx context.Context, t *task.Task, pRef *model.ProjectRef, req apimodels.SourceCacheCredentialsRequest) (*sourceCachePlan, error) {
	namespace, err := sourceCacheNamespaceForTask(ctx, t)
	if err != nil {
		return nil, errors.Wrap(err, "resolving the namespace")
	}
	prRevision, err := sourceCachePRRevision(ctx, t)
	if err != nil {
		return nil, errors.Wrap(err, "resolving the PR head")
	}

	candidates := []sourceCacheCandidate{{revision: t.Revision, namespace: namespace}}
	if prRevision != "" && prRevision != t.Revision {
		candidates = []sourceCacheCandidate{
			{revision: prRevision, namespace: namespace},
			{revision: t.Revision, namespace: evergreen.SourceCacheBaseNamespace},
		}
	} else if namespace != evergreen.SourceCacheBaseNamespace {
		candidates = append(candidates, sourceCacheCandidate{revision: t.Revision, namespace: evergreen.SourceCacheBaseNamespace})
	}

	plan := &sourceCachePlan{}
	for i, candidate := range candidates {
		key := evergreen.SourceCacheObjectKey(pRef.Owner, pRef.Repo, candidate.namespace, req.Branch, candidate.revision, req.CloneDepth, req.RecurseSubmodules)
		restoreKey := apimodels.SourceCacheRestoreKey{Revision: candidate.revision, Key: key}
		plan.restoreKeys = append(plan.restoreKeys, restoreKey)
		if i == 0 {
			plan.saveKey = restoreKey
		}
	}
	return plan, nil
}

// sourceCachePRRevision returns the commit a PR or merge queue checkout leaves HEAD
// at, mirroring the agent's prCheckoutCommit from the patch.
func sourceCachePRRevision(ctx context.Context, t *task.Task) (string, error) {
	p, err := patch.FindOneId(ctx, t.Version)
	if err != nil {
		return "", errors.Wrapf(err, "finding patch '%s'", t.Version)
	}
	if p == nil {
		return "", nil
	}
	// A parent PR checkout leaves the working tree at unreviewed PR code.
	if p.GitHubParentPRCheckout != nil && p.GitHubParentPRCheckout.ForSource {
		return p.GitHubParentPRCheckout.HeadHash, nil
	}
	switch t.Requester {
	case evergreen.GithubPRRequester:
		return p.GithubPatchData.HeadHash, nil
	case evergreen.GithubMergeRequester:
		return p.GithubMergeData.HeadSHA, nil
	}
	return "", nil
}

// sourceCacheSessionPolicy admits reads of the exact restore keys and writes of the
// exact save key, so a session cannot touch any other artifact. PutObject covers the
// single-part put and the multipart init, upload, and complete calls pail makes;
// the client sends If-None-Match on create-only puts, so corruption repair still works.
func sourceCacheSessionPolicy(bucketName string, restoreKeys []apimodels.SourceCacheRestoreKey, saveKey apimodels.SourceCacheRestoreKey) (string, error) {
	type statement struct {
		Sid      string
		Effect   string
		Action   []string
		Resource []string
	}
	bucketARN := fmt.Sprintf("arn:aws:s3:::%s", bucketName)
	readResources := make([]string, 0, len(restoreKeys))
	for _, key := range restoreKeys {
		readResources = append(readResources, fmt.Sprintf("%s/%s", bucketARN, key.Key))
	}
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
				Resource: readResources,
			},
			{
				Sid:      "SourceCacheWrite",
				Effect:   "Allow",
				Action:   []string{"s3:PutObject", "s3:AbortMultipartUpload"},
				Resource: []string{fmt.Sprintf("%s/%s", bucketARN, saveKey.Key)},
			},
		},
	})
	return string(policy), errors.Wrap(err, "marshalling the policy")
}
