// THIS FILE IS AUTOMATICALLY GENERATED. DO NOT EDIT.

// Package applicationdiscoveryserviceiface provides an interface to enable mocking the AWS Application Discovery Service service client
// for testing your code.
//
// It is important to note that this interface will have breaking changes
// when the service model is updated and adds new API operations, paginators,
// and waiters.
package applicationdiscoveryserviceiface

import (
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/applicationdiscoveryservice"
)

// ApplicationDiscoveryServiceAPI provides an interface to enable mocking the
// applicationdiscoveryservice.ApplicationDiscoveryService service client's API operation,
// paginators, and waiters. This make unit testing your code that calls out
// to the SDK's service client's calls easier.
//
// The best way to use this interface is so the SDK's service client's calls
// can be stubbed out for unit testing your code with the SDK without needing
// to inject custom request handlers into the the SDK's request pipeline.
//
//    // myFunc uses an SDK service client to make a request to
//    // AWS Application Discovery Service.
//    func myFunc(svc applicationdiscoveryserviceiface.ApplicationDiscoveryServiceAPI) bool {
//        // Make svc.CreateTags request
//    }
//
//    func main() {
//        sess := session.New()
//        svc := applicationdiscoveryservice.New(sess)
//
//        myFunc(svc)
//    }
//
// In your _test.go file:
//
//    // Define a mock struct to be used in your unit tests of myFunc.
//    type mockApplicationDiscoveryServiceClient struct {
//        applicationdiscoveryserviceiface.ApplicationDiscoveryServiceAPI
//    }
//    func (m *mockApplicationDiscoveryServiceClient) CreateTags(input *applicationdiscoveryservice.CreateTagsInput) (*applicationdiscoveryservice.CreateTagsOutput, error) {
//        // mock response/functionality
//    }
//
//    TestMyFunc(t *testing.T) {
//        // Setup Test
//        mockSvc := &mockApplicationDiscoveryServiceClient{}
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
type ApplicationDiscoveryServiceAPI interface {
	CreateTagsRequest(*applicationdiscoveryservice.CreateTagsInput) (*request.Request, *applicationdiscoveryservice.CreateTagsOutput)

	CreateTags(*applicationdiscoveryservice.CreateTagsInput) (*applicationdiscoveryservice.CreateTagsOutput, error)

	DeleteTagsRequest(*applicationdiscoveryservice.DeleteTagsInput) (*request.Request, *applicationdiscoveryservice.DeleteTagsOutput)

	DeleteTags(*applicationdiscoveryservice.DeleteTagsInput) (*applicationdiscoveryservice.DeleteTagsOutput, error)

	DescribeAgentsRequest(*applicationdiscoveryservice.DescribeAgentsInput) (*request.Request, *applicationdiscoveryservice.DescribeAgentsOutput)

	DescribeAgents(*applicationdiscoveryservice.DescribeAgentsInput) (*applicationdiscoveryservice.DescribeAgentsOutput, error)

	DescribeConfigurationsRequest(*applicationdiscoveryservice.DescribeConfigurationsInput) (*request.Request, *applicationdiscoveryservice.DescribeConfigurationsOutput)

	DescribeConfigurations(*applicationdiscoveryservice.DescribeConfigurationsInput) (*applicationdiscoveryservice.DescribeConfigurationsOutput, error)

	DescribeExportConfigurationsRequest(*applicationdiscoveryservice.DescribeExportConfigurationsInput) (*request.Request, *applicationdiscoveryservice.DescribeExportConfigurationsOutput)

	DescribeExportConfigurations(*applicationdiscoveryservice.DescribeExportConfigurationsInput) (*applicationdiscoveryservice.DescribeExportConfigurationsOutput, error)

	DescribeTagsRequest(*applicationdiscoveryservice.DescribeTagsInput) (*request.Request, *applicationdiscoveryservice.DescribeTagsOutput)

	DescribeTags(*applicationdiscoveryservice.DescribeTagsInput) (*applicationdiscoveryservice.DescribeTagsOutput, error)

	ExportConfigurationsRequest(*applicationdiscoveryservice.ExportConfigurationsInput) (*request.Request, *applicationdiscoveryservice.ExportConfigurationsOutput)

	ExportConfigurations(*applicationdiscoveryservice.ExportConfigurationsInput) (*applicationdiscoveryservice.ExportConfigurationsOutput, error)

	ListConfigurationsRequest(*applicationdiscoveryservice.ListConfigurationsInput) (*request.Request, *applicationdiscoveryservice.ListConfigurationsOutput)

	ListConfigurations(*applicationdiscoveryservice.ListConfigurationsInput) (*applicationdiscoveryservice.ListConfigurationsOutput, error)

	StartDataCollectionByAgentIdsRequest(*applicationdiscoveryservice.StartDataCollectionByAgentIdsInput) (*request.Request, *applicationdiscoveryservice.StartDataCollectionByAgentIdsOutput)

	StartDataCollectionByAgentIds(*applicationdiscoveryservice.StartDataCollectionByAgentIdsInput) (*applicationdiscoveryservice.StartDataCollectionByAgentIdsOutput, error)

	StopDataCollectionByAgentIdsRequest(*applicationdiscoveryservice.StopDataCollectionByAgentIdsInput) (*request.Request, *applicationdiscoveryservice.StopDataCollectionByAgentIdsOutput)

	StopDataCollectionByAgentIds(*applicationdiscoveryservice.StopDataCollectionByAgentIdsInput) (*applicationdiscoveryservice.StopDataCollectionByAgentIdsOutput, error)
}

var _ ApplicationDiscoveryServiceAPI = (*applicationdiscoveryservice.ApplicationDiscoveryService)(nil)
