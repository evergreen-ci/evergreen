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

func newSourceCacheClaimRequest(t *testing.T, taskID string) *http.Request {
	request, err := http.NewRequest(http.MethodPost, "/task/"+taskID+"/source_cache/claim", strings.NewReader(`{}`))
	require.NoError(t, err)
	request = gimlet.SetURLVars(request, map[string]string{"task_id": taskID})
	return request
}

// sourceCacheRacerTaskID is a second task simulating a producer racing the one under test.
const sourceCacheRacerTaskID = "source_cache_racer"

func newSourceCacheClaimReleaseRequest(t *testing.T) *http.Request {
	request, err := http.NewRequest(http.MethodDelete, "/task/"+sourceCacheTaskID+"/source_cache/claim", strings.NewReader(`{}`))
	require.NoError(t, err)
	request = gimlet.SetURLVars(request, map[string]string{"task_id": sourceCacheTaskID})
	return request
}

func TestSourceCacheClaimRun(t *testing.T) {
	for tName, tCase := range map[string]struct {
		insertTask     bool
		mode           model.SourceCacheMode
		preClaim       bool
		revision       string
		expectedStatus int
		wantClaimed    bool
	}{
		"UnknownTaskIsNotFound": {
			expectedStatus: http.StatusNotFound,
		},
		"ProjectWithDisabledModeIsRefused": {
			insertTask:     true,
			mode:           model.SourceCacheModeDisabled,
			revision:       "abc123",
			expectedStatus: http.StatusConflict,
		},
		"FirstTaskClaimsTheSave": {
			insertTask:     true,
			mode:           model.SourceCacheModeAll,
			revision:       "abc123",
			expectedStatus: http.StatusOK,
			wantClaimed:    true,
		},
		"RacingTaskIsDeniedTheSave": {
			insertTask:     true,
			mode:           model.SourceCacheModeAll,
			preClaim:       true,
			revision:       "abc123",
			expectedStatus: http.StatusOK,
			wantClaimed:    false,
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(evergreen.SourceCacheClaimsCollection, task.Collection, model.ProjectRefCollection, patch.Collection))
			handler, ok := makeSourceCacheClaim(sourceCacheTestSettings()).(*sourceCacheClaim)
			require.True(t, ok)
			if tCase.insertTask {
				insertSourceCacheTask(t, evergreen.RepotrackerVersionRequester, sourceCacheMainlineVersionID, tCase.revision, "some-org", "some-repo", tCase.mode)
			}
			require.NoError(t, handler.Parse(t.Context(), newSourceCacheClaimRequest(t, sourceCacheTaskID)))

			if tCase.preClaim {
				// A real racer is a different task, so a claim owned by the task under test would be re-taken by its owner.
				racer := task.Task{Id: sourceCacheRacerTaskID, Project: sourceCacheProjectID, Requester: evergreen.RepotrackerVersionRequester, Version: sourceCacheMainlineVersionID, Revision: tCase.revision}
				require.NoError(t, racer.Insert(t.Context()))
				racerHandler := makeSourceCacheClaim(sourceCacheTestSettings())
				require.NoError(t, racerHandler.Parse(t.Context(), newSourceCacheClaimRequest(t, sourceCacheRacerTaskID)))
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
				assert.NotEmpty(t, claim.Key)
			}
		})
	}
}

func TestSourceCacheClaimReleaseFreesTheClaimedKey(t *testing.T) {
	ctx := t.Context()
	require.NoError(t, db.ClearCollections(evergreen.SourceCacheClaimsCollection, task.Collection, model.ProjectRefCollection, patch.Collection))
	insertSourceCacheTask(t, evergreen.RepotrackerVersionRequester, sourceCacheMainlineVersionID, "abc123", "some-org", "some-repo", model.SourceCacheModeAll)

	claimHandler := makeSourceCacheClaim(sourceCacheTestSettings())
	require.NoError(t, claimHandler.Parse(ctx, newSourceCacheClaimRequest(t, sourceCacheTaskID)))
	claimResp := claimHandler.Run(ctx)
	require.Equal(t, http.StatusOK, claimResp.Status(), claimResp.Data())

	releaseHandler := makeSourceCacheClaimRelease(sourceCacheTestSettings())
	require.NoError(t, releaseHandler.Parse(ctx, newSourceCacheClaimReleaseRequest(t)))
	resp := releaseHandler.Run(ctx)
	require.Equal(t, http.StatusOK, resp.Status(), resp.Data())
	release, ok := resp.Data().(apimodels.SourceCacheReleaseResponse)
	require.True(t, ok)
	assert.True(t, release.Released)

	reclaimResp := claimHandler.Run(ctx)
	require.Equal(t, http.StatusOK, reclaimResp.Status(), reclaimResp.Data())
	reclaim, ok := reclaimResp.Data().(apimodels.SourceCacheClaimResponse)
	require.True(t, ok)
	assert.True(t, reclaim.Claimed, "a released key must be claimable again")
}
