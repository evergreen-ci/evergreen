package service

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	serviceTestUtil "github.com/evergreen-ci/evergreen/service/testutil"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerEndpoints(t *testing.T) {
	require.NoError(t, db.Clear(model.ProjectRefCollection))

	testutil.DisablePermissionsForTests()
	defer testutil.EnablePermissionsForTests()
	testConfig := testutil.TestConfig()

	testCases := []struct {
		name                   string
		staticAPIKeysDisabled  bool
		authHeader             map[string]string
		expectedStatusCode     int
		expectedErrorSubstring string
	}{
		{
			name:                  "UsingCookies",
			staticAPIKeysDisabled: false,
			authHeader:            map[string]string{evergreen.AuthTokenCookie: "token"},
			expectedStatusCode:    200,
		},
		{
			name:                  "UsingStaticAPIKeys",
			staticAPIKeysDisabled: false,
			authHeader: map[string]string{
				evergreen.APIUserHeader: serviceTestUtil.MockUser.Id,
				evergreen.APIKeyHeader:  serviceTestUtil.MockUser.APIKey,
			},
			expectedStatusCode: 200,
		},
		{
			name:                  "UsingStaticAPIKeys/DisabledStaticAPIKeys",
			staticAPIKeysDisabled: true,
			authHeader: map[string]string{
				evergreen.APIUserHeader: serviceTestUtil.MockUser.Id,
				evergreen.APIKeyHeader:  serviceTestUtil.MockUser.APIKey,
			},
			expectedStatusCode:     401,
			expectedErrorSubstring: "static API keys are disabled for human users",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			require.NoError(t, db.DropCollections(model.ProjectRefCollection))

			evergreen.GetEnvironment().Settings().ServiceFlags.StaticAPIKeysDisabled = testCase.staticAPIKeysDisabled
			testApiServer, err := CreateTestServer(t.Context(), testConfig, nil, false)
			require.NoError(t, err, "failed to create new API server")
			defer testApiServer.Close()

			const (
				path = "/api/ref/%s"
				ref  = "test-project"
			)
			project := model.ProjectRef{
				Id:                    ref,
				Identifier:            ref,
				Repo:                  "repo",
				GitTagVersionsEnabled: utility.TruePtr(),
				DeactivatePrevious:    utility.TruePtr(),
				DisplayName:           "display",
			}

			require.NoError(t, project.Insert(t.Context()))

			url := testApiServer.URL + path

			request, err := http.NewRequest("GET", fmt.Sprintf(url, ref), bytes.NewBuffer([]byte{}))
			require.NoError(t, err)

			for header, value := range testCase.authHeader {
				if header == evergreen.AuthTokenCookie {
					request.AddCookie(&http.Cookie{Name: evergreen.AuthTokenCookie, Value: value})
				} else {
					request.Header.Set(header, value)
				}
			}

			resp, err := http.DefaultClient.Do(request)
			require.NoError(t, err, "problem making request")
			defer resp.Body.Close()

			require.Equal(t, testCase.expectedStatusCode, resp.StatusCode)

			if testCase.expectedStatusCode != 200 {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				assert.Contains(t, string(body), testCase.expectedErrorSubstring)
				return
			}

			limitedRef := restModel.APIProjectRef{}
			err = utility.ReadJSON(resp.Body, &limitedRef)
			require.NoError(t, err)
			assert.Equal(t, "repo", utility.FromStringPtr(limitedRef.Repo))
			assert.Equal(t, "test-project", utility.FromStringPtr(limitedRef.Id))
			assert.Equal(t, "test-project", utility.FromStringPtr(limitedRef.Identifier))
			assert.Nil(t, limitedRef.GitTagVersionsEnabled)
			assert.Nil(t, limitedRef.DisplayName)
			assert.Nil(t, limitedRef.DeactivatePrevious)
		})
	}
}
