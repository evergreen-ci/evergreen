// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

// Package codestariface provides an interface to enable mocking the AWS CodeStar service client
// for testing your code.
//
// It is important to note that this interface will have breaking changes
// when the service model is updated and adds new API operations, paginators,
// and waiters.
package codestariface

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/codestar"
)

// CodeStarAPI provides an interface to enable mocking the
// codestar.CodeStar service client's API operation,
// paginators, and waiters. This make unit testing your code that calls out
// to the SDK's service client's calls easier.
//
// The best way to use this interface is so the SDK's service client's calls
// can be stubbed out for unit testing your code with the SDK without needing
// to inject custom request handlers into the SDK's request pipeline.
//
//    // myFunc uses an SDK service client to make a request to
//    // AWS CodeStar.
//    func myFunc(svc codestariface.CodeStarAPI) bool {
//        // Make svc.AssociateTeamMember request
//    }
//
//    func main() {
//        sess := session.New()
//        svc := codestar.New(sess)
//
//        myFunc(svc)
//    }
//
// In your _test.go file:
//
//    // Define a mock struct to be used in your unit tests of myFunc.
//    type mockCodeStarClient struct {
//        codestariface.CodeStarAPI
//    }
//    func (m *mockCodeStarClient) AssociateTeamMember(input *codestar.AssociateTeamMemberInput) (*codestar.AssociateTeamMemberOutput, error) {
//        // mock response/functionality
//    }
//
//    func TestMyFunc(t *testing.T) {
//        // Setup Test
//        mockSvc := &mockCodeStarClient{}
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
type CodeStarAPI interface {
	AssociateTeamMember(*codestar.AssociateTeamMemberInput) (*codestar.AssociateTeamMemberOutput, error)
	AssociateTeamMemberWithContext(aws.Context, *codestar.AssociateTeamMemberInput, ...request.Option) (*codestar.AssociateTeamMemberOutput, error)
	AssociateTeamMemberRequest(*codestar.AssociateTeamMemberInput) (*request.Request, *codestar.AssociateTeamMemberOutput)

	CreateProject(*codestar.CreateProjectInput) (*codestar.CreateProjectOutput, error)
	CreateProjectWithContext(aws.Context, *codestar.CreateProjectInput, ...request.Option) (*codestar.CreateProjectOutput, error)
	CreateProjectRequest(*codestar.CreateProjectInput) (*request.Request, *codestar.CreateProjectOutput)

	CreateUserProfile(*codestar.CreateUserProfileInput) (*codestar.CreateUserProfileOutput, error)
	CreateUserProfileWithContext(aws.Context, *codestar.CreateUserProfileInput, ...request.Option) (*codestar.CreateUserProfileOutput, error)
	CreateUserProfileRequest(*codestar.CreateUserProfileInput) (*request.Request, *codestar.CreateUserProfileOutput)

	DeleteProject(*codestar.DeleteProjectInput) (*codestar.DeleteProjectOutput, error)
	DeleteProjectWithContext(aws.Context, *codestar.DeleteProjectInput, ...request.Option) (*codestar.DeleteProjectOutput, error)
	DeleteProjectRequest(*codestar.DeleteProjectInput) (*request.Request, *codestar.DeleteProjectOutput)

	DeleteUserProfile(*codestar.DeleteUserProfileInput) (*codestar.DeleteUserProfileOutput, error)
	DeleteUserProfileWithContext(aws.Context, *codestar.DeleteUserProfileInput, ...request.Option) (*codestar.DeleteUserProfileOutput, error)
	DeleteUserProfileRequest(*codestar.DeleteUserProfileInput) (*request.Request, *codestar.DeleteUserProfileOutput)

	DescribeProject(*codestar.DescribeProjectInput) (*codestar.DescribeProjectOutput, error)
	DescribeProjectWithContext(aws.Context, *codestar.DescribeProjectInput, ...request.Option) (*codestar.DescribeProjectOutput, error)
	DescribeProjectRequest(*codestar.DescribeProjectInput) (*request.Request, *codestar.DescribeProjectOutput)

	DescribeUserProfile(*codestar.DescribeUserProfileInput) (*codestar.DescribeUserProfileOutput, error)
	DescribeUserProfileWithContext(aws.Context, *codestar.DescribeUserProfileInput, ...request.Option) (*codestar.DescribeUserProfileOutput, error)
	DescribeUserProfileRequest(*codestar.DescribeUserProfileInput) (*request.Request, *codestar.DescribeUserProfileOutput)

	DisassociateTeamMember(*codestar.DisassociateTeamMemberInput) (*codestar.DisassociateTeamMemberOutput, error)
	DisassociateTeamMemberWithContext(aws.Context, *codestar.DisassociateTeamMemberInput, ...request.Option) (*codestar.DisassociateTeamMemberOutput, error)
	DisassociateTeamMemberRequest(*codestar.DisassociateTeamMemberInput) (*request.Request, *codestar.DisassociateTeamMemberOutput)

	ListProjects(*codestar.ListProjectsInput) (*codestar.ListProjectsOutput, error)
	ListProjectsWithContext(aws.Context, *codestar.ListProjectsInput, ...request.Option) (*codestar.ListProjectsOutput, error)
	ListProjectsRequest(*codestar.ListProjectsInput) (*request.Request, *codestar.ListProjectsOutput)

	ListResources(*codestar.ListResourcesInput) (*codestar.ListResourcesOutput, error)
	ListResourcesWithContext(aws.Context, *codestar.ListResourcesInput, ...request.Option) (*codestar.ListResourcesOutput, error)
	ListResourcesRequest(*codestar.ListResourcesInput) (*request.Request, *codestar.ListResourcesOutput)

	ListTeamMembers(*codestar.ListTeamMembersInput) (*codestar.ListTeamMembersOutput, error)
	ListTeamMembersWithContext(aws.Context, *codestar.ListTeamMembersInput, ...request.Option) (*codestar.ListTeamMembersOutput, error)
	ListTeamMembersRequest(*codestar.ListTeamMembersInput) (*request.Request, *codestar.ListTeamMembersOutput)

	ListUserProfiles(*codestar.ListUserProfilesInput) (*codestar.ListUserProfilesOutput, error)
	ListUserProfilesWithContext(aws.Context, *codestar.ListUserProfilesInput, ...request.Option) (*codestar.ListUserProfilesOutput, error)
	ListUserProfilesRequest(*codestar.ListUserProfilesInput) (*request.Request, *codestar.ListUserProfilesOutput)

	UpdateProject(*codestar.UpdateProjectInput) (*codestar.UpdateProjectOutput, error)
	UpdateProjectWithContext(aws.Context, *codestar.UpdateProjectInput, ...request.Option) (*codestar.UpdateProjectOutput, error)
	UpdateProjectRequest(*codestar.UpdateProjectInput) (*request.Request, *codestar.UpdateProjectOutput)

	UpdateTeamMember(*codestar.UpdateTeamMemberInput) (*codestar.UpdateTeamMemberOutput, error)
	UpdateTeamMemberWithContext(aws.Context, *codestar.UpdateTeamMemberInput, ...request.Option) (*codestar.UpdateTeamMemberOutput, error)
	UpdateTeamMemberRequest(*codestar.UpdateTeamMemberInput) (*request.Request, *codestar.UpdateTeamMemberOutput)

	UpdateUserProfile(*codestar.UpdateUserProfileInput) (*codestar.UpdateUserProfileOutput, error)
	UpdateUserProfileWithContext(aws.Context, *codestar.UpdateUserProfileInput, ...request.Option) (*codestar.UpdateUserProfileOutput, error)
	UpdateUserProfileRequest(*codestar.UpdateUserProfileInput) (*request.Request, *codestar.UpdateUserProfileOutput)
}

var _ CodeStarAPI = (*codestar.CodeStar)(nil)
