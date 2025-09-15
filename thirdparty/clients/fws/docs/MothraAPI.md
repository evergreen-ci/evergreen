# \MothraAPI

All URIs are relative to *http://localhost*

Method | HTTP request | Description
------------- | ------------- | -------------
[**GetAllTeamsApiMothraAllTeamsGet**](MothraAPI.md#GetAllTeamsApiMothraAllTeamsGet) | **Get** /api/mothra/all_teams | Get All Teams
[**GetCodeownersByRepoAndFilepathApiMothraCodeownerByRepoAndFilepathOrgRepoNameBranchFilePathGet**](MothraAPI.md#GetCodeownersByRepoAndFilepathApiMothraCodeownerByRepoAndFilepathOrgRepoNameBranchFilePathGet) | **Get** /api/mothra/codeowner_by_repo_and_filepath/{org}/{repo_name}/{branch}/{file_path} | Get Codeowners By Repo And Filepath
[**GetTeamByNameAndProjectApiMothraTeamByNameAndProjectNameProjectGet**](MothraAPI.md#GetTeamByNameAndProjectApiMothraTeamByNameAndProjectNameProjectGet) | **Get** /api/mothra/team_by_name_and_project/{name}/{project} | Get Team By Name And Project
[**GetTeamByRepoAndFilepathApiMothraTeamByRepoAndFilepathOrgRepoNameBranchFilePathGet**](MothraAPI.md#GetTeamByRepoAndFilepathApiMothraTeamByRepoAndFilepathOrgRepoNameBranchFilePathGet) | **Get** /api/mothra/team_by_repo_and_filepath/{org}/{repo_name}/{branch}/{file_path} | Get Team By Repo And Filepath
[**GetTeamByTagApiMothraTeamByTagTagGet**](MothraAPI.md#GetTeamByTagApiMothraTeamByTagTagGet) | **Get** /api/mothra/team_by_tag/{tag} | Get Team By Tag
[**GetTeamProjectsByNameApiMothraTeamProjectsByNameNameGet**](MothraAPI.md#GetTeamProjectsByNameApiMothraTeamProjectsByNameNameGet) | **Get** /api/mothra/team_projects_by_name/{name} | Get Team Projects By Name



## GetAllTeamsApiMothraAllTeamsGet

> []TeamData GetAllTeamsApiMothraAllTeamsGet(ctx).Execute()

Get All Teams



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

	configuration := openapiclient.NewConfiguration()
	apiClient := openapiclient.NewAPIClient(configuration)
	resp, r, err := apiClient.MothraAPI.GetAllTeamsApiMothraAllTeamsGet(context.Background()).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error when calling `MothraAPI.GetAllTeamsApiMothraAllTeamsGet``: %v\n", err)
		fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
	}
	// response from `GetAllTeamsApiMothraAllTeamsGet`: []TeamData
	fmt.Fprintf(os.Stdout, "Response from `MothraAPI.GetAllTeamsApiMothraAllTeamsGet`: %v\n", resp)
}
```

### Path Parameters

This endpoint does not need any parameter.

### Other Parameters

Other parameters are passed through a pointer to a apiGetAllTeamsApiMothraAllTeamsGetRequest struct via the builder pattern


### Return type

[**[]TeamData**](TeamData.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)


## GetCodeownersByRepoAndFilepathApiMothraCodeownerByRepoAndFilepathOrgRepoNameBranchFilePathGet

> []*string GetCodeownersByRepoAndFilepathApiMothraCodeownerByRepoAndFilepathOrgRepoNameBranchFilePathGet(ctx, org, repoName, branch, filePath).Execute()

Get Codeowners By Repo And Filepath



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
	org := "org_example" // string | 
	repoName := "repoName_example" // string | 
	branch := "branch_example" // string | 
	filePath := "filePath_example" // string | 

	configuration := openapiclient.NewConfiguration()
	apiClient := openapiclient.NewAPIClient(configuration)
	resp, r, err := apiClient.MothraAPI.GetCodeownersByRepoAndFilepathApiMothraCodeownerByRepoAndFilepathOrgRepoNameBranchFilePathGet(context.Background(), org, repoName, branch, filePath).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error when calling `MothraAPI.GetCodeownersByRepoAndFilepathApiMothraCodeownerByRepoAndFilepathOrgRepoNameBranchFilePathGet``: %v\n", err)
		fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
	}
	// response from `GetCodeownersByRepoAndFilepathApiMothraCodeownerByRepoAndFilepathOrgRepoNameBranchFilePathGet`: []*string
	fmt.Fprintf(os.Stdout, "Response from `MothraAPI.GetCodeownersByRepoAndFilepathApiMothraCodeownerByRepoAndFilepathOrgRepoNameBranchFilePathGet`: %v\n", resp)
}
```

### Path Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
**ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
**org** | **string** |  | 
**repoName** | **string** |  | 
**branch** | **string** |  | 
**filePath** | **string** |  | 

### Other Parameters

Other parameters are passed through a pointer to a apiGetCodeownersByRepoAndFilepathApiMothraCodeownerByRepoAndFilepathOrgRepoNameBranchFilePathGetRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------





### Return type

**[]*string**

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)


## GetTeamByNameAndProjectApiMothraTeamByNameAndProjectNameProjectGet

> TeamData GetTeamByNameAndProjectApiMothraTeamByNameAndProjectNameProjectGet(ctx, name, project).Execute()

Get Team By Name And Project



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
	name := "name_example" // string | 
	project := "project_example" // string | 

	configuration := openapiclient.NewConfiguration()
	apiClient := openapiclient.NewAPIClient(configuration)
	resp, r, err := apiClient.MothraAPI.GetTeamByNameAndProjectApiMothraTeamByNameAndProjectNameProjectGet(context.Background(), name, project).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error when calling `MothraAPI.GetTeamByNameAndProjectApiMothraTeamByNameAndProjectNameProjectGet``: %v\n", err)
		fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
	}
	// response from `GetTeamByNameAndProjectApiMothraTeamByNameAndProjectNameProjectGet`: TeamData
	fmt.Fprintf(os.Stdout, "Response from `MothraAPI.GetTeamByNameAndProjectApiMothraTeamByNameAndProjectNameProjectGet`: %v\n", resp)
}
```

### Path Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
**ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
**name** | **string** |  | 
**project** | **string** |  | 

### Other Parameters

Other parameters are passed through a pointer to a apiGetTeamByNameAndProjectApiMothraTeamByNameAndProjectNameProjectGetRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------



### Return type

[**TeamData**](TeamData.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)


## GetTeamByRepoAndFilepathApiMothraTeamByRepoAndFilepathOrgRepoNameBranchFilePathGet

> TeamData GetTeamByRepoAndFilepathApiMothraTeamByRepoAndFilepathOrgRepoNameBranchFilePathGet(ctx, org, repoName, branch, filePath).Execute()

Get Team By Repo And Filepath



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
	org := "org_example" // string | 
	repoName := "repoName_example" // string | 
	branch := "branch_example" // string | 
	filePath := "filePath_example" // string | 

	configuration := openapiclient.NewConfiguration()
	apiClient := openapiclient.NewAPIClient(configuration)
	resp, r, err := apiClient.MothraAPI.GetTeamByRepoAndFilepathApiMothraTeamByRepoAndFilepathOrgRepoNameBranchFilePathGet(context.Background(), org, repoName, branch, filePath).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error when calling `MothraAPI.GetTeamByRepoAndFilepathApiMothraTeamByRepoAndFilepathOrgRepoNameBranchFilePathGet``: %v\n", err)
		fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
	}
	// response from `GetTeamByRepoAndFilepathApiMothraTeamByRepoAndFilepathOrgRepoNameBranchFilePathGet`: TeamData
	fmt.Fprintf(os.Stdout, "Response from `MothraAPI.GetTeamByRepoAndFilepathApiMothraTeamByRepoAndFilepathOrgRepoNameBranchFilePathGet`: %v\n", resp)
}
```

### Path Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
**ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
**org** | **string** |  | 
**repoName** | **string** |  | 
**branch** | **string** |  | 
**filePath** | **string** |  | 

### Other Parameters

Other parameters are passed through a pointer to a apiGetTeamByRepoAndFilepathApiMothraTeamByRepoAndFilepathOrgRepoNameBranchFilePathGetRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------





### Return type

[**TeamData**](TeamData.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)


## GetTeamByTagApiMothraTeamByTagTagGet

> TeamData GetTeamByTagApiMothraTeamByTagTagGet(ctx, tag).Execute()

Get Team By Tag



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
	tag := "tag_example" // string | 

	configuration := openapiclient.NewConfiguration()
	apiClient := openapiclient.NewAPIClient(configuration)
	resp, r, err := apiClient.MothraAPI.GetTeamByTagApiMothraTeamByTagTagGet(context.Background(), tag).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error when calling `MothraAPI.GetTeamByTagApiMothraTeamByTagTagGet``: %v\n", err)
		fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
	}
	// response from `GetTeamByTagApiMothraTeamByTagTagGet`: TeamData
	fmt.Fprintf(os.Stdout, "Response from `MothraAPI.GetTeamByTagApiMothraTeamByTagTagGet`: %v\n", resp)
}
```

### Path Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
**ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
**tag** | **string** |  | 

### Other Parameters

Other parameters are passed through a pointer to a apiGetTeamByTagApiMothraTeamByTagTagGetRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------


### Return type

[**TeamData**](TeamData.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)


## GetTeamProjectsByNameApiMothraTeamProjectsByNameNameGet

> map[string]TeamData GetTeamProjectsByNameApiMothraTeamProjectsByNameNameGet(ctx, name).Execute()

Get Team Projects By Name



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
	name := "name_example" // string | 

	configuration := openapiclient.NewConfiguration()
	apiClient := openapiclient.NewAPIClient(configuration)
	resp, r, err := apiClient.MothraAPI.GetTeamProjectsByNameApiMothraTeamProjectsByNameNameGet(context.Background(), name).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error when calling `MothraAPI.GetTeamProjectsByNameApiMothraTeamProjectsByNameNameGet``: %v\n", err)
		fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
	}
	// response from `GetTeamProjectsByNameApiMothraTeamProjectsByNameNameGet`: map[string]TeamData
	fmt.Fprintf(os.Stdout, "Response from `MothraAPI.GetTeamProjectsByNameApiMothraTeamProjectsByNameNameGet`: %v\n", resp)
}
```

### Path Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
**ctx** | **context.Context** | context for authentication, logging, cancellation, deadlines, tracing, etc.
**name** | **string** |  | 

### Other Parameters

Other parameters are passed through a pointer to a apiGetTeamProjectsByNameApiMothraTeamProjectsByNameNameGetRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------


### Return type

[**map[string]TeamData**](TeamData.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)

