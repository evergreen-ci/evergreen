package route

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeClonePatchRequest(patchID, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/rest/v2/patches/p/clone", strings.NewReader(body))
	return gimlet.SetURLVars(req, map[string]string{"patch_id": patchID})
}

func TestPatchCloneParseValidation(t *testing.T) {
	for testName, testCase := range map[string]struct {
		patchID     string
		body        string
		expectedErr string
	}{
		"MissingPatchID": {
			patchID:     "",
			body:        "",
			expectedErr: "missing patch ID",
		},
		"InvalidPatchID": {
			patchID:     "not-an-object-id",
			body:        "",
			expectedErr: "not an ObjectId",
		},
		"EmptyBodyIsOptional": {
			patchID: mgobson.NewObjectId().Hex(),
			body:    "",
		},
		"ValidBody": {
			patchID: mgobson.NewObjectId().Hex(),
			body:    `{"description": "cloned patch", "finalize": true}`,
		},
	} {
		t.Run(testName, func(t *testing.T) {
			h := &patchCloneHandler{}
			err := h.Parse(t.Context(), makeClonePatchRequest(testCase.patchID, testCase.body))
			if testCase.expectedErr == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, testCase.expectedErr)
			}
		})
	}
}

func TestPatchCloneRun(t *testing.T) {
	ctx := t.Context()

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

	require.NoError(t, db.ClearCollections(patch.Collection, user.Collection, model.ProjectRefCollection))

	require.NoError(t, (&model.ProjectRef{
		Id:         "project",
		Identifier: "project",
		Enabled:    true,
		Owner:      "evergreen-ci",
		Repo:       "evergreen",
	}).Insert(ctx))

	usr := &user.DBUser{Id: "testuser"}

	t.Run("PatchNotFound", func(t *testing.T) {
		handler := makeClonePatchHandler(env).(*patchCloneHandler)
		handler.patchID = mgobson.NewObjectId().Hex()
		handler.body = apimodels.ClonePatchRequest{}
		res := handler.Run(gimlet.AttachUser(ctx, usr))
		assert.Equal(t, http.StatusNotFound, res.Status())
	})

	t.Run("MergeQueuePatch", func(t *testing.T) {
		source := patch.Patch{
			Id:              mgobson.NewObjectId(),
			Project:         "project",
			Githash:         "abc123",
			GithubMergeData: thirdparty.GithubMergeGroup{HeadSHA: "abcd1234"},
		}
		require.NoError(t, source.Insert(ctx))

		handler := makeClonePatchHandler(env).(*patchCloneHandler)
		handler.patchID = source.Id.Hex()
		handler.body = apimodels.ClonePatchRequest{}
		res := handler.Run(gimlet.AttachUser(ctx, usr))
		assert.Equal(t, http.StatusBadRequest, res.Status())
	})

	t.Run("ImpersonationWithoutPermission", func(t *testing.T) {
		source := patch.Patch{
			Id:      mgobson.NewObjectId(),
			Project: "project",
			Githash: "abc123",
		}
		require.NoError(t, source.Insert(ctx))

		handler := makeClonePatchHandler(env).(*patchCloneHandler)
		handler.patchID = source.Id.Hex()
		handler.body = apimodels.ClonePatchRequest{Author: "someone_else"}
		res := handler.Run(gimlet.AttachUser(ctx, usr))
		assert.Equal(t, http.StatusForbidden, res.Status())
	})
}

func TestCloneIntentParams(t *testing.T) {
	raw := &restModel.APIRawPatch{Patch: restModel.APIRawModule{Diff: "base diff"}}

	t.Run("FinalizedSourceReusesDefinitionViaRepeat", func(t *testing.T) {
		source := patch.Patch{
			Id:          mgobson.NewObjectId(),
			Githash:     "abc123",
			Path:        "custom/evergreen.yml",
			Description: "original description",
			Version:     "some_version",
			Triggers:    patch.TriggerInfo{Aliases: []string{"downstream_alias"}},
		}

		params := cloneIntentParams(&source, raw, "cloned description")

		assert.Equal(t, "abc123", params.BaseGitHash)
		assert.Equal(t, "base diff", params.PatchContent)
		assert.Equal(t, "custom/evergreen.yml", params.Path)
		assert.Equal(t, "cloned description", params.Description)
		// The repeat logic reproduces the source's exact per-variant task mapping.
		assert.True(t, params.RepeatDefinition)
		assert.Equal(t, source.Id.Hex(), params.RepeatPatchId)
		// Trigger aliases must carry over so the clone reproduces the source's child patches.
		assert.Equal(t, []string{"downstream_alias"}, params.TriggerAliases)
	})

	t.Run("UnfinalizedSourceHasNoRepeat", func(t *testing.T) {
		source := patch.Patch{Id: mgobson.NewObjectId(), Githash: "abc123"}

		params := cloneIntentParams(&source, raw, "desc")

		assert.False(t, params.RepeatDefinition)
		assert.Empty(t, params.RepeatPatchId)
		assert.Empty(t, params.TriggerAliases)
	})
}
