package service

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLimitedProjectEndPoint(t *testing.T) {
	require.NoError(t, db.Clear(model.ProjectRefCollection))

	testutil.DisablePermissionsForTests()
	defer testutil.EnablePermissionsForTests()
	testConfig := testutil.TestConfig()
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
	request.AddCookie(&http.Cookie{Name: evergreen.AuthTokenCookie, Value: "token"})
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(request)
	require.NoError(t, err, "problem making request")
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)
	limitedRef := restModel.APIProjectRef{}

	err = utility.ReadJSON(resp.Body, &limitedRef)
	require.NoError(t, err)

	assert.Equal(t, "repo", utility.FromStringPtr(limitedRef.Repo))
	assert.Equal(t, ref, utility.FromStringPtr(limitedRef.Id))
	assert.Equal(t, ref, utility.FromStringPtr(limitedRef.Identifier))
	assert.Nil(t, limitedRef.GitTagVersionsEnabled)
	assert.Nil(t, limitedRef.DisplayName)
	assert.Nil(t, limitedRef.DeactivatePrevious)
}
