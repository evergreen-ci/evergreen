// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

package sagemakeredgemanager

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/private/protocol"
	"github.com/aws/aws-sdk-go/private/protocol/restjson"
)

const opGetDeviceRegistration = "GetDeviceRegistration"

// GetDeviceRegistrationRequest generates a "aws/request.Request" representing the
// client's request for the GetDeviceRegistration operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See GetDeviceRegistration for more information on using the GetDeviceRegistration
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the GetDeviceRegistrationRequest method.
//    req, resp := client.GetDeviceRegistrationRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
//
// See also, https://docs.aws.amazon.com/goto/WebAPI/sagemaker-edge-2020-09-23/GetDeviceRegistration
func (c *SagemakerEdgeManager) GetDeviceRegistrationRequest(input *GetDeviceRegistrationInput) (req *request.Request, output *GetDeviceRegistrationOutput) {
	op := &request.Operation{
		Name:       opGetDeviceRegistration,
		HTTPMethod: "POST",
		HTTPPath:   "/GetDeviceRegistration",
	}

	if input == nil {
		input = &GetDeviceRegistrationInput{}
	}

	output = &GetDeviceRegistrationOutput{}
	req = c.newRequest(op, input, output)
	return
}

// GetDeviceRegistration API operation for Amazon Sagemaker Edge Manager.
//
// Use to check if a device is registered with SageMaker Edge Manager.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Sagemaker Edge Manager's
// API operation GetDeviceRegistration for usage and error information.
//
// Returned Error Types:
//   * InternalServiceException
//   An internal failure occurred. Try your request again. If the problem persists,
//   contact AWS customer support.
//
// See also, https://docs.aws.amazon.com/goto/WebAPI/sagemaker-edge-2020-09-23/GetDeviceRegistration
func (c *SagemakerEdgeManager) GetDeviceRegistration(input *GetDeviceRegistrationInput) (*GetDeviceRegistrationOutput, error) {
	req, out := c.GetDeviceRegistrationRequest(input)
	return out, req.Send()
}

// GetDeviceRegistrationWithContext is the same as GetDeviceRegistration with the addition of
// the ability to pass a context and additional request options.
//
// See GetDeviceRegistration for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *SagemakerEdgeManager) GetDeviceRegistrationWithContext(ctx aws.Context, input *GetDeviceRegistrationInput, opts ...request.Option) (*GetDeviceRegistrationOutput, error) {
	req, out := c.GetDeviceRegistrationRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

const opSendHeartbeat = "SendHeartbeat"

// SendHeartbeatRequest generates a "aws/request.Request" representing the
// client's request for the SendHeartbeat operation. The "output" return
// value will be populated with the request's response once the request completes
// successfully.
//
// Use "Send" method on the returned Request to send the API call to the service.
// the "output" return value is not valid until after Send returns without error.
//
// See SendHeartbeat for more information on using the SendHeartbeat
// API call, and error handling.
//
// This method is useful when you want to inject custom logic or configuration
// into the SDK's request lifecycle. Such as custom headers, or retry logic.
//
//
//    // Example sending a request using the SendHeartbeatRequest method.
//    req, resp := client.SendHeartbeatRequest(params)
//
//    err := req.Send()
//    if err == nil { // resp is now filled
//        fmt.Println(resp)
//    }
//
// See also, https://docs.aws.amazon.com/goto/WebAPI/sagemaker-edge-2020-09-23/SendHeartbeat
func (c *SagemakerEdgeManager) SendHeartbeatRequest(input *SendHeartbeatInput) (req *request.Request, output *SendHeartbeatOutput) {
	op := &request.Operation{
		Name:       opSendHeartbeat,
		HTTPMethod: "POST",
		HTTPPath:   "/SendHeartbeat",
	}

	if input == nil {
		input = &SendHeartbeatInput{}
	}

	output = &SendHeartbeatOutput{}
	req = c.newRequest(op, input, output)
	req.Handlers.Unmarshal.Swap(restjson.UnmarshalHandler.Name, protocol.UnmarshalDiscardBodyHandler)
	return
}

// SendHeartbeat API operation for Amazon Sagemaker Edge Manager.
//
// Use to get the current status of devices registered on SageMaker Edge Manager.
//
// Returns awserr.Error for service API and SDK errors. Use runtime type assertions
// with awserr.Error's Code and Message methods to get detailed information about
// the error.
//
// See the AWS API reference guide for Amazon Sagemaker Edge Manager's
// API operation SendHeartbeat for usage and error information.
//
// Returned Error Types:
//   * InternalServiceException
//   An internal failure occurred. Try your request again. If the problem persists,
//   contact AWS customer support.
//
// See also, https://docs.aws.amazon.com/goto/WebAPI/sagemaker-edge-2020-09-23/SendHeartbeat
func (c *SagemakerEdgeManager) SendHeartbeat(input *SendHeartbeatInput) (*SendHeartbeatOutput, error) {
	req, out := c.SendHeartbeatRequest(input)
	return out, req.Send()
}

// SendHeartbeatWithContext is the same as SendHeartbeat with the addition of
// the ability to pass a context and additional request options.
//
// See SendHeartbeat for details on how to use this API operation.
//
// The context must be non-nil and will be used for request cancellation. If
// the context is nil a panic will occur. In the future the SDK may create
// sub-contexts for http.Requests. See https://golang.org/pkg/context/
// for more information on using Contexts.
func (c *SagemakerEdgeManager) SendHeartbeatWithContext(ctx aws.Context, input *SendHeartbeatInput, opts ...request.Option) (*SendHeartbeatOutput, error) {
	req, out := c.SendHeartbeatRequest(input)
	req.SetContext(ctx)
	req.ApplyOptions(opts...)
	return out, req.Send()
}

// Information required for edge device metrics.
type EdgeMetric struct {
	_ struct{} `type:"structure"`

	// The dimension of metrics published.
	Dimension *string `min:"1" type:"string"`

	// Returns the name of the metric.
	MetricName *string `min:"4" type:"string"`

	// Timestamp of when the metric was requested.
	Timestamp *time.Time `type:"timestamp"`

	// Returns the value of the metric.
	Value *float64 `type:"double"`
}

// String returns the string representation
func (s EdgeMetric) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s EdgeMetric) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *EdgeMetric) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "EdgeMetric"}
	if s.Dimension != nil && len(*s.Dimension) < 1 {
		invalidParams.Add(request.NewErrParamMinLen("Dimension", 1))
	}
	if s.MetricName != nil && len(*s.MetricName) < 4 {
		invalidParams.Add(request.NewErrParamMinLen("MetricName", 4))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetDimension sets the Dimension field's value.
func (s *EdgeMetric) SetDimension(v string) *EdgeMetric {
	s.Dimension = &v
	return s
}

// SetMetricName sets the MetricName field's value.
func (s *EdgeMetric) SetMetricName(v string) *EdgeMetric {
	s.MetricName = &v
	return s
}

// SetTimestamp sets the Timestamp field's value.
func (s *EdgeMetric) SetTimestamp(v time.Time) *EdgeMetric {
	s.Timestamp = &v
	return s
}

// SetValue sets the Value field's value.
func (s *EdgeMetric) SetValue(v float64) *EdgeMetric {
	s.Value = &v
	return s
}

type GetDeviceRegistrationInput struct {
	_ struct{} `type:"structure"`

	// The name of the fleet that the device belongs to.
	//
	// DeviceFleetName is a required field
	DeviceFleetName *string `min:"1" type:"string" required:"true"`

	// The unique name of the device you want to get the registration status from.
	//
	// DeviceName is a required field
	DeviceName *string `min:"1" type:"string" required:"true"`
}

// String returns the string representation
func (s GetDeviceRegistrationInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s GetDeviceRegistrationInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *GetDeviceRegistrationInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "GetDeviceRegistrationInput"}
	if s.DeviceFleetName == nil {
		invalidParams.Add(request.NewErrParamRequired("DeviceFleetName"))
	}
	if s.DeviceFleetName != nil && len(*s.DeviceFleetName) < 1 {
		invalidParams.Add(request.NewErrParamMinLen("DeviceFleetName", 1))
	}
	if s.DeviceName == nil {
		invalidParams.Add(request.NewErrParamRequired("DeviceName"))
	}
	if s.DeviceName != nil && len(*s.DeviceName) < 1 {
		invalidParams.Add(request.NewErrParamMinLen("DeviceName", 1))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetDeviceFleetName sets the DeviceFleetName field's value.
func (s *GetDeviceRegistrationInput) SetDeviceFleetName(v string) *GetDeviceRegistrationInput {
	s.DeviceFleetName = &v
	return s
}

// SetDeviceName sets the DeviceName field's value.
func (s *GetDeviceRegistrationInput) SetDeviceName(v string) *GetDeviceRegistrationInput {
	s.DeviceName = &v
	return s
}

type GetDeviceRegistrationOutput struct {
	_ struct{} `type:"structure"`

	// The amount of time, in seconds, that the registration status is stored on
	// the device’s cache before it is refreshed.
	CacheTTL *string `min:"1" type:"string"`

	// Describes if the device is currently registered with SageMaker Edge Manager.
	DeviceRegistration *string `min:"1" type:"string"`
}

// String returns the string representation
func (s GetDeviceRegistrationOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s GetDeviceRegistrationOutput) GoString() string {
	return s.String()
}

// SetCacheTTL sets the CacheTTL field's value.
func (s *GetDeviceRegistrationOutput) SetCacheTTL(v string) *GetDeviceRegistrationOutput {
	s.CacheTTL = &v
	return s
}

// SetDeviceRegistration sets the DeviceRegistration field's value.
func (s *GetDeviceRegistrationOutput) SetDeviceRegistration(v string) *GetDeviceRegistrationOutput {
	s.DeviceRegistration = &v
	return s
}

// An internal failure occurred. Try your request again. If the problem persists,
// contact AWS customer support.
type InternalServiceException struct {
	_            struct{}                  `type:"structure"`
	RespMetadata protocol.ResponseMetadata `json:"-" xml:"-"`

	Message_ *string `locationName:"Message" type:"string"`
}

// String returns the string representation
func (s InternalServiceException) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s InternalServiceException) GoString() string {
	return s.String()
}

func newErrorInternalServiceException(v protocol.ResponseMetadata) error {
	return &InternalServiceException{
		RespMetadata: v,
	}
}

// Code returns the exception type name.
func (s *InternalServiceException) Code() string {
	return "InternalServiceException"
}

// Message returns the exception's message.
func (s *InternalServiceException) Message() string {
	if s.Message_ != nil {
		return *s.Message_
	}
	return ""
}

// OrigErr always returns nil, satisfies awserr.Error interface.
func (s *InternalServiceException) OrigErr() error {
	return nil
}

func (s *InternalServiceException) Error() string {
	return fmt.Sprintf("%s: %s", s.Code(), s.Message())
}

// Status code returns the HTTP status code for the request's response error.
func (s *InternalServiceException) StatusCode() int {
	return s.RespMetadata.StatusCode
}

// RequestID returns the service's response RequestID for request.
func (s *InternalServiceException) RequestID() string {
	return s.RespMetadata.RequestID
}

// Information about a model deployed on an edge device that is registered with
// SageMaker Edge Manager.
type Model struct {
	_ struct{} `type:"structure"`

	// The timestamp of the last inference that was made.
	LatestInference *time.Time `type:"timestamp"`

	// The timestamp of the last data sample taken.
	LatestSampleTime *time.Time `type:"timestamp"`

	// Information required for model metrics.
	ModelMetrics []*EdgeMetric `type:"list"`

	// The name of the model.
	ModelName *string `min:"4" type:"string"`

	// The version of the model.
	ModelVersion *string `min:"1" type:"string"`
}

// String returns the string representation
func (s Model) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s Model) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *Model) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "Model"}
	if s.ModelName != nil && len(*s.ModelName) < 4 {
		invalidParams.Add(request.NewErrParamMinLen("ModelName", 4))
	}
	if s.ModelVersion != nil && len(*s.ModelVersion) < 1 {
		invalidParams.Add(request.NewErrParamMinLen("ModelVersion", 1))
	}
	if s.ModelMetrics != nil {
		for i, v := range s.ModelMetrics {
			if v == nil {
				continue
			}
			if err := v.Validate(); err != nil {
				invalidParams.AddNested(fmt.Sprintf("%s[%v]", "ModelMetrics", i), err.(request.ErrInvalidParams))
			}
		}
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetLatestInference sets the LatestInference field's value.
func (s *Model) SetLatestInference(v time.Time) *Model {
	s.LatestInference = &v
	return s
}

// SetLatestSampleTime sets the LatestSampleTime field's value.
func (s *Model) SetLatestSampleTime(v time.Time) *Model {
	s.LatestSampleTime = &v
	return s
}

// SetModelMetrics sets the ModelMetrics field's value.
func (s *Model) SetModelMetrics(v []*EdgeMetric) *Model {
	s.ModelMetrics = v
	return s
}

// SetModelName sets the ModelName field's value.
func (s *Model) SetModelName(v string) *Model {
	s.ModelName = &v
	return s
}

// SetModelVersion sets the ModelVersion field's value.
func (s *Model) SetModelVersion(v string) *Model {
	s.ModelVersion = &v
	return s
}

type SendHeartbeatInput struct {
	_ struct{} `type:"structure"`

	// For internal use. Returns a list of SageMaker Edge Manager agent operating
	// metrics.
	AgentMetrics []*EdgeMetric `type:"list"`

	// Returns the version of the agent.
	//
	// AgentVersion is a required field
	AgentVersion *string `min:"1" type:"string" required:"true"`

	// The name of the fleet that the device belongs to.
	//
	// DeviceFleetName is a required field
	DeviceFleetName *string `min:"1" type:"string" required:"true"`

	// The unique name of the device.
	//
	// DeviceName is a required field
	DeviceName *string `min:"1" type:"string" required:"true"`

	// Returns a list of models deployed on the the device.
	Models []*Model `type:"list"`
}

// String returns the string representation
func (s SendHeartbeatInput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s SendHeartbeatInput) GoString() string {
	return s.String()
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *SendHeartbeatInput) Validate() error {
	invalidParams := request.ErrInvalidParams{Context: "SendHeartbeatInput"}
	if s.AgentVersion == nil {
		invalidParams.Add(request.NewErrParamRequired("AgentVersion"))
	}
	if s.AgentVersion != nil && len(*s.AgentVersion) < 1 {
		invalidParams.Add(request.NewErrParamMinLen("AgentVersion", 1))
	}
	if s.DeviceFleetName == nil {
		invalidParams.Add(request.NewErrParamRequired("DeviceFleetName"))
	}
	if s.DeviceFleetName != nil && len(*s.DeviceFleetName) < 1 {
		invalidParams.Add(request.NewErrParamMinLen("DeviceFleetName", 1))
	}
	if s.DeviceName == nil {
		invalidParams.Add(request.NewErrParamRequired("DeviceName"))
	}
	if s.DeviceName != nil && len(*s.DeviceName) < 1 {
		invalidParams.Add(request.NewErrParamMinLen("DeviceName", 1))
	}
	if s.AgentMetrics != nil {
		for i, v := range s.AgentMetrics {
			if v == nil {
				continue
			}
			if err := v.Validate(); err != nil {
				invalidParams.AddNested(fmt.Sprintf("%s[%v]", "AgentMetrics", i), err.(request.ErrInvalidParams))
			}
		}
	}
	if s.Models != nil {
		for i, v := range s.Models {
			if v == nil {
				continue
			}
			if err := v.Validate(); err != nil {
				invalidParams.AddNested(fmt.Sprintf("%s[%v]", "Models", i), err.(request.ErrInvalidParams))
			}
		}
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// SetAgentMetrics sets the AgentMetrics field's value.
func (s *SendHeartbeatInput) SetAgentMetrics(v []*EdgeMetric) *SendHeartbeatInput {
	s.AgentMetrics = v
	return s
}

// SetAgentVersion sets the AgentVersion field's value.
func (s *SendHeartbeatInput) SetAgentVersion(v string) *SendHeartbeatInput {
	s.AgentVersion = &v
	return s
}

// SetDeviceFleetName sets the DeviceFleetName field's value.
func (s *SendHeartbeatInput) SetDeviceFleetName(v string) *SendHeartbeatInput {
	s.DeviceFleetName = &v
	return s
}

// SetDeviceName sets the DeviceName field's value.
func (s *SendHeartbeatInput) SetDeviceName(v string) *SendHeartbeatInput {
	s.DeviceName = &v
	return s
}

// SetModels sets the Models field's value.
func (s *SendHeartbeatInput) SetModels(v []*Model) *SendHeartbeatInput {
	s.Models = v
	return s
}

type SendHeartbeatOutput struct {
	_ struct{} `type:"structure"`
}

// String returns the string representation
func (s SendHeartbeatOutput) String() string {
	return awsutil.Prettify(s)
}

// GoString returns the string representation
func (s SendHeartbeatOutput) GoString() string {
	return s.String()
}
