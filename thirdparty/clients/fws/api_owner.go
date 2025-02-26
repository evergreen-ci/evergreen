/*
Foliage Web Services

Foliage web services, owner: DevProd Services & Integrations team

API version: 1.0.0
*/

// Code generated by OpenAPI Generator (https://openapi-generator.tech); DO NOT EDIT.

package openapi

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
)


// OwnerAPIService OwnerAPI service
type OwnerAPIService service

type ApiByFoliageLogicApiOwnerByFoliageLogicTaskIdGetRequest struct {
	ctx context.Context
	ApiService *OwnerAPIService
	taskId string
	testFileName *string
	offendingVersionId *string
}

func (r ApiByFoliageLogicApiOwnerByFoliageLogicTaskIdGetRequest) TestFileName(testFileName string) ApiByFoliageLogicApiOwnerByFoliageLogicTaskIdGetRequest {
	r.testFileName = &testFileName
	return r
}

func (r ApiByFoliageLogicApiOwnerByFoliageLogicTaskIdGetRequest) OffendingVersionId(offendingVersionId string) ApiByFoliageLogicApiOwnerByFoliageLogicTaskIdGetRequest {
	r.offendingVersionId = &offendingVersionId
	return r
}

func (r ApiByFoliageLogicApiOwnerByFoliageLogicTaskIdGetRequest) Execute() (*FinalAssignmentResults, *http.Response, error) {
	return r.ApiService.ByFoliageLogicApiOwnerByFoliageLogicTaskIdGetExecute(r)
}

/*
ByFoliageLogicApiOwnerByFoliageLogicTaskIdGet By Foliage Logic

Get the owner of a task by foliage logic.

:param task_id: The task id.
:param test_file_name: The test file name.
:param offending_version_id: The offending version id.
:return: The owning team data according to the foliage logic.

 @param ctx context.Context - for authentication, logging, cancellation, deadlines, tracing, etc. Passed from http.Request or context.Background().
 @param taskId
 @return ApiByFoliageLogicApiOwnerByFoliageLogicTaskIdGetRequest
*/
func (a *OwnerAPIService) ByFoliageLogicApiOwnerByFoliageLogicTaskIdGet(ctx context.Context, taskId string) ApiByFoliageLogicApiOwnerByFoliageLogicTaskIdGetRequest {
	return ApiByFoliageLogicApiOwnerByFoliageLogicTaskIdGetRequest{
		ApiService: a,
		ctx: ctx,
		taskId: taskId,
	}
}

// Execute executes the request
//  @return FinalAssignmentResults
func (a *OwnerAPIService) ByFoliageLogicApiOwnerByFoliageLogicTaskIdGetExecute(r ApiByFoliageLogicApiOwnerByFoliageLogicTaskIdGetRequest) (*FinalAssignmentResults, *http.Response, error) {
	var (
		localVarHTTPMethod   = http.MethodGet
		localVarPostBody     interface{}
		formFiles            []formFile
		localVarReturnValue  *FinalAssignmentResults
	)

	localBasePath, err := a.client.cfg.ServerURLWithContext(r.ctx, "OwnerAPIService.ByFoliageLogicApiOwnerByFoliageLogicTaskIdGet")
	if err != nil {
		return localVarReturnValue, nil, &GenericOpenAPIError{error: err.Error()}
	}

	localVarPath := localBasePath + "/api/owner/by_foliage_logic/{task_id}"
	localVarPath = strings.Replace(localVarPath, "{"+"task_id"+"}", url.PathEscape(parameterValueToString(r.taskId, "taskId")), -1)

	localVarHeaderParams := make(map[string]string)
	localVarQueryParams := url.Values{}
	localVarFormParams := url.Values{}

	if r.testFileName != nil {
		parameterAddToHeaderOrQuery(localVarQueryParams, "test_file_name", r.testFileName, "form", "")
	}
	if r.offendingVersionId != nil {
		parameterAddToHeaderOrQuery(localVarQueryParams, "offending_version_id", r.offendingVersionId, "form", "")
	}
	// to determine the Content-Type header
	localVarHTTPContentTypes := []string{}

	// set Content-Type header
	localVarHTTPContentType := selectHeaderContentType(localVarHTTPContentTypes)
	if localVarHTTPContentType != "" {
		localVarHeaderParams["Content-Type"] = localVarHTTPContentType
	}

	// to determine the Accept header
	localVarHTTPHeaderAccepts := []string{"application/json"}

	// set Accept header
	localVarHTTPHeaderAccept := selectHeaderAccept(localVarHTTPHeaderAccepts)
	if localVarHTTPHeaderAccept != "" {
		localVarHeaderParams["Accept"] = localVarHTTPHeaderAccept
	}
	req, err := a.client.prepareRequest(r.ctx, localVarPath, localVarHTTPMethod, localVarPostBody, localVarHeaderParams, localVarQueryParams, localVarFormParams, formFiles)
	if err != nil {
		return localVarReturnValue, nil, err
	}

	localVarHTTPResponse, err := a.client.callAPI(req)
	if err != nil || localVarHTTPResponse == nil {
		return localVarReturnValue, localVarHTTPResponse, err
	}

	localVarBody, err := io.ReadAll(localVarHTTPResponse.Body)
	localVarHTTPResponse.Body.Close()
	localVarHTTPResponse.Body = io.NopCloser(bytes.NewBuffer(localVarBody))
	if err != nil {
		return localVarReturnValue, localVarHTTPResponse, err
	}

	if localVarHTTPResponse.StatusCode >= 300 {
		newErr := &GenericOpenAPIError{
			body:  localVarBody,
			error: localVarHTTPResponse.Status,
		}
		if localVarHTTPResponse.StatusCode == 422 {
			var v HTTPValidationError
			err = a.client.decode(&v, localVarBody, localVarHTTPResponse.Header.Get("Content-Type"))
			if err != nil {
				newErr.error = err.Error()
				return localVarReturnValue, localVarHTTPResponse, newErr
			}
					newErr.error = formatErrorMessage(localVarHTTPResponse.Status, &v)
					newErr.model = v
		}
		return localVarReturnValue, localVarHTTPResponse, newErr
	}

	err = a.client.decode(&localVarReturnValue, localVarBody, localVarHTTPResponse.Header.Get("Content-Type"))
	if err != nil {
		newErr := &GenericOpenAPIError{
			body:  localVarBody,
			error: err.Error(),
		}
		return localVarReturnValue, localVarHTTPResponse, newErr
	}

	return localVarReturnValue, localVarHTTPResponse, nil
}

type ApiByJiraKeyApiOwnerByJiraKeyJiraKeyGetRequest struct {
	ctx context.Context
	ApiService *OwnerAPIService
	jiraKey string
	projectId *string
}

func (r ApiByJiraKeyApiOwnerByJiraKeyJiraKeyGetRequest) ProjectId(projectId string) ApiByJiraKeyApiOwnerByJiraKeyJiraKeyGetRequest {
	r.projectId = &projectId
	return r
}

func (r ApiByJiraKeyApiOwnerByJiraKeyJiraKeyGetRequest) Execute() (*TeamDataWithOwner, *http.Response, error) {
	return r.ApiService.ByJiraKeyApiOwnerByJiraKeyJiraKeyGetExecute(r)
}

/*
ByJiraKeyApiOwnerByJiraKeyJiraKeyGet By Jira Key

Get the owner by a Jira key.

:param jira_key: The Jira key.
:param project_id: The project id, optional for avoiding getting bot user emails.
:return: The owning team data according to the Jira key.

 @param ctx context.Context - for authentication, logging, cancellation, deadlines, tracing, etc. Passed from http.Request or context.Background().
 @param jiraKey
 @return ApiByJiraKeyApiOwnerByJiraKeyJiraKeyGetRequest
*/
func (a *OwnerAPIService) ByJiraKeyApiOwnerByJiraKeyJiraKeyGet(ctx context.Context, jiraKey string) ApiByJiraKeyApiOwnerByJiraKeyJiraKeyGetRequest {
	return ApiByJiraKeyApiOwnerByJiraKeyJiraKeyGetRequest{
		ApiService: a,
		ctx: ctx,
		jiraKey: jiraKey,
	}
}

// Execute executes the request
//  @return TeamDataWithOwner
func (a *OwnerAPIService) ByJiraKeyApiOwnerByJiraKeyJiraKeyGetExecute(r ApiByJiraKeyApiOwnerByJiraKeyJiraKeyGetRequest) (*TeamDataWithOwner, *http.Response, error) {
	var (
		localVarHTTPMethod   = http.MethodGet
		localVarPostBody     interface{}
		formFiles            []formFile
		localVarReturnValue  *TeamDataWithOwner
	)

	localBasePath, err := a.client.cfg.ServerURLWithContext(r.ctx, "OwnerAPIService.ByJiraKeyApiOwnerByJiraKeyJiraKeyGet")
	if err != nil {
		return localVarReturnValue, nil, &GenericOpenAPIError{error: err.Error()}
	}

	localVarPath := localBasePath + "/api/owner/by_jira_key/{jira_key}"
	localVarPath = strings.Replace(localVarPath, "{"+"jira_key"+"}", url.PathEscape(parameterValueToString(r.jiraKey, "jiraKey")), -1)

	localVarHeaderParams := make(map[string]string)
	localVarQueryParams := url.Values{}
	localVarFormParams := url.Values{}

	if r.projectId != nil {
		parameterAddToHeaderOrQuery(localVarQueryParams, "project_id", r.projectId, "form", "")
	}
	// to determine the Content-Type header
	localVarHTTPContentTypes := []string{}

	// set Content-Type header
	localVarHTTPContentType := selectHeaderContentType(localVarHTTPContentTypes)
	if localVarHTTPContentType != "" {
		localVarHeaderParams["Content-Type"] = localVarHTTPContentType
	}

	// to determine the Accept header
	localVarHTTPHeaderAccepts := []string{"application/json"}

	// set Accept header
	localVarHTTPHeaderAccept := selectHeaderAccept(localVarHTTPHeaderAccepts)
	if localVarHTTPHeaderAccept != "" {
		localVarHeaderParams["Accept"] = localVarHTTPHeaderAccept
	}
	req, err := a.client.prepareRequest(r.ctx, localVarPath, localVarHTTPMethod, localVarPostBody, localVarHeaderParams, localVarQueryParams, localVarFormParams, formFiles)
	if err != nil {
		return localVarReturnValue, nil, err
	}

	localVarHTTPResponse, err := a.client.callAPI(req)
	if err != nil || localVarHTTPResponse == nil {
		return localVarReturnValue, localVarHTTPResponse, err
	}

	localVarBody, err := io.ReadAll(localVarHTTPResponse.Body)
	localVarHTTPResponse.Body.Close()
	localVarHTTPResponse.Body = io.NopCloser(bytes.NewBuffer(localVarBody))
	if err != nil {
		return localVarReturnValue, localVarHTTPResponse, err
	}

	if localVarHTTPResponse.StatusCode >= 300 {
		newErr := &GenericOpenAPIError{
			body:  localVarBody,
			error: localVarHTTPResponse.Status,
		}
		if localVarHTTPResponse.StatusCode == 422 {
			var v HTTPValidationError
			err = a.client.decode(&v, localVarBody, localVarHTTPResponse.Header.Get("Content-Type"))
			if err != nil {
				newErr.error = err.Error()
				return localVarReturnValue, localVarHTTPResponse, newErr
			}
					newErr.error = formatErrorMessage(localVarHTTPResponse.Status, &v)
					newErr.model = v
		}
		return localVarReturnValue, localVarHTTPResponse, newErr
	}

	err = a.client.decode(&localVarReturnValue, localVarBody, localVarHTTPResponse.Header.Get("Content-Type"))
	if err != nil {
		newErr := &GenericOpenAPIError{
			body:  localVarBody,
			error: err.Error(),
		}
		return localVarReturnValue, localVarHTTPResponse, newErr
	}

	return localVarReturnValue, localVarHTTPResponse, nil
}

type ApiGetRegexMappingApiOwnerRegexByProjectProjectIdGetRequest struct {
	ctx context.Context
	ApiService *OwnerAPIService
	projectId string
}

func (r ApiGetRegexMappingApiOwnerRegexByProjectProjectIdGetRequest) Execute() (*map[string]GetRegexMappingApiOwnerRegexByProjectProjectIdGet200ResponseValue, *http.Response, error) {
	return r.ApiService.GetRegexMappingApiOwnerRegexByProjectProjectIdGetExecute(r)
}

/*
GetRegexMappingApiOwnerRegexByProjectProjectIdGet Get Regex Mapping

Get a mapping from a test file regular expression to the owning team

:param project_id: The project identifier.
:return: The mapping from test file regular expression to the owning team data.

 @param ctx context.Context - for authentication, logging, cancellation, deadlines, tracing, etc. Passed from http.Request or context.Background().
 @param projectId
 @return ApiGetRegexMappingApiOwnerRegexByProjectProjectIdGetRequest
*/
func (a *OwnerAPIService) GetRegexMappingApiOwnerRegexByProjectProjectIdGet(ctx context.Context, projectId string) ApiGetRegexMappingApiOwnerRegexByProjectProjectIdGetRequest {
	return ApiGetRegexMappingApiOwnerRegexByProjectProjectIdGetRequest{
		ApiService: a,
		ctx: ctx,
		projectId: projectId,
	}
}

// Execute executes the request
//  @return map[string]GetRegexMappingApiOwnerRegexByProjectProjectIdGet200ResponseValue
func (a *OwnerAPIService) GetRegexMappingApiOwnerRegexByProjectProjectIdGetExecute(r ApiGetRegexMappingApiOwnerRegexByProjectProjectIdGetRequest) (*map[string]GetRegexMappingApiOwnerRegexByProjectProjectIdGet200ResponseValue, *http.Response, error) {
	var (
		localVarHTTPMethod   = http.MethodGet
		localVarPostBody     interface{}
		formFiles            []formFile
		localVarReturnValue  *map[string]GetRegexMappingApiOwnerRegexByProjectProjectIdGet200ResponseValue
	)

	localBasePath, err := a.client.cfg.ServerURLWithContext(r.ctx, "OwnerAPIService.GetRegexMappingApiOwnerRegexByProjectProjectIdGet")
	if err != nil {
		return localVarReturnValue, nil, &GenericOpenAPIError{error: err.Error()}
	}

	localVarPath := localBasePath + "/api/owner/regex_by_project/{project_id}"
	localVarPath = strings.Replace(localVarPath, "{"+"project_id"+"}", url.PathEscape(parameterValueToString(r.projectId, "projectId")), -1)

	localVarHeaderParams := make(map[string]string)
	localVarQueryParams := url.Values{}
	localVarFormParams := url.Values{}

	// to determine the Content-Type header
	localVarHTTPContentTypes := []string{}

	// set Content-Type header
	localVarHTTPContentType := selectHeaderContentType(localVarHTTPContentTypes)
	if localVarHTTPContentType != "" {
		localVarHeaderParams["Content-Type"] = localVarHTTPContentType
	}

	// to determine the Accept header
	localVarHTTPHeaderAccepts := []string{"application/json"}

	// set Accept header
	localVarHTTPHeaderAccept := selectHeaderAccept(localVarHTTPHeaderAccepts)
	if localVarHTTPHeaderAccept != "" {
		localVarHeaderParams["Accept"] = localVarHTTPHeaderAccept
	}
	req, err := a.client.prepareRequest(r.ctx, localVarPath, localVarHTTPMethod, localVarPostBody, localVarHeaderParams, localVarQueryParams, localVarFormParams, formFiles)
	if err != nil {
		return localVarReturnValue, nil, err
	}

	localVarHTTPResponse, err := a.client.callAPI(req)
	if err != nil || localVarHTTPResponse == nil {
		return localVarReturnValue, localVarHTTPResponse, err
	}

	localVarBody, err := io.ReadAll(localVarHTTPResponse.Body)
	localVarHTTPResponse.Body.Close()
	localVarHTTPResponse.Body = io.NopCloser(bytes.NewBuffer(localVarBody))
	if err != nil {
		return localVarReturnValue, localVarHTTPResponse, err
	}

	if localVarHTTPResponse.StatusCode >= 300 {
		newErr := &GenericOpenAPIError{
			body:  localVarBody,
			error: localVarHTTPResponse.Status,
		}
		if localVarHTTPResponse.StatusCode == 422 {
			var v HTTPValidationError
			err = a.client.decode(&v, localVarBody, localVarHTTPResponse.Header.Get("Content-Type"))
			if err != nil {
				newErr.error = err.Error()
				return localVarReturnValue, localVarHTTPResponse, newErr
			}
					newErr.error = formatErrorMessage(localVarHTTPResponse.Status, &v)
					newErr.model = v
		}
		return localVarReturnValue, localVarHTTPResponse, newErr
	}

	err = a.client.decode(&localVarReturnValue, localVarBody, localVarHTTPResponse.Header.Get("Content-Type"))
	if err != nil {
		newErr := &GenericOpenAPIError{
			body:  localVarBody,
			error: err.Error(),
		}
		return localVarReturnValue, localVarHTTPResponse, newErr
	}

	return localVarReturnValue, localVarHTTPResponse, nil
}
