// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

// Package s3controliface provides an interface to enable mocking the AWS S3 Control service client
// for testing your code.
//
// It is important to note that this interface will have breaking changes
// when the service model is updated and adds new API operations, paginators,
// and waiters.
package s3controliface

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/s3control"
)

// S3ControlAPI provides an interface to enable mocking the
// s3control.S3Control service client's API operation,
// paginators, and waiters. This make unit testing your code that calls out
// to the SDK's service client's calls easier.
//
// The best way to use this interface is so the SDK's service client's calls
// can be stubbed out for unit testing your code with the SDK without needing
// to inject custom request handlers into the SDK's request pipeline.
//
//    // myFunc uses an SDK service client to make a request to
//    // AWS S3 Control.
//    func myFunc(svc s3controliface.S3ControlAPI) bool {
//        // Make svc.CreateJob request
//    }
//
//    func main() {
//        sess := session.New()
//        svc := s3control.New(sess)
//
//        myFunc(svc)
//    }
//
// In your _test.go file:
//
//    // Define a mock struct to be used in your unit tests of myFunc.
//    type mockS3ControlClient struct {
//        s3controliface.S3ControlAPI
//    }
//    func (m *mockS3ControlClient) CreateJob(input *s3control.CreateJobInput) (*s3control.CreateJobOutput, error) {
//        // mock response/functionality
//    }
//
//    func TestMyFunc(t *testing.T) {
//        // Setup Test
//        mockSvc := &mockS3ControlClient{}
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
type S3ControlAPI interface {
	CreateJob(*s3control.CreateJobInput) (*s3control.CreateJobOutput, error)
	CreateJobWithContext(aws.Context, *s3control.CreateJobInput, ...request.Option) (*s3control.CreateJobOutput, error)
	CreateJobRequest(*s3control.CreateJobInput) (*request.Request, *s3control.CreateJobOutput)

	DeletePublicAccessBlock(*s3control.DeletePublicAccessBlockInput) (*s3control.DeletePublicAccessBlockOutput, error)
	DeletePublicAccessBlockWithContext(aws.Context, *s3control.DeletePublicAccessBlockInput, ...request.Option) (*s3control.DeletePublicAccessBlockOutput, error)
	DeletePublicAccessBlockRequest(*s3control.DeletePublicAccessBlockInput) (*request.Request, *s3control.DeletePublicAccessBlockOutput)

	DescribeJob(*s3control.DescribeJobInput) (*s3control.DescribeJobOutput, error)
	DescribeJobWithContext(aws.Context, *s3control.DescribeJobInput, ...request.Option) (*s3control.DescribeJobOutput, error)
	DescribeJobRequest(*s3control.DescribeJobInput) (*request.Request, *s3control.DescribeJobOutput)

	GetPublicAccessBlock(*s3control.GetPublicAccessBlockInput) (*s3control.GetPublicAccessBlockOutput, error)
	GetPublicAccessBlockWithContext(aws.Context, *s3control.GetPublicAccessBlockInput, ...request.Option) (*s3control.GetPublicAccessBlockOutput, error)
	GetPublicAccessBlockRequest(*s3control.GetPublicAccessBlockInput) (*request.Request, *s3control.GetPublicAccessBlockOutput)

	ListJobs(*s3control.ListJobsInput) (*s3control.ListJobsOutput, error)
	ListJobsWithContext(aws.Context, *s3control.ListJobsInput, ...request.Option) (*s3control.ListJobsOutput, error)
	ListJobsRequest(*s3control.ListJobsInput) (*request.Request, *s3control.ListJobsOutput)

	ListJobsPages(*s3control.ListJobsInput, func(*s3control.ListJobsOutput, bool) bool) error
	ListJobsPagesWithContext(aws.Context, *s3control.ListJobsInput, func(*s3control.ListJobsOutput, bool) bool, ...request.Option) error

	PutPublicAccessBlock(*s3control.PutPublicAccessBlockInput) (*s3control.PutPublicAccessBlockOutput, error)
	PutPublicAccessBlockWithContext(aws.Context, *s3control.PutPublicAccessBlockInput, ...request.Option) (*s3control.PutPublicAccessBlockOutput, error)
	PutPublicAccessBlockRequest(*s3control.PutPublicAccessBlockInput) (*request.Request, *s3control.PutPublicAccessBlockOutput)

	UpdateJobPriority(*s3control.UpdateJobPriorityInput) (*s3control.UpdateJobPriorityOutput, error)
	UpdateJobPriorityWithContext(aws.Context, *s3control.UpdateJobPriorityInput, ...request.Option) (*s3control.UpdateJobPriorityOutput, error)
	UpdateJobPriorityRequest(*s3control.UpdateJobPriorityInput) (*request.Request, *s3control.UpdateJobPriorityOutput)

	UpdateJobStatus(*s3control.UpdateJobStatusInput) (*s3control.UpdateJobStatusOutput, error)
	UpdateJobStatusWithContext(aws.Context, *s3control.UpdateJobStatusInput, ...request.Option) (*s3control.UpdateJobStatusOutput, error)
	UpdateJobStatusRequest(*s3control.UpdateJobStatusInput) (*request.Request, *s3control.UpdateJobStatusOutput)
}

var _ S3ControlAPI = (*s3control.S3Control)(nil)
