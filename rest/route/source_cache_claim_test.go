package route

import (
	"net/http"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sourceCacheRevision = "abc123"

// sourceCacheRacerTaskID is a second task simulating a producer racing the one under test.
const sourceCacheRacerTaskID = "source_cache_racer"

func findStoredClaim(t *testing.T, id string) *model.SourceCacheClaim {
	claim, err := model.FindSourceCacheClaim(t.Context(), id)
	require.NoError(t, err)
	return claim
}

func newSourceCacheClaimRequest(t *testing.T, taskID string, body string) *http.Request {
	request, err := http.NewRequest(http.MethodPost, "/task/"+taskID+"/source_cache/claim", strings.NewReader(body))
	require.NoError(t, err)
	request = gimlet.SetURLVars(request, map[string]string{"task_id": taskID})
	return request
}

func TestSourceCacheClaimParse(t *testing.T) {
	for tName, tCase := range map[string]struct {
		taskID    string
		body      string
		shouldErr bool
	}{
		"ValidBodyParses": {
			taskID: sourceCacheTaskID,
			body:   `{}`,
		},
		"MissingTaskIDIsAnError": {
			body:      `{}`,
			shouldErr: true,
		},
		"MalformedBodyIsAnError": {
			taskID:    sourceCacheTaskID,
			body:      `{invalid`,
			shouldErr: true,
		},
	} {
		t.Run(tName, func(t *testing.T) {
			handler := makeSourceCacheClaim(sourceCacheTestSettings())
			err := handler.Parse(t.Context(), newSourceCacheClaimRequest(t, tCase.taskID, tCase.body))
			if tCase.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSourceCacheClaimRun(t *testing.T) {
	for tName, tCase := range map[string]struct {
		insertTask     bool
		mode           model.SourceCacheMode
		preClaim       bool
		expectedStatus int
		wantClaimed    bool
	}{
		"UnknownTaskIsNotFound": {
			expectedStatus: http.StatusNotFound,
		},
		"ProjectWithDisabledModeIsRefused": {
			insertTask:     true,
			mode:           model.SourceCacheModeDisabled,
			expectedStatus: http.StatusConflict,
		},
		"FirstTaskClaimsTheSave": {
			insertTask:     true,
			mode:           model.SourceCacheModeAll,
			expectedStatus: http.StatusOK,
			wantClaimed:    true,
		},
		"RacingTaskIsDeniedTheSave": {
			insertTask:     true,
			mode:           model.SourceCacheModeAll,
			preClaim:       true,
			expectedStatus: http.StatusOK,
			wantClaimed:    false,
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(evergreen.SourceCacheClaimsCollection, task.Collection, model.ProjectRefCollection, patch.Collection))
			handler, ok := makeSourceCacheClaim(sourceCacheTestSettings()).(*sourceCacheClaim)
			require.True(t, ok)
			if tCase.insertTask {
				insertSourceCacheTask(t, evergreen.RepotrackerVersionRequester, sourceCacheMainlineVersionID, sourceCacheRevision, "some-org", "some-repo", tCase.mode)
			}
			require.NoError(t, handler.Parse(t.Context(), newSourceCacheClaimRequest(t, sourceCacheTaskID, `{}`)))

			if tCase.preClaim {
				// A real racer is a different task, so a claim owned by the task under test would be re-taken by its owner.
				racer := task.Task{Id: sourceCacheRacerTaskID, Project: sourceCacheProjectID, Requester: evergreen.RepotrackerVersionRequester, Version: sourceCacheMainlineVersionID, Revision: sourceCacheRevision}
				require.NoError(t, racer.Insert(t.Context()))
				racerHandler := makeSourceCacheClaim(sourceCacheTestSettings())
				require.NoError(t, racerHandler.Parse(t.Context(), newSourceCacheClaimRequest(t, sourceCacheRacerTaskID, `{}`)))
				preClaimResp := racerHandler.Run(t.Context())
				require.Equal(t, http.StatusOK, preClaimResp.Status(), preClaimResp.Data())
			}

			resp := handler.Run(t.Context())
			require.Equal(t, tCase.expectedStatus, resp.Status(), resp.Data())
			if tCase.expectedStatus != http.StatusOK {
				return
			}

			claim, ok := resp.Data().(apimodels.SourceCacheClaimResponse)
			require.True(t, ok)
			assert.Equal(t, tCase.wantClaimed, claim.Claimed)
			if tCase.wantClaimed {
				// The claim ID is keyed on the mainline namespace because the test task is not a patch.
				claimKey := evergreen.SourceCacheClaimID(sourceCacheTestSettings().Buckets.SourceCacheBucket.Name,
					evergreen.SourceCacheObjectKey("some-org", "some-repo", evergreen.SourceCacheBaseNamespace, "", sourceCacheRevision, 0, false))
				stored := findStoredClaim(t, claimKey)
				require.NotNil(t, stored)
				assert.Equal(t, sourceCacheTaskID, stored.Owner, "the claim must be owned by the task that won it")
			}
		})
	}
}

func TestSourceCacheClaimResolveConflicts(t *testing.T) {
	for tName, tCase := range map[string]struct {
		blankBucket    bool
		repo           string
		expectedStatus int
	}{
		"NoBucketConfiguredIsRefused": {
			blankBucket:    true,
			repo:           "some-repo",
			expectedStatus: http.StatusConflict,
		},
		"NoOwnerAndRepoIsRefused": {
			repo:           "",
			expectedStatus: http.StatusConflict,
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(task.Collection, model.ProjectRefCollection, patch.Collection))
			insertSourceCacheTask(t, evergreen.RepotrackerVersionRequester, sourceCacheMainlineVersionID, sourceCacheRevision, "some-org", tCase.repo, model.SourceCacheModeAll)
			settings := sourceCacheTestSettings()
			if tCase.blankBucket {
				settings.Buckets.SourceCacheBucket.Name = ""
			}
			handler := makeSourceCacheClaim(settings)
			require.NoError(t, handler.Parse(t.Context(), newSourceCacheClaimRequest(t, sourceCacheTaskID, `{}`)))
			resp := handler.Run(t.Context())
			assert.Equal(t, tCase.expectedStatus, resp.Status(), resp.Data())
		})
	}
}
