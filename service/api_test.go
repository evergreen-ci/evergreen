package service

import (
	"bytes"
	"context"
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	assert.NoError(db.Clear(model.ProjectRefCollection))

	testutil.DisablePermissionsForTests()
	defer testutil.EnablePermissionsForTests()
	testConfig := testutil.TestConfig()
	testApiServer, err := CreateTestServer(ctx, testConfig, nil, false)
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
	assert.NoError(err)
	resp, err := http.DefaultClient.Do(request)
	require.NoError(t, err, "problem making request")
	defer resp.Body.Close()
	assert.Equal(200, resp.StatusCode)

	limitedRef := restModel.APIProjectRef{}

	err = utility.ReadJSON(resp.Body, &limitedRef)
	assert.NoError(err)

	assert.Equal("repo", utility.FromStringPtr(limitedRef.Repo))
	assert.Equal(ref, utility.FromStringPtr(limitedRef.Id))
	assert.Equal(ref, utility.FromStringPtr(limitedRef.Identifier))
	assert.Nil(limitedRef.GitTagVersionsEnabled)
	assert.Nil(limitedRef.DisplayName)
	assert.Nil(limitedRef.DeactivatePrevious)

}
