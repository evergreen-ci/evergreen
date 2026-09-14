# \JiraAPI

All URIs are relative to *http://localhost*

Method | HTTP request | Description
------------- | ------------- | -------------
[**GetJiraProjectAccessApiJiraProjectAccessGet**](JiraAPI.md#GetJiraProjectAccessApiJiraProjectAccessGet) | **Get** /api/jira/project-access | Get Jira Project Access
[**GetJiraProjectTeamsApiJiraProjectTeamsGet**](JiraAPI.md#GetJiraProjectTeamsApiJiraProjectTeamsGet) | **Get** /api/jira/project-teams | Get Jira Project Teams



## GetJiraProjectAccessApiJiraProjectAccessGet

> JiraProjectAccessResponse GetJiraProjectAccessApiJiraProjectAccessGet(ctx).Project(project).Execute()

Get Jira Project Access



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
	project := "project_example" // string |

	configuration := openapiclient.NewConfiguration()
	apiClient := openapiclient.NewAPIClient(configuration)
	resp, r, err := apiClient.JiraAPI.GetJiraProjectAccessApiJiraProjectAccessGet(context.Background()).Project(project).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error when calling `JiraAPI.GetJiraProjectAccessApiJiraProjectAccessGet``: %v\n", err)
		fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
	}
	// response from `GetJiraProjectAccessApiJiraProjectAccessGet`: JiraProjectAccessResponse
	fmt.Fprintf(os.Stdout, "Response from `JiraAPI.GetJiraProjectAccessApiJiraProjectAccessGet`: %v\n", resp)
}
```

### Path Parameters



### Other Parameters

Other parameters are passed through a pointer to a apiGetJiraProjectAccessApiJiraProjectAccessGetRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **project** | **string** |  |

### Return type

[**JiraProjectAccessResponse**](JiraProjectAccessResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)


## GetJiraProjectTeamsApiJiraProjectTeamsGet

> JiraProjectTeamsResponse GetJiraProjectTeamsApiJiraProjectTeamsGet(ctx).Project(project).Execute()

Get Jira Project Teams



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
	project := "project_example" // string |

	configuration := openapiclient.NewConfiguration()
	apiClient := openapiclient.NewAPIClient(configuration)
	resp, r, err := apiClient.JiraAPI.GetJiraProjectTeamsApiJiraProjectTeamsGet(context.Background()).Project(project).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error when calling `JiraAPI.GetJiraProjectTeamsApiJiraProjectTeamsGet``: %v\n", err)
		fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
	}
	// response from `GetJiraProjectTeamsApiJiraProjectTeamsGet`: JiraProjectTeamsResponse
	fmt.Fprintf(os.Stdout, "Response from `JiraAPI.GetJiraProjectTeamsApiJiraProjectTeamsGet`: %v\n", resp)
}
```

### Path Parameters



### Other Parameters

Other parameters are passed through a pointer to a apiGetJiraProjectTeamsApiJiraProjectTeamsGetRequest struct via the builder pattern


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **project** | **string** |  |

### Return type

[**JiraProjectTeamsResponse**](JiraProjectTeamsResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints)
[[Back to Model list]](../README.md#documentation-for-models)
[[Back to README]](../README.md)

