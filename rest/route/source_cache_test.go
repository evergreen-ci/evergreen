package route

import (
	"net/http"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	sourceCacheTaskID    = "source_cache_task"
	sourceCacheHostID    = "source_cache_host"
	sourceCacheProjectID = "source_cache_project"
)

func sourceCacheTestSettings() *evergreen.Settings {
	return &evergreen.Settings{
		Buckets: evergreen.BucketsConfig{
			SourceCacheBucket:   evergreen.BucketConfig{Name: "source-cache", RoleARN: "role_arn"},
			SourceCacheProjects: []string{sourceCacheProjectID},
		},
	}
}

func newSourceCacheCredentialsRequest(t *testing.T) *http.Request {
	request, err := http.NewRequest(http.MethodPost, "/task/"+sourceCacheTaskID+"/source_cache/credentials", strings.NewReader(`{}`))
	require.NoError(t, err)
	request = gimlet.SetURLVars(request, map[string]string{"task_id": sourceCacheTaskID})
	request.Header.Set(evergreen.HostHeader, sourceCacheHostID)
	return request
}

func setupSourceCacheCredentialsHandler(t *testing.T, settings *evergreen.Settings) *sourceCacheCredentials {
	require.NoError(t, db.ClearCollections(task.Collection, model.ProjectRefCollection, host.Collection, patch.Collection))

	env := &mock.Environment{}
	require.NoError(t, env.Configure(t.Context()))

	h := host.Host{Id: sourceCacheHostID, Status: evergreen.HostRunning, RunningTask: sourceCacheTaskID}
	require.NoError(t, h.Insert(t.Context()))

	handler, ok := makeSourceCacheCredentials(settings, cloud.GetSTSManager(true)).(*sourceCacheCredentials)
	require.True(t, ok)
	return handler
}

// insertSourceCacheTask inserts a task and its project ref. An empty owner or repo
// leaves them unset on the project ref.
func insertSourceCacheTask(t *testing.T, requester, versionID, owner, repo string) {
	tsk := task.Task{Id: sourceCacheTaskID, Project: sourceCacheProjectID, Requester: requester, Version: versionID, Revision: "abc123"}
	require.NoError(t, tsk.Insert(t.Context()))
	pRef := model.ProjectRef{Id: sourceCacheProjectID, Owner: owner, Repo: repo}
	require.NoError(t, pRef.Insert(t.Context()))
}

func TestSourceCacheCredentialsParse(t *testing.T) {
	handler := &sourceCacheCredentials{}
	require.NoError(t, handler.Parse(t.Context(), newSourceCacheCredentialsRequest(t)))
	assert.Equal(t, sourceCacheTaskID, handler.taskID)
	assert.Equal(t, sourceCacheHostID, handler.hostID)
}

func TestSourceCacheCredentialsRun(t *testing.T) {
	for tName, tCase := range map[string]struct {
		mutateSettings func(*evergreen.Settings)
		insertTask     bool
		requester      string
		owner, repo    string
		expectedStatus int
	}{
		"UnknownTaskIsNotFound": {
			expectedStatus: http.StatusNotFound,
		},
		"ProjectNotOptedInIsRefused": {
			mutateSettings: func(s *evergreen.Settings) { s.Buckets.SourceCacheProjects = nil },
			insertTask:     true, owner: "some-org", repo: "some-repo",
			expectedStatus: http.StatusConflict,
		},
		"BucketWithNoRoleIsRefused": {
			mutateSettings: func(s *evergreen.Settings) { s.Buckets.SourceCacheBucket.RoleARN = "" },
			insertTask:     true, owner: "some-org", repo: "some-repo",
			expectedStatus: http.StatusConflict,
		},
		// With no prefix to scope to, unscoped credentials must not be handed out.
		"ProjectWithNoOwnerAndRepoIsRefused": {
			insertTask:     true,
			expectedStatus: http.StatusConflict,
		},
		// An owner or repo that could widen the IAM resource pattern or escape the
		// repo prefix must not be used to build the policy.
		"ProjectWithATraversalOwnerIsRefused": {
			insertTask: true, owner: "..", repo: "some-repo",
			expectedStatus: http.StatusConflict,
		},
		"ProjectWithAWildcardRepoIsRefused": {
			insertTask: true, owner: "some-org", repo: "*",
			expectedStatus: http.StatusConflict,
		},
		"ProjectWithASeparatorOwnerIsRefused": {
			insertTask: true, owner: "some/org", repo: "some-repo",
			expectedStatus: http.StatusConflict,
		},
		"MainlineTaskGetsARestorePlan": {
			insertTask: true, owner: "some-org", repo: "some-repo",
			expectedStatus: http.StatusOK,
		},
		"PullRequestTaskGetsARestorePlan": {
			insertTask: true, requester: evergreen.GithubPRRequester, owner: "some-org", repo: "some-repo",
			expectedStatus: http.StatusOK,
		},
	} {
		t.Run(tName, func(t *testing.T) {
			settings := sourceCacheTestSettings()
			if tCase.mutateSettings != nil {
				tCase.mutateSettings(settings)
			}
			handler := setupSourceCacheCredentialsHandler(t, settings)
			if tCase.insertTask {
				requester := tCase.requester
				if requester == "" {
					requester = evergreen.RepotrackerVersionRequester
				}
				insertSourceCacheTask(t, requester, "5bedc62ee4055d31f0340b1d", tCase.owner, tCase.repo)
			}
			require.NoError(t, handler.Parse(t.Context(), newSourceCacheCredentialsRequest(t)))

			resp := handler.Run(t.Context())
			require.NotNil(t, resp)
			require.Equal(t, tCase.expectedStatus, resp.Status(), resp.Data())
			if tCase.expectedStatus != http.StatusOK {
				return
			}

			creds, ok := resp.Data().(apimodels.SourceCacheCredentialsResponse)
			require.True(t, ok)
			assert.NotEmpty(t, creds.AccessKeyID)
			assert.NotEmpty(t, creds.SessionToken)
			// The fixed external ID lets the role's trust policy reject the generic route.
			assert.Equal(t, evergreen.SourceCacheExternalID, creds.ExternalID)
			require.NotEmpty(t, creds.RestoreKeys)
			assert.Equal(t, "abc123", creds.SaveKey.Revision)
			assert.Equal(t, creds.RestoreKeys[0], creds.SaveKey)
			assert.True(t, strings.HasPrefix(creds.SaveKey.Key, "source_cache/v1/some-org/some-repo/"))
			assert.True(t, strings.HasSuffix(creds.SaveKey.Key, ".tgz"))
		})
	}
}

func TestSourceCacheNamespaceForTask(t *testing.T) {
	for tName, tCase := range map[string]struct {
		requester     string
		parentPR      *patch.GitHubParentPRCheckout
		wantNamespace string
	}{
		"MainlineCommitGetsTheBaseNamespace": {
			requester:     evergreen.RepotrackerVersionRequester,
			wantNamespace: evergreen.SourceCacheBaseNamespace,
		},
		"PullRequestGetsThePRNamespace": {
			requester:     evergreen.GithubPRRequester,
			wantNamespace: evergreen.SourceCachePRNamespace,
		},
		"MergeQueueGetsThePRNamespace": {
			requester:     evergreen.GithubMergeRequester,
			wantNamespace: evergreen.SourceCachePRNamespace,
		},
		"PlainPatchGetsTheBaseNamespace": {
			requester:     evergreen.PatchVersionRequester,
			wantNamespace: evergreen.SourceCacheBaseNamespace,
		},
		"PatchWithAParentPRSourceCheckoutGetsThePRNamespace": {
			requester:     evergreen.PatchVersionRequester,
			parentPR:      &patch.GitHubParentPRCheckout{ForSource: true, PRNumber: 9001, HeadHash: "abc123"},
			wantNamespace: evergreen.SourceCachePRNamespace,
		},
		"PatchWithAParentPRModuleCheckoutGetsTheBaseNamespace": {
			requester:     evergreen.PatchVersionRequester,
			parentPR:      &patch.GitHubParentPRCheckout{ForModule: "some-module", PRNumber: 9001, HeadHash: "abc123"},
			wantNamespace: evergreen.SourceCacheBaseNamespace,
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(patch.Collection))

			const versionID = "5bedc62ee4055d31f0340b1d"
			if tCase.parentPR != nil {
				patchDoc := patch.Patch{
					Id:                     mgobson.ObjectIdHex(versionID),
					GitHubParentPRCheckout: tCase.parentPR,
				}
				require.NoError(t, patchDoc.Insert(t.Context()))
			}

			tsk := &task.Task{Id: sourceCacheTaskID, Requester: tCase.requester, Version: versionID}
			namespace, err := sourceCacheNamespaceForTask(t.Context(), tsk)
			require.NoError(t, err)
			assert.Equal(t, tCase.wantNamespace, namespace)
		})
	}
}

func TestValidateSourceCacheRepoComponents(t *testing.T) {
	for tName, tCase := range map[string]struct {
		owner, repo string
		expectError bool
	}{
		"ValidOwnerAndRepo": {
			owner: "some-org", repo: "some-repo",
		},
		"ValidOwnerAndRepoWithUnderlines": {
			owner: "org_name", repo: "repo_name",
		},
		"SingleDotOwnerFails":        {owner: ".", repo: "some-repo", expectError: true},
		"ParentDirOwnerFails":        {owner: "..", repo: "some-repo", expectError: true},
		"ParentDirRepoFails":         {owner: "some-org", repo: "..", expectError: true},
		"SlashInOwnerFails":          {owner: "some/org", repo: "some-repo", expectError: true},
		"IAMWildcardInRepoFails":     {owner: "some-org", repo: "some*", expectError: true},
		"IAMSingleCharWildcardFails": {owner: "some-org", repo: "some?", expectError: true},
		"EmptyOwnerFails":            {owner: "", repo: "some-repo", expectError: true},
		"NonASCIIRepoFails":          {owner: "some-org", repo: "répo", expectError: true},
	} {
		t.Run(tName, func(t *testing.T) {
			err := validateSourceCacheRepoComponents(tCase.owner, tCase.repo)
			if tCase.expectError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestSourceCacheSessionPolicy(t *testing.T) {
	restoreKeys := []apimodels.SourceCacheRestoreKey{
		{Revision: "pr-head", Key: "source_cache/v1/some-org/some-repo/pr/pr-head/k1.tgz"},
		{Revision: "abc123", Key: "source_cache/v1/some-org/some-repo/base/abc123/k2.tgz"},
	}
	policy, err := sourceCacheSessionPolicy("source-cache", restoreKeys, restoreKeys[0])
	require.NoError(t, err)

	// Reads are the exact restore keys and writes are the exact save key, so a
	// session cannot touch any other artifact.
	assert.JSONEq(t, `{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Sid": "SourceCacheRead",
				"Effect": "Allow",
				"Action": ["s3:GetObject"],
				"Resource": [
					"arn:aws:s3:::source-cache/source_cache/v1/some-org/some-repo/pr/pr-head/k1.tgz",
					"arn:aws:s3:::source-cache/source_cache/v1/some-org/some-repo/base/abc123/k2.tgz"
				]
			},
			{
				"Sid": "SourceCacheWrite",
				"Effect": "Allow",
				"Action": ["s3:PutObject", "s3:CreateMultipartUpload", "s3:UploadPart", "s3:CompleteMultipartUpload", "s3:AbortMultipartUpload"],
				"Resource": ["arn:aws:s3:::source-cache/source_cache/v1/some-org/some-repo/pr/pr-head/k1.tgz"]
			}
		]
	}`, policy)
}

// sourceCachePlanKeyParts returns the namespace and revision the key's object sits under.
func sourceCachePlanKeyParts(key string) (namespace, revision string) {
	parts := strings.Split(key, "/")
	return parts[4], parts[5]
}

func TestSourceCachePlan(t *testing.T) {
	for tName, tCase := range map[string]struct {
		requester string
		head      string
		want      [][2]string
	}{
		"MainlineRestoresTheBaseArtifact": {
			requester: evergreen.RepotrackerVersionRequester,
			want:      [][2]string{{"base", "abc123"}},
		},
		"PullRequestRestoresThePRHeadThenTheBaseArtifact": {
			requester: evergreen.GithubPRRequester,
			head:      "55ca6286e3e4f4fba5d0448333fa99fc5a404a73",
			want:      [][2]string{{"pr", "55ca6286e3e4f4fba5d0448333fa99fc5a404a73"}, {"base", "abc123"}},
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(task.Collection, model.ProjectRefCollection, patch.Collection))
			const versionID = "5bedc62ee4055d31f0340b1d"
			if tCase.head != "" {
				patchDoc := patch.Patch{
					Id:              mgobson.ObjectIdHex(versionID),
					GithubPatchData: thirdparty.GithubPatch{HeadHash: tCase.head},
				}
				require.NoError(t, patchDoc.Insert(t.Context()))
			}
			tsk := &task.Task{Id: sourceCacheTaskID, Requester: tCase.requester, Version: versionID, Revision: "abc123"}
			pRef := &model.ProjectRef{Id: sourceCacheProjectID, Owner: "some-org", Repo: "some-repo"}

			plan, err := buildSourceCachePlan(t.Context(), tsk, pRef, apimodels.SourceCacheCredentialsRequest{Branch: "main", CloneDepth: 1000})
			require.NoError(t, err)
			require.Len(t, plan.restoreKeys, len(tCase.want))
			for i, want := range tCase.want {
				namespace, revision := sourceCachePlanKeyParts(plan.restoreKeys[i].Key)
				assert.Equal(t, want[0], namespace)
				assert.Equal(t, want[1], revision)
			}
			assert.Equal(t, plan.restoreKeys[0], plan.saveKey)
		})
	}
}
