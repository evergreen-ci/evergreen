// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

// Package workspacesiface provides an interface to enable mocking the Amazon WorkSpaces service client
// for testing your code.
//
// It is important to note that this interface will have breaking changes
// when the service model is updated and adds new API operations, paginators,
// and waiters.
package workspacesiface

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/workspaces"
)

// WorkSpacesAPI provides an interface to enable mocking the
// workspaces.WorkSpaces service client's API operation,
// paginators, and waiters. This make unit testing your code that calls out
// to the SDK's service client's calls easier.
//
// The best way to use this interface is so the SDK's service client's calls
// can be stubbed out for unit testing your code with the SDK without needing
// to inject custom request handlers into the SDK's request pipeline.
//
//    // myFunc uses an SDK service client to make a request to
//    // Amazon WorkSpaces.
//    func myFunc(svc workspacesiface.WorkSpacesAPI) bool {
//        // Make svc.AssociateConnectionAlias request
//    }
//
//    func main() {
//        sess := session.New()
//        svc := workspaces.New(sess)
//
//        myFunc(svc)
//    }
//
// In your _test.go file:
//
//    // Define a mock struct to be used in your unit tests of myFunc.
//    type mockWorkSpacesClient struct {
//        workspacesiface.WorkSpacesAPI
//    }
//    func (m *mockWorkSpacesClient) AssociateConnectionAlias(input *workspaces.AssociateConnectionAliasInput) (*workspaces.AssociateConnectionAliasOutput, error) {
//        // mock response/functionality
//    }
//
//    func TestMyFunc(t *testing.T) {
//        // Setup Test
//        mockSvc := &mockWorkSpacesClient{}
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
type WorkSpacesAPI interface {
	AssociateConnectionAlias(*workspaces.AssociateConnectionAliasInput) (*workspaces.AssociateConnectionAliasOutput, error)
	AssociateConnectionAliasWithContext(aws.Context, *workspaces.AssociateConnectionAliasInput, ...request.Option) (*workspaces.AssociateConnectionAliasOutput, error)
	AssociateConnectionAliasRequest(*workspaces.AssociateConnectionAliasInput) (*request.Request, *workspaces.AssociateConnectionAliasOutput)

	AssociateIpGroups(*workspaces.AssociateIpGroupsInput) (*workspaces.AssociateIpGroupsOutput, error)
	AssociateIpGroupsWithContext(aws.Context, *workspaces.AssociateIpGroupsInput, ...request.Option) (*workspaces.AssociateIpGroupsOutput, error)
	AssociateIpGroupsRequest(*workspaces.AssociateIpGroupsInput) (*request.Request, *workspaces.AssociateIpGroupsOutput)

	AuthorizeIpRules(*workspaces.AuthorizeIpRulesInput) (*workspaces.AuthorizeIpRulesOutput, error)
	AuthorizeIpRulesWithContext(aws.Context, *workspaces.AuthorizeIpRulesInput, ...request.Option) (*workspaces.AuthorizeIpRulesOutput, error)
	AuthorizeIpRulesRequest(*workspaces.AuthorizeIpRulesInput) (*request.Request, *workspaces.AuthorizeIpRulesOutput)

	CopyWorkspaceImage(*workspaces.CopyWorkspaceImageInput) (*workspaces.CopyWorkspaceImageOutput, error)
	CopyWorkspaceImageWithContext(aws.Context, *workspaces.CopyWorkspaceImageInput, ...request.Option) (*workspaces.CopyWorkspaceImageOutput, error)
	CopyWorkspaceImageRequest(*workspaces.CopyWorkspaceImageInput) (*request.Request, *workspaces.CopyWorkspaceImageOutput)

	CreateConnectionAlias(*workspaces.CreateConnectionAliasInput) (*workspaces.CreateConnectionAliasOutput, error)
	CreateConnectionAliasWithContext(aws.Context, *workspaces.CreateConnectionAliasInput, ...request.Option) (*workspaces.CreateConnectionAliasOutput, error)
	CreateConnectionAliasRequest(*workspaces.CreateConnectionAliasInput) (*request.Request, *workspaces.CreateConnectionAliasOutput)

	CreateIpGroup(*workspaces.CreateIpGroupInput) (*workspaces.CreateIpGroupOutput, error)
	CreateIpGroupWithContext(aws.Context, *workspaces.CreateIpGroupInput, ...request.Option) (*workspaces.CreateIpGroupOutput, error)
	CreateIpGroupRequest(*workspaces.CreateIpGroupInput) (*request.Request, *workspaces.CreateIpGroupOutput)

	CreateTags(*workspaces.CreateTagsInput) (*workspaces.CreateTagsOutput, error)
	CreateTagsWithContext(aws.Context, *workspaces.CreateTagsInput, ...request.Option) (*workspaces.CreateTagsOutput, error)
	CreateTagsRequest(*workspaces.CreateTagsInput) (*request.Request, *workspaces.CreateTagsOutput)

	CreateWorkspaceBundle(*workspaces.CreateWorkspaceBundleInput) (*workspaces.CreateWorkspaceBundleOutput, error)
	CreateWorkspaceBundleWithContext(aws.Context, *workspaces.CreateWorkspaceBundleInput, ...request.Option) (*workspaces.CreateWorkspaceBundleOutput, error)
	CreateWorkspaceBundleRequest(*workspaces.CreateWorkspaceBundleInput) (*request.Request, *workspaces.CreateWorkspaceBundleOutput)

	CreateWorkspaces(*workspaces.CreateWorkspacesInput) (*workspaces.CreateWorkspacesOutput, error)
	CreateWorkspacesWithContext(aws.Context, *workspaces.CreateWorkspacesInput, ...request.Option) (*workspaces.CreateWorkspacesOutput, error)
	CreateWorkspacesRequest(*workspaces.CreateWorkspacesInput) (*request.Request, *workspaces.CreateWorkspacesOutput)

	DeleteConnectionAlias(*workspaces.DeleteConnectionAliasInput) (*workspaces.DeleteConnectionAliasOutput, error)
	DeleteConnectionAliasWithContext(aws.Context, *workspaces.DeleteConnectionAliasInput, ...request.Option) (*workspaces.DeleteConnectionAliasOutput, error)
	DeleteConnectionAliasRequest(*workspaces.DeleteConnectionAliasInput) (*request.Request, *workspaces.DeleteConnectionAliasOutput)

	DeleteIpGroup(*workspaces.DeleteIpGroupInput) (*workspaces.DeleteIpGroupOutput, error)
	DeleteIpGroupWithContext(aws.Context, *workspaces.DeleteIpGroupInput, ...request.Option) (*workspaces.DeleteIpGroupOutput, error)
	DeleteIpGroupRequest(*workspaces.DeleteIpGroupInput) (*request.Request, *workspaces.DeleteIpGroupOutput)

	DeleteTags(*workspaces.DeleteTagsInput) (*workspaces.DeleteTagsOutput, error)
	DeleteTagsWithContext(aws.Context, *workspaces.DeleteTagsInput, ...request.Option) (*workspaces.DeleteTagsOutput, error)
	DeleteTagsRequest(*workspaces.DeleteTagsInput) (*request.Request, *workspaces.DeleteTagsOutput)

	DeleteWorkspaceBundle(*workspaces.DeleteWorkspaceBundleInput) (*workspaces.DeleteWorkspaceBundleOutput, error)
	DeleteWorkspaceBundleWithContext(aws.Context, *workspaces.DeleteWorkspaceBundleInput, ...request.Option) (*workspaces.DeleteWorkspaceBundleOutput, error)
	DeleteWorkspaceBundleRequest(*workspaces.DeleteWorkspaceBundleInput) (*request.Request, *workspaces.DeleteWorkspaceBundleOutput)

	DeleteWorkspaceImage(*workspaces.DeleteWorkspaceImageInput) (*workspaces.DeleteWorkspaceImageOutput, error)
	DeleteWorkspaceImageWithContext(aws.Context, *workspaces.DeleteWorkspaceImageInput, ...request.Option) (*workspaces.DeleteWorkspaceImageOutput, error)
	DeleteWorkspaceImageRequest(*workspaces.DeleteWorkspaceImageInput) (*request.Request, *workspaces.DeleteWorkspaceImageOutput)

	DeregisterWorkspaceDirectory(*workspaces.DeregisterWorkspaceDirectoryInput) (*workspaces.DeregisterWorkspaceDirectoryOutput, error)
	DeregisterWorkspaceDirectoryWithContext(aws.Context, *workspaces.DeregisterWorkspaceDirectoryInput, ...request.Option) (*workspaces.DeregisterWorkspaceDirectoryOutput, error)
	DeregisterWorkspaceDirectoryRequest(*workspaces.DeregisterWorkspaceDirectoryInput) (*request.Request, *workspaces.DeregisterWorkspaceDirectoryOutput)

	DescribeAccount(*workspaces.DescribeAccountInput) (*workspaces.DescribeAccountOutput, error)
	DescribeAccountWithContext(aws.Context, *workspaces.DescribeAccountInput, ...request.Option) (*workspaces.DescribeAccountOutput, error)
	DescribeAccountRequest(*workspaces.DescribeAccountInput) (*request.Request, *workspaces.DescribeAccountOutput)

	DescribeAccountModifications(*workspaces.DescribeAccountModificationsInput) (*workspaces.DescribeAccountModificationsOutput, error)
	DescribeAccountModificationsWithContext(aws.Context, *workspaces.DescribeAccountModificationsInput, ...request.Option) (*workspaces.DescribeAccountModificationsOutput, error)
	DescribeAccountModificationsRequest(*workspaces.DescribeAccountModificationsInput) (*request.Request, *workspaces.DescribeAccountModificationsOutput)

	DescribeClientProperties(*workspaces.DescribeClientPropertiesInput) (*workspaces.DescribeClientPropertiesOutput, error)
	DescribeClientPropertiesWithContext(aws.Context, *workspaces.DescribeClientPropertiesInput, ...request.Option) (*workspaces.DescribeClientPropertiesOutput, error)
	DescribeClientPropertiesRequest(*workspaces.DescribeClientPropertiesInput) (*request.Request, *workspaces.DescribeClientPropertiesOutput)

	DescribeConnectionAliasPermissions(*workspaces.DescribeConnectionAliasPermissionsInput) (*workspaces.DescribeConnectionAliasPermissionsOutput, error)
	DescribeConnectionAliasPermissionsWithContext(aws.Context, *workspaces.DescribeConnectionAliasPermissionsInput, ...request.Option) (*workspaces.DescribeConnectionAliasPermissionsOutput, error)
	DescribeConnectionAliasPermissionsRequest(*workspaces.DescribeConnectionAliasPermissionsInput) (*request.Request, *workspaces.DescribeConnectionAliasPermissionsOutput)

	DescribeConnectionAliases(*workspaces.DescribeConnectionAliasesInput) (*workspaces.DescribeConnectionAliasesOutput, error)
	DescribeConnectionAliasesWithContext(aws.Context, *workspaces.DescribeConnectionAliasesInput, ...request.Option) (*workspaces.DescribeConnectionAliasesOutput, error)
	DescribeConnectionAliasesRequest(*workspaces.DescribeConnectionAliasesInput) (*request.Request, *workspaces.DescribeConnectionAliasesOutput)

	DescribeIpGroups(*workspaces.DescribeIpGroupsInput) (*workspaces.DescribeIpGroupsOutput, error)
	DescribeIpGroupsWithContext(aws.Context, *workspaces.DescribeIpGroupsInput, ...request.Option) (*workspaces.DescribeIpGroupsOutput, error)
	DescribeIpGroupsRequest(*workspaces.DescribeIpGroupsInput) (*request.Request, *workspaces.DescribeIpGroupsOutput)

	DescribeTags(*workspaces.DescribeTagsInput) (*workspaces.DescribeTagsOutput, error)
	DescribeTagsWithContext(aws.Context, *workspaces.DescribeTagsInput, ...request.Option) (*workspaces.DescribeTagsOutput, error)
	DescribeTagsRequest(*workspaces.DescribeTagsInput) (*request.Request, *workspaces.DescribeTagsOutput)

	DescribeWorkspaceBundles(*workspaces.DescribeWorkspaceBundlesInput) (*workspaces.DescribeWorkspaceBundlesOutput, error)
	DescribeWorkspaceBundlesWithContext(aws.Context, *workspaces.DescribeWorkspaceBundlesInput, ...request.Option) (*workspaces.DescribeWorkspaceBundlesOutput, error)
	DescribeWorkspaceBundlesRequest(*workspaces.DescribeWorkspaceBundlesInput) (*request.Request, *workspaces.DescribeWorkspaceBundlesOutput)

	DescribeWorkspaceBundlesPages(*workspaces.DescribeWorkspaceBundlesInput, func(*workspaces.DescribeWorkspaceBundlesOutput, bool) bool) error
	DescribeWorkspaceBundlesPagesWithContext(aws.Context, *workspaces.DescribeWorkspaceBundlesInput, func(*workspaces.DescribeWorkspaceBundlesOutput, bool) bool, ...request.Option) error

	DescribeWorkspaceDirectories(*workspaces.DescribeWorkspaceDirectoriesInput) (*workspaces.DescribeWorkspaceDirectoriesOutput, error)
	DescribeWorkspaceDirectoriesWithContext(aws.Context, *workspaces.DescribeWorkspaceDirectoriesInput, ...request.Option) (*workspaces.DescribeWorkspaceDirectoriesOutput, error)
	DescribeWorkspaceDirectoriesRequest(*workspaces.DescribeWorkspaceDirectoriesInput) (*request.Request, *workspaces.DescribeWorkspaceDirectoriesOutput)

	DescribeWorkspaceDirectoriesPages(*workspaces.DescribeWorkspaceDirectoriesInput, func(*workspaces.DescribeWorkspaceDirectoriesOutput, bool) bool) error
	DescribeWorkspaceDirectoriesPagesWithContext(aws.Context, *workspaces.DescribeWorkspaceDirectoriesInput, func(*workspaces.DescribeWorkspaceDirectoriesOutput, bool) bool, ...request.Option) error

	DescribeWorkspaceImagePermissions(*workspaces.DescribeWorkspaceImagePermissionsInput) (*workspaces.DescribeWorkspaceImagePermissionsOutput, error)
	DescribeWorkspaceImagePermissionsWithContext(aws.Context, *workspaces.DescribeWorkspaceImagePermissionsInput, ...request.Option) (*workspaces.DescribeWorkspaceImagePermissionsOutput, error)
	DescribeWorkspaceImagePermissionsRequest(*workspaces.DescribeWorkspaceImagePermissionsInput) (*request.Request, *workspaces.DescribeWorkspaceImagePermissionsOutput)

	DescribeWorkspaceImages(*workspaces.DescribeWorkspaceImagesInput) (*workspaces.DescribeWorkspaceImagesOutput, error)
	DescribeWorkspaceImagesWithContext(aws.Context, *workspaces.DescribeWorkspaceImagesInput, ...request.Option) (*workspaces.DescribeWorkspaceImagesOutput, error)
	DescribeWorkspaceImagesRequest(*workspaces.DescribeWorkspaceImagesInput) (*request.Request, *workspaces.DescribeWorkspaceImagesOutput)

	DescribeWorkspaceSnapshots(*workspaces.DescribeWorkspaceSnapshotsInput) (*workspaces.DescribeWorkspaceSnapshotsOutput, error)
	DescribeWorkspaceSnapshotsWithContext(aws.Context, *workspaces.DescribeWorkspaceSnapshotsInput, ...request.Option) (*workspaces.DescribeWorkspaceSnapshotsOutput, error)
	DescribeWorkspaceSnapshotsRequest(*workspaces.DescribeWorkspaceSnapshotsInput) (*request.Request, *workspaces.DescribeWorkspaceSnapshotsOutput)

	DescribeWorkspaces(*workspaces.DescribeWorkspacesInput) (*workspaces.DescribeWorkspacesOutput, error)
	DescribeWorkspacesWithContext(aws.Context, *workspaces.DescribeWorkspacesInput, ...request.Option) (*workspaces.DescribeWorkspacesOutput, error)
	DescribeWorkspacesRequest(*workspaces.DescribeWorkspacesInput) (*request.Request, *workspaces.DescribeWorkspacesOutput)

	DescribeWorkspacesPages(*workspaces.DescribeWorkspacesInput, func(*workspaces.DescribeWorkspacesOutput, bool) bool) error
	DescribeWorkspacesPagesWithContext(aws.Context, *workspaces.DescribeWorkspacesInput, func(*workspaces.DescribeWorkspacesOutput, bool) bool, ...request.Option) error

	DescribeWorkspacesConnectionStatus(*workspaces.DescribeWorkspacesConnectionStatusInput) (*workspaces.DescribeWorkspacesConnectionStatusOutput, error)
	DescribeWorkspacesConnectionStatusWithContext(aws.Context, *workspaces.DescribeWorkspacesConnectionStatusInput, ...request.Option) (*workspaces.DescribeWorkspacesConnectionStatusOutput, error)
	DescribeWorkspacesConnectionStatusRequest(*workspaces.DescribeWorkspacesConnectionStatusInput) (*request.Request, *workspaces.DescribeWorkspacesConnectionStatusOutput)

	DisassociateConnectionAlias(*workspaces.DisassociateConnectionAliasInput) (*workspaces.DisassociateConnectionAliasOutput, error)
	DisassociateConnectionAliasWithContext(aws.Context, *workspaces.DisassociateConnectionAliasInput, ...request.Option) (*workspaces.DisassociateConnectionAliasOutput, error)
	DisassociateConnectionAliasRequest(*workspaces.DisassociateConnectionAliasInput) (*request.Request, *workspaces.DisassociateConnectionAliasOutput)

	DisassociateIpGroups(*workspaces.DisassociateIpGroupsInput) (*workspaces.DisassociateIpGroupsOutput, error)
	DisassociateIpGroupsWithContext(aws.Context, *workspaces.DisassociateIpGroupsInput, ...request.Option) (*workspaces.DisassociateIpGroupsOutput, error)
	DisassociateIpGroupsRequest(*workspaces.DisassociateIpGroupsInput) (*request.Request, *workspaces.DisassociateIpGroupsOutput)

	ImportWorkspaceImage(*workspaces.ImportWorkspaceImageInput) (*workspaces.ImportWorkspaceImageOutput, error)
	ImportWorkspaceImageWithContext(aws.Context, *workspaces.ImportWorkspaceImageInput, ...request.Option) (*workspaces.ImportWorkspaceImageOutput, error)
	ImportWorkspaceImageRequest(*workspaces.ImportWorkspaceImageInput) (*request.Request, *workspaces.ImportWorkspaceImageOutput)

	ListAvailableManagementCidrRanges(*workspaces.ListAvailableManagementCidrRangesInput) (*workspaces.ListAvailableManagementCidrRangesOutput, error)
	ListAvailableManagementCidrRangesWithContext(aws.Context, *workspaces.ListAvailableManagementCidrRangesInput, ...request.Option) (*workspaces.ListAvailableManagementCidrRangesOutput, error)
	ListAvailableManagementCidrRangesRequest(*workspaces.ListAvailableManagementCidrRangesInput) (*request.Request, *workspaces.ListAvailableManagementCidrRangesOutput)

	MigrateWorkspace(*workspaces.MigrateWorkspaceInput) (*workspaces.MigrateWorkspaceOutput, error)
	MigrateWorkspaceWithContext(aws.Context, *workspaces.MigrateWorkspaceInput, ...request.Option) (*workspaces.MigrateWorkspaceOutput, error)
	MigrateWorkspaceRequest(*workspaces.MigrateWorkspaceInput) (*request.Request, *workspaces.MigrateWorkspaceOutput)

	ModifyAccount(*workspaces.ModifyAccountInput) (*workspaces.ModifyAccountOutput, error)
	ModifyAccountWithContext(aws.Context, *workspaces.ModifyAccountInput, ...request.Option) (*workspaces.ModifyAccountOutput, error)
	ModifyAccountRequest(*workspaces.ModifyAccountInput) (*request.Request, *workspaces.ModifyAccountOutput)

	ModifyClientProperties(*workspaces.ModifyClientPropertiesInput) (*workspaces.ModifyClientPropertiesOutput, error)
	ModifyClientPropertiesWithContext(aws.Context, *workspaces.ModifyClientPropertiesInput, ...request.Option) (*workspaces.ModifyClientPropertiesOutput, error)
	ModifyClientPropertiesRequest(*workspaces.ModifyClientPropertiesInput) (*request.Request, *workspaces.ModifyClientPropertiesOutput)

	ModifySelfservicePermissions(*workspaces.ModifySelfservicePermissionsInput) (*workspaces.ModifySelfservicePermissionsOutput, error)
	ModifySelfservicePermissionsWithContext(aws.Context, *workspaces.ModifySelfservicePermissionsInput, ...request.Option) (*workspaces.ModifySelfservicePermissionsOutput, error)
	ModifySelfservicePermissionsRequest(*workspaces.ModifySelfservicePermissionsInput) (*request.Request, *workspaces.ModifySelfservicePermissionsOutput)

	ModifyWorkspaceAccessProperties(*workspaces.ModifyWorkspaceAccessPropertiesInput) (*workspaces.ModifyWorkspaceAccessPropertiesOutput, error)
	ModifyWorkspaceAccessPropertiesWithContext(aws.Context, *workspaces.ModifyWorkspaceAccessPropertiesInput, ...request.Option) (*workspaces.ModifyWorkspaceAccessPropertiesOutput, error)
	ModifyWorkspaceAccessPropertiesRequest(*workspaces.ModifyWorkspaceAccessPropertiesInput) (*request.Request, *workspaces.ModifyWorkspaceAccessPropertiesOutput)

	ModifyWorkspaceCreationProperties(*workspaces.ModifyWorkspaceCreationPropertiesInput) (*workspaces.ModifyWorkspaceCreationPropertiesOutput, error)
	ModifyWorkspaceCreationPropertiesWithContext(aws.Context, *workspaces.ModifyWorkspaceCreationPropertiesInput, ...request.Option) (*workspaces.ModifyWorkspaceCreationPropertiesOutput, error)
	ModifyWorkspaceCreationPropertiesRequest(*workspaces.ModifyWorkspaceCreationPropertiesInput) (*request.Request, *workspaces.ModifyWorkspaceCreationPropertiesOutput)

	ModifyWorkspaceProperties(*workspaces.ModifyWorkspacePropertiesInput) (*workspaces.ModifyWorkspacePropertiesOutput, error)
	ModifyWorkspacePropertiesWithContext(aws.Context, *workspaces.ModifyWorkspacePropertiesInput, ...request.Option) (*workspaces.ModifyWorkspacePropertiesOutput, error)
	ModifyWorkspacePropertiesRequest(*workspaces.ModifyWorkspacePropertiesInput) (*request.Request, *workspaces.ModifyWorkspacePropertiesOutput)

	ModifyWorkspaceState(*workspaces.ModifyWorkspaceStateInput) (*workspaces.ModifyWorkspaceStateOutput, error)
	ModifyWorkspaceStateWithContext(aws.Context, *workspaces.ModifyWorkspaceStateInput, ...request.Option) (*workspaces.ModifyWorkspaceStateOutput, error)
	ModifyWorkspaceStateRequest(*workspaces.ModifyWorkspaceStateInput) (*request.Request, *workspaces.ModifyWorkspaceStateOutput)

	RebootWorkspaces(*workspaces.RebootWorkspacesInput) (*workspaces.RebootWorkspacesOutput, error)
	RebootWorkspacesWithContext(aws.Context, *workspaces.RebootWorkspacesInput, ...request.Option) (*workspaces.RebootWorkspacesOutput, error)
	RebootWorkspacesRequest(*workspaces.RebootWorkspacesInput) (*request.Request, *workspaces.RebootWorkspacesOutput)

	RebuildWorkspaces(*workspaces.RebuildWorkspacesInput) (*workspaces.RebuildWorkspacesOutput, error)
	RebuildWorkspacesWithContext(aws.Context, *workspaces.RebuildWorkspacesInput, ...request.Option) (*workspaces.RebuildWorkspacesOutput, error)
	RebuildWorkspacesRequest(*workspaces.RebuildWorkspacesInput) (*request.Request, *workspaces.RebuildWorkspacesOutput)

	RegisterWorkspaceDirectory(*workspaces.RegisterWorkspaceDirectoryInput) (*workspaces.RegisterWorkspaceDirectoryOutput, error)
	RegisterWorkspaceDirectoryWithContext(aws.Context, *workspaces.RegisterWorkspaceDirectoryInput, ...request.Option) (*workspaces.RegisterWorkspaceDirectoryOutput, error)
	RegisterWorkspaceDirectoryRequest(*workspaces.RegisterWorkspaceDirectoryInput) (*request.Request, *workspaces.RegisterWorkspaceDirectoryOutput)

	RestoreWorkspace(*workspaces.RestoreWorkspaceInput) (*workspaces.RestoreWorkspaceOutput, error)
	RestoreWorkspaceWithContext(aws.Context, *workspaces.RestoreWorkspaceInput, ...request.Option) (*workspaces.RestoreWorkspaceOutput, error)
	RestoreWorkspaceRequest(*workspaces.RestoreWorkspaceInput) (*request.Request, *workspaces.RestoreWorkspaceOutput)

	RevokeIpRules(*workspaces.RevokeIpRulesInput) (*workspaces.RevokeIpRulesOutput, error)
	RevokeIpRulesWithContext(aws.Context, *workspaces.RevokeIpRulesInput, ...request.Option) (*workspaces.RevokeIpRulesOutput, error)
	RevokeIpRulesRequest(*workspaces.RevokeIpRulesInput) (*request.Request, *workspaces.RevokeIpRulesOutput)

	StartWorkspaces(*workspaces.StartWorkspacesInput) (*workspaces.StartWorkspacesOutput, error)
	StartWorkspacesWithContext(aws.Context, *workspaces.StartWorkspacesInput, ...request.Option) (*workspaces.StartWorkspacesOutput, error)
	StartWorkspacesRequest(*workspaces.StartWorkspacesInput) (*request.Request, *workspaces.StartWorkspacesOutput)

	StopWorkspaces(*workspaces.StopWorkspacesInput) (*workspaces.StopWorkspacesOutput, error)
	StopWorkspacesWithContext(aws.Context, *workspaces.StopWorkspacesInput, ...request.Option) (*workspaces.StopWorkspacesOutput, error)
	StopWorkspacesRequest(*workspaces.StopWorkspacesInput) (*request.Request, *workspaces.StopWorkspacesOutput)

	TerminateWorkspaces(*workspaces.TerminateWorkspacesInput) (*workspaces.TerminateWorkspacesOutput, error)
	TerminateWorkspacesWithContext(aws.Context, *workspaces.TerminateWorkspacesInput, ...request.Option) (*workspaces.TerminateWorkspacesOutput, error)
	TerminateWorkspacesRequest(*workspaces.TerminateWorkspacesInput) (*request.Request, *workspaces.TerminateWorkspacesOutput)

	UpdateConnectionAliasPermission(*workspaces.UpdateConnectionAliasPermissionInput) (*workspaces.UpdateConnectionAliasPermissionOutput, error)
	UpdateConnectionAliasPermissionWithContext(aws.Context, *workspaces.UpdateConnectionAliasPermissionInput, ...request.Option) (*workspaces.UpdateConnectionAliasPermissionOutput, error)
	UpdateConnectionAliasPermissionRequest(*workspaces.UpdateConnectionAliasPermissionInput) (*request.Request, *workspaces.UpdateConnectionAliasPermissionOutput)

	UpdateRulesOfIpGroup(*workspaces.UpdateRulesOfIpGroupInput) (*workspaces.UpdateRulesOfIpGroupOutput, error)
	UpdateRulesOfIpGroupWithContext(aws.Context, *workspaces.UpdateRulesOfIpGroupInput, ...request.Option) (*workspaces.UpdateRulesOfIpGroupOutput, error)
	UpdateRulesOfIpGroupRequest(*workspaces.UpdateRulesOfIpGroupInput) (*request.Request, *workspaces.UpdateRulesOfIpGroupOutput)

	UpdateWorkspaceBundle(*workspaces.UpdateWorkspaceBundleInput) (*workspaces.UpdateWorkspaceBundleOutput, error)
	UpdateWorkspaceBundleWithContext(aws.Context, *workspaces.UpdateWorkspaceBundleInput, ...request.Option) (*workspaces.UpdateWorkspaceBundleOutput, error)
	UpdateWorkspaceBundleRequest(*workspaces.UpdateWorkspaceBundleInput) (*request.Request, *workspaces.UpdateWorkspaceBundleOutput)

	UpdateWorkspaceImagePermission(*workspaces.UpdateWorkspaceImagePermissionInput) (*workspaces.UpdateWorkspaceImagePermissionOutput, error)
	UpdateWorkspaceImagePermissionWithContext(aws.Context, *workspaces.UpdateWorkspaceImagePermissionInput, ...request.Option) (*workspaces.UpdateWorkspaceImagePermissionOutput, error)
	UpdateWorkspaceImagePermissionRequest(*workspaces.UpdateWorkspaceImagePermissionInput) (*request.Request, *workspaces.UpdateWorkspaceImagePermissionOutput)
}

var _ WorkSpacesAPI = (*workspaces.WorkSpaces)(nil)
