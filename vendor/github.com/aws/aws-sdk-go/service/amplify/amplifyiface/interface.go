// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

// Package amplifyiface provides an interface to enable mocking the AWS Amplify service client
// for testing your code.
//
// It is important to note that this interface will have breaking changes
// when the service model is updated and adds new API operations, paginators,
// and waiters.
package amplifyiface

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/amplify"
)

// AmplifyAPI provides an interface to enable mocking the
// amplify.Amplify service client's API operation,
// paginators, and waiters. This make unit testing your code that calls out
// to the SDK's service client's calls easier.
//
// The best way to use this interface is so the SDK's service client's calls
// can be stubbed out for unit testing your code with the SDK without needing
// to inject custom request handlers into the SDK's request pipeline.
//
//    // myFunc uses an SDK service client to make a request to
//    // AWS Amplify.
//    func myFunc(svc amplifyiface.AmplifyAPI) bool {
//        // Make svc.CreateApp request
//    }
//
//    func main() {
//        sess := session.New()
//        svc := amplify.New(sess)
//
//        myFunc(svc)
//    }
//
// In your _test.go file:
//
//    // Define a mock struct to be used in your unit tests of myFunc.
//    type mockAmplifyClient struct {
//        amplifyiface.AmplifyAPI
//    }
//    func (m *mockAmplifyClient) CreateApp(input *amplify.CreateAppInput) (*amplify.CreateAppOutput, error) {
//        // mock response/functionality
//    }
//
//    func TestMyFunc(t *testing.T) {
//        // Setup Test
//        mockSvc := &mockAmplifyClient{}
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
type AmplifyAPI interface {
	CreateApp(*amplify.CreateAppInput) (*amplify.CreateAppOutput, error)
	CreateAppWithContext(aws.Context, *amplify.CreateAppInput, ...request.Option) (*amplify.CreateAppOutput, error)
	CreateAppRequest(*amplify.CreateAppInput) (*request.Request, *amplify.CreateAppOutput)

	CreateBackendEnvironment(*amplify.CreateBackendEnvironmentInput) (*amplify.CreateBackendEnvironmentOutput, error)
	CreateBackendEnvironmentWithContext(aws.Context, *amplify.CreateBackendEnvironmentInput, ...request.Option) (*amplify.CreateBackendEnvironmentOutput, error)
	CreateBackendEnvironmentRequest(*amplify.CreateBackendEnvironmentInput) (*request.Request, *amplify.CreateBackendEnvironmentOutput)

	CreateBranch(*amplify.CreateBranchInput) (*amplify.CreateBranchOutput, error)
	CreateBranchWithContext(aws.Context, *amplify.CreateBranchInput, ...request.Option) (*amplify.CreateBranchOutput, error)
	CreateBranchRequest(*amplify.CreateBranchInput) (*request.Request, *amplify.CreateBranchOutput)

	CreateDeployment(*amplify.CreateDeploymentInput) (*amplify.CreateDeploymentOutput, error)
	CreateDeploymentWithContext(aws.Context, *amplify.CreateDeploymentInput, ...request.Option) (*amplify.CreateDeploymentOutput, error)
	CreateDeploymentRequest(*amplify.CreateDeploymentInput) (*request.Request, *amplify.CreateDeploymentOutput)

	CreateDomainAssociation(*amplify.CreateDomainAssociationInput) (*amplify.CreateDomainAssociationOutput, error)
	CreateDomainAssociationWithContext(aws.Context, *amplify.CreateDomainAssociationInput, ...request.Option) (*amplify.CreateDomainAssociationOutput, error)
	CreateDomainAssociationRequest(*amplify.CreateDomainAssociationInput) (*request.Request, *amplify.CreateDomainAssociationOutput)

	CreateWebhook(*amplify.CreateWebhookInput) (*amplify.CreateWebhookOutput, error)
	CreateWebhookWithContext(aws.Context, *amplify.CreateWebhookInput, ...request.Option) (*amplify.CreateWebhookOutput, error)
	CreateWebhookRequest(*amplify.CreateWebhookInput) (*request.Request, *amplify.CreateWebhookOutput)

	DeleteApp(*amplify.DeleteAppInput) (*amplify.DeleteAppOutput, error)
	DeleteAppWithContext(aws.Context, *amplify.DeleteAppInput, ...request.Option) (*amplify.DeleteAppOutput, error)
	DeleteAppRequest(*amplify.DeleteAppInput) (*request.Request, *amplify.DeleteAppOutput)

	DeleteBackendEnvironment(*amplify.DeleteBackendEnvironmentInput) (*amplify.DeleteBackendEnvironmentOutput, error)
	DeleteBackendEnvironmentWithContext(aws.Context, *amplify.DeleteBackendEnvironmentInput, ...request.Option) (*amplify.DeleteBackendEnvironmentOutput, error)
	DeleteBackendEnvironmentRequest(*amplify.DeleteBackendEnvironmentInput) (*request.Request, *amplify.DeleteBackendEnvironmentOutput)

	DeleteBranch(*amplify.DeleteBranchInput) (*amplify.DeleteBranchOutput, error)
	DeleteBranchWithContext(aws.Context, *amplify.DeleteBranchInput, ...request.Option) (*amplify.DeleteBranchOutput, error)
	DeleteBranchRequest(*amplify.DeleteBranchInput) (*request.Request, *amplify.DeleteBranchOutput)

	DeleteDomainAssociation(*amplify.DeleteDomainAssociationInput) (*amplify.DeleteDomainAssociationOutput, error)
	DeleteDomainAssociationWithContext(aws.Context, *amplify.DeleteDomainAssociationInput, ...request.Option) (*amplify.DeleteDomainAssociationOutput, error)
	DeleteDomainAssociationRequest(*amplify.DeleteDomainAssociationInput) (*request.Request, *amplify.DeleteDomainAssociationOutput)

	DeleteJob(*amplify.DeleteJobInput) (*amplify.DeleteJobOutput, error)
	DeleteJobWithContext(aws.Context, *amplify.DeleteJobInput, ...request.Option) (*amplify.DeleteJobOutput, error)
	DeleteJobRequest(*amplify.DeleteJobInput) (*request.Request, *amplify.DeleteJobOutput)

	DeleteWebhook(*amplify.DeleteWebhookInput) (*amplify.DeleteWebhookOutput, error)
	DeleteWebhookWithContext(aws.Context, *amplify.DeleteWebhookInput, ...request.Option) (*amplify.DeleteWebhookOutput, error)
	DeleteWebhookRequest(*amplify.DeleteWebhookInput) (*request.Request, *amplify.DeleteWebhookOutput)

	GenerateAccessLogs(*amplify.GenerateAccessLogsInput) (*amplify.GenerateAccessLogsOutput, error)
	GenerateAccessLogsWithContext(aws.Context, *amplify.GenerateAccessLogsInput, ...request.Option) (*amplify.GenerateAccessLogsOutput, error)
	GenerateAccessLogsRequest(*amplify.GenerateAccessLogsInput) (*request.Request, *amplify.GenerateAccessLogsOutput)

	GetApp(*amplify.GetAppInput) (*amplify.GetAppOutput, error)
	GetAppWithContext(aws.Context, *amplify.GetAppInput, ...request.Option) (*amplify.GetAppOutput, error)
	GetAppRequest(*amplify.GetAppInput) (*request.Request, *amplify.GetAppOutput)

	GetArtifactUrl(*amplify.GetArtifactUrlInput) (*amplify.GetArtifactUrlOutput, error)
	GetArtifactUrlWithContext(aws.Context, *amplify.GetArtifactUrlInput, ...request.Option) (*amplify.GetArtifactUrlOutput, error)
	GetArtifactUrlRequest(*amplify.GetArtifactUrlInput) (*request.Request, *amplify.GetArtifactUrlOutput)

	GetBackendEnvironment(*amplify.GetBackendEnvironmentInput) (*amplify.GetBackendEnvironmentOutput, error)
	GetBackendEnvironmentWithContext(aws.Context, *amplify.GetBackendEnvironmentInput, ...request.Option) (*amplify.GetBackendEnvironmentOutput, error)
	GetBackendEnvironmentRequest(*amplify.GetBackendEnvironmentInput) (*request.Request, *amplify.GetBackendEnvironmentOutput)

	GetBranch(*amplify.GetBranchInput) (*amplify.GetBranchOutput, error)
	GetBranchWithContext(aws.Context, *amplify.GetBranchInput, ...request.Option) (*amplify.GetBranchOutput, error)
	GetBranchRequest(*amplify.GetBranchInput) (*request.Request, *amplify.GetBranchOutput)

	GetDomainAssociation(*amplify.GetDomainAssociationInput) (*amplify.GetDomainAssociationOutput, error)
	GetDomainAssociationWithContext(aws.Context, *amplify.GetDomainAssociationInput, ...request.Option) (*amplify.GetDomainAssociationOutput, error)
	GetDomainAssociationRequest(*amplify.GetDomainAssociationInput) (*request.Request, *amplify.GetDomainAssociationOutput)

	GetJob(*amplify.GetJobInput) (*amplify.GetJobOutput, error)
	GetJobWithContext(aws.Context, *amplify.GetJobInput, ...request.Option) (*amplify.GetJobOutput, error)
	GetJobRequest(*amplify.GetJobInput) (*request.Request, *amplify.GetJobOutput)

	GetWebhook(*amplify.GetWebhookInput) (*amplify.GetWebhookOutput, error)
	GetWebhookWithContext(aws.Context, *amplify.GetWebhookInput, ...request.Option) (*amplify.GetWebhookOutput, error)
	GetWebhookRequest(*amplify.GetWebhookInput) (*request.Request, *amplify.GetWebhookOutput)

	ListApps(*amplify.ListAppsInput) (*amplify.ListAppsOutput, error)
	ListAppsWithContext(aws.Context, *amplify.ListAppsInput, ...request.Option) (*amplify.ListAppsOutput, error)
	ListAppsRequest(*amplify.ListAppsInput) (*request.Request, *amplify.ListAppsOutput)

	ListArtifacts(*amplify.ListArtifactsInput) (*amplify.ListArtifactsOutput, error)
	ListArtifactsWithContext(aws.Context, *amplify.ListArtifactsInput, ...request.Option) (*amplify.ListArtifactsOutput, error)
	ListArtifactsRequest(*amplify.ListArtifactsInput) (*request.Request, *amplify.ListArtifactsOutput)

	ListBackendEnvironments(*amplify.ListBackendEnvironmentsInput) (*amplify.ListBackendEnvironmentsOutput, error)
	ListBackendEnvironmentsWithContext(aws.Context, *amplify.ListBackendEnvironmentsInput, ...request.Option) (*amplify.ListBackendEnvironmentsOutput, error)
	ListBackendEnvironmentsRequest(*amplify.ListBackendEnvironmentsInput) (*request.Request, *amplify.ListBackendEnvironmentsOutput)

	ListBranches(*amplify.ListBranchesInput) (*amplify.ListBranchesOutput, error)
	ListBranchesWithContext(aws.Context, *amplify.ListBranchesInput, ...request.Option) (*amplify.ListBranchesOutput, error)
	ListBranchesRequest(*amplify.ListBranchesInput) (*request.Request, *amplify.ListBranchesOutput)

	ListDomainAssociations(*amplify.ListDomainAssociationsInput) (*amplify.ListDomainAssociationsOutput, error)
	ListDomainAssociationsWithContext(aws.Context, *amplify.ListDomainAssociationsInput, ...request.Option) (*amplify.ListDomainAssociationsOutput, error)
	ListDomainAssociationsRequest(*amplify.ListDomainAssociationsInput) (*request.Request, *amplify.ListDomainAssociationsOutput)

	ListJobs(*amplify.ListJobsInput) (*amplify.ListJobsOutput, error)
	ListJobsWithContext(aws.Context, *amplify.ListJobsInput, ...request.Option) (*amplify.ListJobsOutput, error)
	ListJobsRequest(*amplify.ListJobsInput) (*request.Request, *amplify.ListJobsOutput)

	ListTagsForResource(*amplify.ListTagsForResourceInput) (*amplify.ListTagsForResourceOutput, error)
	ListTagsForResourceWithContext(aws.Context, *amplify.ListTagsForResourceInput, ...request.Option) (*amplify.ListTagsForResourceOutput, error)
	ListTagsForResourceRequest(*amplify.ListTagsForResourceInput) (*request.Request, *amplify.ListTagsForResourceOutput)

	ListWebhooks(*amplify.ListWebhooksInput) (*amplify.ListWebhooksOutput, error)
	ListWebhooksWithContext(aws.Context, *amplify.ListWebhooksInput, ...request.Option) (*amplify.ListWebhooksOutput, error)
	ListWebhooksRequest(*amplify.ListWebhooksInput) (*request.Request, *amplify.ListWebhooksOutput)

	StartDeployment(*amplify.StartDeploymentInput) (*amplify.StartDeploymentOutput, error)
	StartDeploymentWithContext(aws.Context, *amplify.StartDeploymentInput, ...request.Option) (*amplify.StartDeploymentOutput, error)
	StartDeploymentRequest(*amplify.StartDeploymentInput) (*request.Request, *amplify.StartDeploymentOutput)

	StartJob(*amplify.StartJobInput) (*amplify.StartJobOutput, error)
	StartJobWithContext(aws.Context, *amplify.StartJobInput, ...request.Option) (*amplify.StartJobOutput, error)
	StartJobRequest(*amplify.StartJobInput) (*request.Request, *amplify.StartJobOutput)

	StopJob(*amplify.StopJobInput) (*amplify.StopJobOutput, error)
	StopJobWithContext(aws.Context, *amplify.StopJobInput, ...request.Option) (*amplify.StopJobOutput, error)
	StopJobRequest(*amplify.StopJobInput) (*request.Request, *amplify.StopJobOutput)

	TagResource(*amplify.TagResourceInput) (*amplify.TagResourceOutput, error)
	TagResourceWithContext(aws.Context, *amplify.TagResourceInput, ...request.Option) (*amplify.TagResourceOutput, error)
	TagResourceRequest(*amplify.TagResourceInput) (*request.Request, *amplify.TagResourceOutput)

	UntagResource(*amplify.UntagResourceInput) (*amplify.UntagResourceOutput, error)
	UntagResourceWithContext(aws.Context, *amplify.UntagResourceInput, ...request.Option) (*amplify.UntagResourceOutput, error)
	UntagResourceRequest(*amplify.UntagResourceInput) (*request.Request, *amplify.UntagResourceOutput)

	UpdateApp(*amplify.UpdateAppInput) (*amplify.UpdateAppOutput, error)
	UpdateAppWithContext(aws.Context, *amplify.UpdateAppInput, ...request.Option) (*amplify.UpdateAppOutput, error)
	UpdateAppRequest(*amplify.UpdateAppInput) (*request.Request, *amplify.UpdateAppOutput)

	UpdateBranch(*amplify.UpdateBranchInput) (*amplify.UpdateBranchOutput, error)
	UpdateBranchWithContext(aws.Context, *amplify.UpdateBranchInput, ...request.Option) (*amplify.UpdateBranchOutput, error)
	UpdateBranchRequest(*amplify.UpdateBranchInput) (*request.Request, *amplify.UpdateBranchOutput)

	UpdateDomainAssociation(*amplify.UpdateDomainAssociationInput) (*amplify.UpdateDomainAssociationOutput, error)
	UpdateDomainAssociationWithContext(aws.Context, *amplify.UpdateDomainAssociationInput, ...request.Option) (*amplify.UpdateDomainAssociationOutput, error)
	UpdateDomainAssociationRequest(*amplify.UpdateDomainAssociationInput) (*request.Request, *amplify.UpdateDomainAssociationOutput)

	UpdateWebhook(*amplify.UpdateWebhookInput) (*amplify.UpdateWebhookOutput, error)
	UpdateWebhookWithContext(aws.Context, *amplify.UpdateWebhookInput, ...request.Option) (*amplify.UpdateWebhookOutput, error)
	UpdateWebhookRequest(*amplify.UpdateWebhookInput) (*request.Request, *amplify.UpdateWebhookOutput)
}

var _ AmplifyAPI = (*amplify.Amplify)(nil)
