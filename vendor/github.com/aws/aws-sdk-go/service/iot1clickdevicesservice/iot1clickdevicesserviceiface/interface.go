// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

// Package iot1clickdevicesserviceiface provides an interface to enable mocking the AWS IoT 1-Click Devices Service service client
// for testing your code.
//
// It is important to note that this interface will have breaking changes
// when the service model is updated and adds new API operations, paginators,
// and waiters.
package iot1clickdevicesserviceiface

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/iot1clickdevicesservice"
)

// IoT1ClickDevicesServiceAPI provides an interface to enable mocking the
// iot1clickdevicesservice.IoT1ClickDevicesService service client's API operation,
// paginators, and waiters. This make unit testing your code that calls out
// to the SDK's service client's calls easier.
//
// The best way to use this interface is so the SDK's service client's calls
// can be stubbed out for unit testing your code with the SDK without needing
// to inject custom request handlers into the SDK's request pipeline.
//
//    // myFunc uses an SDK service client to make a request to
//    // AWS IoT 1-Click Devices Service.
//    func myFunc(svc iot1clickdevicesserviceiface.IoT1ClickDevicesServiceAPI) bool {
//        // Make svc.ClaimDevicesByClaimCode request
//    }
//
//    func main() {
//        sess := session.New()
//        svc := iot1clickdevicesservice.New(sess)
//
//        myFunc(svc)
//    }
//
// In your _test.go file:
//
//    // Define a mock struct to be used in your unit tests of myFunc.
//    type mockIoT1ClickDevicesServiceClient struct {
//        iot1clickdevicesserviceiface.IoT1ClickDevicesServiceAPI
//    }
//    func (m *mockIoT1ClickDevicesServiceClient) ClaimDevicesByClaimCode(input *iot1clickdevicesservice.ClaimDevicesByClaimCodeInput) (*iot1clickdevicesservice.ClaimDevicesByClaimCodeOutput, error) {
//        // mock response/functionality
//    }
//
//    func TestMyFunc(t *testing.T) {
//        // Setup Test
//        mockSvc := &mockIoT1ClickDevicesServiceClient{}
//
//        myfunc(mockSvc)
//
//        // Verify myFunc's functionality
//    }
//
// It is important to note that this interface will have breaking changes
// when the service model is updated and adds new API operations, paginators,
// and waiters. Its suggested to use the pattern above for testing, or using
// tooling to generate mocks to satisfy the interfaces.
type IoT1ClickDevicesServiceAPI interface {
	ClaimDevicesByClaimCode(*iot1clickdevicesservice.ClaimDevicesByClaimCodeInput) (*iot1clickdevicesservice.ClaimDevicesByClaimCodeOutput, error)
	ClaimDevicesByClaimCodeWithContext(aws.Context, *iot1clickdevicesservice.ClaimDevicesByClaimCodeInput, ...request.Option) (*iot1clickdevicesservice.ClaimDevicesByClaimCodeOutput, error)
	ClaimDevicesByClaimCodeRequest(*iot1clickdevicesservice.ClaimDevicesByClaimCodeInput) (*request.Request, *iot1clickdevicesservice.ClaimDevicesByClaimCodeOutput)

	DescribeDevice(*iot1clickdevicesservice.DescribeDeviceInput) (*iot1clickdevicesservice.DescribeDeviceOutput, error)
	DescribeDeviceWithContext(aws.Context, *iot1clickdevicesservice.DescribeDeviceInput, ...request.Option) (*iot1clickdevicesservice.DescribeDeviceOutput, error)
	DescribeDeviceRequest(*iot1clickdevicesservice.DescribeDeviceInput) (*request.Request, *iot1clickdevicesservice.DescribeDeviceOutput)

	FinalizeDeviceClaim(*iot1clickdevicesservice.FinalizeDeviceClaimInput) (*iot1clickdevicesservice.FinalizeDeviceClaimOutput, error)
	FinalizeDeviceClaimWithContext(aws.Context, *iot1clickdevicesservice.FinalizeDeviceClaimInput, ...request.Option) (*iot1clickdevicesservice.FinalizeDeviceClaimOutput, error)
	FinalizeDeviceClaimRequest(*iot1clickdevicesservice.FinalizeDeviceClaimInput) (*request.Request, *iot1clickdevicesservice.FinalizeDeviceClaimOutput)

	GetDeviceMethods(*iot1clickdevicesservice.GetDeviceMethodsInput) (*iot1clickdevicesservice.GetDeviceMethodsOutput, error)
	GetDeviceMethodsWithContext(aws.Context, *iot1clickdevicesservice.GetDeviceMethodsInput, ...request.Option) (*iot1clickdevicesservice.GetDeviceMethodsOutput, error)
	GetDeviceMethodsRequest(*iot1clickdevicesservice.GetDeviceMethodsInput) (*request.Request, *iot1clickdevicesservice.GetDeviceMethodsOutput)

	InitiateDeviceClaim(*iot1clickdevicesservice.InitiateDeviceClaimInput) (*iot1clickdevicesservice.InitiateDeviceClaimOutput, error)
	InitiateDeviceClaimWithContext(aws.Context, *iot1clickdevicesservice.InitiateDeviceClaimInput, ...request.Option) (*iot1clickdevicesservice.InitiateDeviceClaimOutput, error)
	InitiateDeviceClaimRequest(*iot1clickdevicesservice.InitiateDeviceClaimInput) (*request.Request, *iot1clickdevicesservice.InitiateDeviceClaimOutput)

	InvokeDeviceMethod(*iot1clickdevicesservice.InvokeDeviceMethodInput) (*iot1clickdevicesservice.InvokeDeviceMethodOutput, error)
	InvokeDeviceMethodWithContext(aws.Context, *iot1clickdevicesservice.InvokeDeviceMethodInput, ...request.Option) (*iot1clickdevicesservice.InvokeDeviceMethodOutput, error)
	InvokeDeviceMethodRequest(*iot1clickdevicesservice.InvokeDeviceMethodInput) (*request.Request, *iot1clickdevicesservice.InvokeDeviceMethodOutput)

	ListDeviceEvents(*iot1clickdevicesservice.ListDeviceEventsInput) (*iot1clickdevicesservice.ListDeviceEventsOutput, error)
	ListDeviceEventsWithContext(aws.Context, *iot1clickdevicesservice.ListDeviceEventsInput, ...request.Option) (*iot1clickdevicesservice.ListDeviceEventsOutput, error)
	ListDeviceEventsRequest(*iot1clickdevicesservice.ListDeviceEventsInput) (*request.Request, *iot1clickdevicesservice.ListDeviceEventsOutput)

	ListDevices(*iot1clickdevicesservice.ListDevicesInput) (*iot1clickdevicesservice.ListDevicesOutput, error)
	ListDevicesWithContext(aws.Context, *iot1clickdevicesservice.ListDevicesInput, ...request.Option) (*iot1clickdevicesservice.ListDevicesOutput, error)
	ListDevicesRequest(*iot1clickdevicesservice.ListDevicesInput) (*request.Request, *iot1clickdevicesservice.ListDevicesOutput)

	ListTagsForResource(*iot1clickdevicesservice.ListTagsForResourceInput) (*iot1clickdevicesservice.ListTagsForResourceOutput, error)
	ListTagsForResourceWithContext(aws.Context, *iot1clickdevicesservice.ListTagsForResourceInput, ...request.Option) (*iot1clickdevicesservice.ListTagsForResourceOutput, error)
	ListTagsForResourceRequest(*iot1clickdevicesservice.ListTagsForResourceInput) (*request.Request, *iot1clickdevicesservice.ListTagsForResourceOutput)

	TagResource(*iot1clickdevicesservice.TagResourceInput) (*iot1clickdevicesservice.TagResourceOutput, error)
	TagResourceWithContext(aws.Context, *iot1clickdevicesservice.TagResourceInput, ...request.Option) (*iot1clickdevicesservice.TagResourceOutput, error)
	TagResourceRequest(*iot1clickdevicesservice.TagResourceInput) (*request.Request, *iot1clickdevicesservice.TagResourceOutput)

	UnclaimDevice(*iot1clickdevicesservice.UnclaimDeviceInput) (*iot1clickdevicesservice.UnclaimDeviceOutput, error)
	UnclaimDeviceWithContext(aws.Context, *iot1clickdevicesservice.UnclaimDeviceInput, ...request.Option) (*iot1clickdevicesservice.UnclaimDeviceOutput, error)
	UnclaimDeviceRequest(*iot1clickdevicesservice.UnclaimDeviceInput) (*request.Request, *iot1clickdevicesservice.UnclaimDeviceOutput)

	UntagResource(*iot1clickdevicesservice.UntagResourceInput) (*iot1clickdevicesservice.UntagResourceOutput, error)
	UntagResourceWithContext(aws.Context, *iot1clickdevicesservice.UntagResourceInput, ...request.Option) (*iot1clickdevicesservice.UntagResourceOutput, error)
	UntagResourceRequest(*iot1clickdevicesservice.UntagResourceInput) (*request.Request, *iot1clickdevicesservice.UntagResourceOutput)

	UpdateDeviceState(*iot1clickdevicesservice.UpdateDeviceStateInput) (*iot1clickdevicesservice.UpdateDeviceStateOutput, error)
	UpdateDeviceStateWithContext(aws.Context, *iot1clickdevicesservice.UpdateDeviceStateInput, ...request.Option) (*iot1clickdevicesservice.UpdateDeviceStateOutput, error)
	UpdateDeviceStateRequest(*iot1clickdevicesservice.UpdateDeviceStateInput) (*request.Request, *iot1clickdevicesservice.UpdateDeviceStateOutput)
}

var _ IoT1ClickDevicesServiceAPI = (*iot1clickdevicesservice.IoT1ClickDevicesService)(nil)
