// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

package migrationhubconfig

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/private/protocol"
)

const opCreateHomeRegionControl = "CreateHomeRegionControl"

// CreateHomeRegionControlRequest generates a "aws/request.Request" representing the
// client's request for the CreateHomeRegionControl operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See CreateHomeRegionControl for more information on using the CreateHomeRegionControl
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the CreateHomeRegionControlRequest method.
//    req, resp := client.CreateHomeRegionControlRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
//
// See also, https://docs.aws.amazon.com/goto/WebAPI/migrationhub-config-2019-06-30/CreateHomeRegionControl
func (c *MigrationHubConfig) CreateHomeRegionControlRequest(input *CreateHomeRegionControlInput) (req *request.Request, output *CreateHomeRegionControlOutput) {
	op := &request.Operation{
		Name:       opCreateHomeRegionControl,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &CreateHomeRegionControlInput{}
	}

	output = &CreateHomeRegionControlOutput{}
	req = c.newRequest(op, input, output)
	return
}

// CreateHomeRegionControl API operation for AWS Migration Hub Config.
//
// This API sets up the home region for the calling account only.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for AWS Migration Hub Config's
// API operation CreateHomeRegionControl for usage and error information.
//
// Returned Error Types:
//   * InternalServerError
//   Exception raised when an internal, configuration, or dependency error is
//   encountered.
//
//   * ServiceUnavailableException
//   Exception raised when a request fails due to temporary unavailability of
//   the service.
//
//   * AccessDeniedException
//   You do not have sufficient access to perform this action.
//
//   * DryRunOperation
//   Exception raised to indicate that authorization of an action was successful,
//   when the DryRun flag is set to true.
//
//   * InvalidInputException
//   Exception raised when the provided input violates a policy constraint or
//   is entered in the wrong format or data type.
//
// See also, https://docs.aws.amazon.com/goto/WebAPI/migrationhub-config-2019-06-30/CreateHomeRegionControl
func (c *MigrationHubConfig) CreateHomeRegionControl(input *CreateHomeRegionControlInput) (*CreateHomeRegionControlOutput, error) {
	req, out := c.CreateHomeRegionControlRequest(input)
	return out, req.Send()
}

// CreateHomeRegionControlWithContext is the same as CreateHomeRegionControl with the addition of
// the ability to pass a context and additional request options.
//
// See CreateHomeRegionControl for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *MigrationHubConfig) CreateHomeRegionControlWithContext(ctx aws.Context, input *CreateHomeRegionControlInput, opts ...request.Option) (*CreateHomeRegionControlOutput, error) {
	req, out := c.CreateHomeRegionControlRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opDescribeHomeRegionControls = "DescribeHomeRegionControls"

// DescribeHomeRegionControlsRequest generates a "aws/request.Request" representing the
// client's request for the DescribeHomeRegionControls operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See DescribeHomeRegionControls for more information on using the DescribeHomeRegionControls
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the DescribeHomeRegionControlsRequest method.
//    req, resp := client.DescribeHomeRegionControlsRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
//
// See also, https://docs.aws.amazon.com/goto/WebAPI/migrationhub-config-2019-06-30/DescribeHomeRegionControls
func (c *MigrationHubConfig) DescribeHomeRegionControlsRequest(input *DescribeHomeRegionControlsInput) (req *request.Request, output *DescribeHomeRegionControlsOutput) {
	op := &request.Operation{
		Name:       opDescribeHomeRegionControls,
		HTTPMethod: "POST",
		HTTPPath:   "/",
		Paginator: &request.Paginator{
			InputTokens:     []string{"NextToken"},
			OutputTokens:    []string{"NextToken"},
			LimitToken:      "MaxResults",
			TruncationToken: "",
		},
	}

	if input == nil {
		input = &DescribeHomeRegionControlsInput{}
	}

	output = &DescribeHomeRegionControlsOutput{}
	req = c.newRequest(op, input, output)
	return
}

// DescribeHomeRegionControls API operation for AWS Migration Hub Config.
//
// This API permits filtering on the ControlId, HomeRegion, and RegionControlScope
// fields.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for AWS Migration Hub Config's
// API operation DescribeHomeRegionControls for usage and error information.
//
// Returned Error Types:
//   * InternalServerError
//   Exception raised when an internal, configuration, or dependency error is
//   encountered.
//
//   * ServiceUnavailableException
//   Exception raised when a request fails due to temporary unavailability of
//   the service.
//
//   * AccessDeniedException
//   You do not have sufficient access to perform this action.
//
//   * InvalidInputException
//   Exception raised when the provided input violates a policy constraint or
//   is entered in the wrong format or data type.
//
// See also, https://docs.aws.amazon.com/goto/WebAPI/migrationhub-config-2019-06-30/DescribeHomeRegionControls
func (c *MigrationHubConfig) DescribeHomeRegionControls(input *DescribeHomeRegionControlsInput) (*DescribeHomeRegionControlsOutput, error) {
	req, out := c.DescribeHomeRegionControlsRequest(input)
	return out, req.Send()
}

// DescribeHomeRegionControlsWithContext is the same as DescribeHomeRegionControls with the addition of
// the ability to pass a context and additional request options.
//
// See DescribeHomeRegionControls for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *MigrationHubConfig) DescribeHomeRegionControlsWithContext(ctx aws.Context, input *DescribeHomeRegionControlsInput, opts ...request.Option) (*DescribeHomeRegionControlsOutput, error) {
	req, out := c.DescribeHomeRegionControlsRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

// DescribeHomeRegionControlsPages iterates over the pages of a DescribeHomeRegionControls operation,
// calling the "fn" function with the response data for each page. To stop
// iterating, return false from the fn function.
//
// See DescribeHomeRegionControls method for more information on how to use this operation.
//
// Note: This operation can generate multiple requests to a service.
//
//    // Example iterating over at most 3 pages of a DescribeHomeRegionControls operation.
//    pageNum := 0
//    err := client.DescribeHomeRegionControlsPages(params,
//        func(page *migrationhubconfig.DescribeHomeRegionControlsOutput, lastPage bool) bool {
//            pageNum++
//            fmt.Println(page)
//            return pageNum <= 3
//        })
//
func (c *MigrationHubConfig) DescribeHomeRegionControlsPages(input *DescribeHomeRegionControlsInput, fn func(*DescribeHomeRegionControlsOutput, bool) bool) error {
	return c.DescribeHomeRegionControlsPagesWithContext(aws.BackgroundContext(), input, fn)
}

// DescribeHomeRegionControlsPagesWithContext same as DescribeHomeRegionControlsPages except
// it takes a Context and allows setting request options on the pages.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *MigrationHubConfig) DescribeHomeRegionControlsPagesWithContext(ctx aws.Context, input *DescribeHomeRegionControlsInput, fn func(*DescribeHomeRegionControlsOutput, bool) bool, opts ...request.Option) error {
	p := request.Pagination{
		NewRequest: func() (*request.Request, error) {
			var inCpy *DescribeHomeRegionControlsInput
			if input != nil {
				tmp := *input
				inCpy = &tmp
			}
			req, _ := c.DescribeHomeRegionControlsRequest(inCpy)
			req.SetContext(ctx)
			req.ApplyOptions(opts...)
			return req, nil
		},
	}

	for p.Next() {
		if !fn(p.Page().(*DescribeHomeRegionControlsOutput), !p.HasNextPage()) {
			break
		}
	}

	return p.Err()
}

const opGetHomeRegion = "GetHomeRegion"

// GetHomeRegionRequest generates a "aws/request.Request" representing the
// client's request for the GetHomeRegion operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See GetHomeRegion for more information on using the GetHomeRegion
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the GetHomeRegionRequest method.
//    req, resp := client.GetHomeRegionRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
//
// See also, https://docs.aws.amazon.com/goto/WebAPI/migrationhub-config-2019-06-30/GetHomeRegion
func (c *MigrationHubConfig) GetHomeRegionRequest(input *GetHomeRegionInput) (req *request.Request, output *GetHomeRegionOutput) {
	op := &request.Operation{
		Name:       opGetHomeRegion,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &GetHomeRegionInput{}
	}

	output = &GetHomeRegionOutput{}
	req = c.newRequest(op, input, output)
	return
}

// GetHomeRegion API operation for AWS Migration Hub Config.
//
// Returns the calling account’s home region, if configured. This API is used
// by other AWS services to determine the regional endpoint for calling AWS
// Application Discovery Service and Migration Hub. You must call GetHomeRegion
// at least once before you call any other AWS Application Discovery Service
// and AWS Migration Hub APIs, to obtain the account's Migration Hub home region.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for AWS Migration Hub Config's
// API operation GetHomeRegion for usage and error information.
//
// Returned Error Types:
//   * InternalServerError
//   Exception raised when an internal, configuration, or dependency error is
//   encountered.
//
//   * ServiceUnavailableException
//   Exception raised when a request fails due to temporary unavailability of
//   the service.
//
//   * AccessDeniedException
//   You do not have sufficient access to perform this action.
//
//   * InvalidInputException
//   Exception raised when the provided input violates a policy constraint or
//   is entered in the wrong format or data type.
//
// See also, https://docs.aws.amazon.com/goto/WebAPI/migrationhub-config-2019-06-30/GetHomeRegion
func (c *MigrationHubConfig) GetHomeRegion(input *GetHomeRegionInput) (*GetHomeRegionOutput, error) {
	req, out := c.GetHomeRegionRequest(input)
	return out, req.Send()
}

// GetHomeRegionWithContext is the same as GetHomeRegion with the addition of
// the ability to pass a context and additional request options.
//
// See GetHomeRegion for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *MigrationHubConfig) GetHomeRegionWithContext(ctx aws.Context, input *GetHomeRegionInput, opts ...request.Option) (*GetHomeRegionOutput, error) {
	req, out := c.GetHomeRegionRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

// You do not have sufficient access to perform this action.
type AccessDeniedException struct {
	_            struct{} `type:"structure"`
	respMetadata protocol.ResponseMetadata

	Message_ *string `locationName:"Message" type:"string"`
}

// String returns the string representation
func (s AccessDeniedException) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s AccessDeniedException) GoString() string {
	return s.String()
}

func newErrorAccessDeniedException(v protocol.ResponseMetadata) error {
	return &AccessDeniedException{
		respMetadata: v,
	}
}

// Code returns the exception type name.
func (s AccessDeniedException) Code() string {
	return "AccessDeniedException"
}

// Message returns the exception's message.
func (s AccessDeniedException) Message() string {
	if s.Message_ != nil {
		return *s.Message_
	}
	return ""
}

// OrigErr always returns nil, satisfies awserr.Error interface.
func (s AccessDeniedException) OrigErr() error {
	return nil
}

func (s AccessDeniedException) Error() string {
	return fmt.Sprintf("%s: %s", s.Code(), s.Message())
}

// Status code returns the HTTP status code for the request's response error.
func (s AccessDeniedException) StatusCode() int {
	return s.respMetadata.StatusCode
}

// RequestID returns the service's response RequestID for request.
func (s AccessDeniedException) RequestID() string {
	return s.respMetadata.RequestID
}

type CreateHomeRegionControlInput struct {
	_ struct{} `type:"structure"`

	// Optional Boolean flag to indicate whether any effect should take place. It
	// tests whether the caller has permission to make the call.
	DryRun *bool `type:"boolean"`

	// The name of the home region of the calling account.
	//
	// HomeRegion is a required field
	HomeRegion *string `min:"1" type:"string" required:"true"`

	// The account for which this command sets up a home region control. The Target
	// is always of type ACCOUNT.
	//
	// Target is a required field
	Target *Target `type:"structure" required:"true"`
}

// String returns the string representation
func (s CreateHomeRegionControlInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s CreateHomeRegionControlInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *CreateHomeRegionControlInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "CreateHomeRegionControlInput"}
	if s.HomeRegion == nil {
		invalidParams.Add(request.NewErrParamRequired("HomeRegion"))
	}
	if s.HomeRegion != nil && len(*s.HomeRegion) < 1 {
		invalidParams.Add(request.NewErrParamMinLen("HomeRegion", 1))
	}
	if s.Target == nil {
		invalidParams.Add(request.NewErrParamRequired("Target"))
	}
	if s.Target != nil {
		if err := s.Target.Validate(); err != nil {
			invalidParams.AddNested("Target", err.(request.ErrInvalidParams))
		}
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetDryRun sets the DryRun field's value.
func (s *CreateHomeRegionControlInput) SetDryRun(v bool) *CreateHomeRegionControlInput {
	s.DryRun = &v
	return s
}

// SetHomeRegion sets the HomeRegion field's value.
func (s *CreateHomeRegionControlInput) SetHomeRegion(v string) *CreateHomeRegionControlInput {
	s.HomeRegion = &v
	return s
}

// SetTarget sets the Target field's value.
func (s *CreateHomeRegionControlInput) SetTarget(v *Target) *CreateHomeRegionControlInput {
	s.Target = v
	return s
}

type CreateHomeRegionControlOutput struct {
	_ struct{} `type:"structure"`

	// This object is the HomeRegionControl object that's returned by a successful
	// call to CreateHomeRegionControl.
	HomeRegionControl *HomeRegionControl `type:"structure"`
}

// String returns the string representation
func (s CreateHomeRegionControlOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s CreateHomeRegionControlOutput) GoString() string {
	return s.String()
}

// SetHomeRegionControl sets the HomeRegionControl field's value.
func (s *CreateHomeRegionControlOutput) SetHomeRegionControl(v *HomeRegionControl) *CreateHomeRegionControlOutput {
	s.HomeRegionControl = v
	return s
}

type DescribeHomeRegionControlsInput struct {
	_ struct{} `type:"structure"`

	// The ControlID is a unique identifier string of your HomeRegionControl object.
	ControlId *string `min:"1" type:"string"`

	// The name of the home region you'd like to view.
	HomeRegion *string `min:"1" type:"string"`

	// The maximum number of filtering results to display per page.
	MaxResults *int64 `min:"1" type:"integer"`

	// If a NextToken was returned by a previous call, more results are available.
	// To retrieve the next page of results, make the call again using the returned
	// token in NextToken.
	NextToken *string `type:"string"`

	// The target parameter specifies the identifier to which the home region is
	// applied, which is always of type ACCOUNT. It applies the home region to the
	// current ACCOUNT.
	Target *Target `type:"structure"`
}

// String returns the string representation
func (s DescribeHomeRegionControlsInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DescribeHomeRegionControlsInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *DescribeHomeRegionControlsInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "DescribeHomeRegionControlsInput"}
	if s.ControlId != nil && len(*s.ControlId) < 1 {
		invalidParams.Add(request.NewErrParamMinLen("ControlId", 1))
	}
	if s.HomeRegion != nil && len(*s.HomeRegion) < 1 {
		invalidParams.Add(request.NewErrParamMinLen("HomeRegion", 1))
	}
	if s.MaxResults != nil && *s.MaxResults < 1 {
		invalidParams.Add(request.NewErrParamMinValue("MaxResults", 1))
	}
	if s.Target != nil {
		if err := s.Target.Validate(); err != nil {
			invalidParams.AddNested("Target", err.(request.ErrInvalidParams))
		}
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetControlId sets the ControlId field's value.
func (s *DescribeHomeRegionControlsInput) SetControlId(v string) *DescribeHomeRegionControlsInput {
	s.ControlId = &v
	return s
}

// SetHomeRegion sets the HomeRegion field's value.
func (s *DescribeHomeRegionControlsInput) SetHomeRegion(v string) *DescribeHomeRegionControlsInput {
	s.HomeRegion = &v
	return s
}

// SetMaxResults sets the MaxResults field's value.
func (s *DescribeHomeRegionControlsInput) SetMaxResults(v int64) *DescribeHomeRegionControlsInput {
	s.MaxResults = &v
	return s
}

// SetNextToken sets the NextToken field's value.
func (s *DescribeHomeRegionControlsInput) SetNextToken(v string) *DescribeHomeRegionControlsInput {
	s.NextToken = &v
	return s
}

// SetTarget sets the Target field's value.
func (s *DescribeHomeRegionControlsInput) SetTarget(v *Target) *DescribeHomeRegionControlsInput {
	s.Target = v
	return s
}

type DescribeHomeRegionControlsOutput struct {
	_ struct{} `type:"structure"`

	// An array that contains your HomeRegionControl objects.
	HomeRegionControls []*HomeRegionControl `type:"list"`

	// If a NextToken was returned by a previous call, more results are available.
	// To retrieve the next page of results, make the call again using the returned
	// token in NextToken.
	NextToken *string `type:"string"`
}

// String returns the string representation
func (s DescribeHomeRegionControlsOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DescribeHomeRegionControlsOutput) GoString() string {
	return s.String()
}

// SetHomeRegionControls sets the HomeRegionControls field's value.
func (s *DescribeHomeRegionControlsOutput) SetHomeRegionControls(v []*HomeRegionControl) *DescribeHomeRegionControlsOutput {
	s.HomeRegionControls = v
	return s
}

// SetNextToken sets the NextToken field's value.
func (s *DescribeHomeRegionControlsOutput) SetNextToken(v string) *DescribeHomeRegionControlsOutput {
	s.NextToken = &v
	return s
}

// Exception raised to indicate that authorization of an action was successful,
// when the DryRun flag is set to true.
type DryRunOperation struct {
	_            struct{} `type:"structure"`
	respMetadata protocol.ResponseMetadata

	Message_ *string `locationName:"Message" type:"string"`
}

// String returns the string representation
func (s DryRunOperation) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DryRunOperation) GoString() string {
	return s.String()
}

func newErrorDryRunOperation(v protocol.ResponseMetadata) error {
	return &DryRunOperation{
		respMetadata: v,
	}
}

// Code returns the exception type name.
func (s DryRunOperation) Code() string {
	return "DryRunOperation"
}

// Message returns the exception's message.
func (s DryRunOperation) Message() string {
	if s.Message_ != nil {
		return *s.Message_
	}
	return ""
}

// OrigErr always returns nil, satisfies awserr.Error interface.
func (s DryRunOperation) OrigErr() error {
	return nil
}

func (s DryRunOperation) Error() string {
	return fmt.Sprintf("%s: %s", s.Code(), s.Message())
}

// Status code returns the HTTP status code for the request's response error.
func (s DryRunOperation) StatusCode() int {
	return s.respMetadata.StatusCode
}

// RequestID returns the service's response RequestID for request.
func (s DryRunOperation) RequestID() string {
	return s.respMetadata.RequestID
}

type GetHomeRegionInput struct {
	_ struct{} `type:"structure"`
}

// String returns the string representation
func (s GetHomeRegionInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s GetHomeRegionInput) GoString() string {
	return s.String()
}

type GetHomeRegionOutput struct {
	_ struct{} `type:"structure"`

	// The name of the home region of the calling account.
	HomeRegion *string `min:"1" type:"string"`
}

// String returns the string representation
func (s GetHomeRegionOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s GetHomeRegionOutput) GoString() string {
	return s.String()
}

// SetHomeRegion sets the HomeRegion field's value.
func (s *GetHomeRegionOutput) SetHomeRegion(v string) *GetHomeRegionOutput {
	s.HomeRegion = &v
	return s
}

// A home region control is an object that specifies the home region for an
// account, with some additional information. It contains a target (always of
// type ACCOUNT), an ID, and a time at which the home region was set.
type HomeRegionControl struct {
	_ struct{} `type:"structure"`

	// A unique identifier that's generated for each home region control. It's always
	// a string that begins with "hrc-" followed by 12 lowercase letters and numbers.
	ControlId *string `min:"1" type:"string"`

	// The AWS Region that's been set as home region. For example, "us-west-2" or
	// "eu-central-1" are valid home regions.
	HomeRegion *string `min:"1" type:"string"`

	// A timestamp representing the time when the customer called CreateHomeregionControl
	// and set the home region for the account.
	RequestedTime *time.Time `type:"timestamp"`

	// The target parameter specifies the identifier to which the home region is
	// applied, which is always an ACCOUNT. It applies the home region to the current
	// ACCOUNT.
	Target *Target `type:"structure"`
}

// String returns the string representation
func (s HomeRegionControl) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s HomeRegionControl) GoString() string {
	return s.String()
}

// SetControlId sets the ControlId field's value.
func (s *HomeRegionControl) SetControlId(v string) *HomeRegionControl {
	s.ControlId = &v
	return s
}

// SetHomeRegion sets the HomeRegion field's value.
func (s *HomeRegionControl) SetHomeRegion(v string) *HomeRegionControl {
	s.HomeRegion = &v
	return s
}

// SetRequestedTime sets the RequestedTime field's value.
func (s *HomeRegionControl) SetRequestedTime(v time.Time) *HomeRegionControl {
	s.RequestedTime = &v
	return s
}

// SetTarget sets the Target field's value.
func (s *HomeRegionControl) SetTarget(v *Target) *HomeRegionControl {
	s.Target = v
	return s
}

// Exception raised when an internal, configuration, or dependency error is
// encountered.
type InternalServerError struct {
	_            struct{} `type:"structure"`
	respMetadata protocol.ResponseMetadata

	Message_ *string `locationName:"Message" type:"string"`
}

// String returns the string representation
func (s InternalServerError) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s InternalServerError) GoString() string {
	return s.String()
}

func newErrorInternalServerError(v protocol.ResponseMetadata) error {
	return &InternalServerError{
		respMetadata: v,
	}
}

// Code returns the exception type name.
func (s InternalServerError) Code() string {
	return "InternalServerError"
}

// Message returns the exception's message.
func (s InternalServerError) Message() string {
	if s.Message_ != nil {
		return *s.Message_
	}
	return ""
}

// OrigErr always returns nil, satisfies awserr.Error interface.
func (s InternalServerError) OrigErr() error {
	return nil
}

func (s InternalServerError) Error() string {
	return fmt.Sprintf("%s: %s", s.Code(), s.Message())
}

// Status code returns the HTTP status code for the request's response error.
func (s InternalServerError) StatusCode() int {
	return s.respMetadata.StatusCode
}

// RequestID returns the service's response RequestID for request.
func (s InternalServerError) RequestID() string {
	return s.respMetadata.RequestID
}

// Exception raised when the provided input violates a policy constraint or
// is entered in the wrong format or data type.
type InvalidInputException struct {
	_            struct{} `type:"structure"`
	respMetadata protocol.ResponseMetadata

	Message_ *string `locationName:"Message" type:"string"`
}

// String returns the string representation
func (s InvalidInputException) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s InvalidInputException) GoString() string {
	return s.String()
}

func newErrorInvalidInputException(v protocol.ResponseMetadata) error {
	return &InvalidInputException{
		respMetadata: v,
	}
}

// Code returns the exception type name.
func (s InvalidInputException) Code() string {
	return "InvalidInputException"
}

// Message returns the exception's message.
func (s InvalidInputException) Message() string {
	if s.Message_ != nil {
		return *s.Message_
	}
	return ""
}

// OrigErr always returns nil, satisfies awserr.Error interface.
func (s InvalidInputException) OrigErr() error {
	return nil
}

func (s InvalidInputException) Error() string {
	return fmt.Sprintf("%s: %s", s.Code(), s.Message())
}

// Status code returns the HTTP status code for the request's response error.
func (s InvalidInputException) StatusCode() int {
	return s.respMetadata.StatusCode
}

// RequestID returns the service's response RequestID for request.
func (s InvalidInputException) RequestID() string {
	return s.respMetadata.RequestID
}

// Exception raised when a request fails due to temporary unavailability of
// the service.
type ServiceUnavailableException struct {
	_            struct{} `type:"structure"`
	respMetadata protocol.ResponseMetadata

	Message_ *string `locationName:"Message" type:"string"`
}

// String returns the string representation
func (s ServiceUnavailableException) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ServiceUnavailableException) GoString() string {
	return s.String()
}

func newErrorServiceUnavailableException(v protocol.ResponseMetadata) error {
	return &ServiceUnavailableException{
		respMetadata: v,
	}
}

// Code returns the exception type name.
func (s ServiceUnavailableException) Code() string {
	return "ServiceUnavailableException"
}

// Message returns the exception's message.
func (s ServiceUnavailableException) Message() string {
	if s.Message_ != nil {
		return *s.Message_
	}
	return ""
}

// OrigErr always returns nil, satisfies awserr.Error interface.
func (s ServiceUnavailableException) OrigErr() error {
	return nil
}

func (s ServiceUnavailableException) Error() string {
	return fmt.Sprintf("%s: %s", s.Code(), s.Message())
}

// Status code returns the HTTP status code for the request's response error.
func (s ServiceUnavailableException) StatusCode() int {
	return s.respMetadata.StatusCode
}

// RequestID returns the service's response RequestID for request.
func (s ServiceUnavailableException) RequestID() string {
	return s.respMetadata.RequestID
}

// The target parameter specifies the identifier to which the home region is
// applied, which is always an ACCOUNT. It applies the home region to the current
// ACCOUNT.
type Target struct {
	_ struct{} `type:"structure"`

	// The TargetID is a 12-character identifier of the ACCOUNT for which the control
	// was created. (This must be the current account.)
	Id *string `min:"12" type:"string"`

	// The target type is always an ACCOUNT.
	//
	// Type is a required field
	Type *string `type:"string" required:"true" enum:"TargetType"`
}

// String returns the string representation
func (s Target) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s Target) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *Target) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "Target"}
	if s.Id != nil && len(*s.Id) < 12 {
		invalidParams.Add(request.NewErrParamMinLen("Id", 12))
	}
	if s.Type == nil {
		invalidParams.Add(request.NewErrParamRequired("Type"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetId sets the Id field's value.
func (s *Target) SetId(v string) *Target {
	s.Id = &v
	return s
}

// SetType sets the Type field's value.
func (s *Target) SetType(v string) *Target {
	s.Type = &v
	return s
}

const (
	// TargetTypeAccount is a TargetType enum value
	TargetTypeAccount = "ACCOUNT"
)
