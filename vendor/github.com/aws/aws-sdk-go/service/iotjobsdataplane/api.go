// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

package iotjobsdataplane

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/aws/request"
)

const opDescribeJobExecution = "DescribeJobExecution"

// DescribeJobExecutionRequest generates a "aws/request.Request" representing the
// client's request for the DescribeJobExecution operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See DescribeJobExecution for more information on using the DescribeJobExecution
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the DescribeJobExecutionRequest method.
//    req, resp := client.DescribeJobExecutionRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
//
// See also, https://docs.aws.amazon.com/goto/WebAPI/iot-jobs-data-2017-09-29/DescribeJobExecution
func (c *IoTJobsDataPlane) DescribeJobExecutionRequest(input *DescribeJobExecutionInput) (req *request.Request, output *DescribeJobExecutionOutput) {
	op := &request.Operation{
		Name:       opDescribeJobExecution,
		HTTPMethod: "GET",
		HTTPPath:   "/things/{thingName}/jobs/{jobId}",
	}

	if input == nil {
		input = &DescribeJobExecutionInput{}
	}

	output = &DescribeJobExecutionOutput{}
	req = c.newRequest(op, input, output)
	return
}

// DescribeJobExecution API operation for AWS IoT Jobs Data Plane.
//
// Gets details of a job execution.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for AWS IoT Jobs Data Plane's
// API operation DescribeJobExecution for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeInvalidRequestException "InvalidRequestException"
//   The contents of the request were invalid. For example, this code is returned
//   when an UpdateJobExecution request contains invalid status details. The message
//   contains details about the error.
//
//   * ErrCodeResourceNotFoundException "ResourceNotFoundException"
//   The specified resource does not exist.
//
//   * ErrCodeThrottlingException "ThrottlingException"
//   The rate exceeds the limit.
//
//   * ErrCodeServiceUnavailableException "ServiceUnavailableException"
//   The service is temporarily unavailable.
//
//   * ErrCodeCertificateValidationException "CertificateValidationException"
//   The certificate is invalid.
//
//   * ErrCodeTerminalStateException "TerminalStateException"
//   The job is in a terminal state.
//
// See also, https://docs.aws.amazon.com/goto/WebAPI/iot-jobs-data-2017-09-29/DescribeJobExecution
func (c *IoTJobsDataPlane) DescribeJobExecution(input *DescribeJobExecutionInput) (*DescribeJobExecutionOutput, error) {
	req, out := c.DescribeJobExecutionRequest(input)
	return out, req.Send()
}

// DescribeJobExecutionWithContext is the same as DescribeJobExecution with the addition of
// the ability to pass a context and additional request options.
//
// See DescribeJobExecution for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *IoTJobsDataPlane) DescribeJobExecutionWithContext(ctx aws.Context, input *DescribeJobExecutionInput, opts ...request.Option) (*DescribeJobExecutionOutput, error) {
	req, out := c.DescribeJobExecutionRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opGetPendingJobExecutions = "GetPendingJobExecutions"

// GetPendingJobExecutionsRequest generates a "aws/request.Request" representing the
// client's request for the GetPendingJobExecutions operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See GetPendingJobExecutions for more information on using the GetPendingJobExecutions
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the GetPendingJobExecutionsRequest method.
//    req, resp := client.GetPendingJobExecutionsRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
//
// See also, https://docs.aws.amazon.com/goto/WebAPI/iot-jobs-data-2017-09-29/GetPendingJobExecutions
func (c *IoTJobsDataPlane) GetPendingJobExecutionsRequest(input *GetPendingJobExecutionsInput) (req *request.Request, output *GetPendingJobExecutionsOutput) {
	op := &request.Operation{
		Name:       opGetPendingJobExecutions,
		HTTPMethod: "GET",
		HTTPPath:   "/things/{thingName}/jobs",
	}

	if input == nil {
		input = &GetPendingJobExecutionsInput{}
	}

	output = &GetPendingJobExecutionsOutput{}
	req = c.newRequest(op, input, output)
	return
}

// GetPendingJobExecutions API operation for AWS IoT Jobs Data Plane.
//
// Gets the list of all jobs for a thing that are not in a terminal status.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for AWS IoT Jobs Data Plane's
// API operation GetPendingJobExecutions for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeInvalidRequestException "InvalidRequestException"
//   The contents of the request were invalid. For example, this code is returned
//   when an UpdateJobExecution request contains invalid status details. The message
//   contains details about the error.
//
//   * ErrCodeResourceNotFoundException "ResourceNotFoundException"
//   The specified resource does not exist.
//
//   * ErrCodeThrottlingException "ThrottlingException"
//   The rate exceeds the limit.
//
//   * ErrCodeServiceUnavailableException "ServiceUnavailableException"
//   The service is temporarily unavailable.
//
//   * ErrCodeCertificateValidationException "CertificateValidationException"
//   The certificate is invalid.
//
// See also, https://docs.aws.amazon.com/goto/WebAPI/iot-jobs-data-2017-09-29/GetPendingJobExecutions
func (c *IoTJobsDataPlane) GetPendingJobExecutions(input *GetPendingJobExecutionsInput) (*GetPendingJobExecutionsOutput, error) {
	req, out := c.GetPendingJobExecutionsRequest(input)
	return out, req.Send()
}

// GetPendingJobExecutionsWithContext is the same as GetPendingJobExecutions with the addition of
// the ability to pass a context and additional request options.
//
// See GetPendingJobExecutions for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *IoTJobsDataPlane) GetPendingJobExecutionsWithContext(ctx aws.Context, input *GetPendingJobExecutionsInput, opts ...request.Option) (*GetPendingJobExecutionsOutput, error) {
	req, out := c.GetPendingJobExecutionsRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opStartNextPendingJobExecution = "StartNextPendingJobExecution"

// StartNextPendingJobExecutionRequest generates a "aws/request.Request" representing the
// client's request for the StartNextPendingJobExecution operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See StartNextPendingJobExecution for more information on using the StartNextPendingJobExecution
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the StartNextPendingJobExecutionRequest method.
//    req, resp := client.StartNextPendingJobExecutionRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
//
// See also, https://docs.aws.amazon.com/goto/WebAPI/iot-jobs-data-2017-09-29/StartNextPendingJobExecution
func (c *IoTJobsDataPlane) StartNextPendingJobExecutionRequest(input *StartNextPendingJobExecutionInput) (req *request.Request, output *StartNextPendingJobExecutionOutput) {
	op := &request.Operation{
		Name:       opStartNextPendingJobExecution,
		HTTPMethod: "PUT",
		HTTPPath:   "/things/{thingName}/jobs/$next",
	}

	if input == nil {
		input = &StartNextPendingJobExecutionInput{}
	}

	output = &StartNextPendingJobExecutionOutput{}
	req = c.newRequest(op, input, output)
	return
}

// StartNextPendingJobExecution API operation for AWS IoT Jobs Data Plane.
//
// Gets and starts the next pending (status IN_PROGRESS or QUEUED) job execution
// for a thing.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for AWS IoT Jobs Data Plane's
// API operation StartNextPendingJobExecution for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeInvalidRequestException "InvalidRequestException"
//   The contents of the request were invalid. For example, this code is returned
//   when an UpdateJobExecution request contains invalid status details. The message
//   contains details about the error.
//
//   * ErrCodeResourceNotFoundException "ResourceNotFoundException"
//   The specified resource does not exist.
//
//   * ErrCodeThrottlingException "ThrottlingException"
//   The rate exceeds the limit.
//
//   * ErrCodeServiceUnavailableException "ServiceUnavailableException"
//   The service is temporarily unavailable.
//
//   * ErrCodeCertificateValidationException "CertificateValidationException"
//   The certificate is invalid.
//
// See also, https://docs.aws.amazon.com/goto/WebAPI/iot-jobs-data-2017-09-29/StartNextPendingJobExecution
func (c *IoTJobsDataPlane) StartNextPendingJobExecution(input *StartNextPendingJobExecutionInput) (*StartNextPendingJobExecutionOutput, error) {
	req, out := c.StartNextPendingJobExecutionRequest(input)
	return out, req.Send()
}

// StartNextPendingJobExecutionWithContext is the same as StartNextPendingJobExecution with the addition of
// the ability to pass a context and additional request options.
//
// See StartNextPendingJobExecution for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *IoTJobsDataPlane) StartNextPendingJobExecutionWithContext(ctx aws.Context, input *StartNextPendingJobExecutionInput, opts ...request.Option) (*StartNextPendingJobExecutionOutput, error) {
	req, out := c.StartNextPendingJobExecutionRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opUpdateJobExecution = "UpdateJobExecution"

// UpdateJobExecutionRequest generates a "aws/request.Request" representing the
// client's request for the UpdateJobExecution operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See UpdateJobExecution for more information on using the UpdateJobExecution
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the UpdateJobExecutionRequest method.
//    req, resp := client.UpdateJobExecutionRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
//
// See also, https://docs.aws.amazon.com/goto/WebAPI/iot-jobs-data-2017-09-29/UpdateJobExecution
func (c *IoTJobsDataPlane) UpdateJobExecutionRequest(input *UpdateJobExecutionInput) (req *request.Request, output *UpdateJobExecutionOutput) {
	op := &request.Operation{
		Name:       opUpdateJobExecution,
		HTTPMethod: "POST",
		HTTPPath:   "/things/{thingName}/jobs/{jobId}",
	}

	if input == nil {
		input = &UpdateJobExecutionInput{}
	}

	output = &UpdateJobExecutionOutput{}
	req = c.newRequest(op, input, output)
	return
}

// UpdateJobExecution API operation for AWS IoT Jobs Data Plane.
//
// Updates the status of a job execution.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for AWS IoT Jobs Data Plane's
// API operation UpdateJobExecution for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeInvalidRequestException "InvalidRequestException"
//   The contents of the request were invalid. For example, this code is returned
//   when an UpdateJobExecution request contains invalid status details. The message
//   contains details about the error.
//
//   * ErrCodeResourceNotFoundException "ResourceNotFoundException"
//   The specified resource does not exist.
//
//   * ErrCodeThrottlingException "ThrottlingException"
//   The rate exceeds the limit.
//
//   * ErrCodeServiceUnavailableException "ServiceUnavailableException"
//   The service is temporarily unavailable.
//
//   * ErrCodeCertificateValidationException "CertificateValidationException"
//   The certificate is invalid.
//
//   * ErrCodeInvalidStateTransitionException "InvalidStateTransitionException"
//   An update attempted to change the job execution to a state that is invalid
//   because of the job execution's current state (for example, an attempt to
//   change a request in state SUCCESS to state IN_PROGRESS). In this case, the
//   body of the error message also contains the executionState field.
//
// See also, https://docs.aws.amazon.com/goto/WebAPI/iot-jobs-data-2017-09-29/UpdateJobExecution
func (c *IoTJobsDataPlane) UpdateJobExecution(input *UpdateJobExecutionInput) (*UpdateJobExecutionOutput, error) {
	req, out := c.UpdateJobExecutionRequest(input)
	return out, req.Send()
}

// UpdateJobExecutionWithContext is the same as UpdateJobExecution with the addition of
// the ability to pass a context and additional request options.
//
// See UpdateJobExecution for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *IoTJobsDataPlane) UpdateJobExecutionWithContext(ctx aws.Context, input *UpdateJobExecutionInput, opts ...request.Option) (*UpdateJobExecutionOutput, error) {
	req, out := c.UpdateJobExecutionRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

type DescribeJobExecutionInput struct {
	_ struct{} `type:"structure"`

	// Optional. A number that identifies a particular job execution on a particular
	// device. If not specified, the latest job execution is returned.
	ExecutionNumber *int64 `location:"querystring" locationName:"executionNumber" type:"long"`

	// Optional. When set to true, the response contains the job document. The default
	// is false.
	IncludeJobDocument *bool `location:"querystring" locationName:"includeJobDocument" type:"boolean"`

	// The unique identifier assigned to this job when it was created.
	//
	// JobId is a required field
	JobId *string `location:"uri" locationName:"jobId" type:"string" required:"true"`

	// The thing name associated with the device the job execution is running on.
	//
	// ThingName is a required field
	ThingName *string `location:"uri" locationName:"thingName" min:"1" type:"string" required:"true"`
}

// String returns the string representation
func (s DescribeJobExecutionInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DescribeJobExecutionInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *DescribeJobExecutionInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "DescribeJobExecutionInput"}
	if s.JobId == nil {
		invalidParams.Add(request.NewErrParamRequired("JobId"))
	}
	if s.JobId != nil && len(*s.JobId) < 1 {
		invalidParams.Add(request.NewErrParamMinLen("JobId", 1))
	}
	if s.ThingName == nil {
		invalidParams.Add(request.NewErrParamRequired("ThingName"))
	}
	if s.ThingName != nil && len(*s.ThingName) < 1 {
		invalidParams.Add(request.NewErrParamMinLen("ThingName", 1))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetExecutionNumber sets the ExecutionNumber field's value.
func (s *DescribeJobExecutionInput) SetExecutionNumber(v int64) *DescribeJobExecutionInput {
	s.ExecutionNumber = &v
	return s
}

// SetIncludeJobDocument sets the IncludeJobDocument field's value.
func (s *DescribeJobExecutionInput) SetIncludeJobDocument(v bool) *DescribeJobExecutionInput {
	s.IncludeJobDocument = &v
	return s
}

// SetJobId sets the JobId field's value.
func (s *DescribeJobExecutionInput) SetJobId(v string) *DescribeJobExecutionInput {
	s.JobId = &v
	return s
}

// SetThingName sets the ThingName field's value.
func (s *DescribeJobExecutionInput) SetThingName(v string) *DescribeJobExecutionInput {
	s.ThingName = &v
	return s
}

type DescribeJobExecutionOutput struct {
	_ struct{} `type:"structure"`

	// Contains data about a job execution.
	Execution *JobExecution `locationName:"execution" type:"structure"`
}

// String returns the string representation
func (s DescribeJobExecutionOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DescribeJobExecutionOutput) GoString() string {
	return s.String()
}

// SetExecution sets the Execution field's value.
func (s *DescribeJobExecutionOutput) SetExecution(v *JobExecution) *DescribeJobExecutionOutput {
	s.Execution = v
	return s
}

type GetPendingJobExecutionsInput struct {
	_ struct{} `type:"structure"`

	// The name of the thing that is executing the job.
	//
	// ThingName is a required field
	ThingName *string `location:"uri" locationName:"thingName" min:"1" type:"string" required:"true"`
}

// String returns the string representation
func (s GetPendingJobExecutionsInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s GetPendingJobExecutionsInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *GetPendingJobExecutionsInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "GetPendingJobExecutionsInput"}
	if s.ThingName == nil {
		invalidParams.Add(request.NewErrParamRequired("ThingName"))
	}
	if s.ThingName != nil && len(*s.ThingName) < 1 {
		invalidParams.Add(request.NewErrParamMinLen("ThingName", 1))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetThingName sets the ThingName field's value.
func (s *GetPendingJobExecutionsInput) SetThingName(v string) *GetPendingJobExecutionsInput {
	s.ThingName = &v
	return s
}

type GetPendingJobExecutionsOutput struct {
	_ struct{} `type:"structure"`

	// A list of JobExecutionSummary objects with status IN_PROGRESS.
	InProgressJobs []*JobExecutionSummary `locationName:"inProgressJobs" type:"list"`

	// A list of JobExecutionSummary objects with status QUEUED.
	QueuedJobs []*JobExecutionSummary `locationName:"queuedJobs" type:"list"`
}

// String returns the string representation
func (s GetPendingJobExecutionsOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s GetPendingJobExecutionsOutput) GoString() string {
	return s.String()
}

// SetInProgressJobs sets the InProgressJobs field's value.
func (s *GetPendingJobExecutionsOutput) SetInProgressJobs(v []*JobExecutionSummary) *GetPendingJobExecutionsOutput {
	s.InProgressJobs = v
	return s
}

// SetQueuedJobs sets the QueuedJobs field's value.
func (s *GetPendingJobExecutionsOutput) SetQueuedJobs(v []*JobExecutionSummary) *GetPendingJobExecutionsOutput {
	s.QueuedJobs = v
	return s
}

// Contains data about a job execution.
type JobExecution struct {
	_ struct{} `type:"structure"`

	// The estimated number of seconds that remain before the job execution status
	// will be changed to TIMED_OUT.
	ApproximateSecondsBeforeTimedOut *int64 `locationName:"approximateSecondsBeforeTimedOut" type:"long"`

	// A number that identifies a particular job execution on a particular device.
	// It can be used later in commands that return or update job execution information.
	ExecutionNumber *int64 `locationName:"executionNumber" type:"long"`

	// The content of the job document.
	JobDocument *string `locationName:"jobDocument" type:"string"`

	// The unique identifier you assigned to this job when it was created.
	JobId *string `locationName:"jobId" min:"1" type:"string"`

	// The time, in milliseconds since the epoch, when the job execution was last
	// updated.
	LastUpdatedAt *int64 `locationName:"lastUpdatedAt" type:"long"`

	// The time, in milliseconds since the epoch, when the job execution was enqueued.
	QueuedAt *int64 `locationName:"queuedAt" type:"long"`

	// The time, in milliseconds since the epoch, when the job execution was started.
	StartedAt *int64 `locationName:"startedAt" type:"long"`

	// The status of the job execution. Can be one of: "QUEUED", "IN_PROGRESS",
	// "FAILED", "SUCCESS", "CANCELED", "REJECTED", or "REMOVED".
	Status *string `locationName:"status" type:"string" enum:"JobExecutionStatus"`

	// A collection of name/value pairs that describe the status of the job execution.
	StatusDetails map[string]*string `locationName:"statusDetails" type:"map"`

	// The name of the thing that is executing the job.
	ThingName *string `locationName:"thingName" min:"1" type:"string"`

	// The version of the job execution. Job execution versions are incremented
	// each time they are updated by a device.
	VersionNumber *int64 `locationName:"versionNumber" type:"long"`
}

// String returns the string representation
func (s JobExecution) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s JobExecution) GoString() string {
	return s.String()
}

// SetApproximateSecondsBeforeTimedOut sets the ApproximateSecondsBeforeTimedOut field's value.
func (s *JobExecution) SetApproximateSecondsBeforeTimedOut(v int64) *JobExecution {
	s.ApproximateSecondsBeforeTimedOut = &v
	return s
}

// SetExecutionNumber sets the ExecutionNumber field's value.
func (s *JobExecution) SetExecutionNumber(v int64) *JobExecution {
	s.ExecutionNumber = &v
	return s
}

// SetJobDocument sets the JobDocument field's value.
func (s *JobExecution) SetJobDocument(v string) *JobExecution {
	s.JobDocument = &v
	return s
}

// SetJobId sets the JobId field's value.
func (s *JobExecution) SetJobId(v string) *JobExecution {
	s.JobId = &v
	return s
}

// SetLastUpdatedAt sets the LastUpdatedAt field's value.
func (s *JobExecution) SetLastUpdatedAt(v int64) *JobExecution {
	s.LastUpdatedAt = &v
	return s
}

// SetQueuedAt sets the QueuedAt field's value.
func (s *JobExecution) SetQueuedAt(v int64) *JobExecution {
	s.QueuedAt = &v
	return s
}

// SetStartedAt sets the StartedAt field's value.
func (s *JobExecution) SetStartedAt(v int64) *JobExecution {
	s.StartedAt = &v
	return s
}

// SetStatus sets the Status field's value.
func (s *JobExecution) SetStatus(v string) *JobExecution {
	s.Status = &v
	return s
}

// SetStatusDetails sets the StatusDetails field's value.
func (s *JobExecution) SetStatusDetails(v map[string]*string) *JobExecution {
	s.StatusDetails = v
	return s
}

// SetThingName sets the ThingName field's value.
func (s *JobExecution) SetThingName(v string) *JobExecution {
	s.ThingName = &v
	return s
}

// SetVersionNumber sets the VersionNumber field's value.
func (s *JobExecution) SetVersionNumber(v int64) *JobExecution {
	s.VersionNumber = &v
	return s
}

// Contains data about the state of a job execution.
type JobExecutionState struct {
	_ struct{} `type:"structure"`

	// The status of the job execution. Can be one of: "QUEUED", "IN_PROGRESS",
	// "FAILED", "SUCCESS", "CANCELED", "REJECTED", or "REMOVED".
	Status *string `locationName:"status" type:"string" enum:"JobExecutionStatus"`

	// A collection of name/value pairs that describe the status of the job execution.
	StatusDetails map[string]*string `locationName:"statusDetails" type:"map"`

	// The version of the job execution. Job execution versions are incremented
	// each time they are updated by a device.
	VersionNumber *int64 `locationName:"versionNumber" type:"long"`
}

// String returns the string representation
func (s JobExecutionState) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s JobExecutionState) GoString() string {
	return s.String()
}

// SetStatus sets the Status field's value.
func (s *JobExecutionState) SetStatus(v string) *JobExecutionState {
	s.Status = &v
	return s
}

// SetStatusDetails sets the StatusDetails field's value.
func (s *JobExecutionState) SetStatusDetails(v map[string]*string) *JobExecutionState {
	s.StatusDetails = v
	return s
}

// SetVersionNumber sets the VersionNumber field's value.
func (s *JobExecutionState) SetVersionNumber(v int64) *JobExecutionState {
	s.VersionNumber = &v
	return s
}

// Contains a subset of information about a job execution.
type JobExecutionSummary struct {
	_ struct{} `type:"structure"`

	// A number that identifies a particular job execution on a particular device.
	ExecutionNumber *int64 `locationName:"executionNumber" type:"long"`

	// The unique identifier you assigned to this job when it was created.
	JobId *string `locationName:"jobId" min:"1" type:"string"`

	// The time, in milliseconds since the epoch, when the job execution was last
	// updated.
	LastUpdatedAt *int64 `locationName:"lastUpdatedAt" type:"long"`

	// The time, in milliseconds since the epoch, when the job execution was enqueued.
	QueuedAt *int64 `locationName:"queuedAt" type:"long"`

	// The time, in milliseconds since the epoch, when the job execution started.
	StartedAt *int64 `locationName:"startedAt" type:"long"`

	// The version of the job execution. Job execution versions are incremented
	// each time AWS IoT Jobs receives an update from a device.
	VersionNumber *int64 `locationName:"versionNumber" type:"long"`
}

// String returns the string representation
func (s JobExecutionSummary) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s JobExecutionSummary) GoString() string {
	return s.String()
}

// SetExecutionNumber sets the ExecutionNumber field's value.
func (s *JobExecutionSummary) SetExecutionNumber(v int64) *JobExecutionSummary {
	s.ExecutionNumber = &v
	return s
}

// SetJobId sets the JobId field's value.
func (s *JobExecutionSummary) SetJobId(v string) *JobExecutionSummary {
	s.JobId = &v
	return s
}

// SetLastUpdatedAt sets the LastUpdatedAt field's value.
func (s *JobExecutionSummary) SetLastUpdatedAt(v int64) *JobExecutionSummary {
	s.LastUpdatedAt = &v
	return s
}

// SetQueuedAt sets the QueuedAt field's value.
func (s *JobExecutionSummary) SetQueuedAt(v int64) *JobExecutionSummary {
	s.QueuedAt = &v
	return s
}

// SetStartedAt sets the StartedAt field's value.
func (s *JobExecutionSummary) SetStartedAt(v int64) *JobExecutionSummary {
	s.StartedAt = &v
	return s
}

// SetVersionNumber sets the VersionNumber field's value.
func (s *JobExecutionSummary) SetVersionNumber(v int64) *JobExecutionSummary {
	s.VersionNumber = &v
	return s
}

type StartNextPendingJobExecutionInput struct {
	_ struct{} `type:"structure"`

	// A collection of name/value pairs that describe the status of the job execution.
	// If not specified, the statusDetails are unchanged.
	StatusDetails map[string]*string `locationName:"statusDetails" type:"map"`

	// Specifies the amount of time this device has to finish execution of this
	// job. If the job execution status is not set to a terminal state before this
	// timer expires, or before the timer is reset (by calling UpdateJobExecution,
	// setting the status to IN_PROGRESS and specifying a new timeout value in field
	// stepTimeoutInMinutes) the job execution status will be automatically set
	// to TIMED_OUT. Note that setting this timeout has no effect on that job execution
	// timeout which may have been specified when the job was created (CreateJob
	// using field timeoutConfig).
	StepTimeoutInMinutes *int64 `locationName:"stepTimeoutInMinutes" type:"long"`

	// The name of the thing associated with the device.
	//
	// ThingName is a required field
	ThingName *string `location:"uri" locationName:"thingName" min:"1" type:"string" required:"true"`
}

// String returns the string representation
func (s StartNextPendingJobExecutionInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s StartNextPendingJobExecutionInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *StartNextPendingJobExecutionInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "StartNextPendingJobExecutionInput"}
	if s.ThingName == nil {
		invalidParams.Add(request.NewErrParamRequired("ThingName"))
	}
	if s.ThingName != nil && len(*s.ThingName) < 1 {
		invalidParams.Add(request.NewErrParamMinLen("ThingName", 1))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetStatusDetails sets the StatusDetails field's value.
func (s *StartNextPendingJobExecutionInput) SetStatusDetails(v map[string]*string) *StartNextPendingJobExecutionInput {
	s.StatusDetails = v
	return s
}

// SetStepTimeoutInMinutes sets the StepTimeoutInMinutes field's value.
func (s *StartNextPendingJobExecutionInput) SetStepTimeoutInMinutes(v int64) *StartNextPendingJobExecutionInput {
	s.StepTimeoutInMinutes = &v
	return s
}

// SetThingName sets the ThingName field's value.
func (s *StartNextPendingJobExecutionInput) SetThingName(v string) *StartNextPendingJobExecutionInput {
	s.ThingName = &v
	return s
}

type StartNextPendingJobExecutionOutput struct {
	_ struct{} `type:"structure"`

	// A JobExecution object.
	Execution *JobExecution `locationName:"execution" type:"structure"`
}

// String returns the string representation
func (s StartNextPendingJobExecutionOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s StartNextPendingJobExecutionOutput) GoString() string {
	return s.String()
}

// SetExecution sets the Execution field's value.
func (s *StartNextPendingJobExecutionOutput) SetExecution(v *JobExecution) *StartNextPendingJobExecutionOutput {
	s.Execution = v
	return s
}

type UpdateJobExecutionInput struct {
	_ struct{} `type:"structure"`

	// Optional. A number that identifies a particular job execution on a particular
	// device.
	ExecutionNumber *int64 `locationName:"executionNumber" type:"long"`

	// Optional. The expected current version of the job execution. Each time you
	// update the job execution, its version is incremented. If the version of the
	// job execution stored in Jobs does not match, the update is rejected with
	// a VersionMismatch error, and an ErrorResponse that contains the current job
	// execution status data is returned. (This makes it unnecessary to perform
	// a separate DescribeJobExecution request in order to obtain the job execution
	// status data.)
	ExpectedVersion *int64 `locationName:"expectedVersion" type:"long"`

	// Optional. When set to true, the response contains the job document. The default
	// is false.
	IncludeJobDocument *bool `locationName:"includeJobDocument" type:"boolean"`

	// Optional. When included and set to true, the response contains the JobExecutionState
	// data. The default is false.
	IncludeJobExecutionState *bool `locationName:"includeJobExecutionState" type:"boolean"`

	// The unique identifier assigned to this job when it was created.
	//
	// JobId is a required field
	JobId *string `location:"uri" locationName:"jobId" min:"1" type:"string" required:"true"`

	// The new status for the job execution (IN_PROGRESS, FAILED, SUCCESS, or REJECTED).
	// This must be specified on every update.
	//
	// Status is a required field
	Status *string `locationName:"status" type:"string" required:"true" enum:"JobExecutionStatus"`

	// Optional. A collection of name/value pairs that describe the status of the
	// job execution. If not specified, the statusDetails are unchanged.
	StatusDetails map[string]*string `locationName:"statusDetails" type:"map"`

	// Specifies the amount of time this device has to finish execution of this
	// job. If the job execution status is not set to a terminal state before this
	// timer expires, or before the timer is reset (by again calling UpdateJobExecution,
	// setting the status to IN_PROGRESS and specifying a new timeout value in this
	// field) the job execution status will be automatically set to TIMED_OUT. Note
	// that setting or resetting this timeout has no effect on that job execution
	// timeout which may have been specified when the job was created (CreateJob
	// using field timeoutConfig).
	StepTimeoutInMinutes *int64 `locationName:"stepTimeoutInMinutes" type:"long"`

	// The name of the thing associated with the device.
	//
	// ThingName is a required field
	ThingName *string `location:"uri" locationName:"thingName" min:"1" type:"string" required:"true"`
}

// String returns the string representation
func (s UpdateJobExecutionInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s UpdateJobExecutionInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *UpdateJobExecutionInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "UpdateJobExecutionInput"}
	if s.JobId == nil {
		invalidParams.Add(request.NewErrParamRequired("JobId"))
	}
	if s.JobId != nil && len(*s.JobId) < 1 {
		invalidParams.Add(request.NewErrParamMinLen("JobId", 1))
	}
	if s.Status == nil {
		invalidParams.Add(request.NewErrParamRequired("Status"))
	}
	if s.ThingName == nil {
		invalidParams.Add(request.NewErrParamRequired("ThingName"))
	}
	if s.ThingName != nil && len(*s.ThingName) < 1 {
		invalidParams.Add(request.NewErrParamMinLen("ThingName", 1))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetExecutionNumber sets the ExecutionNumber field's value.
func (s *UpdateJobExecutionInput) SetExecutionNumber(v int64) *UpdateJobExecutionInput {
	s.ExecutionNumber = &v
	return s
}

// SetExpectedVersion sets the ExpectedVersion field's value.
func (s *UpdateJobExecutionInput) SetExpectedVersion(v int64) *UpdateJobExecutionInput {
	s.ExpectedVersion = &v
	return s
}

// SetIncludeJobDocument sets the IncludeJobDocument field's value.
func (s *UpdateJobExecutionInput) SetIncludeJobDocument(v bool) *UpdateJobExecutionInput {
	s.IncludeJobDocument = &v
	return s
}

// SetIncludeJobExecutionState sets the IncludeJobExecutionState field's value.
func (s *UpdateJobExecutionInput) SetIncludeJobExecutionState(v bool) *UpdateJobExecutionInput {
	s.IncludeJobExecutionState = &v
	return s
}

// SetJobId sets the JobId field's value.
func (s *UpdateJobExecutionInput) SetJobId(v string) *UpdateJobExecutionInput {
	s.JobId = &v
	return s
}

// SetStatus sets the Status field's value.
func (s *UpdateJobExecutionInput) SetStatus(v string) *UpdateJobExecutionInput {
	s.Status = &v
	return s
}

// SetStatusDetails sets the StatusDetails field's value.
func (s *UpdateJobExecutionInput) SetStatusDetails(v map[string]*string) *UpdateJobExecutionInput {
	s.StatusDetails = v
	return s
}

// SetStepTimeoutInMinutes sets the StepTimeoutInMinutes field's value.
func (s *UpdateJobExecutionInput) SetStepTimeoutInMinutes(v int64) *UpdateJobExecutionInput {
	s.StepTimeoutInMinutes = &v
	return s
}

// SetThingName sets the ThingName field's value.
func (s *UpdateJobExecutionInput) SetThingName(v string) *UpdateJobExecutionInput {
	s.ThingName = &v
	return s
}

type UpdateJobExecutionOutput struct {
	_ struct{} `type:"structure"`

	// A JobExecutionState object.
	ExecutionState *JobExecutionState `locationName:"executionState" type:"structure"`

	// The contents of the Job Documents.
	JobDocument *string `locationName:"jobDocument" type:"string"`
}

// String returns the string representation
func (s UpdateJobExecutionOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s UpdateJobExecutionOutput) GoString() string {
	return s.String()
}

// SetExecutionState sets the ExecutionState field's value.
func (s *UpdateJobExecutionOutput) SetExecutionState(v *JobExecutionState) *UpdateJobExecutionOutput {
	s.ExecutionState = v
	return s
}

// SetJobDocument sets the JobDocument field's value.
func (s *UpdateJobExecutionOutput) SetJobDocument(v string) *UpdateJobExecutionOutput {
	s.JobDocument = &v
	return s
}

const (
	// JobExecutionStatusQueued is a JobExecutionStatus enum value
	JobExecutionStatusQueued = "QUEUED"

	// JobExecutionStatusInProgress is a JobExecutionStatus enum value
	JobExecutionStatusInProgress = "IN_PROGRESS"

	// JobExecutionStatusSucceeded is a JobExecutionStatus enum value
	JobExecutionStatusSucceeded = "SUCCEEDED"

	// JobExecutionStatusFailed is a JobExecutionStatus enum value
	JobExecutionStatusFailed = "FAILED"

	// JobExecutionStatusTimedOut is a JobExecutionStatus enum value
	JobExecutionStatusTimedOut = "TIMED_OUT"

	// JobExecutionStatusRejected is a JobExecutionStatus enum value
	JobExecutionStatusRejected = "REJECTED"

	// JobExecutionStatusRemoved is a JobExecutionStatus enum value
	JobExecutionStatusRemoved = "REMOVED"

	// JobExecutionStatusCanceled is a JobExecutionStatus enum value
	JobExecutionStatusCanceled = "CANCELED"
)
