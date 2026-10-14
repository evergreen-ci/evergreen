package route

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeCreatePatchRequest(t *testing.T, projectID, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/rest/v2/projects/p/patches", strings.NewReader(body))
	return gimlet.SetURLVars(req, map[string]string{"project_id": projectID})
}

func parseCreatePatchBody(t *testing.T, body string) apimodels.CreatePatchRequest {
	var req apimodels.CreatePatchRequest
	require.NoError(t, json.Unmarshal([]byte(body), &req))
	require.NoError(t, req.Validate())
	return req
}

func TestPatchCreateParseValidation(t *testing.T) {
	h := &patchCreateHandler{}

	for testName, testCase := range map[string]struct {
		projectID   string
		body        string
		expectedErr string
	}{
		"MissingProjectID": {
			projectID:   "",
			body:        `{"base_githash": "abc"}`,
			expectedErr: "missing project ID",
		},
		"MissingBaseGithash": {
			projectID:   "p",
			body:        `{"description": "no base"}`,
			expectedErr: "must specify 'base_githash'",
		},
		"MultipleRepeatOptions": {
			projectID:   "p",
			body:        `{"base_githash": "abc", "reuse_last_definition": true, "repeat_patch_id": "1234"}`,
			expectedErr: "mutually exclusive",
		},
		"EmptyModuleName": {
			projectID:   "p",
			body:        `{"base_githash": "abc", "modules": [{"module_name": "", "githash": "def", "patch": "x"}]}`,
			expectedErr: "module_name must not be empty",
		},
		"DuplicateModuleNames": {
			projectID:   "p",
			body:        `{"base_githash": "abc", "modules": [{"module_name": "m", "githash": "def", "patch": "x"}, {"module_name": "m", "githash": "def", "patch": "y"}]}`,
			expectedErr: "specified more than once",
		},
		"EmptyParameterKey": {
			projectID:   "p",
			body:        `{"base_githash": "abc", "parameters": [{"key": "", "value": "v"}]}`,
			expectedErr: "parameter key must not be empty",
		},
		"FinalizeWithoutTasksOrVariants": {
			projectID:   "p",
			body:        `{"base_githash": "abc", "finalize": true}`,
			expectedErr: "cannot finalize a patch with no tasks or variants",
		},
		"ValidRequestWithoutDiff": {
			projectID: "p",
			body:      `{"base_githash": "abc"}`,
		},
	} {
		t.Run(testName, func(t *testing.T) {
			h.body = apimodels.CreatePatchRequest{}
			err := h.Parse(context.Background(), makeCreatePatchRequest(t, testCase.projectID, testCase.body))
			if testCase.expectedErr == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, testCase.expectedErr)
			}
		})
	}
}

func TestPatchCreateRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

	require.NoError(t, db.ClearCollections(model.ProjectRefCollection, user.Collection, model.VersionCollection))

	require.NoError(t, (&model.ProjectRef{
		Id:         "enabled_project",
		Identifier: "enabled_project",
		Enabled:    true,
		Owner:      "evergreen-ci",
		Repo:       "evergreen",
	}).Insert(ctx))
	require.NoError(t, (&model.ProjectRef{
		Id:               "disabled_project",
		Identifier:       "disabled_project",
		Enabled:          true,
		PatchingDisabled: utility.TruePtr(),
		Owner:            "evergreen-ci",
		Repo:             "evergreen",
	}).Insert(ctx))

	usr := &user.DBUser{Id: "testuser"}

	handler := makeCreatePatchHandler(env).(*patchCreateHandler)

	t.Run("ProjectNotFound", func(t *testing.T) {
		handler.projectID = "no_such_project"
		handler.body = parseCreatePatchBody(t, `{"base_githash": "abc"}`)
		res := handler.Run(gimlet.AttachUser(ctx, usr))
		assert.Equal(t, http.StatusNotFound, res.Status())
	})

	t.Run("PatchingDisabled", func(t *testing.T) {
		handler.projectID = "disabled_project"
		handler.body = parseCreatePatchBody(t, `{"base_githash": "abc"}`)
		res := handler.Run(gimlet.AttachUser(ctx, usr))
		assert.Equal(t, http.StatusBadRequest, res.Status())
	})
}
