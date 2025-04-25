# \OwnerAPI

All URIs are relative to *http://localhost*

Method | HTTP request | Description
------------- | ------------- | -------------
[**ByFoliageLogicApiOwnerByFoliageLogicTaskIdGet**](OwnerAPI.md#ByFoliageLogicApiOwnerByFoliageLogicTaskIdGet) | **Get** /api/owner/by_foliage_logic/{task_id} | By Foliage Logic
[**ByJiraKeyApiOwnerByJiraKeyJiraKeyGet**](OwnerAPI.md#ByJiraKeyApiOwnerByJiraKeyJiraKeyGet) | **Get** /api/owner/by_jira_key/{jira_key} | By Jira Key
[**GetRegexMappingApiOwnerRegexByProjectProjectIdGet**](OwnerAPI.md#GetRegexMappingApiOwnerRegexByProjectProjectIdGet) | **Get** /api/owner/regex_by_project/{project_id} | Get Regex Mapping



## ByFoliageLogicApiOwnerByFoliageLogicTaskIdGet

> FinalAssignmentResults ByFoliageLogicApiOwnerByFoliageLogicTaskIdGet(ctx, taskId).TestFileName(testFileName).OffendingVersionId(offendingVersionId).Execute()

By Foliage Logic



### Example

```go
package main

import (
	"context"
	"fmt"
	"os"
	openapiclient "github.com/evergreen-ci/evergreen"
)

func main() {
	taskId := "taskId_example" // string | 
	testFileName := "testFileName_example" // string |  (optional)
	offendingVersionId := "offendingVersionId_example" // string |  (optional)

	configuration := openapiclient.NewConfiguration()
	apiClient := openapiclient.NewAPIClient(configuration)
	resp, r, err := apiClient.OwnerAPI.ByFoliageLogicApiOwnerByFoliageLogicTaskIdGet(context.Background(), taskId).TestFileName(testFileName).OffendingVersionId(offendingVersionId).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error when calling `OwnerAPI.ByFoliageLogicApiOwnerByFoliageLogicTaskIdGet``: %v\n", err)
		fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
	}
	// response from `ByFoliageLogicApiOwnerByFoliageLogicTaskIdGet`: FinalAssignmentResults
	fmt.Fprintf(os.Stdout, "Response from `OwnerAPI.ByFoliageLogicApiOwnerByFoliageLogicTaskIdGet`: %v\n", resp)
}
```

### Path Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
**ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
**taskId** | **string** |  | 

### Other Parameters

Other parameters are passed through a pointer to a apiByFoliageLogicApiOwnerByFoliageLogicTaskIdGetRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------

 **testFileName** | **string** |  | 
 **offendingVersionId** | **string** |  | 

### Return type

[**FinalAssignmentResults**](FinalAssignmentResults.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)


## ByJiraKeyApiOwnerByJiraKeyJiraKeyGet

> TeamDataWithOwner ByJiraKeyApiOwnerByJiraKeyJiraKeyGet(ctx, jiraKey).ProjectId(projectId).Execute()

By Jira Key



### Example

```go
package main

import (
	"context"
	"fmt"
	"os"
	openapiclient "github.com/evergreen-ci/evergreen"
)

func main() {
	jiraKey := "jiraKey_example" // string | 
	projectId := "projectId_example" // string |  (optional)

	configuration := openapiclient.NewConfiguration()
	apiClient := openapiclient.NewAPIClient(configuration)
	resp, r, err := apiClient.OwnerAPI.ByJiraKeyApiOwnerByJiraKeyJiraKeyGet(context.Background(), jiraKey).ProjectId(projectId).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error when calling `OwnerAPI.ByJiraKeyApiOwnerByJiraKeyJiraKeyGet``: %v\n", err)
		fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
	}
	// response from `ByJiraKeyApiOwnerByJiraKeyJiraKeyGet`: TeamDataWithOwner
	fmt.Fprintf(os.Stdout, "Response from `OwnerAPI.ByJiraKeyApiOwnerByJiraKeyJiraKeyGet`: %v\n", resp)
}
```

### Path Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
**ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
**jiraKey** | **string** |  | 

### Other Parameters

Other parameters are passed through a pointer to a apiByJiraKeyApiOwnerByJiraKeyJiraKeyGetRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------

 **projectId** | **string** |  | 

### Return type

[**TeamDataWithOwner**](TeamDataWithOwner.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)


## GetRegexMappingApiOwnerRegexByProjectProjectIdGet

> map[string]GetRegexMappingApiOwnerRegexByProjectProjectIdGet200ResponseValue GetRegexMappingApiOwnerRegexByProjectProjectIdGet(ctx, projectId).Execute()

Get Regex Mapping



### Example

```go
package main

import (
	"context"
	"fmt"
	"os"
	openapiclient "github.com/evergreen-ci/evergreen"
)

func main() {
	projectId := "projectId_example" // string | 

	configuration := openapiclient.NewConfiguration()
	apiClient := openapiclient.NewAPIClient(configuration)
	resp, r, err := apiClient.OwnerAPI.GetRegexMappingApiOwnerRegexByProjectProjectIdGet(context.Background(), projectId).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error when calling `OwnerAPI.GetRegexMappingApiOwnerRegexByProjectProjectIdGet``: %v\n", err)
		fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
	}
	// response from `GetRegexMappingApiOwnerRegexByProjectProjectIdGet`: map[string]GetRegexMappingApiOwnerRegexByProjectProjectIdGet200ResponseValue
	fmt.Fprintf(os.Stdout, "Response from `OwnerAPI.GetRegexMappingApiOwnerRegexByProjectProjectIdGet`: %v\n", resp)
}
```

### Path Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
**ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
**projectId** | **string** |  | 

### Other Parameters

Other parameters are passed through a pointer to a apiGetRegexMappingApiOwnerRegexByProjectProjectIdGetRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------


### Return type

[**map[string]GetRegexMappingApiOwnerRegexByProjectProjectIdGet200ResponseValue**](GetRegexMappingApiOwnerRegexByProjectProjectIdGet200ResponseValue.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)

