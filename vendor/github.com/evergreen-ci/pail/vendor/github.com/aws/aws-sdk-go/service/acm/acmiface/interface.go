// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

// Package acmiface provides an interface to enable mocking the AWS Certificate Manager service client
// for testing your code.
//
// It is important to note that this interface will have breaking changes
// when the service model is updated and adds new API operations, paginators,
// and waiters.
package acmiface

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/acm"
)

// ACMAPI provides an interface to enable mocking the
// acm.ACM service client's API operation,
// paginators, and waiters. This make unit testing your code that calls out
// to the SDK's service client's calls easier.
//
// The best way to use this interface is so the SDK's service client's calls
// can be stubbed out for unit testing your code with the SDK without needing
// to inject custom request handlers into the SDK's request pipeline.
//
//    // myFunc uses an SDK service client to make a request to
//    // AWS Certificate Manager.
//    func myFunc(svc acmiface.ACMAPI) bool {
//        // Make svc.AddTagsToCertificate request
//    }
//
//    func main() {
//        sess := session.New()
//        svc := acm.New(sess)
//
//        myFunc(svc)
//    }
//
// In your _test.go file:
//
//    // Define a mock struct to be used in your unit tests of myFunc.
//    type mockACMClient struct {
//        acmiface.ACMAPI
//    }
//    func (m *mockACMClient) AddTagsToCertificate(input *acm.AddTagsToCertificateInput) (*acm.AddTagsToCertificateOutput, error) {
//        // mock response/functionality
//    }
//
//    func TestMyFunc(t *testing.T) {
//        // Setup Test
//        mockSvc := &mockACMClient{}
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
type ACMAPI interface {
	AddTagsToCertificate(*acm.AddTagsToCertificateInput) (*acm.AddTagsToCertificateOutput, error)
	AddTagsToCertificateWithContext(aws.Context, *acm.AddTagsToCertificateInput, ...request.Option) (*acm.AddTagsToCertificateOutput, error)
	AddTagsToCertificateRequest(*acm.AddTagsToCertificateInput) (*request.Request, *acm.AddTagsToCertificateOutput)

	DeleteCertificate(*acm.DeleteCertificateInput) (*acm.DeleteCertificateOutput, error)
	DeleteCertificateWithContext(aws.Context, *acm.DeleteCertificateInput, ...request.Option) (*acm.DeleteCertificateOutput, error)
	DeleteCertificateRequest(*acm.DeleteCertificateInput) (*request.Request, *acm.DeleteCertificateOutput)

	DescribeCertificate(*acm.DescribeCertificateInput) (*acm.DescribeCertificateOutput, error)
	DescribeCertificateWithContext(aws.Context, *acm.DescribeCertificateInput, ...request.Option) (*acm.DescribeCertificateOutput, error)
	DescribeCertificateRequest(*acm.DescribeCertificateInput) (*request.Request, *acm.DescribeCertificateOutput)

	GetCertificate(*acm.GetCertificateInput) (*acm.GetCertificateOutput, error)
	GetCertificateWithContext(aws.Context, *acm.GetCertificateInput, ...request.Option) (*acm.GetCertificateOutput, error)
	GetCertificateRequest(*acm.GetCertificateInput) (*request.Request, *acm.GetCertificateOutput)

	ImportCertificate(*acm.ImportCertificateInput) (*acm.ImportCertificateOutput, error)
	ImportCertificateWithContext(aws.Context, *acm.ImportCertificateInput, ...request.Option) (*acm.ImportCertificateOutput, error)
	ImportCertificateRequest(*acm.ImportCertificateInput) (*request.Request, *acm.ImportCertificateOutput)

	ListCertificates(*acm.ListCertificatesInput) (*acm.ListCertificatesOutput, error)
	ListCertificatesWithContext(aws.Context, *acm.ListCertificatesInput, ...request.Option) (*acm.ListCertificatesOutput, error)
	ListCertificatesRequest(*acm.ListCertificatesInput) (*request.Request, *acm.ListCertificatesOutput)

	ListCertificatesPages(*acm.ListCertificatesInput, func(*acm.ListCertificatesOutput, bool) bool) error
	ListCertificatesPagesWithContext(aws.Context, *acm.ListCertificatesInput, func(*acm.ListCertificatesOutput, bool) bool, ...request.Option) error

	ListTagsForCertificate(*acm.ListTagsForCertificateInput) (*acm.ListTagsForCertificateOutput, error)
	ListTagsForCertificateWithContext(aws.Context, *acm.ListTagsForCertificateInput, ...request.Option) (*acm.ListTagsForCertificateOutput, error)
	ListTagsForCertificateRequest(*acm.ListTagsForCertificateInput) (*request.Request, *acm.ListTagsForCertificateOutput)

	RemoveTagsFromCertificate(*acm.RemoveTagsFromCertificateInput) (*acm.RemoveTagsFromCertificateOutput, error)
	RemoveTagsFromCertificateWithContext(aws.Context, *acm.RemoveTagsFromCertificateInput, ...request.Option) (*acm.RemoveTagsFromCertificateOutput, error)
	RemoveTagsFromCertificateRequest(*acm.RemoveTagsFromCertificateInput) (*request.Request, *acm.RemoveTagsFromCertificateOutput)

	RequestCertificate(*acm.RequestCertificateInput) (*acm.RequestCertificateOutput, error)
	RequestCertificateWithContext(aws.Context, *acm.RequestCertificateInput, ...request.Option) (*acm.RequestCertificateOutput, error)
	RequestCertificateRequest(*acm.RequestCertificateInput) (*request.Request, *acm.RequestCertificateOutput)

	ResendValidationEmail(*acm.ResendValidationEmailInput) (*acm.ResendValidationEmailOutput, error)
	ResendValidationEmailWithContext(aws.Context, *acm.ResendValidationEmailInput, ...request.Option) (*acm.ResendValidationEmailOutput, error)
	ResendValidationEmailRequest(*acm.ResendValidationEmailInput) (*request.Request, *acm.ResendValidationEmailOutput)
}

var _ ACMAPI = (*acm.ACM)(nil)
