// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

// Package workmailiface provides an interface to enable mocking the Amazon WorkMail service client
// for testing your code.
//
// It is important to note that this interface will have breaking changes
// when the service model is updated and adds new API operations, paginators,
// and waiters.
package workmailiface

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/workmail"
)

// WorkMailAPI provides an interface to enable mocking the
// workmail.WorkMail service client's API operation,
// paginators, and waiters. This make unit testing your code that calls out
// to the SDK's service client's calls easier.
//
// The best way to use this interface is so the SDK's service client's calls
// can be stubbed out for unit testing your code with the SDK without needing
// to inject custom request handlers into the SDK's request pipeline.
//
//    // myFunc uses an SDK service client to make a request to
//    // Amazon WorkMail.
//    func myFunc(svc workmailiface.WorkMailAPI) bool {
//        // Make svc.AssociateDelegateToResource request
//    }
//
//    func main() {
//        sess := session.New()
//        svc := workmail.New(sess)
//
//        myFunc(svc)
//    }
//
// In your _test.go file:
//
//    // Define a mock struct to be used in your unit tests of myFunc.
//    type mockWorkMailClient struct {
//        workmailiface.WorkMailAPI
//    }
//    func (m *mockWorkMailClient) AssociateDelegateToResource(input *workmail.AssociateDelegateToResourceInput) (*workmail.AssociateDelegateToResourceOutput, error) {
//        // mock response/functionality
//    }
//
//    func TestMyFunc(t *testing.T) {
//        // Setup Test
//        mockSvc := &mockWorkMailClient{}
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
type WorkMailAPI interface {
	AssociateDelegateToResource(*workmail.AssociateDelegateToResourceInput) (*workmail.AssociateDelegateToResourceOutput, error)
	AssociateDelegateToResourceWithContext(aws.Context, *workmail.AssociateDelegateToResourceInput, ...request.Option) (*workmail.AssociateDelegateToResourceOutput, error)
	AssociateDelegateToResourceRequest(*workmail.AssociateDelegateToResourceInput) (*request.Request, *workmail.AssociateDelegateToResourceOutput)

	AssociateMemberToGroup(*workmail.AssociateMemberToGroupInput) (*workmail.AssociateMemberToGroupOutput, error)
	AssociateMemberToGroupWithContext(aws.Context, *workmail.AssociateMemberToGroupInput, ...request.Option) (*workmail.AssociateMemberToGroupOutput, error)
	AssociateMemberToGroupRequest(*workmail.AssociateMemberToGroupInput) (*request.Request, *workmail.AssociateMemberToGroupOutput)

	CancelMailboxExportJob(*workmail.CancelMailboxExportJobInput) (*workmail.CancelMailboxExportJobOutput, error)
	CancelMailboxExportJobWithContext(aws.Context, *workmail.CancelMailboxExportJobInput, ...request.Option) (*workmail.CancelMailboxExportJobOutput, error)
	CancelMailboxExportJobRequest(*workmail.CancelMailboxExportJobInput) (*request.Request, *workmail.CancelMailboxExportJobOutput)

	CreateAlias(*workmail.CreateAliasInput) (*workmail.CreateAliasOutput, error)
	CreateAliasWithContext(aws.Context, *workmail.CreateAliasInput, ...request.Option) (*workmail.CreateAliasOutput, error)
	CreateAliasRequest(*workmail.CreateAliasInput) (*request.Request, *workmail.CreateAliasOutput)

	CreateGroup(*workmail.CreateGroupInput) (*workmail.CreateGroupOutput, error)
	CreateGroupWithContext(aws.Context, *workmail.CreateGroupInput, ...request.Option) (*workmail.CreateGroupOutput, error)
	CreateGroupRequest(*workmail.CreateGroupInput) (*request.Request, *workmail.CreateGroupOutput)

	CreateMobileDeviceAccessRule(*workmail.CreateMobileDeviceAccessRuleInput) (*workmail.CreateMobileDeviceAccessRuleOutput, error)
	CreateMobileDeviceAccessRuleWithContext(aws.Context, *workmail.CreateMobileDeviceAccessRuleInput, ...request.Option) (*workmail.CreateMobileDeviceAccessRuleOutput, error)
	CreateMobileDeviceAccessRuleRequest(*workmail.CreateMobileDeviceAccessRuleInput) (*request.Request, *workmail.CreateMobileDeviceAccessRuleOutput)

	CreateOrganization(*workmail.CreateOrganizationInput) (*workmail.CreateOrganizationOutput, error)
	CreateOrganizationWithContext(aws.Context, *workmail.CreateOrganizationInput, ...request.Option) (*workmail.CreateOrganizationOutput, error)
	CreateOrganizationRequest(*workmail.CreateOrganizationInput) (*request.Request, *workmail.CreateOrganizationOutput)

	CreateResource(*workmail.CreateResourceInput) (*workmail.CreateResourceOutput, error)
	CreateResourceWithContext(aws.Context, *workmail.CreateResourceInput, ...request.Option) (*workmail.CreateResourceOutput, error)
	CreateResourceRequest(*workmail.CreateResourceInput) (*request.Request, *workmail.CreateResourceOutput)

	CreateUser(*workmail.CreateUserInput) (*workmail.CreateUserOutput, error)
	CreateUserWithContext(aws.Context, *workmail.CreateUserInput, ...request.Option) (*workmail.CreateUserOutput, error)
	CreateUserRequest(*workmail.CreateUserInput) (*request.Request, *workmail.CreateUserOutput)

	DeleteAccessControlRule(*workmail.DeleteAccessControlRuleInput) (*workmail.DeleteAccessControlRuleOutput, error)
	DeleteAccessControlRuleWithContext(aws.Context, *workmail.DeleteAccessControlRuleInput, ...request.Option) (*workmail.DeleteAccessControlRuleOutput, error)
	DeleteAccessControlRuleRequest(*workmail.DeleteAccessControlRuleInput) (*request.Request, *workmail.DeleteAccessControlRuleOutput)

	DeleteAlias(*workmail.DeleteAliasInput) (*workmail.DeleteAliasOutput, error)
	DeleteAliasWithContext(aws.Context, *workmail.DeleteAliasInput, ...request.Option) (*workmail.DeleteAliasOutput, error)
	DeleteAliasRequest(*workmail.DeleteAliasInput) (*request.Request, *workmail.DeleteAliasOutput)

	DeleteGroup(*workmail.DeleteGroupInput) (*workmail.DeleteGroupOutput, error)
	DeleteGroupWithContext(aws.Context, *workmail.DeleteGroupInput, ...request.Option) (*workmail.DeleteGroupOutput, error)
	DeleteGroupRequest(*workmail.DeleteGroupInput) (*request.Request, *workmail.DeleteGroupOutput)

	DeleteMailboxPermissions(*workmail.DeleteMailboxPermissionsInput) (*workmail.DeleteMailboxPermissionsOutput, error)
	DeleteMailboxPermissionsWithContext(aws.Context, *workmail.DeleteMailboxPermissionsInput, ...request.Option) (*workmail.DeleteMailboxPermissionsOutput, error)
	DeleteMailboxPermissionsRequest(*workmail.DeleteMailboxPermissionsInput) (*request.Request, *workmail.DeleteMailboxPermissionsOutput)

	DeleteMobileDeviceAccessRule(*workmail.DeleteMobileDeviceAccessRuleInput) (*workmail.DeleteMobileDeviceAccessRuleOutput, error)
	DeleteMobileDeviceAccessRuleWithContext(aws.Context, *workmail.DeleteMobileDeviceAccessRuleInput, ...request.Option) (*workmail.DeleteMobileDeviceAccessRuleOutput, error)
	DeleteMobileDeviceAccessRuleRequest(*workmail.DeleteMobileDeviceAccessRuleInput) (*request.Request, *workmail.DeleteMobileDeviceAccessRuleOutput)

	DeleteOrganization(*workmail.DeleteOrganizationInput) (*workmail.DeleteOrganizationOutput, error)
	DeleteOrganizationWithContext(aws.Context, *workmail.DeleteOrganizationInput, ...request.Option) (*workmail.DeleteOrganizationOutput, error)
	DeleteOrganizationRequest(*workmail.DeleteOrganizationInput) (*request.Request, *workmail.DeleteOrganizationOutput)

	DeleteResource(*workmail.DeleteResourceInput) (*workmail.DeleteResourceOutput, error)
	DeleteResourceWithContext(aws.Context, *workmail.DeleteResourceInput, ...request.Option) (*workmail.DeleteResourceOutput, error)
	DeleteResourceRequest(*workmail.DeleteResourceInput) (*request.Request, *workmail.DeleteResourceOutput)

	DeleteRetentionPolicy(*workmail.DeleteRetentionPolicyInput) (*workmail.DeleteRetentionPolicyOutput, error)
	DeleteRetentionPolicyWithContext(aws.Context, *workmail.DeleteRetentionPolicyInput, ...request.Option) (*workmail.DeleteRetentionPolicyOutput, error)
	DeleteRetentionPolicyRequest(*workmail.DeleteRetentionPolicyInput) (*request.Request, *workmail.DeleteRetentionPolicyOutput)

	DeleteUser(*workmail.DeleteUserInput) (*workmail.DeleteUserOutput, error)
	DeleteUserWithContext(aws.Context, *workmail.DeleteUserInput, ...request.Option) (*workmail.DeleteUserOutput, error)
	DeleteUserRequest(*workmail.DeleteUserInput) (*request.Request, *workmail.DeleteUserOutput)

	DeregisterFromWorkMail(*workmail.DeregisterFromWorkMailInput) (*workmail.DeregisterFromWorkMailOutput, error)
	DeregisterFromWorkMailWithContext(aws.Context, *workmail.DeregisterFromWorkMailInput, ...request.Option) (*workmail.DeregisterFromWorkMailOutput, error)
	DeregisterFromWorkMailRequest(*workmail.DeregisterFromWorkMailInput) (*request.Request, *workmail.DeregisterFromWorkMailOutput)

	DescribeGroup(*workmail.DescribeGroupInput) (*workmail.DescribeGroupOutput, error)
	DescribeGroupWithContext(aws.Context, *workmail.DescribeGroupInput, ...request.Option) (*workmail.DescribeGroupOutput, error)
	DescribeGroupRequest(*workmail.DescribeGroupInput) (*request.Request, *workmail.DescribeGroupOutput)

	DescribeMailboxExportJob(*workmail.DescribeMailboxExportJobInput) (*workmail.DescribeMailboxExportJobOutput, error)
	DescribeMailboxExportJobWithContext(aws.Context, *workmail.DescribeMailboxExportJobInput, ...request.Option) (*workmail.DescribeMailboxExportJobOutput, error)
	DescribeMailboxExportJobRequest(*workmail.DescribeMailboxExportJobInput) (*request.Request, *workmail.DescribeMailboxExportJobOutput)

	DescribeOrganization(*workmail.DescribeOrganizationInput) (*workmail.DescribeOrganizationOutput, error)
	DescribeOrganizationWithContext(aws.Context, *workmail.DescribeOrganizationInput, ...request.Option) (*workmail.DescribeOrganizationOutput, error)
	DescribeOrganizationRequest(*workmail.DescribeOrganizationInput) (*request.Request, *workmail.DescribeOrganizationOutput)

	DescribeResource(*workmail.DescribeResourceInput) (*workmail.DescribeResourceOutput, error)
	DescribeResourceWithContext(aws.Context, *workmail.DescribeResourceInput, ...request.Option) (*workmail.DescribeResourceOutput, error)
	DescribeResourceRequest(*workmail.DescribeResourceInput) (*request.Request, *workmail.DescribeResourceOutput)

	DescribeUser(*workmail.DescribeUserInput) (*workmail.DescribeUserOutput, error)
	DescribeUserWithContext(aws.Context, *workmail.DescribeUserInput, ...request.Option) (*workmail.DescribeUserOutput, error)
	DescribeUserRequest(*workmail.DescribeUserInput) (*request.Request, *workmail.DescribeUserOutput)

	DisassociateDelegateFromResource(*workmail.DisassociateDelegateFromResourceInput) (*workmail.DisassociateDelegateFromResourceOutput, error)
	DisassociateDelegateFromResourceWithContext(aws.Context, *workmail.DisassociateDelegateFromResourceInput, ...request.Option) (*workmail.DisassociateDelegateFromResourceOutput, error)
	DisassociateDelegateFromResourceRequest(*workmail.DisassociateDelegateFromResourceInput) (*request.Request, *workmail.DisassociateDelegateFromResourceOutput)

	DisassociateMemberFromGroup(*workmail.DisassociateMemberFromGroupInput) (*workmail.DisassociateMemberFromGroupOutput, error)
	DisassociateMemberFromGroupWithContext(aws.Context, *workmail.DisassociateMemberFromGroupInput, ...request.Option) (*workmail.DisassociateMemberFromGroupOutput, error)
	DisassociateMemberFromGroupRequest(*workmail.DisassociateMemberFromGroupInput) (*request.Request, *workmail.DisassociateMemberFromGroupOutput)

	GetAccessControlEffect(*workmail.GetAccessControlEffectInput) (*workmail.GetAccessControlEffectOutput, error)
	GetAccessControlEffectWithContext(aws.Context, *workmail.GetAccessControlEffectInput, ...request.Option) (*workmail.GetAccessControlEffectOutput, error)
	GetAccessControlEffectRequest(*workmail.GetAccessControlEffectInput) (*request.Request, *workmail.GetAccessControlEffectOutput)

	GetDefaultRetentionPolicy(*workmail.GetDefaultRetentionPolicyInput) (*workmail.GetDefaultRetentionPolicyOutput, error)
	GetDefaultRetentionPolicyWithContext(aws.Context, *workmail.GetDefaultRetentionPolicyInput, ...request.Option) (*workmail.GetDefaultRetentionPolicyOutput, error)
	GetDefaultRetentionPolicyRequest(*workmail.GetDefaultRetentionPolicyInput) (*request.Request, *workmail.GetDefaultRetentionPolicyOutput)

	GetMailboxDetails(*workmail.GetMailboxDetailsInput) (*workmail.GetMailboxDetailsOutput, error)
	GetMailboxDetailsWithContext(aws.Context, *workmail.GetMailboxDetailsInput, ...request.Option) (*workmail.GetMailboxDetailsOutput, error)
	GetMailboxDetailsRequest(*workmail.GetMailboxDetailsInput) (*request.Request, *workmail.GetMailboxDetailsOutput)

	GetMobileDeviceAccessEffect(*workmail.GetMobileDeviceAccessEffectInput) (*workmail.GetMobileDeviceAccessEffectOutput, error)
	GetMobileDeviceAccessEffectWithContext(aws.Context, *workmail.GetMobileDeviceAccessEffectInput, ...request.Option) (*workmail.GetMobileDeviceAccessEffectOutput, error)
	GetMobileDeviceAccessEffectRequest(*workmail.GetMobileDeviceAccessEffectInput) (*request.Request, *workmail.GetMobileDeviceAccessEffectOutput)

	ListAccessControlRules(*workmail.ListAccessControlRulesInput) (*workmail.ListAccessControlRulesOutput, error)
	ListAccessControlRulesWithContext(aws.Context, *workmail.ListAccessControlRulesInput, ...request.Option) (*workmail.ListAccessControlRulesOutput, error)
	ListAccessControlRulesRequest(*workmail.ListAccessControlRulesInput) (*request.Request, *workmail.ListAccessControlRulesOutput)

	ListAliases(*workmail.ListAliasesInput) (*workmail.ListAliasesOutput, error)
	ListAliasesWithContext(aws.Context, *workmail.ListAliasesInput, ...request.Option) (*workmail.ListAliasesOutput, error)
	ListAliasesRequest(*workmail.ListAliasesInput) (*request.Request, *workmail.ListAliasesOutput)

	ListAliasesPages(*workmail.ListAliasesInput, func(*workmail.ListAliasesOutput, bool) bool) error
	ListAliasesPagesWithContext(aws.Context, *workmail.ListAliasesInput, func(*workmail.ListAliasesOutput, bool) bool, ...request.Option) error

	ListGroupMembers(*workmail.ListGroupMembersInput) (*workmail.ListGroupMembersOutput, error)
	ListGroupMembersWithContext(aws.Context, *workmail.ListGroupMembersInput, ...request.Option) (*workmail.ListGroupMembersOutput, error)
	ListGroupMembersRequest(*workmail.ListGroupMembersInput) (*request.Request, *workmail.ListGroupMembersOutput)

	ListGroupMembersPages(*workmail.ListGroupMembersInput, func(*workmail.ListGroupMembersOutput, bool) bool) error
	ListGroupMembersPagesWithContext(aws.Context, *workmail.ListGroupMembersInput, func(*workmail.ListGroupMembersOutput, bool) bool, ...request.Option) error

	ListGroups(*workmail.ListGroupsInput) (*workmail.ListGroupsOutput, error)
	ListGroupsWithContext(aws.Context, *workmail.ListGroupsInput, ...request.Option) (*workmail.ListGroupsOutput, error)
	ListGroupsRequest(*workmail.ListGroupsInput) (*request.Request, *workmail.ListGroupsOutput)

	ListGroupsPages(*workmail.ListGroupsInput, func(*workmail.ListGroupsOutput, bool) bool) error
	ListGroupsPagesWithContext(aws.Context, *workmail.ListGroupsInput, func(*workmail.ListGroupsOutput, bool) bool, ...request.Option) error

	ListMailboxExportJobs(*workmail.ListMailboxExportJobsInput) (*workmail.ListMailboxExportJobsOutput, error)
	ListMailboxExportJobsWithContext(aws.Context, *workmail.ListMailboxExportJobsInput, ...request.Option) (*workmail.ListMailboxExportJobsOutput, error)
	ListMailboxExportJobsRequest(*workmail.ListMailboxExportJobsInput) (*request.Request, *workmail.ListMailboxExportJobsOutput)

	ListMailboxExportJobsPages(*workmail.ListMailboxExportJobsInput, func(*workmail.ListMailboxExportJobsOutput, bool) bool) error
	ListMailboxExportJobsPagesWithContext(aws.Context, *workmail.ListMailboxExportJobsInput, func(*workmail.ListMailboxExportJobsOutput, bool) bool, ...request.Option) error

	ListMailboxPermissions(*workmail.ListMailboxPermissionsInput) (*workmail.ListMailboxPermissionsOutput, error)
	ListMailboxPermissionsWithContext(aws.Context, *workmail.ListMailboxPermissionsInput, ...request.Option) (*workmail.ListMailboxPermissionsOutput, error)
	ListMailboxPermissionsRequest(*workmail.ListMailboxPermissionsInput) (*request.Request, *workmail.ListMailboxPermissionsOutput)

	ListMailboxPermissionsPages(*workmail.ListMailboxPermissionsInput, func(*workmail.ListMailboxPermissionsOutput, bool) bool) error
	ListMailboxPermissionsPagesWithContext(aws.Context, *workmail.ListMailboxPermissionsInput, func(*workmail.ListMailboxPermissionsOutput, bool) bool, ...request.Option) error

	ListMobileDeviceAccessRules(*workmail.ListMobileDeviceAccessRulesInput) (*workmail.ListMobileDeviceAccessRulesOutput, error)
	ListMobileDeviceAccessRulesWithContext(aws.Context, *workmail.ListMobileDeviceAccessRulesInput, ...request.Option) (*workmail.ListMobileDeviceAccessRulesOutput, error)
	ListMobileDeviceAccessRulesRequest(*workmail.ListMobileDeviceAccessRulesInput) (*request.Request, *workmail.ListMobileDeviceAccessRulesOutput)

	ListOrganizations(*workmail.ListOrganizationsInput) (*workmail.ListOrganizationsOutput, error)
	ListOrganizationsWithContext(aws.Context, *workmail.ListOrganizationsInput, ...request.Option) (*workmail.ListOrganizationsOutput, error)
	ListOrganizationsRequest(*workmail.ListOrganizationsInput) (*request.Request, *workmail.ListOrganizationsOutput)

	ListOrganizationsPages(*workmail.ListOrganizationsInput, func(*workmail.ListOrganizationsOutput, bool) bool) error
	ListOrganizationsPagesWithContext(aws.Context, *workmail.ListOrganizationsInput, func(*workmail.ListOrganizationsOutput, bool) bool, ...request.Option) error

	ListResourceDelegates(*workmail.ListResourceDelegatesInput) (*workmail.ListResourceDelegatesOutput, error)
	ListResourceDelegatesWithContext(aws.Context, *workmail.ListResourceDelegatesInput, ...request.Option) (*workmail.ListResourceDelegatesOutput, error)
	ListResourceDelegatesRequest(*workmail.ListResourceDelegatesInput) (*request.Request, *workmail.ListResourceDelegatesOutput)

	ListResourceDelegatesPages(*workmail.ListResourceDelegatesInput, func(*workmail.ListResourceDelegatesOutput, bool) bool) error
	ListResourceDelegatesPagesWithContext(aws.Context, *workmail.ListResourceDelegatesInput, func(*workmail.ListResourceDelegatesOutput, bool) bool, ...request.Option) error

	ListResources(*workmail.ListResourcesInput) (*workmail.ListResourcesOutput, error)
	ListResourcesWithContext(aws.Context, *workmail.ListResourcesInput, ...request.Option) (*workmail.ListResourcesOutput, error)
	ListResourcesRequest(*workmail.ListResourcesInput) (*request.Request, *workmail.ListResourcesOutput)

	ListResourcesPages(*workmail.ListResourcesInput, func(*workmail.ListResourcesOutput, bool) bool) error
	ListResourcesPagesWithContext(aws.Context, *workmail.ListResourcesInput, func(*workmail.ListResourcesOutput, bool) bool, ...request.Option) error

	ListTagsForResource(*workmail.ListTagsForResourceInput) (*workmail.ListTagsForResourceOutput, error)
	ListTagsForResourceWithContext(aws.Context, *workmail.ListTagsForResourceInput, ...request.Option) (*workmail.ListTagsForResourceOutput, error)
	ListTagsForResourceRequest(*workmail.ListTagsForResourceInput) (*request.Request, *workmail.ListTagsForResourceOutput)

	ListUsers(*workmail.ListUsersInput) (*workmail.ListUsersOutput, error)
	ListUsersWithContext(aws.Context, *workmail.ListUsersInput, ...request.Option) (*workmail.ListUsersOutput, error)
	ListUsersRequest(*workmail.ListUsersInput) (*request.Request, *workmail.ListUsersOutput)

	ListUsersPages(*workmail.ListUsersInput, func(*workmail.ListUsersOutput, bool) bool) error
	ListUsersPagesWithContext(aws.Context, *workmail.ListUsersInput, func(*workmail.ListUsersOutput, bool) bool, ...request.Option) error

	PutAccessControlRule(*workmail.PutAccessControlRuleInput) (*workmail.PutAccessControlRuleOutput, error)
	PutAccessControlRuleWithContext(aws.Context, *workmail.PutAccessControlRuleInput, ...request.Option) (*workmail.PutAccessControlRuleOutput, error)
	PutAccessControlRuleRequest(*workmail.PutAccessControlRuleInput) (*request.Request, *workmail.PutAccessControlRuleOutput)

	PutMailboxPermissions(*workmail.PutMailboxPermissionsInput) (*workmail.PutMailboxPermissionsOutput, error)
	PutMailboxPermissionsWithContext(aws.Context, *workmail.PutMailboxPermissionsInput, ...request.Option) (*workmail.PutMailboxPermissionsOutput, error)
	PutMailboxPermissionsRequest(*workmail.PutMailboxPermissionsInput) (*request.Request, *workmail.PutMailboxPermissionsOutput)

	PutRetentionPolicy(*workmail.PutRetentionPolicyInput) (*workmail.PutRetentionPolicyOutput, error)
	PutRetentionPolicyWithContext(aws.Context, *workmail.PutRetentionPolicyInput, ...request.Option) (*workmail.PutRetentionPolicyOutput, error)
	PutRetentionPolicyRequest(*workmail.PutRetentionPolicyInput) (*request.Request, *workmail.PutRetentionPolicyOutput)

	RegisterToWorkMail(*workmail.RegisterToWorkMailInput) (*workmail.RegisterToWorkMailOutput, error)
	RegisterToWorkMailWithContext(aws.Context, *workmail.RegisterToWorkMailInput, ...request.Option) (*workmail.RegisterToWorkMailOutput, error)
	RegisterToWorkMailRequest(*workmail.RegisterToWorkMailInput) (*request.Request, *workmail.RegisterToWorkMailOutput)

	ResetPassword(*workmail.ResetPasswordInput) (*workmail.ResetPasswordOutput, error)
	ResetPasswordWithContext(aws.Context, *workmail.ResetPasswordInput, ...request.Option) (*workmail.ResetPasswordOutput, error)
	ResetPasswordRequest(*workmail.ResetPasswordInput) (*request.Request, *workmail.ResetPasswordOutput)

	StartMailboxExportJob(*workmail.StartMailboxExportJobInput) (*workmail.StartMailboxExportJobOutput, error)
	StartMailboxExportJobWithContext(aws.Context, *workmail.StartMailboxExportJobInput, ...request.Option) (*workmail.StartMailboxExportJobOutput, error)
	StartMailboxExportJobRequest(*workmail.StartMailboxExportJobInput) (*request.Request, *workmail.StartMailboxExportJobOutput)

	TagResource(*workmail.TagResourceInput) (*workmail.TagResourceOutput, error)
	TagResourceWithContext(aws.Context, *workmail.TagResourceInput, ...request.Option) (*workmail.TagResourceOutput, error)
	TagResourceRequest(*workmail.TagResourceInput) (*request.Request, *workmail.TagResourceOutput)

	UntagResource(*workmail.UntagResourceInput) (*workmail.UntagResourceOutput, error)
	UntagResourceWithContext(aws.Context, *workmail.UntagResourceInput, ...request.Option) (*workmail.UntagResourceOutput, error)
	UntagResourceRequest(*workmail.UntagResourceInput) (*request.Request, *workmail.UntagResourceOutput)

	UpdateMailboxQuota(*workmail.UpdateMailboxQuotaInput) (*workmail.UpdateMailboxQuotaOutput, error)
	UpdateMailboxQuotaWithContext(aws.Context, *workmail.UpdateMailboxQuotaInput, ...request.Option) (*workmail.UpdateMailboxQuotaOutput, error)
	UpdateMailboxQuotaRequest(*workmail.UpdateMailboxQuotaInput) (*request.Request, *workmail.UpdateMailboxQuotaOutput)

	UpdateMobileDeviceAccessRule(*workmail.UpdateMobileDeviceAccessRuleInput) (*workmail.UpdateMobileDeviceAccessRuleOutput, error)
	UpdateMobileDeviceAccessRuleWithContext(aws.Context, *workmail.UpdateMobileDeviceAccessRuleInput, ...request.Option) (*workmail.UpdateMobileDeviceAccessRuleOutput, error)
	UpdateMobileDeviceAccessRuleRequest(*workmail.UpdateMobileDeviceAccessRuleInput) (*request.Request, *workmail.UpdateMobileDeviceAccessRuleOutput)

	UpdatePrimaryEmailAddress(*workmail.UpdatePrimaryEmailAddressInput) (*workmail.UpdatePrimaryEmailAddressOutput, error)
	UpdatePrimaryEmailAddressWithContext(aws.Context, *workmail.UpdatePrimaryEmailAddressInput, ...request.Option) (*workmail.UpdatePrimaryEmailAddressOutput, error)
	UpdatePrimaryEmailAddressRequest(*workmail.UpdatePrimaryEmailAddressInput) (*request.Request, *workmail.UpdatePrimaryEmailAddressOutput)

	UpdateResource(*workmail.UpdateResourceInput) (*workmail.UpdateResourceOutput, error)
	UpdateResourceWithContext(aws.Context, *workmail.UpdateResourceInput, ...request.Option) (*workmail.UpdateResourceOutput, error)
	UpdateResourceRequest(*workmail.UpdateResourceInput) (*request.Request, *workmail.UpdateResourceOutput)
}

var _ WorkMailAPI = (*workmail.WorkMail)(nil)
