/*
Foliage Web Services

Testing OwnerAPIService

*/

// Code generated by OpenAPI Generator (https://openapi-generator.tech);

package openapi

import (
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	openapiclient "github.com/GIT_USER_ID/GIT_REPO_ID"
)

func Test_openapi_OwnerAPIService(t *testing.T) {

	configuration := openapiclient.NewConfiguration()
	apiClient := openapiclient.NewAPIClient(configuration)

	t.Run("Test OwnerAPIService ByFoliageLogicApiOwnerByFoliageLogicTaskIdGet", func(t *testing.T) {

		t.Skip("skip test")  // remove to run test

		var taskId string

		resp, httpRes, err := apiClient.OwnerAPI.ByFoliageLogicApiOwnerByFoliageLogicTaskIdGet(context.Background(), taskId).Execute()

		require.Nil(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, 200, httpRes.StatusCode)

	})

	t.Run("Test OwnerAPIService ByJiraKeyApiOwnerByJiraKeyJiraKeyGet", func(t *testing.T) {

		t.Skip("skip test")  // remove to run test

		var jiraKey string

		resp, httpRes, err := apiClient.OwnerAPI.ByJiraKeyApiOwnerByJiraKeyJiraKeyGet(context.Background(), jiraKey).Execute()

		require.Nil(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, 200, httpRes.StatusCode)

	})

	t.Run("Test OwnerAPIService GetRegexMappingApiOwnerRegexByProjectProjectIdGet", func(t *testing.T) {

		t.Skip("skip test")  // remove to run test

		var projectId string

		resp, httpRes, err := apiClient.OwnerAPI.GetRegexMappingApiOwnerRegexByProjectProjectIdGet(context.Background(), projectId).Execute()

		require.Nil(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, 200, httpRes.StatusCode)

	})

}
