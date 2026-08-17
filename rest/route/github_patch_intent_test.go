package route

import (
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGithubPatchIntentProcessingError(t *testing.T) {
	require.NoError(t, db.ClearCollections(patch.IntentCollection, patch.Collection))

	intent, err := patch.NewGithubIntent(t.Context(), "intent-id", "", "", "", "", testutil.NewGithubPR(448,
		"evergreen-ci/evergreen", "base-hash", "octocat/evergreen", "head-hash", "octocat", "PR title"))
	require.NoError(t, err)
	require.NoError(t, intent.Insert(t.Context()))

	processingError := "building GitHub patch document: GitHub returned 500"
	require.NoError(t, patch.SetIntentProcessingError(t.Context(), intent.ID(), intent.GetType(), processingError))

	t.Run("ReturnsStoredProcessingError", func(t *testing.T) {
		handler := makeGithubPatchIntentProcessingError().(*githubPatchIntentProcessingErrorHandler)
		req, err := http.NewRequest(http.MethodGet, "/github/patch-intents/intent-id", nil)
		require.NoError(t, err)
		req = gimlet.SetURLVars(req, map[string]string{"intent_id": intent.ID()})
		require.NoError(t, handler.Parse(t.Context(), req))

		resp := handler.Run(t.Context())
		require.Equal(t, http.StatusOK, resp.Status())

		data, ok := resp.Data().(githubPatchIntentProcessingErrorResponse)
		require.True(t, ok)
		assert.Equal(t, intent.ID(), data.ID)
		assert.Equal(t, patch.GithubIntentType, data.IntentType)
		assert.Equal(t, processingError, data.ProcessingError)
		assert.Equal(t, "evergreen-ci/evergreen", data.BaseRepoName)
		assert.Equal(t, "octocat/evergreen", data.HeadRepoName)
		assert.Equal(t, 448, data.PRNumber)
		assert.Equal(t, "head-hash", data.HeadHash)
		assert.Equal(t, "https://github.com/evergreen-ci/evergreen/pull/448", data.GitHubPRURL)
	})

	t.Run("ReturnsNotFoundForMissingIntent", func(t *testing.T) {
		handler := &githubPatchIntentProcessingErrorHandler{intentID: "missing"}
		resp := handler.Run(t.Context())
		require.Equal(t, http.StatusNotFound, resp.Status())
	})
}
