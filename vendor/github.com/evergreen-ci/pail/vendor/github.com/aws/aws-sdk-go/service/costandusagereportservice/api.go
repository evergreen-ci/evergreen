// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

package costandusagereportservice

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/aws/request"
)

const opDeleteReportDefinition = "DeleteReportDefinition"

// DeleteReportDefinitionRequest generates a "aws/request.Request" representing the
// client's request for the DeleteReportDefinition operation. The "output" return
// value will be populated with the request's response once the request complets
// successfuly.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See DeleteReportDefinition for more information on using the DeleteReportDefinition
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the DeleteReportDefinitionRequest method.
//    req, resp := client.DeleteReportDefinitionRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
//
// Please also see https://docs.aws.amazon.com/goto/WebAPI/cur-2017-01-06/DeleteReportDefinition
func (c *CostandUsageReportService) DeleteReportDefinitionRequest(input *DeleteReportDefinitionInput) (req *request.Request, output *DeleteReportDefinitionOutput) {
	op := &request.Operation{
		Name:       opDeleteReportDefinition,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &DeleteReportDefinitionInput{}
	}

	output = &DeleteReportDefinitionOutput{}
	req = c.newRequest(op, input, output)
	return
}

// DeleteReportDefinition API operation for AWS Cost and Usage Report Service.
//
// Delete a specified report definition
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for AWS Cost and Usage Report Service's
// API operation DeleteReportDefinition for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeInternalErrorException "InternalErrorException"
//   This exception is thrown on a known dependency failure.
//
//   * ErrCodeValidationException "ValidationException"
//   This exception is thrown when providing an invalid input. eg. Put a report
//   preference with an invalid report name, or Delete a report preference with
//   an empty report name.
//
// Please also see https://docs.aws.amazon.com/goto/WebAPI/cur-2017-01-06/DeleteReportDefinition
func (c *CostandUsageReportService) DeleteReportDefinition(input *DeleteReportDefinitionInput) (*DeleteReportDefinitionOutput, error) {
	req, out := c.DeleteReportDefinitionRequest(input)
	return out, req.Send()
}

// DeleteReportDefinitionWithContext is the same as DeleteReportDefinition with the addition of
// the ability to pass a context and additional request options.
//
// See DeleteReportDefinition for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *CostandUsageReportService) DeleteReportDefinitionWithContext(ctx aws.Context, input *DeleteReportDefinitionInput, opts ...request.Option) (*DeleteReportDefinitionOutput, error) {
	req, out := c.DeleteReportDefinitionRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opDescribeReportDefinitions = "DescribeReportDefinitions"

// DescribeReportDefinitionsRequest generates a "aws/request.Request" representing the
// client's request for the DescribeReportDefinitions operation. The "output" return
// value will be populated with the request's response once the request complets
// successfuly.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See DescribeReportDefinitions for more information on using the DescribeReportDefinitions
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the DescribeReportDefinitionsRequest method.
//    req, resp := client.DescribeReportDefinitionsRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
//
// Please also see https://docs.aws.amazon.com/goto/WebAPI/cur-2017-01-06/DescribeReportDefinitions
func (c *CostandUsageReportService) DescribeReportDefinitionsRequest(input *DescribeReportDefinitionsInput) (req *request.Request, output *DescribeReportDefinitionsOutput) {
	op := &request.Operation{
		Name:       opDescribeReportDefinitions,
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
		input = &DescribeReportDefinitionsInput{}
	}

	output = &DescribeReportDefinitionsOutput{}
	req = c.newRequest(op, input, output)
	return
}

// DescribeReportDefinitions API operation for AWS Cost and Usage Report Service.
//
// Describe a list of report definitions owned by the account
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for AWS Cost and Usage Report Service's
// API operation DescribeReportDefinitions for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeInternalErrorException "InternalErrorException"
//   This exception is thrown on a known dependency failure.
//
// Please also see https://docs.aws.amazon.com/goto/WebAPI/cur-2017-01-06/DescribeReportDefinitions
func (c *CostandUsageReportService) DescribeReportDefinitions(input *DescribeReportDefinitionsInput) (*DescribeReportDefinitionsOutput, error) {
	req, out := c.DescribeReportDefinitionsRequest(input)
	return out, req.Send()
}

// DescribeReportDefinitionsWithContext is the same as DescribeReportDefinitions with the addition of
// the ability to pass a context and additional request options.
//
// See DescribeReportDefinitions for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *CostandUsageReportService) DescribeReportDefinitionsWithContext(ctx aws.Context, input *DescribeReportDefinitionsInput, opts ...request.Option) (*DescribeReportDefinitionsOutput, error) {
	req, out := c.DescribeReportDefinitionsRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

// DescribeReportDefinitionsPages iterates over the pages of a DescribeReportDefinitions operation,
// calling the "fn" function with the response data for each page. To stop
// iterating, return false from the fn function.
//
// See DescribeReportDefinitions method for more information on how to use this operation.
//
// Note: This operation can generate multiple requests to a service.
//
//    // Example iterating over at most 3 pages of a DescribeReportDefinitions operation.
//    pageNum := 0
//    err := client.DescribeReportDefinitionsPages(params,
//        func(page *DescribeReportDefinitionsOutput, lastPage bool) bool {
//            pageNum++
//            fmt.Println(page)
//            return pageNum <= 3
//        })
//
func (c *CostandUsageReportService) DescribeReportDefinitionsPages(input *DescribeReportDefinitionsInput, fn func(*DescribeReportDefinitionsOutput, bool) bool) error {
	return c.DescribeReportDefinitionsPagesWithContext(aws.BackgroundContext(), input, fn)
}

// DescribeReportDefinitionsPagesWithContext same as DescribeReportDefinitionsPages except
// it takes a Context and allows setting request options on the pages.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *CostandUsageReportService) DescribeReportDefinitionsPagesWithContext(ctx aws.Context, input *DescribeReportDefinitionsInput, fn func(*DescribeReportDefinitionsOutput, bool) bool, opts ...request.Option) error {
	p := request.Pagination{
		NewRequest: func() (*request.Request, error) {
			var inCpy *DescribeReportDefinitionsInput
			if input != nil {
				tmp := *input
				inCpy = &tmp
			}
			req, _ := c.DescribeReportDefinitionsRequest(inCpy)
			req.SetContext(ctx)
			req.ApplyOptions(opts...)
			return req, nil
		},
	}

	cont := true
	for p.Next() && cont {
		cont = fn(p.Page().(*DescribeReportDefinitionsOutput), !p.HasNextPage())
	}
	return p.Err()
}

const opPutReportDefinition = "PutReportDefinition"

// PutReportDefinitionRequest generates a "aws/request.Request" representing the
// client's request for the PutReportDefinition operation. The "output" return
// value will be populated with the request's response once the request complets
// successfuly.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See PutReportDefinition for more information on using the PutReportDefinition
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the PutReportDefinitionRequest method.
//    req, resp := client.PutReportDefinitionRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
//
// Please also see https://docs.aws.amazon.com/goto/WebAPI/cur-2017-01-06/PutReportDefinition
func (c *CostandUsageReportService) PutReportDefinitionRequest(input *PutReportDefinitionInput) (req *request.Request, output *PutReportDefinitionOutput) {
	op := &request.Operation{
		Name:       opPutReportDefinition,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &PutReportDefinitionInput{}
	}

	output = &PutReportDefinitionOutput{}
	req = c.newRequest(op, input, output)
	return
}

// PutReportDefinition API operation for AWS Cost and Usage Report Service.
//
// Create a new report definition
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for AWS Cost and Usage Report Service's
// API operation PutReportDefinition for usage and error information.
//
// Returned Error Codes:
//   * ErrCodeDuplicateReportNameException "DuplicateReportNameException"
//   This exception is thrown when putting a report preference with a name that
//   already exists.
//
//   * ErrCodeReportLimitReachedException "ReportLimitReachedException"
//   This exception is thrown when the number of report preference reaches max
//   limit. The max number is 5.
//
//   * ErrCodeInternalErrorException "InternalErrorException"
//   This exception is thrown on a known dependency failure.
//
//   * ErrCodeValidationException "ValidationException"
//   This exception is thrown when providing an invalid input. eg. Put a report
//   preference with an invalid report name, or Delete a report preference with
//   an empty report name.
//
// Please also see https://docs.aws.amazon.com/goto/WebAPI/cur-2017-01-06/PutReportDefinition
func (c *CostandUsageReportService) PutReportDefinition(input *PutReportDefinitionInput) (*PutReportDefinitionOutput, error) {
	req, out := c.PutReportDefinitionRequest(input)
	return out, req.Send()
}

// PutReportDefinitionWithContext is the same as PutReportDefinition with the addition of
// the ability to pass a context and additional request options.
//
// See PutReportDefinition for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *CostandUsageReportService) PutReportDefinitionWithContext(ctx aws.Context, input *PutReportDefinitionInput, opts ...request.Option) (*PutReportDefinitionOutput, error) {
	req, out := c.PutReportDefinitionRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

// Request of DeleteReportDefinition
// Please also see https://docs.aws.amazon.com/goto/WebAPI/cur-2017-01-06/DeleteReportDefinitionRequest
type DeleteReportDefinitionInput struct {
	_ struct{} `type:"structure"`

	// Preferred name for a report, it has to be unique. Must starts with a number/letter,
	// case sensitive. Limited to 256 characters.
	ReportName *string `type:"string"`
}

// String returns the string representation
func (s DeleteReportDefinitionInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DeleteReportDefinitionInput) GoString() string {
	return s.String()
}

// SetReportName sets the ReportName field's value.
func (s *DeleteReportDefinitionInput) SetReportName(v string) *DeleteReportDefinitionInput {
	s.ReportName = &v
	return s
}

// Response of DeleteReportDefinition
// Please also see https://docs.aws.amazon.com/goto/WebAPI/cur-2017-01-06/DeleteReportDefinitionResponse
type DeleteReportDefinitionOutput struct {
	_ struct{} `type:"structure"`

	// A message indicates if the deletion is successful.
	ResponseMessage *string `type:"string"`
}

// String returns the string representation
func (s DeleteReportDefinitionOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DeleteReportDefinitionOutput) GoString() string {
	return s.String()
}

// SetResponseMessage sets the ResponseMessage field's value.
func (s *DeleteReportDefinitionOutput) SetResponseMessage(v string) *DeleteReportDefinitionOutput {
	s.ResponseMessage = &v
	return s
}

// Request of DescribeReportDefinitions
// Please also see https://docs.aws.amazon.com/goto/WebAPI/cur-2017-01-06/DescribeReportDefinitionsRequest
type DescribeReportDefinitionsInput struct {
	_ struct{} `type:"structure"`

	// The max number of results returned by the operation.
	MaxResults *int64 `min:"5" type:"integer"`

	// A generic string.
	NextToken *string `type:"string"`
}

// String returns the string representation
func (s DescribeReportDefinitionsInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DescribeReportDefinitionsInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *DescribeReportDefinitionsInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "DescribeReportDefinitionsInput"}
	if s.MaxResults != nil && *s.MaxResults < 5 {
		invalidParams.Add(request.NewErrParamMinValue("MaxResults", 5))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetMaxResults sets the MaxResults field's value.
func (s *DescribeReportDefinitionsInput) SetMaxResults(v int64) *DescribeReportDefinitionsInput {
	s.MaxResults = &v
	return s
}

// SetNextToken sets the NextToken field's value.
func (s *DescribeReportDefinitionsInput) SetNextToken(v string) *DescribeReportDefinitionsInput {
	s.NextToken = &v
	return s
}

// Response of DescribeReportDefinitions
// Please also see https://docs.aws.amazon.com/goto/WebAPI/cur-2017-01-06/DescribeReportDefinitionsResponse
type DescribeReportDefinitionsOutput struct {
	_ struct{} `type:"structure"`

	// A generic string.
	NextToken *string `type:"string"`

	// A list of report definitions.
	ReportDefinitions []*ReportDefinition `type:"list"`
}

// String returns the string representation
func (s DescribeReportDefinitionsOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DescribeReportDefinitionsOutput) GoString() string {
	return s.String()
}

// SetNextToken sets the NextToken field's value.
func (s *DescribeReportDefinitionsOutput) SetNextToken(v string) *DescribeReportDefinitionsOutput {
	s.NextToken = &v
	return s
}

// SetReportDefinitions sets the ReportDefinitions field's value.
func (s *DescribeReportDefinitionsOutput) SetReportDefinitions(v []*ReportDefinition) *DescribeReportDefinitionsOutput {
	s.ReportDefinitions = v
	return s
}

// Request of PutReportDefinition
// Please also see https://docs.aws.amazon.com/goto/WebAPI/cur-2017-01-06/PutReportDefinitionRequest
type PutReportDefinitionInput struct {
	_ struct{} `type:"structure"`

	// The definition of AWS Cost and Usage Report. Customer can specify the report
	// name, time unit, report format, compression format, S3 bucket and additional
	// artifacts and schema elements in the definition.
	//
	// ReportDefinition is a required field
	ReportDefinition *ReportDefinition `type:"structure" required:"true"`
}

// String returns the string representation
func (s PutReportDefinitionInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s PutReportDefinitionInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *PutReportDefinitionInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "PutReportDefinitionInput"}
	if s.ReportDefinition == nil {
		invalidParams.Add(request.NewErrParamRequired("ReportDefinition"))
	}
	if s.ReportDefinition != nil {
		if err := s.ReportDefinition.Validate(); err != nil {
			invalidParams.AddNested("ReportDefinition", err.(request.ErrInvalidParams))
		}
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetReportDefinition sets the ReportDefinition field's value.
func (s *PutReportDefinitionInput) SetReportDefinition(v *ReportDefinition) *PutReportDefinitionInput {
	s.ReportDefinition = v
	return s
}

// Response of PutReportDefinition
// Please also see https://docs.aws.amazon.com/goto/WebAPI/cur-2017-01-06/PutReportDefinitionResponse
type PutReportDefinitionOutput struct {
	_ struct{} `type:"structure"`
}

// String returns the string representation
func (s PutReportDefinitionOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s PutReportDefinitionOutput) GoString() string {
	return s.String()
}

// The definition of AWS Cost and Usage Report. Customer can specify the report
// name, time unit, report format, compression format, S3 bucket and additional
// artifacts and schema elements in the definition.
// Please also see https://docs.aws.amazon.com/goto/WebAPI/cur-2017-01-06/ReportDefinition
type ReportDefinition struct {
	_ struct{} `type:"structure"`

	// A list of additional artifacts.
	AdditionalArtifacts []*string `type:"list"`

	// A list of schema elements.
	//
	// AdditionalSchemaElements is a required field
	AdditionalSchemaElements []*string `type:"list" required:"true"`

	// Preferred compression format for report.
	//
	// Compression is a required field
	Compression *string `type:"string" required:"true" enum:"CompressionFormat"`

	// Preferred format for report.
	//
	// Format is a required field
	Format *string `type:"string" required:"true" enum:"ReportFormat"`

	// Preferred name for a report, it has to be unique. Must starts with a number/letter,
	// case sensitive. Limited to 256 characters.
	//
	// ReportName is a required field
	ReportName *string `type:"string" required:"true"`

	// Name of customer S3 bucket.
	//
	// S3Bucket is a required field
	S3Bucket *string `type:"string" required:"true"`

	// Preferred report path prefix. Limited to 256 characters.
	//
	// S3Prefix is a required field
	S3Prefix *string `type:"string" required:"true"`

	// Region of customer S3 bucket.
	//
	// S3Region is a required field
	S3Region *string `type:"string" required:"true" enum:"AWSRegion"`

	// The frequency on which report data are measured and displayed.
	//
	// TimeUnit is a required field
	TimeUnit *string `type:"string" required:"true" enum:"TimeUnit"`
}

// String returns the string representation
func (s ReportDefinition) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ReportDefinition) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *ReportDefinition) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "ReportDefinition"}
	if s.AdditionalSchemaElements == nil {
		invalidParams.Add(request.NewErrParamRequired("AdditionalSchemaElements"))
	}
	if s.Compression == nil {
		invalidParams.Add(request.NewErrParamRequired("Compression"))
	}
	if s.Format == nil {
		invalidParams.Add(request.NewErrParamRequired("Format"))
	}
	if s.ReportName == nil {
		invalidParams.Add(request.NewErrParamRequired("ReportName"))
	}
	if s.S3Bucket == nil {
		invalidParams.Add(request.NewErrParamRequired("S3Bucket"))
	}
	if s.S3Prefix == nil {
		invalidParams.Add(request.NewErrParamRequired("S3Prefix"))
	}
	if s.S3Region == nil {
		invalidParams.Add(request.NewErrParamRequired("S3Region"))
	}
	if s.TimeUnit == nil {
		invalidParams.Add(request.NewErrParamRequired("TimeUnit"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetAdditionalArtifacts sets the AdditionalArtifacts field's value.
func (s *ReportDefinition) SetAdditionalArtifacts(v []*string) *ReportDefinition {
	s.AdditionalArtifacts = v
	return s
}

// SetAdditionalSchemaElements sets the AdditionalSchemaElements field's value.
func (s *ReportDefinition) SetAdditionalSchemaElements(v []*string) *ReportDefinition {
	s.AdditionalSchemaElements = v
	return s
}

// SetCompression sets the Compression field's value.
func (s *ReportDefinition) SetCompression(v string) *ReportDefinition {
	s.Compression = &v
	return s
}

// SetFormat sets the Format field's value.
func (s *ReportDefinition) SetFormat(v string) *ReportDefinition {
	s.Format = &v
	return s
}

// SetReportName sets the ReportName field's value.
func (s *ReportDefinition) SetReportName(v string) *ReportDefinition {
	s.ReportName = &v
	return s
}

// SetS3Bucket sets the S3Bucket field's value.
func (s *ReportDefinition) SetS3Bucket(v string) *ReportDefinition {
	s.S3Bucket = &v
	return s
}

// SetS3Prefix sets the S3Prefix field's value.
func (s *ReportDefinition) SetS3Prefix(v string) *ReportDefinition {
	s.S3Prefix = &v
	return s
}

// SetS3Region sets the S3Region field's value.
func (s *ReportDefinition) SetS3Region(v string) *ReportDefinition {
	s.S3Region = &v
	return s
}

// SetTimeUnit sets the TimeUnit field's value.
func (s *ReportDefinition) SetTimeUnit(v string) *ReportDefinition {
	s.TimeUnit = &v
	return s
}

// Region of customer S3 bucket.
const (
	// AWSRegionUsEast1 is a AWSRegion enum value
	AWSRegionUsEast1 = "us-east-1"

	// AWSRegionUsWest1 is a AWSRegion enum value
	AWSRegionUsWest1 = "us-west-1"

	// AWSRegionUsWest2 is a AWSRegion enum value
	AWSRegionUsWest2 = "us-west-2"

	// AWSRegionEuCentral1 is a AWSRegion enum value
	AWSRegionEuCentral1 = "eu-central-1"

	// AWSRegionEuWest1 is a AWSRegion enum value
	AWSRegionEuWest1 = "eu-west-1"

	// AWSRegionApSoutheast1 is a AWSRegion enum value
	AWSRegionApSoutheast1 = "ap-southeast-1"

	// AWSRegionApSoutheast2 is a AWSRegion enum value
	AWSRegionApSoutheast2 = "ap-southeast-2"

	// AWSRegionApNortheast1 is a AWSRegion enum value
	AWSRegionApNortheast1 = "ap-northeast-1"
)

// Enable support for Redshift and/or QuickSight.
const (
	// AdditionalArtifactRedshift is a AdditionalArtifact enum value
	AdditionalArtifactRedshift = "REDSHIFT"

	// AdditionalArtifactQuicksight is a AdditionalArtifact enum value
	AdditionalArtifactQuicksight = "QUICKSIGHT"
)

// Preferred compression format for report.
const (
	// CompressionFormatZip is a CompressionFormat enum value
	CompressionFormatZip = "ZIP"

	// CompressionFormatGzip is a CompressionFormat enum value
	CompressionFormatGzip = "GZIP"
)

// Preferred format for report.
const (
	// ReportFormatTextOrcsv is a ReportFormat enum value
	ReportFormatTextOrcsv = "textORcsv"
)

// Preference of including Resource IDs. You can include additional details
// about individual resource IDs in your report.
const (
	// SchemaElementResources is a SchemaElement enum value
	SchemaElementResources = "RESOURCES"
)

// The frequency on which report data are measured and displayed.
const (
	// TimeUnitHourly is a TimeUnit enum value
	TimeUnitHourly = "HOURLY"

	// TimeUnitDaily is a TimeUnit enum value
	TimeUnitDaily = "DAILY"
)
