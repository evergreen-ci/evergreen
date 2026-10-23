package route

import (
	"context"
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
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeClonePatchRequest(t *testing.T, patchID, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/rest/v2/patches/p/clone", strings.NewReader(body))
	return gimlet.SetURLVars(req, map[string]string{"patch_id": patchID})
}

func TestPatchCloneParseValidation(t *testing.T) {
	h := &patchCloneHandler{}

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
			h.body = apimodels.ClonePatchRequest{}
			err := h.Parse(context.Background(), makeClonePatchRequest(t, testCase.patchID, testCase.body))
			if testCase.expectedErr == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, testCase.expectedErr)
			}
		})
	}
}

func TestPatchCloneRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
	handler := makeClonePatchHandler(env).(*patchCloneHandler)

	t.Run("PatchNotFound", func(t *testing.T) {
		handler.patchID = mgobson.NewObjectId().Hex()
		handler.body = apimodels.ClonePatchRequest{}
		res := handler.Run(gimlet.AttachUser(ctx, usr))
		assert.Equal(t, http.StatusNotFound, res.Status())
	})

	t.Run("FinalizeWithoutTasksOrVariants", func(t *testing.T) {
		source := patch.Patch{
			Id:          mgobson.NewObjectId(),
			Description: "no definition",
			Project:     "project",
			Githash:     "abc123",
		}
		require.NoError(t, source.Insert(ctx))

		handler.patchID = source.Id.Hex()
		handler.body = apimodels.ClonePatchRequest{Finalize: true}
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

		handler.patchID = source.Id.Hex()
		handler.body = apimodels.ClonePatchRequest{Author: "someone_else"}
		res := handler.Run(gimlet.AttachUser(ctx, usr))
		assert.Equal(t, http.StatusForbidden, res.Status())
	})
}
