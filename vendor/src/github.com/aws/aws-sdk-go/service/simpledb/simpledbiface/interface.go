// THIS FILE IS AUTOMATICALLY GENERATED. DO NOT EDIT.

// Package simpledbiface provides an interface to enable mocking the Amazon SimpleDB service client
// for testing your code.
//
// It is important to note that this interface will have breaking changes
// when the service model is updated and adds new API operations, paginators,
// and waiters.
package simpledbiface

import (
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/simpledb"
)

// SimpleDBAPI provides an interface to enable mocking the
// simpledb.SimpleDB service client's API operation,
// paginators, and waiters. This make unit testing your code that calls out
// to the SDK's service client's calls easier.
//
// The best way to use this interface is so the SDK's service client's calls
// can be stubbed out for unit testing your code with the SDK without needing
// to inject custom request handlers into the the SDK's request pipeline.
//
//    // myFunc uses an SDK service client to make a request to
//    // Amazon SimpleDB.
//    func myFunc(svc simpledbiface.SimpleDBAPI) bool {
//        // Make svc.BatchDeleteAttributes request
//    }
//
//    func main() {
//        sess := session.New()
//        svc := simpledb.New(sess)
//
//        myFunc(svc)
//    }
//
// In your _test.go file:
//
//    // Define a mock struct to be used in your unit tests of myFunc.
//    type mockSimpleDBClient struct {
//        simpledbiface.SimpleDBAPI
//    }
//    func (m *mockSimpleDBClient) BatchDeleteAttributes(input *simpledb.BatchDeleteAttributesInput) (*simpledb.BatchDeleteAttributesOutput, error) {
//        // mock response/functionality
//    }
//
//    TestMyFunc(t *testing.T) {
//        // Setup Test
//        mockSvc := &mockSimpleDBClient{}
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
type SimpleDBAPI interface {
	BatchDeleteAttributesRequest(*simpledb.BatchDeleteAttributesInput) (*request.Request, *simpledb.BatchDeleteAttributesOutput)

	BatchDeleteAttributes(*simpledb.BatchDeleteAttributesInput) (*simpledb.BatchDeleteAttributesOutput, error)

	BatchPutAttributesRequest(*simpledb.BatchPutAttributesInput) (*request.Request, *simpledb.BatchPutAttributesOutput)

	BatchPutAttributes(*simpledb.BatchPutAttributesInput) (*simpledb.BatchPutAttributesOutput, error)

	CreateDomainRequest(*simpledb.CreateDomainInput) (*request.Request, *simpledb.CreateDomainOutput)

	CreateDomain(*simpledb.CreateDomainInput) (*simpledb.CreateDomainOutput, error)

	DeleteAttributesRequest(*simpledb.DeleteAttributesInput) (*request.Request, *simpledb.DeleteAttributesOutput)

	DeleteAttributes(*simpledb.DeleteAttributesInput) (*simpledb.DeleteAttributesOutput, error)

	DeleteDomainRequest(*simpledb.DeleteDomainInput) (*request.Request, *simpledb.DeleteDomainOutput)

	DeleteDomain(*simpledb.DeleteDomainInput) (*simpledb.DeleteDomainOutput, error)

	DomainMetadataRequest(*simpledb.DomainMetadataInput) (*request.Request, *simpledb.DomainMetadataOutput)

	DomainMetadata(*simpledb.DomainMetadataInput) (*simpledb.DomainMetadataOutput, error)

	GetAttributesRequest(*simpledb.GetAttributesInput) (*request.Request, *simpledb.GetAttributesOutput)

	GetAttributes(*simpledb.GetAttributesInput) (*simpledb.GetAttributesOutput, error)

	ListDomainsRequest(*simpledb.ListDomainsInput) (*request.Request, *simpledb.ListDomainsOutput)

	ListDomains(*simpledb.ListDomainsInput) (*simpledb.ListDomainsOutput, error)

	ListDomainsPages(*simpledb.ListDomainsInput, func(*simpledb.ListDomainsOutput, bool) bool) error

	PutAttributesRequest(*simpledb.PutAttributesInput) (*request.Request, *simpledb.PutAttributesOutput)

	PutAttributes(*simpledb.PutAttributesInput) (*simpledb.PutAttributesOutput, error)

	SelectRequest(*simpledb.SelectInput) (*request.Request, *simpledb.SelectOutput)

	Select(*simpledb.SelectInput) (*simpledb.SelectOutput, error)

	SelectPages(*simpledb.SelectInput, func(*simpledb.SelectOutput, bool) bool) error
}

var _ SimpleDBAPI = (*simpledb.SimpleDB)(nil)
