// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

package iotdataplane

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/private/protocol"
	"github.com/aws/aws-sdk-go/private/protocol/restjson"
)

const opDeleteThingShadow = "DeleteThingShadow"

// DeleteThingShadowRequest generates a "aws/request.Request" representing the
// client's request for the DeleteThingShadow operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See DeleteThingShadow for more information on using the DeleteThingShadow
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the DeleteThingShadowRequest method.
//    req, resp := client.DeleteThingShadowRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *IoTDataPlane) DeleteThingShadowRequest(input *DeleteThingShadowInput) (req *request.Request, output *DeleteThingShadowOutput) {
	op := &request.Operation{
		Name:       opDeleteThingShadow,
		HTTPMethod: "DELETE",
		HTTPPath:   "/things/{thingName}/shadow",
	}

	if input == nil {
		input = &DeleteThingShadowInput{}
	}

	output = &DeleteThingShadowOutput{}
	req = c.newRequest(op, input, output)
	return
}

// DeleteThingShadow API operation for AWS IoT Data Plane.
//
// Deletes the thing shadow for the specified thing.
//
// For more information, see DeleteThingShadow (http://docs.aws.amazon.com/iot/latest/developerguide/API_DeleteThingShadow.html)
// in the AWS IoT Developer Guide.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for AWS IoT Data Plane's
// API operation DeleteThingShadow for usage and error information.
//
// Returned Error Types:
//   * ResourceNotFoundException
//   The specified resource does not exist.
//
//   * InvalidRequestException
//   The request is not valid.
//
//   * ThrottlingException
//   The rate exceeds the limit.
//
//   * UnauthorizedException
//   You are not authorized to perform this operation.
//
//   * ServiceUnavailableException
//   The service is temporarily unavailable.
//
//   * InternalFailureException
//   An unexpected error has occurred.
//
//   * MethodNotAllowedException
//   The specified combination of HTTP verb and URI is not supported.
//
//   * UnsupportedDocumentEncodingException
//   The document encoding is not supported.
//
func (c *IoTDataPlane) DeleteThingShadow(input *DeleteThingShadowInput) (*DeleteThingShadowOutput, error) {
	req, out := c.DeleteThingShadowRequest(input)
	return out, req.Send()
}

// DeleteThingShadowWithContext is the same as DeleteThingShadow with the addition of
// the ability to pass a context and additional request options.
//
// See DeleteThingShadow for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *IoTDataPlane) DeleteThingShadowWithContext(ctx aws.Context, input *DeleteThingShadowInput, opts ...request.Option) (*DeleteThingShadowOutput, error) {
	req, out := c.DeleteThingShadowRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opGetThingShadow = "GetThingShadow"

// GetThingShadowRequest generates a "aws/request.Request" representing the
// client's request for the GetThingShadow operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See GetThingShadow for more information on using the GetThingShadow
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the GetThingShadowRequest method.
//    req, resp := client.GetThingShadowRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *IoTDataPlane) GetThingShadowRequest(input *GetThingShadowInput) (req *request.Request, output *GetThingShadowOutput) {
	op := &request.Operation{
		Name:       opGetThingShadow,
		HTTPMethod: "GET",
		HTTPPath:   "/things/{thingName}/shadow",
	}

	if input == nil {
		input = &GetThingShadowInput{}
	}

	output = &GetThingShadowOutput{}
	req = c.newRequest(op, input, output)
	return
}

// GetThingShadow API operation for AWS IoT Data Plane.
//
// Gets the thing shadow for the specified thing.
//
// For more information, see GetThingShadow (http://docs.aws.amazon.com/iot/latest/developerguide/API_GetThingShadow.html)
// in the AWS IoT Developer Guide.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for AWS IoT Data Plane's
// API operation GetThingShadow for usage and error information.
//
// Returned Error Types:
//   * InvalidRequestException
//   The request is not valid.
//
//   * ResourceNotFoundException
//   The specified resource does not exist.
//
//   * ThrottlingException
//   The rate exceeds the limit.
//
//   * UnauthorizedException
//   You are not authorized to perform this operation.
//
//   * ServiceUnavailableException
//   The service is temporarily unavailable.
//
//   * InternalFailureException
//   An unexpected error has occurred.
//
//   * MethodNotAllowedException
//   The specified combination of HTTP verb and URI is not supported.
//
//   * UnsupportedDocumentEncodingException
//   The document encoding is not supported.
//
func (c *IoTDataPlane) GetThingShadow(input *GetThingShadowInput) (*GetThingShadowOutput, error) {
	req, out := c.GetThingShadowRequest(input)
	return out, req.Send()
}

// GetThingShadowWithContext is the same as GetThingShadow with the addition of
// the ability to pass a context and additional request options.
//
// See GetThingShadow for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *IoTDataPlane) GetThingShadowWithContext(ctx aws.Context, input *GetThingShadowInput, opts ...request.Option) (*GetThingShadowOutput, error) {
	req, out := c.GetThingShadowRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opPublish = "Publish"

// PublishRequest generates a "aws/request.Request" representing the
// client's request for the Publish operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See Publish for more information on using the Publish
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the PublishRequest method.
//    req, resp := client.PublishRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *IoTDataPlane) PublishRequest(input *PublishInput) (req *request.Request, output *PublishOutput) {
	op := &request.Operation{
		Name:       opPublish,
		HTTPMethod: "POST",
		HTTPPath:   "/topics/{topic}",
	}

	if input == nil {
		input = &PublishInput{}
	}

	output = &PublishOutput{}
	req = c.newRequest(op, input, output)
	req.Handlers.Unmarshal.Swap(restjson.UnmarshalHandler.Name, protocol.UnmarshalDiscardBodyHandler)
	return
}

// Publish API operation for AWS IoT Data Plane.
//
// Publishes state information.
//
// For more information, see HTTP Protocol (http://docs.aws.amazon.com/iot/latest/developerguide/protocols.html#http)
// in the AWS IoT Developer Guide.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for AWS IoT Data Plane's
// API operation Publish for usage and error information.
//
// Returned Error Types:
//   * InternalFailureException
//   An unexpected error has occurred.
//
//   * InvalidRequestException
//   The request is not valid.
//
//   * UnauthorizedException
//   You are not authorized to perform this operation.
//
//   * MethodNotAllowedException
//   The specified combination of HTTP verb and URI is not supported.
//
func (c *IoTDataPlane) Publish(input *PublishInput) (*PublishOutput, error) {
	req, out := c.PublishRequest(input)
	return out, req.Send()
}

// PublishWithContext is the same as Publish with the addition of
// the ability to pass a context and additional request options.
//
// See Publish for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *IoTDataPlane) PublishWithContext(ctx aws.Context, input *PublishInput, opts ...request.Option) (*PublishOutput, error) {
	req, out := c.PublishRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opUpdateThingShadow = "UpdateThingShadow"

// UpdateThingShadowRequest generates a "aws/request.Request" representing the
// client's request for the UpdateThingShadow operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See UpdateThingShadow for more information on using the UpdateThingShadow
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the UpdateThingShadowRequest method.
//    req, resp := client.UpdateThingShadowRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
func (c *IoTDataPlane) UpdateThingShadowRequest(input *UpdateThingShadowInput) (req *request.Request, output *UpdateThingShadowOutput) {
	op := &request.Operation{
		Name:       opUpdateThingShadow,
		HTTPMethod: "POST",
		HTTPPath:   "/things/{thingName}/shadow",
	}

	if input == nil {
		input = &UpdateThingShadowInput{}
	}

	output = &UpdateThingShadowOutput{}
	req = c.newRequest(op, input, output)
	return
}

// UpdateThingShadow API operation for AWS IoT Data Plane.
//
// Updates the thing shadow for the specified thing.
//
// For more information, see UpdateThingShadow (http://docs.aws.amazon.com/iot/latest/developerguide/API_UpdateThingShadow.html)
// in the AWS IoT Developer Guide.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for AWS IoT Data Plane's
// API operation UpdateThingShadow for usage and error information.
//
// Returned Error Types:
//   * ConflictException
//   The specified version does not match the version of the document.
//
//   * RequestEntityTooLargeException
//   The payload exceeds the maximum size allowed.
//
//   * InvalidRequestException
//   The request is not valid.
//
//   * ThrottlingException
//   The rate exceeds the limit.
//
//   * UnauthorizedException
//   You are not authorized to perform this operation.
//
//   * ServiceUnavailableException
//   The service is temporarily unavailable.
//
//   * InternalFailureException
//   An unexpected error has occurred.
//
//   * MethodNotAllowedException
//   The specified combination of HTTP verb and URI is not supported.
//
//   * UnsupportedDocumentEncodingException
//   The document encoding is not supported.
//
func (c *IoTDataPlane) UpdateThingShadow(input *UpdateThingShadowInput) (*UpdateThingShadowOutput, error) {
	req, out := c.UpdateThingShadowRequest(input)
	return out, req.Send()
}

// UpdateThingShadowWithContext is the same as UpdateThingShadow with the addition of
// the ability to pass a context and additional request options.
//
// See UpdateThingShadow for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *IoTDataPlane) UpdateThingShadowWithContext(ctx aws.Context, input *UpdateThingShadowInput, opts ...request.Option) (*UpdateThingShadowOutput, error) {
	req, out := c.UpdateThingShadowRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

// The specified version does not match the version of the document.
type ConflictException struct {
	_            struct{} `type:"structure"`
	respMetadata protocol.ResponseMetadata

	// The message for the exception.
	Message_ *string `locationName:"message" type:"string"`
}

// String returns the string representation
func (s ConflictException) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ConflictException) GoString() string {
	return s.String()
}

func newErrorConflictException(v protocol.ResponseMetadata) error {
	return &ConflictException{
		respMetadata: v,
	}
}

// Code returns the exception type name.
func (s ConflictException) Code() string {
	return "ConflictException"
}

// Message returns the exception's message.
func (s ConflictException) Message() string {
	if s.Message_ != nil {
		return *s.Message_
	}
	return ""
}

// OrigErr always returns nil, satisfies awserr.Error interface.
func (s ConflictException) OrigErr() error {
	return nil
}

func (s ConflictException) Error() string {
	return fmt.Sprintf("%s: %s", s.Code(), s.Message())
}

// Status code returns the HTTP status code for the request's response error.
func (s ConflictException) StatusCode() int {
	return s.respMetadata.StatusCode
}

// RequestID returns the service's response RequestID for request.
func (s ConflictException) RequestID() string {
	return s.respMetadata.RequestID
}

// The input for the DeleteThingShadow operation.
type DeleteThingShadowInput struct {
	_ struct{} `type:"structure"`

	// The name of the thing.
	//
	// ThingName is a required field
	ThingName *string `location:"uri" locationName:"thingName" min:"1" type:"string" required:"true"`
}

// String returns the string representation
func (s DeleteThingShadowInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DeleteThingShadowInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *DeleteThingShadowInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "DeleteThingShadowInput"}
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
func (s *DeleteThingShadowInput) SetThingName(v string) *DeleteThingShadowInput {
	s.ThingName = &v
	return s
}

// The output from the DeleteThingShadow operation.
type DeleteThingShadowOutput struct {
	_ struct{} `type:"structure" payload:"Payload"`

	// The state information, in JSON format.
	//
	// Payload is a required field
	Payload []byte `locationName:"payload" type:"blob" required:"true"`
}

// String returns the string representation
func (s DeleteThingShadowOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s DeleteThingShadowOutput) GoString() string {
	return s.String()
}

// SetPayload sets the Payload field's value.
func (s *DeleteThingShadowOutput) SetPayload(v []byte) *DeleteThingShadowOutput {
	s.Payload = v
	return s
}

// The input for the GetThingShadow operation.
type GetThingShadowInput struct {
	_ struct{} `type:"structure"`

	// The name of the thing.
	//
	// ThingName is a required field
	ThingName *string `location:"uri" locationName:"thingName" min:"1" type:"string" required:"true"`
}

// String returns the string representation
func (s GetThingShadowInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s GetThingShadowInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *GetThingShadowInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "GetThingShadowInput"}
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
func (s *GetThingShadowInput) SetThingName(v string) *GetThingShadowInput {
	s.ThingName = &v
	return s
}

// The output from the GetThingShadow operation.
type GetThingShadowOutput struct {
	_ struct{} `type:"structure" payload:"Payload"`

	// The state information, in JSON format.
	Payload []byte `locationName:"payload" type:"blob"`
}

// String returns the string representation
func (s GetThingShadowOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s GetThingShadowOutput) GoString() string {
	return s.String()
}

// SetPayload sets the Payload field's value.
func (s *GetThingShadowOutput) SetPayload(v []byte) *GetThingShadowOutput {
	s.Payload = v
	return s
}

// An unexpected error has occurred.
type InternalFailureException struct {
	_            struct{} `type:"structure"`
	respMetadata protocol.ResponseMetadata

	// The message for the exception.
	Message_ *string `locationName:"message" type:"string"`
}

// String returns the string representation
func (s InternalFailureException) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s InternalFailureException) GoString() string {
	return s.String()
}

func newErrorInternalFailureException(v protocol.ResponseMetadata) error {
	return &InternalFailureException{
		respMetadata: v,
	}
}

// Code returns the exception type name.
func (s InternalFailureException) Code() string {
	return "InternalFailureException"
}

// Message returns the exception's message.
func (s InternalFailureException) Message() string {
	if s.Message_ != nil {
		return *s.Message_
	}
	return ""
}

// OrigErr always returns nil, satisfies awserr.Error interface.
func (s InternalFailureException) OrigErr() error {
	return nil
}

func (s InternalFailureException) Error() string {
	return fmt.Sprintf("%s: %s", s.Code(), s.Message())
}

// Status code returns the HTTP status code for the request's response error.
func (s InternalFailureException) StatusCode() int {
	return s.respMetadata.StatusCode
}

// RequestID returns the service's response RequestID for request.
func (s InternalFailureException) RequestID() string {
	return s.respMetadata.RequestID
}

// The request is not valid.
type InvalidRequestException struct {
	_            struct{} `type:"structure"`
	respMetadata protocol.ResponseMetadata

	// The message for the exception.
	Message_ *string `locationName:"message" type:"string"`
}

// String returns the string representation
func (s InvalidRequestException) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s InvalidRequestException) GoString() string {
	return s.String()
}

func newErrorInvalidRequestException(v protocol.ResponseMetadata) error {
	return &InvalidRequestException{
		respMetadata: v,
	}
}

// Code returns the exception type name.
func (s InvalidRequestException) Code() string {
	return "InvalidRequestException"
}

// Message returns the exception's message.
func (s InvalidRequestException) Message() string {
	if s.Message_ != nil {
		return *s.Message_
	}
	return ""
}

// OrigErr always returns nil, satisfies awserr.Error interface.
func (s InvalidRequestException) OrigErr() error {
	return nil
}

func (s InvalidRequestException) Error() string {
	return fmt.Sprintf("%s: %s", s.Code(), s.Message())
}

// Status code returns the HTTP status code for the request's response error.
func (s InvalidRequestException) StatusCode() int {
	return s.respMetadata.StatusCode
}

// RequestID returns the service's response RequestID for request.
func (s InvalidRequestException) RequestID() string {
	return s.respMetadata.RequestID
}

// The specified combination of HTTP verb and URI is not supported.
type MethodNotAllowedException struct {
	_            struct{} `type:"structure"`
	respMetadata protocol.ResponseMetadata

	// The message for the exception.
	Message_ *string `locationName:"message" type:"string"`
}

// String returns the string representation
func (s MethodNotAllowedException) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s MethodNotAllowedException) GoString() string {
	return s.String()
}

func newErrorMethodNotAllowedException(v protocol.ResponseMetadata) error {
	return &MethodNotAllowedException{
		respMetadata: v,
	}
}

// Code returns the exception type name.
func (s MethodNotAllowedException) Code() string {
	return "MethodNotAllowedException"
}

// Message returns the exception's message.
func (s MethodNotAllowedException) Message() string {
	if s.Message_ != nil {
		return *s.Message_
	}
	return ""
}

// OrigErr always returns nil, satisfies awserr.Error interface.
func (s MethodNotAllowedException) OrigErr() error {
	return nil
}

func (s MethodNotAllowedException) Error() string {
	return fmt.Sprintf("%s: %s", s.Code(), s.Message())
}

// Status code returns the HTTP status code for the request's response error.
func (s MethodNotAllowedException) StatusCode() int {
	return s.respMetadata.StatusCode
}

// RequestID returns the service's response RequestID for request.
func (s MethodNotAllowedException) RequestID() string {
	return s.respMetadata.RequestID
}

// The input for the Publish operation.
type PublishInput struct {
	_ struct{} `type:"structure" payload:"Payload"`

	// The state information, in JSON format.
	Payload []byte `locationName:"payload" type:"blob"`

	// The Quality of Service (QoS) level.
	Qos *int64 `location:"querystring" locationName:"qos" type:"integer"`

	// The name of the MQTT topic.
	//
	// Topic is a required field
	Topic *string `location:"uri" locationName:"topic" type:"string" required:"true"`
}

// String returns the string representation
func (s PublishInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s PublishInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *PublishInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "PublishInput"}
	if s.Topic == nil {
		invalidParams.Add(request.NewErrParamRequired("Topic"))
	}
	if s.Topic != nil && len(*s.Topic) < 1 {
		invalidParams.Add(request.NewErrParamMinLen("Topic", 1))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetPayload sets the Payload field's value.
func (s *PublishInput) SetPayload(v []byte) *PublishInput {
	s.Payload = v
	return s
}

// SetQos sets the Qos field's value.
func (s *PublishInput) SetQos(v int64) *PublishInput {
	s.Qos = &v
	return s
}

// SetTopic sets the Topic field's value.
func (s *PublishInput) SetTopic(v string) *PublishInput {
	s.Topic = &v
	return s
}

type PublishOutput struct {
	_ struct{} `type:"structure"`
}

// String returns the string representation
func (s PublishOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s PublishOutput) GoString() string {
	return s.String()
}

// The payload exceeds the maximum size allowed.
type RequestEntityTooLargeException struct {
	_            struct{} `type:"structure"`
	respMetadata protocol.ResponseMetadata

	// The message for the exception.
	Message_ *string `locationName:"message" type:"string"`
}

// String returns the string representation
func (s RequestEntityTooLargeException) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s RequestEntityTooLargeException) GoString() string {
	return s.String()
}

func newErrorRequestEntityTooLargeException(v protocol.ResponseMetadata) error {
	return &RequestEntityTooLargeException{
		respMetadata: v,
	}
}

// Code returns the exception type name.
func (s RequestEntityTooLargeException) Code() string {
	return "RequestEntityTooLargeException"
}

// Message returns the exception's message.
func (s RequestEntityTooLargeException) Message() string {
	if s.Message_ != nil {
		return *s.Message_
	}
	return ""
}

// OrigErr always returns nil, satisfies awserr.Error interface.
func (s RequestEntityTooLargeException) OrigErr() error {
	return nil
}

func (s RequestEntityTooLargeException) Error() string {
	return fmt.Sprintf("%s: %s", s.Code(), s.Message())
}

// Status code returns the HTTP status code for the request's response error.
func (s RequestEntityTooLargeException) StatusCode() int {
	return s.respMetadata.StatusCode
}

// RequestID returns the service's response RequestID for request.
func (s RequestEntityTooLargeException) RequestID() string {
	return s.respMetadata.RequestID
}

// The specified resource does not exist.
type ResourceNotFoundException struct {
	_            struct{} `type:"structure"`
	respMetadata protocol.ResponseMetadata

	// The message for the exception.
	Message_ *string `locationName:"message" type:"string"`
}

// String returns the string representation
func (s ResourceNotFoundException) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ResourceNotFoundException) GoString() string {
	return s.String()
}

func newErrorResourceNotFoundException(v protocol.ResponseMetadata) error {
	return &ResourceNotFoundException{
		respMetadata: v,
	}
}

// Code returns the exception type name.
func (s ResourceNotFoundException) Code() string {
	return "ResourceNotFoundException"
}

// Message returns the exception's message.
func (s ResourceNotFoundException) Message() string {
	if s.Message_ != nil {
		return *s.Message_
	}
	return ""
}

// OrigErr always returns nil, satisfies awserr.Error interface.
func (s ResourceNotFoundException) OrigErr() error {
	return nil
}

func (s ResourceNotFoundException) Error() string {
	return fmt.Sprintf("%s: %s", s.Code(), s.Message())
}

// Status code returns the HTTP status code for the request's response error.
func (s ResourceNotFoundException) StatusCode() int {
	return s.respMetadata.StatusCode
}

// RequestID returns the service's response RequestID for request.
func (s ResourceNotFoundException) RequestID() string {
	return s.respMetadata.RequestID
}

// The service is temporarily unavailable.
type ServiceUnavailableException struct {
	_            struct{} `type:"structure"`
	respMetadata protocol.ResponseMetadata

	// The message for the exception.
	Message_ *string `locationName:"message" type:"string"`
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

// The rate exceeds the limit.
type ThrottlingException struct {
	_            struct{} `type:"structure"`
	respMetadata protocol.ResponseMetadata

	// The message for the exception.
	Message_ *string `locationName:"message" type:"string"`
}

// String returns the string representation
func (s ThrottlingException) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s ThrottlingException) GoString() string {
	return s.String()
}

func newErrorThrottlingException(v protocol.ResponseMetadata) error {
	return &ThrottlingException{
		respMetadata: v,
	}
}

// Code returns the exception type name.
func (s ThrottlingException) Code() string {
	return "ThrottlingException"
}

// Message returns the exception's message.
func (s ThrottlingException) Message() string {
	if s.Message_ != nil {
		return *s.Message_
	}
	return ""
}

// OrigErr always returns nil, satisfies awserr.Error interface.
func (s ThrottlingException) OrigErr() error {
	return nil
}

func (s ThrottlingException) Error() string {
	return fmt.Sprintf("%s: %s", s.Code(), s.Message())
}

// Status code returns the HTTP status code for the request's response error.
func (s ThrottlingException) StatusCode() int {
	return s.respMetadata.StatusCode
}

// RequestID returns the service's response RequestID for request.
func (s ThrottlingException) RequestID() string {
	return s.respMetadata.RequestID
}

// You are not authorized to perform this operation.
type UnauthorizedException struct {
	_            struct{} `type:"structure"`
	respMetadata protocol.ResponseMetadata

	// The message for the exception.
	Message_ *string `locationName:"message" type:"string"`
}

// String returns the string representation
func (s UnauthorizedException) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s UnauthorizedException) GoString() string {
	return s.String()
}

func newErrorUnauthorizedException(v protocol.ResponseMetadata) error {
	return &UnauthorizedException{
		respMetadata: v,
	}
}

// Code returns the exception type name.
func (s UnauthorizedException) Code() string {
	return "UnauthorizedException"
}

// Message returns the exception's message.
func (s UnauthorizedException) Message() string {
	if s.Message_ != nil {
		return *s.Message_
	}
	return ""
}

// OrigErr always returns nil, satisfies awserr.Error interface.
func (s UnauthorizedException) OrigErr() error {
	return nil
}

func (s UnauthorizedException) Error() string {
	return fmt.Sprintf("%s: %s", s.Code(), s.Message())
}

// Status code returns the HTTP status code for the request's response error.
func (s UnauthorizedException) StatusCode() int {
	return s.respMetadata.StatusCode
}

// RequestID returns the service's response RequestID for request.
func (s UnauthorizedException) RequestID() string {
	return s.respMetadata.RequestID
}

// The document encoding is not supported.
type UnsupportedDocumentEncodingException struct {
	_            struct{} `type:"structure"`
	respMetadata protocol.ResponseMetadata

	// The message for the exception.
	Message_ *string `locationName:"message" type:"string"`
}

// String returns the string representation
func (s UnsupportedDocumentEncodingException) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s UnsupportedDocumentEncodingException) GoString() string {
	return s.String()
}

func newErrorUnsupportedDocumentEncodingException(v protocol.ResponseMetadata) error {
	return &UnsupportedDocumentEncodingException{
		respMetadata: v,
	}
}

// Code returns the exception type name.
func (s UnsupportedDocumentEncodingException) Code() string {
	return "UnsupportedDocumentEncodingException"
}

// Message returns the exception's message.
func (s UnsupportedDocumentEncodingException) Message() string {
	if s.Message_ != nil {
		return *s.Message_
	}
	return ""
}

// OrigErr always returns nil, satisfies awserr.Error interface.
func (s UnsupportedDocumentEncodingException) OrigErr() error {
	return nil
}

func (s UnsupportedDocumentEncodingException) Error() string {
	return fmt.Sprintf("%s: %s", s.Code(), s.Message())
}

// Status code returns the HTTP status code for the request's response error.
func (s UnsupportedDocumentEncodingException) StatusCode() int {
	return s.respMetadata.StatusCode
}

// RequestID returns the service's response RequestID for request.
func (s UnsupportedDocumentEncodingException) RequestID() string {
	return s.respMetadata.RequestID
}

// The input for the UpdateThingShadow operation.
type UpdateThingShadowInput struct {
	_ struct{} `type:"structure" payload:"Payload"`

	// The state information, in JSON format.
	//
	// Payload is a required field
	Payload []byte `locationName:"payload" type:"blob" required:"true"`

	// The name of the thing.
	//
	// ThingName is a required field
	ThingName *string `location:"uri" locationName:"thingName" min:"1" type:"string" required:"true"`
}

// String returns the string representation
func (s UpdateThingShadowInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s UpdateThingShadowInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *UpdateThingShadowInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "UpdateThingShadowInput"}
	if s.Payload == nil {
		invalidParams.Add(request.NewErrParamRequired("Payload"))
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

// SetPayload sets the Payload field's value.
func (s *UpdateThingShadowInput) SetPayload(v []byte) *UpdateThingShadowInput {
	s.Payload = v
	return s
}

// SetThingName sets the ThingName field's value.
func (s *UpdateThingShadowInput) SetThingName(v string) *UpdateThingShadowInput {
	s.ThingName = &v
	return s
}

// The output from the UpdateThingShadow operation.
type UpdateThingShadowOutput struct {
	_ struct{} `type:"structure" payload:"Payload"`

	// The state information, in JSON format.
	Payload []byte `locationName:"payload" type:"blob"`
}

// String returns the string representation
func (s UpdateThingShadowOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s UpdateThingShadowOutput) GoString() string {
	return s.String()
}

// SetPayload sets the Payload field's value.
func (s *UpdateThingShadowOutput) SetPayload(v []byte) *UpdateThingShadowOutput {
	s.Payload = v
	return s
}
