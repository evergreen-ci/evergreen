// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

// Package workdocsiface provides an interface to enable mocking the Amazon WorkDocs service client
// for testing your code.
//
// It is important to note that this interface will have breaking changes
// when the service model is updated and adds new API operations, paginators,
// and waiters.
package workdocsiface

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/workdocs"
)

// WorkDocsAPI provides an interface to enable mocking the
// workdocs.WorkDocs service client's API operation,
// paginators, and waiters. This make unit testing your code that calls out
// to the SDK's service client's calls easier.
//
// The best way to use this interface is so the SDK's service client's calls
// can be stubbed out for unit testing your code with the SDK without needing
// to inject custom request handlers into the SDK's request pipeline.
//
//    // myFunc uses an SDK service client to make a request to
//    // Amazon WorkDocs.
//    func myFunc(svc workdocsiface.WorkDocsAPI) bool {
//        // Make svc.AbortDocumentVersionUpload request
//    }
//
//    func main() {
//        sess := session.New()
//        svc := workdocs.New(sess)
//
//        myFunc(svc)
//    }
//
// In your _test.go file:
//
//    // Define a mock struct to be used in your unit tests of myFunc.
//    type mockWorkDocsClient struct {
//        workdocsiface.WorkDocsAPI
//    }
//    func (m *mockWorkDocsClient) AbortDocumentVersionUpload(input *workdocs.AbortDocumentVersionUploadInput) (*workdocs.AbortDocumentVersionUploadOutput, error) {
//        // mock response/functionality
//    }
//
//    func TestMyFunc(t *testing.T) {
//        // Setup Test
//        mockSvc := &mockWorkDocsClient{}
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
type WorkDocsAPI interface {
	AbortDocumentVersionUpload(*workdocs.AbortDocumentVersionUploadInput) (*workdocs.AbortDocumentVersionUploadOutput, error)
	AbortDocumentVersionUploadWithContext(aws.Context, *workdocs.AbortDocumentVersionUploadInput, ...request.Option) (*workdocs.AbortDocumentVersionUploadOutput, error)
	AbortDocumentVersionUploadRequest(*workdocs.AbortDocumentVersionUploadInput) (*request.Request, *workdocs.AbortDocumentVersionUploadOutput)

	ActivateUser(*workdocs.ActivateUserInput) (*workdocs.ActivateUserOutput, error)
	ActivateUserWithContext(aws.Context, *workdocs.ActivateUserInput, ...request.Option) (*workdocs.ActivateUserOutput, error)
	ActivateUserRequest(*workdocs.ActivateUserInput) (*request.Request, *workdocs.ActivateUserOutput)

	AddResourcePermissions(*workdocs.AddResourcePermissionsInput) (*workdocs.AddResourcePermissionsOutput, error)
	AddResourcePermissionsWithContext(aws.Context, *workdocs.AddResourcePermissionsInput, ...request.Option) (*workdocs.AddResourcePermissionsOutput, error)
	AddResourcePermissionsRequest(*workdocs.AddResourcePermissionsInput) (*request.Request, *workdocs.AddResourcePermissionsOutput)

	CreateComment(*workdocs.CreateCommentInput) (*workdocs.CreateCommentOutput, error)
	CreateCommentWithContext(aws.Context, *workdocs.CreateCommentInput, ...request.Option) (*workdocs.CreateCommentOutput, error)
	CreateCommentRequest(*workdocs.CreateCommentInput) (*request.Request, *workdocs.CreateCommentOutput)

	CreateCustomMetadata(*workdocs.CreateCustomMetadataInput) (*workdocs.CreateCustomMetadataOutput, error)
	CreateCustomMetadataWithContext(aws.Context, *workdocs.CreateCustomMetadataInput, ...request.Option) (*workdocs.CreateCustomMetadataOutput, error)
	CreateCustomMetadataRequest(*workdocs.CreateCustomMetadataInput) (*request.Request, *workdocs.CreateCustomMetadataOutput)

	CreateFolder(*workdocs.CreateFolderInput) (*workdocs.CreateFolderOutput, error)
	CreateFolderWithContext(aws.Context, *workdocs.CreateFolderInput, ...request.Option) (*workdocs.CreateFolderOutput, error)
	CreateFolderRequest(*workdocs.CreateFolderInput) (*request.Request, *workdocs.CreateFolderOutput)

	CreateLabels(*workdocs.CreateLabelsInput) (*workdocs.CreateLabelsOutput, error)
	CreateLabelsWithContext(aws.Context, *workdocs.CreateLabelsInput, ...request.Option) (*workdocs.CreateLabelsOutput, error)
	CreateLabelsRequest(*workdocs.CreateLabelsInput) (*request.Request, *workdocs.CreateLabelsOutput)

	CreateNotificationSubscription(*workdocs.CreateNotificationSubscriptionInput) (*workdocs.CreateNotificationSubscriptionOutput, error)
	CreateNotificationSubscriptionWithContext(aws.Context, *workdocs.CreateNotificationSubscriptionInput, ...request.Option) (*workdocs.CreateNotificationSubscriptionOutput, error)
	CreateNotificationSubscriptionRequest(*workdocs.CreateNotificationSubscriptionInput) (*request.Request, *workdocs.CreateNotificationSubscriptionOutput)

	CreateUser(*workdocs.CreateUserInput) (*workdocs.CreateUserOutput, error)
	CreateUserWithContext(aws.Context, *workdocs.CreateUserInput, ...request.Option) (*workdocs.CreateUserOutput, error)
	CreateUserRequest(*workdocs.CreateUserInput) (*request.Request, *workdocs.CreateUserOutput)

	DeactivateUser(*workdocs.DeactivateUserInput) (*workdocs.DeactivateUserOutput, error)
	DeactivateUserWithContext(aws.Context, *workdocs.DeactivateUserInput, ...request.Option) (*workdocs.DeactivateUserOutput, error)
	DeactivateUserRequest(*workdocs.DeactivateUserInput) (*request.Request, *workdocs.DeactivateUserOutput)

	DeleteComment(*workdocs.DeleteCommentInput) (*workdocs.DeleteCommentOutput, error)
	DeleteCommentWithContext(aws.Context, *workdocs.DeleteCommentInput, ...request.Option) (*workdocs.DeleteCommentOutput, error)
	DeleteCommentRequest(*workdocs.DeleteCommentInput) (*request.Request, *workdocs.DeleteCommentOutput)

	DeleteCustomMetadata(*workdocs.DeleteCustomMetadataInput) (*workdocs.DeleteCustomMetadataOutput, error)
	DeleteCustomMetadataWithContext(aws.Context, *workdocs.DeleteCustomMetadataInput, ...request.Option) (*workdocs.DeleteCustomMetadataOutput, error)
	DeleteCustomMetadataRequest(*workdocs.DeleteCustomMetadataInput) (*request.Request, *workdocs.DeleteCustomMetadataOutput)

	DeleteDocument(*workdocs.DeleteDocumentInput) (*workdocs.DeleteDocumentOutput, error)
	DeleteDocumentWithContext(aws.Context, *workdocs.DeleteDocumentInput, ...request.Option) (*workdocs.DeleteDocumentOutput, error)
	DeleteDocumentRequest(*workdocs.DeleteDocumentInput) (*request.Request, *workdocs.DeleteDocumentOutput)

	DeleteFolder(*workdocs.DeleteFolderInput) (*workdocs.DeleteFolderOutput, error)
	DeleteFolderWithContext(aws.Context, *workdocs.DeleteFolderInput, ...request.Option) (*workdocs.DeleteFolderOutput, error)
	DeleteFolderRequest(*workdocs.DeleteFolderInput) (*request.Request, *workdocs.DeleteFolderOutput)

	DeleteFolderContents(*workdocs.DeleteFolderContentsInput) (*workdocs.DeleteFolderContentsOutput, error)
	DeleteFolderContentsWithContext(aws.Context, *workdocs.DeleteFolderContentsInput, ...request.Option) (*workdocs.DeleteFolderContentsOutput, error)
	DeleteFolderContentsRequest(*workdocs.DeleteFolderContentsInput) (*request.Request, *workdocs.DeleteFolderContentsOutput)

	DeleteLabels(*workdocs.DeleteLabelsInput) (*workdocs.DeleteLabelsOutput, error)
	DeleteLabelsWithContext(aws.Context, *workdocs.DeleteLabelsInput, ...request.Option) (*workdocs.DeleteLabelsOutput, error)
	DeleteLabelsRequest(*workdocs.DeleteLabelsInput) (*request.Request, *workdocs.DeleteLabelsOutput)

	DeleteNotificationSubscription(*workdocs.DeleteNotificationSubscriptionInput) (*workdocs.DeleteNotificationSubscriptionOutput, error)
	DeleteNotificationSubscriptionWithContext(aws.Context, *workdocs.DeleteNotificationSubscriptionInput, ...request.Option) (*workdocs.DeleteNotificationSubscriptionOutput, error)
	DeleteNotificationSubscriptionRequest(*workdocs.DeleteNotificationSubscriptionInput) (*request.Request, *workdocs.DeleteNotificationSubscriptionOutput)

	DeleteUser(*workdocs.DeleteUserInput) (*workdocs.DeleteUserOutput, error)
	DeleteUserWithContext(aws.Context, *workdocs.DeleteUserInput, ...request.Option) (*workdocs.DeleteUserOutput, error)
	DeleteUserRequest(*workdocs.DeleteUserInput) (*request.Request, *workdocs.DeleteUserOutput)

	DescribeActivities(*workdocs.DescribeActivitiesInput) (*workdocs.DescribeActivitiesOutput, error)
	DescribeActivitiesWithContext(aws.Context, *workdocs.DescribeActivitiesInput, ...request.Option) (*workdocs.DescribeActivitiesOutput, error)
	DescribeActivitiesRequest(*workdocs.DescribeActivitiesInput) (*request.Request, *workdocs.DescribeActivitiesOutput)

	DescribeComments(*workdocs.DescribeCommentsInput) (*workdocs.DescribeCommentsOutput, error)
	DescribeCommentsWithContext(aws.Context, *workdocs.DescribeCommentsInput, ...request.Option) (*workdocs.DescribeCommentsOutput, error)
	DescribeCommentsRequest(*workdocs.DescribeCommentsInput) (*request.Request, *workdocs.DescribeCommentsOutput)

	DescribeDocumentVersions(*workdocs.DescribeDocumentVersionsInput) (*workdocs.DescribeDocumentVersionsOutput, error)
	DescribeDocumentVersionsWithContext(aws.Context, *workdocs.DescribeDocumentVersionsInput, ...request.Option) (*workdocs.DescribeDocumentVersionsOutput, error)
	DescribeDocumentVersionsRequest(*workdocs.DescribeDocumentVersionsInput) (*request.Request, *workdocs.DescribeDocumentVersionsOutput)

	DescribeDocumentVersionsPages(*workdocs.DescribeDocumentVersionsInput, func(*workdocs.DescribeDocumentVersionsOutput, bool) bool) error
	DescribeDocumentVersionsPagesWithContext(aws.Context, *workdocs.DescribeDocumentVersionsInput, func(*workdocs.DescribeDocumentVersionsOutput, bool) bool, ...request.Option) error

	DescribeFolderContents(*workdocs.DescribeFolderContentsInput) (*workdocs.DescribeFolderContentsOutput, error)
	DescribeFolderContentsWithContext(aws.Context, *workdocs.DescribeFolderContentsInput, ...request.Option) (*workdocs.DescribeFolderContentsOutput, error)
	DescribeFolderContentsRequest(*workdocs.DescribeFolderContentsInput) (*request.Request, *workdocs.DescribeFolderContentsOutput)

	DescribeFolderContentsPages(*workdocs.DescribeFolderContentsInput, func(*workdocs.DescribeFolderContentsOutput, bool) bool) error
	DescribeFolderContentsPagesWithContext(aws.Context, *workdocs.DescribeFolderContentsInput, func(*workdocs.DescribeFolderContentsOutput, bool) bool, ...request.Option) error

	DescribeNotificationSubscriptions(*workdocs.DescribeNotificationSubscriptionsInput) (*workdocs.DescribeNotificationSubscriptionsOutput, error)
	DescribeNotificationSubscriptionsWithContext(aws.Context, *workdocs.DescribeNotificationSubscriptionsInput, ...request.Option) (*workdocs.DescribeNotificationSubscriptionsOutput, error)
	DescribeNotificationSubscriptionsRequest(*workdocs.DescribeNotificationSubscriptionsInput) (*request.Request, *workdocs.DescribeNotificationSubscriptionsOutput)

	DescribeResourcePermissions(*workdocs.DescribeResourcePermissionsInput) (*workdocs.DescribeResourcePermissionsOutput, error)
	DescribeResourcePermissionsWithContext(aws.Context, *workdocs.DescribeResourcePermissionsInput, ...request.Option) (*workdocs.DescribeResourcePermissionsOutput, error)
	DescribeResourcePermissionsRequest(*workdocs.DescribeResourcePermissionsInput) (*request.Request, *workdocs.DescribeResourcePermissionsOutput)

	DescribeRootFolders(*workdocs.DescribeRootFoldersInput) (*workdocs.DescribeRootFoldersOutput, error)
	DescribeRootFoldersWithContext(aws.Context, *workdocs.DescribeRootFoldersInput, ...request.Option) (*workdocs.DescribeRootFoldersOutput, error)
	DescribeRootFoldersRequest(*workdocs.DescribeRootFoldersInput) (*request.Request, *workdocs.DescribeRootFoldersOutput)

	DescribeUsers(*workdocs.DescribeUsersInput) (*workdocs.DescribeUsersOutput, error)
	DescribeUsersWithContext(aws.Context, *workdocs.DescribeUsersInput, ...request.Option) (*workdocs.DescribeUsersOutput, error)
	DescribeUsersRequest(*workdocs.DescribeUsersInput) (*request.Request, *workdocs.DescribeUsersOutput)

	DescribeUsersPages(*workdocs.DescribeUsersInput, func(*workdocs.DescribeUsersOutput, bool) bool) error
	DescribeUsersPagesWithContext(aws.Context, *workdocs.DescribeUsersInput, func(*workdocs.DescribeUsersOutput, bool) bool, ...request.Option) error

	GetCurrentUser(*workdocs.GetCurrentUserInput) (*workdocs.GetCurrentUserOutput, error)
	GetCurrentUserWithContext(aws.Context, *workdocs.GetCurrentUserInput, ...request.Option) (*workdocs.GetCurrentUserOutput, error)
	GetCurrentUserRequest(*workdocs.GetCurrentUserInput) (*request.Request, *workdocs.GetCurrentUserOutput)

	GetDocument(*workdocs.GetDocumentInput) (*workdocs.GetDocumentOutput, error)
	GetDocumentWithContext(aws.Context, *workdocs.GetDocumentInput, ...request.Option) (*workdocs.GetDocumentOutput, error)
	GetDocumentRequest(*workdocs.GetDocumentInput) (*request.Request, *workdocs.GetDocumentOutput)

	GetDocumentPath(*workdocs.GetDocumentPathInput) (*workdocs.GetDocumentPathOutput, error)
	GetDocumentPathWithContext(aws.Context, *workdocs.GetDocumentPathInput, ...request.Option) (*workdocs.GetDocumentPathOutput, error)
	GetDocumentPathRequest(*workdocs.GetDocumentPathInput) (*request.Request, *workdocs.GetDocumentPathOutput)

	GetDocumentVersion(*workdocs.GetDocumentVersionInput) (*workdocs.GetDocumentVersionOutput, error)
	GetDocumentVersionWithContext(aws.Context, *workdocs.GetDocumentVersionInput, ...request.Option) (*workdocs.GetDocumentVersionOutput, error)
	GetDocumentVersionRequest(*workdocs.GetDocumentVersionInput) (*request.Request, *workdocs.GetDocumentVersionOutput)

	GetFolder(*workdocs.GetFolderInput) (*workdocs.GetFolderOutput, error)
	GetFolderWithContext(aws.Context, *workdocs.GetFolderInput, ...request.Option) (*workdocs.GetFolderOutput, error)
	GetFolderRequest(*workdocs.GetFolderInput) (*request.Request, *workdocs.GetFolderOutput)

	GetFolderPath(*workdocs.GetFolderPathInput) (*workdocs.GetFolderPathOutput, error)
	GetFolderPathWithContext(aws.Context, *workdocs.GetFolderPathInput, ...request.Option) (*workdocs.GetFolderPathOutput, error)
	GetFolderPathRequest(*workdocs.GetFolderPathInput) (*request.Request, *workdocs.GetFolderPathOutput)

	InitiateDocumentVersionUpload(*workdocs.InitiateDocumentVersionUploadInput) (*workdocs.InitiateDocumentVersionUploadOutput, error)
	InitiateDocumentVersionUploadWithContext(aws.Context, *workdocs.InitiateDocumentVersionUploadInput, ...request.Option) (*workdocs.InitiateDocumentVersionUploadOutput, error)
	InitiateDocumentVersionUploadRequest(*workdocs.InitiateDocumentVersionUploadInput) (*request.Request, *workdocs.InitiateDocumentVersionUploadOutput)

	RemoveAllResourcePermissions(*workdocs.RemoveAllResourcePermissionsInput) (*workdocs.RemoveAllResourcePermissionsOutput, error)
	RemoveAllResourcePermissionsWithContext(aws.Context, *workdocs.RemoveAllResourcePermissionsInput, ...request.Option) (*workdocs.RemoveAllResourcePermissionsOutput, error)
	RemoveAllResourcePermissionsRequest(*workdocs.RemoveAllResourcePermissionsInput) (*request.Request, *workdocs.RemoveAllResourcePermissionsOutput)

	RemoveResourcePermission(*workdocs.RemoveResourcePermissionInput) (*workdocs.RemoveResourcePermissionOutput, error)
	RemoveResourcePermissionWithContext(aws.Context, *workdocs.RemoveResourcePermissionInput, ...request.Option) (*workdocs.RemoveResourcePermissionOutput, error)
	RemoveResourcePermissionRequest(*workdocs.RemoveResourcePermissionInput) (*request.Request, *workdocs.RemoveResourcePermissionOutput)

	UpdateDocument(*workdocs.UpdateDocumentInput) (*workdocs.UpdateDocumentOutput, error)
	UpdateDocumentWithContext(aws.Context, *workdocs.UpdateDocumentInput, ...request.Option) (*workdocs.UpdateDocumentOutput, error)
	UpdateDocumentRequest(*workdocs.UpdateDocumentInput) (*request.Request, *workdocs.UpdateDocumentOutput)

	UpdateDocumentVersion(*workdocs.UpdateDocumentVersionInput) (*workdocs.UpdateDocumentVersionOutput, error)
	UpdateDocumentVersionWithContext(aws.Context, *workdocs.UpdateDocumentVersionInput, ...request.Option) (*workdocs.UpdateDocumentVersionOutput, error)
	UpdateDocumentVersionRequest(*workdocs.UpdateDocumentVersionInput) (*request.Request, *workdocs.UpdateDocumentVersionOutput)

	UpdateFolder(*workdocs.UpdateFolderInput) (*workdocs.UpdateFolderOutput, error)
	UpdateFolderWithContext(aws.Context, *workdocs.UpdateFolderInput, ...request.Option) (*workdocs.UpdateFolderOutput, error)
	UpdateFolderRequest(*workdocs.UpdateFolderInput) (*request.Request, *workdocs.UpdateFolderOutput)

	UpdateUser(*workdocs.UpdateUserInput) (*workdocs.UpdateUserOutput, error)
	UpdateUserWithContext(aws.Context, *workdocs.UpdateUserInput, ...request.Option) (*workdocs.UpdateUserOutput, error)
	UpdateUserRequest(*workdocs.UpdateUserInput) (*request.Request, *workdocs.UpdateUserOutput)
}

var _ WorkDocsAPI = (*workdocs.WorkDocs)(nil)
