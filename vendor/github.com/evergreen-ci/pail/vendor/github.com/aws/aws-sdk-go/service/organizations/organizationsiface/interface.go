// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

// Package organizationsiface provides an interface to enable mocking the AWS Organizations service client
// for testing your code.
//
// It is important to note that this interface will have breaking changes
// when the service model is updated and adds new API operations, paginators,
// and waiters.
package organizationsiface

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/organizations"
)

// OrganizationsAPI provides an interface to enable mocking the
// organizations.Organizations service client's API operation,
// paginators, and waiters. This make unit testing your code that calls out
// to the SDK's service client's calls easier.
//
// The best way to use this interface is so the SDK's service client's calls
// can be stubbed out for unit testing your code with the SDK without needing
// to inject custom request handlers into the SDK's request pipeline.
//
//    // myFunc uses an SDK service client to make a request to
//    // AWS Organizations.
//    func myFunc(svc organizationsiface.OrganizationsAPI) bool {
//        // Make svc.AcceptHandshake request
//    }
//
//    func main() {
//        sess := session.New()
//        svc := organizations.New(sess)
//
//        myFunc(svc)
//    }
//
// In your _test.go file:
//
//    // Define a mock struct to be used in your unit tests of myFunc.
//    type mockOrganizationsClient struct {
//        organizationsiface.OrganizationsAPI
//    }
//    func (m *mockOrganizationsClient) AcceptHandshake(input *organizations.AcceptHandshakeInput) (*organizations.AcceptHandshakeOutput, error) {
//        // mock response/functionality
//    }
//
//    func TestMyFunc(t *testing.T) {
//        // Setup Test
//        mockSvc := &mockOrganizationsClient{}
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
type OrganizationsAPI interface {
	AcceptHandshake(*organizations.AcceptHandshakeInput) (*organizations.AcceptHandshakeOutput, error)
	AcceptHandshakeWithContext(aws.Context, *organizations.AcceptHandshakeInput, ...request.Option) (*organizations.AcceptHandshakeOutput, error)
	AcceptHandshakeRequest(*organizations.AcceptHandshakeInput) (*request.Request, *organizations.AcceptHandshakeOutput)

	AttachPolicy(*organizations.AttachPolicyInput) (*organizations.AttachPolicyOutput, error)
	AttachPolicyWithContext(aws.Context, *organizations.AttachPolicyInput, ...request.Option) (*organizations.AttachPolicyOutput, error)
	AttachPolicyRequest(*organizations.AttachPolicyInput) (*request.Request, *organizations.AttachPolicyOutput)

	CancelHandshake(*organizations.CancelHandshakeInput) (*organizations.CancelHandshakeOutput, error)
	CancelHandshakeWithContext(aws.Context, *organizations.CancelHandshakeInput, ...request.Option) (*organizations.CancelHandshakeOutput, error)
	CancelHandshakeRequest(*organizations.CancelHandshakeInput) (*request.Request, *organizations.CancelHandshakeOutput)

	CreateAccount(*organizations.CreateAccountInput) (*organizations.CreateAccountOutput, error)
	CreateAccountWithContext(aws.Context, *organizations.CreateAccountInput, ...request.Option) (*organizations.CreateAccountOutput, error)
	CreateAccountRequest(*organizations.CreateAccountInput) (*request.Request, *organizations.CreateAccountOutput)

	CreateOrganization(*organizations.CreateOrganizationInput) (*organizations.CreateOrganizationOutput, error)
	CreateOrganizationWithContext(aws.Context, *organizations.CreateOrganizationInput, ...request.Option) (*organizations.CreateOrganizationOutput, error)
	CreateOrganizationRequest(*organizations.CreateOrganizationInput) (*request.Request, *organizations.CreateOrganizationOutput)

	CreateOrganizationalUnit(*organizations.CreateOrganizationalUnitInput) (*organizations.CreateOrganizationalUnitOutput, error)
	CreateOrganizationalUnitWithContext(aws.Context, *organizations.CreateOrganizationalUnitInput, ...request.Option) (*organizations.CreateOrganizationalUnitOutput, error)
	CreateOrganizationalUnitRequest(*organizations.CreateOrganizationalUnitInput) (*request.Request, *organizations.CreateOrganizationalUnitOutput)

	CreatePolicy(*organizations.CreatePolicyInput) (*organizations.CreatePolicyOutput, error)
	CreatePolicyWithContext(aws.Context, *organizations.CreatePolicyInput, ...request.Option) (*organizations.CreatePolicyOutput, error)
	CreatePolicyRequest(*organizations.CreatePolicyInput) (*request.Request, *organizations.CreatePolicyOutput)

	DeclineHandshake(*organizations.DeclineHandshakeInput) (*organizations.DeclineHandshakeOutput, error)
	DeclineHandshakeWithContext(aws.Context, *organizations.DeclineHandshakeInput, ...request.Option) (*organizations.DeclineHandshakeOutput, error)
	DeclineHandshakeRequest(*organizations.DeclineHandshakeInput) (*request.Request, *organizations.DeclineHandshakeOutput)

	DeleteOrganization(*organizations.DeleteOrganizationInput) (*organizations.DeleteOrganizationOutput, error)
	DeleteOrganizationWithContext(aws.Context, *organizations.DeleteOrganizationInput, ...request.Option) (*organizations.DeleteOrganizationOutput, error)
	DeleteOrganizationRequest(*organizations.DeleteOrganizationInput) (*request.Request, *organizations.DeleteOrganizationOutput)

	DeleteOrganizationalUnit(*organizations.DeleteOrganizationalUnitInput) (*organizations.DeleteOrganizationalUnitOutput, error)
	DeleteOrganizationalUnitWithContext(aws.Context, *organizations.DeleteOrganizationalUnitInput, ...request.Option) (*organizations.DeleteOrganizationalUnitOutput, error)
	DeleteOrganizationalUnitRequest(*organizations.DeleteOrganizationalUnitInput) (*request.Request, *organizations.DeleteOrganizationalUnitOutput)

	DeletePolicy(*organizations.DeletePolicyInput) (*organizations.DeletePolicyOutput, error)
	DeletePolicyWithContext(aws.Context, *organizations.DeletePolicyInput, ...request.Option) (*organizations.DeletePolicyOutput, error)
	DeletePolicyRequest(*organizations.DeletePolicyInput) (*request.Request, *organizations.DeletePolicyOutput)

	DescribeAccount(*organizations.DescribeAccountInput) (*organizations.DescribeAccountOutput, error)
	DescribeAccountWithContext(aws.Context, *organizations.DescribeAccountInput, ...request.Option) (*organizations.DescribeAccountOutput, error)
	DescribeAccountRequest(*organizations.DescribeAccountInput) (*request.Request, *organizations.DescribeAccountOutput)

	DescribeCreateAccountStatus(*organizations.DescribeCreateAccountStatusInput) (*organizations.DescribeCreateAccountStatusOutput, error)
	DescribeCreateAccountStatusWithContext(aws.Context, *organizations.DescribeCreateAccountStatusInput, ...request.Option) (*organizations.DescribeCreateAccountStatusOutput, error)
	DescribeCreateAccountStatusRequest(*organizations.DescribeCreateAccountStatusInput) (*request.Request, *organizations.DescribeCreateAccountStatusOutput)

	DescribeHandshake(*organizations.DescribeHandshakeInput) (*organizations.DescribeHandshakeOutput, error)
	DescribeHandshakeWithContext(aws.Context, *organizations.DescribeHandshakeInput, ...request.Option) (*organizations.DescribeHandshakeOutput, error)
	DescribeHandshakeRequest(*organizations.DescribeHandshakeInput) (*request.Request, *organizations.DescribeHandshakeOutput)

	DescribeOrganization(*organizations.DescribeOrganizationInput) (*organizations.DescribeOrganizationOutput, error)
	DescribeOrganizationWithContext(aws.Context, *organizations.DescribeOrganizationInput, ...request.Option) (*organizations.DescribeOrganizationOutput, error)
	DescribeOrganizationRequest(*organizations.DescribeOrganizationInput) (*request.Request, *organizations.DescribeOrganizationOutput)

	DescribeOrganizationalUnit(*organizations.DescribeOrganizationalUnitInput) (*organizations.DescribeOrganizationalUnitOutput, error)
	DescribeOrganizationalUnitWithContext(aws.Context, *organizations.DescribeOrganizationalUnitInput, ...request.Option) (*organizations.DescribeOrganizationalUnitOutput, error)
	DescribeOrganizationalUnitRequest(*organizations.DescribeOrganizationalUnitInput) (*request.Request, *organizations.DescribeOrganizationalUnitOutput)

	DescribePolicy(*organizations.DescribePolicyInput) (*organizations.DescribePolicyOutput, error)
	DescribePolicyWithContext(aws.Context, *organizations.DescribePolicyInput, ...request.Option) (*organizations.DescribePolicyOutput, error)
	DescribePolicyRequest(*organizations.DescribePolicyInput) (*request.Request, *organizations.DescribePolicyOutput)

	DetachPolicy(*organizations.DetachPolicyInput) (*organizations.DetachPolicyOutput, error)
	DetachPolicyWithContext(aws.Context, *organizations.DetachPolicyInput, ...request.Option) (*organizations.DetachPolicyOutput, error)
	DetachPolicyRequest(*organizations.DetachPolicyInput) (*request.Request, *organizations.DetachPolicyOutput)

	DisablePolicyType(*organizations.DisablePolicyTypeInput) (*organizations.DisablePolicyTypeOutput, error)
	DisablePolicyTypeWithContext(aws.Context, *organizations.DisablePolicyTypeInput, ...request.Option) (*organizations.DisablePolicyTypeOutput, error)
	DisablePolicyTypeRequest(*organizations.DisablePolicyTypeInput) (*request.Request, *organizations.DisablePolicyTypeOutput)

	EnableAllFeatures(*organizations.EnableAllFeaturesInput) (*organizations.EnableAllFeaturesOutput, error)
	EnableAllFeaturesWithContext(aws.Context, *organizations.EnableAllFeaturesInput, ...request.Option) (*organizations.EnableAllFeaturesOutput, error)
	EnableAllFeaturesRequest(*organizations.EnableAllFeaturesInput) (*request.Request, *organizations.EnableAllFeaturesOutput)

	EnablePolicyType(*organizations.EnablePolicyTypeInput) (*organizations.EnablePolicyTypeOutput, error)
	EnablePolicyTypeWithContext(aws.Context, *organizations.EnablePolicyTypeInput, ...request.Option) (*organizations.EnablePolicyTypeOutput, error)
	EnablePolicyTypeRequest(*organizations.EnablePolicyTypeInput) (*request.Request, *organizations.EnablePolicyTypeOutput)

	InviteAccountToOrganization(*organizations.InviteAccountToOrganizationInput) (*organizations.InviteAccountToOrganizationOutput, error)
	InviteAccountToOrganizationWithContext(aws.Context, *organizations.InviteAccountToOrganizationInput, ...request.Option) (*organizations.InviteAccountToOrganizationOutput, error)
	InviteAccountToOrganizationRequest(*organizations.InviteAccountToOrganizationInput) (*request.Request, *organizations.InviteAccountToOrganizationOutput)

	LeaveOrganization(*organizations.LeaveOrganizationInput) (*organizations.LeaveOrganizationOutput, error)
	LeaveOrganizationWithContext(aws.Context, *organizations.LeaveOrganizationInput, ...request.Option) (*organizations.LeaveOrganizationOutput, error)
	LeaveOrganizationRequest(*organizations.LeaveOrganizationInput) (*request.Request, *organizations.LeaveOrganizationOutput)

	ListAccounts(*organizations.ListAccountsInput) (*organizations.ListAccountsOutput, error)
	ListAccountsWithContext(aws.Context, *organizations.ListAccountsInput, ...request.Option) (*organizations.ListAccountsOutput, error)
	ListAccountsRequest(*organizations.ListAccountsInput) (*request.Request, *organizations.ListAccountsOutput)

	ListAccountsPages(*organizations.ListAccountsInput, func(*organizations.ListAccountsOutput, bool) bool) error
	ListAccountsPagesWithContext(aws.Context, *organizations.ListAccountsInput, func(*organizations.ListAccountsOutput, bool) bool, ...request.Option) error

	ListAccountsForParent(*organizations.ListAccountsForParentInput) (*organizations.ListAccountsForParentOutput, error)
	ListAccountsForParentWithContext(aws.Context, *organizations.ListAccountsForParentInput, ...request.Option) (*organizations.ListAccountsForParentOutput, error)
	ListAccountsForParentRequest(*organizations.ListAccountsForParentInput) (*request.Request, *organizations.ListAccountsForParentOutput)

	ListAccountsForParentPages(*organizations.ListAccountsForParentInput, func(*organizations.ListAccountsForParentOutput, bool) bool) error
	ListAccountsForParentPagesWithContext(aws.Context, *organizations.ListAccountsForParentInput, func(*organizations.ListAccountsForParentOutput, bool) bool, ...request.Option) error

	ListChildren(*organizations.ListChildrenInput) (*organizations.ListChildrenOutput, error)
	ListChildrenWithContext(aws.Context, *organizations.ListChildrenInput, ...request.Option) (*organizations.ListChildrenOutput, error)
	ListChildrenRequest(*organizations.ListChildrenInput) (*request.Request, *organizations.ListChildrenOutput)

	ListChildrenPages(*organizations.ListChildrenInput, func(*organizations.ListChildrenOutput, bool) bool) error
	ListChildrenPagesWithContext(aws.Context, *organizations.ListChildrenInput, func(*organizations.ListChildrenOutput, bool) bool, ...request.Option) error

	ListCreateAccountStatus(*organizations.ListCreateAccountStatusInput) (*organizations.ListCreateAccountStatusOutput, error)
	ListCreateAccountStatusWithContext(aws.Context, *organizations.ListCreateAccountStatusInput, ...request.Option) (*organizations.ListCreateAccountStatusOutput, error)
	ListCreateAccountStatusRequest(*organizations.ListCreateAccountStatusInput) (*request.Request, *organizations.ListCreateAccountStatusOutput)

	ListCreateAccountStatusPages(*organizations.ListCreateAccountStatusInput, func(*organizations.ListCreateAccountStatusOutput, bool) bool) error
	ListCreateAccountStatusPagesWithContext(aws.Context, *organizations.ListCreateAccountStatusInput, func(*organizations.ListCreateAccountStatusOutput, bool) bool, ...request.Option) error

	ListHandshakesForAccount(*organizations.ListHandshakesForAccountInput) (*organizations.ListHandshakesForAccountOutput, error)
	ListHandshakesForAccountWithContext(aws.Context, *organizations.ListHandshakesForAccountInput, ...request.Option) (*organizations.ListHandshakesForAccountOutput, error)
	ListHandshakesForAccountRequest(*organizations.ListHandshakesForAccountInput) (*request.Request, *organizations.ListHandshakesForAccountOutput)

	ListHandshakesForAccountPages(*organizations.ListHandshakesForAccountInput, func(*organizations.ListHandshakesForAccountOutput, bool) bool) error
	ListHandshakesForAccountPagesWithContext(aws.Context, *organizations.ListHandshakesForAccountInput, func(*organizations.ListHandshakesForAccountOutput, bool) bool, ...request.Option) error

	ListHandshakesForOrganization(*organizations.ListHandshakesForOrganizationInput) (*organizations.ListHandshakesForOrganizationOutput, error)
	ListHandshakesForOrganizationWithContext(aws.Context, *organizations.ListHandshakesForOrganizationInput, ...request.Option) (*organizations.ListHandshakesForOrganizationOutput, error)
	ListHandshakesForOrganizationRequest(*organizations.ListHandshakesForOrganizationInput) (*request.Request, *organizations.ListHandshakesForOrganizationOutput)

	ListHandshakesForOrganizationPages(*organizations.ListHandshakesForOrganizationInput, func(*organizations.ListHandshakesForOrganizationOutput, bool) bool) error
	ListHandshakesForOrganizationPagesWithContext(aws.Context, *organizations.ListHandshakesForOrganizationInput, func(*organizations.ListHandshakesForOrganizationOutput, bool) bool, ...request.Option) error

	ListOrganizationalUnitsForParent(*organizations.ListOrganizationalUnitsForParentInput) (*organizations.ListOrganizationalUnitsForParentOutput, error)
	ListOrganizationalUnitsForParentWithContext(aws.Context, *organizations.ListOrganizationalUnitsForParentInput, ...request.Option) (*organizations.ListOrganizationalUnitsForParentOutput, error)
	ListOrganizationalUnitsForParentRequest(*organizations.ListOrganizationalUnitsForParentInput) (*request.Request, *organizations.ListOrganizationalUnitsForParentOutput)

	ListOrganizationalUnitsForParentPages(*organizations.ListOrganizationalUnitsForParentInput, func(*organizations.ListOrganizationalUnitsForParentOutput, bool) bool) error
	ListOrganizationalUnitsForParentPagesWithContext(aws.Context, *organizations.ListOrganizationalUnitsForParentInput, func(*organizations.ListOrganizationalUnitsForParentOutput, bool) bool, ...request.Option) error

	ListParents(*organizations.ListParentsInput) (*organizations.ListParentsOutput, error)
	ListParentsWithContext(aws.Context, *organizations.ListParentsInput, ...request.Option) (*organizations.ListParentsOutput, error)
	ListParentsRequest(*organizations.ListParentsInput) (*request.Request, *organizations.ListParentsOutput)

	ListParentsPages(*organizations.ListParentsInput, func(*organizations.ListParentsOutput, bool) bool) error
	ListParentsPagesWithContext(aws.Context, *organizations.ListParentsInput, func(*organizations.ListParentsOutput, bool) bool, ...request.Option) error

	ListPolicies(*organizations.ListPoliciesInput) (*organizations.ListPoliciesOutput, error)
	ListPoliciesWithContext(aws.Context, *organizations.ListPoliciesInput, ...request.Option) (*organizations.ListPoliciesOutput, error)
	ListPoliciesRequest(*organizations.ListPoliciesInput) (*request.Request, *organizations.ListPoliciesOutput)

	ListPoliciesPages(*organizations.ListPoliciesInput, func(*organizations.ListPoliciesOutput, bool) bool) error
	ListPoliciesPagesWithContext(aws.Context, *organizations.ListPoliciesInput, func(*organizations.ListPoliciesOutput, bool) bool, ...request.Option) error

	ListPoliciesForTarget(*organizations.ListPoliciesForTargetInput) (*organizations.ListPoliciesForTargetOutput, error)
	ListPoliciesForTargetWithContext(aws.Context, *organizations.ListPoliciesForTargetInput, ...request.Option) (*organizations.ListPoliciesForTargetOutput, error)
	ListPoliciesForTargetRequest(*organizations.ListPoliciesForTargetInput) (*request.Request, *organizations.ListPoliciesForTargetOutput)

	ListPoliciesForTargetPages(*organizations.ListPoliciesForTargetInput, func(*organizations.ListPoliciesForTargetOutput, bool) bool) error
	ListPoliciesForTargetPagesWithContext(aws.Context, *organizations.ListPoliciesForTargetInput, func(*organizations.ListPoliciesForTargetOutput, bool) bool, ...request.Option) error

	ListRoots(*organizations.ListRootsInput) (*organizations.ListRootsOutput, error)
	ListRootsWithContext(aws.Context, *organizations.ListRootsInput, ...request.Option) (*organizations.ListRootsOutput, error)
	ListRootsRequest(*organizations.ListRootsInput) (*request.Request, *organizations.ListRootsOutput)

	ListRootsPages(*organizations.ListRootsInput, func(*organizations.ListRootsOutput, bool) bool) error
	ListRootsPagesWithContext(aws.Context, *organizations.ListRootsInput, func(*organizations.ListRootsOutput, bool) bool, ...request.Option) error

	ListTargetsForPolicy(*organizations.ListTargetsForPolicyInput) (*organizations.ListTargetsForPolicyOutput, error)
	ListTargetsForPolicyWithContext(aws.Context, *organizations.ListTargetsForPolicyInput, ...request.Option) (*organizations.ListTargetsForPolicyOutput, error)
	ListTargetsForPolicyRequest(*organizations.ListTargetsForPolicyInput) (*request.Request, *organizations.ListTargetsForPolicyOutput)

	ListTargetsForPolicyPages(*organizations.ListTargetsForPolicyInput, func(*organizations.ListTargetsForPolicyOutput, bool) bool) error
	ListTargetsForPolicyPagesWithContext(aws.Context, *organizations.ListTargetsForPolicyInput, func(*organizations.ListTargetsForPolicyOutput, bool) bool, ...request.Option) error

	MoveAccount(*organizations.MoveAccountInput) (*organizations.MoveAccountOutput, error)
	MoveAccountWithContext(aws.Context, *organizations.MoveAccountInput, ...request.Option) (*organizations.MoveAccountOutput, error)
	MoveAccountRequest(*organizations.MoveAccountInput) (*request.Request, *organizations.MoveAccountOutput)

	RemoveAccountFromOrganization(*organizations.RemoveAccountFromOrganizationInput) (*organizations.RemoveAccountFromOrganizationOutput, error)
	RemoveAccountFromOrganizationWithContext(aws.Context, *organizations.RemoveAccountFromOrganizationInput, ...request.Option) (*organizations.RemoveAccountFromOrganizationOutput, error)
	RemoveAccountFromOrganizationRequest(*organizations.RemoveAccountFromOrganizationInput) (*request.Request, *organizations.RemoveAccountFromOrganizationOutput)

	UpdateOrganizationalUnit(*organizations.UpdateOrganizationalUnitInput) (*organizations.UpdateOrganizationalUnitOutput, error)
	UpdateOrganizationalUnitWithContext(aws.Context, *organizations.UpdateOrganizationalUnitInput, ...request.Option) (*organizations.UpdateOrganizationalUnitOutput, error)
	UpdateOrganizationalUnitRequest(*organizations.UpdateOrganizationalUnitInput) (*request.Request, *organizations.UpdateOrganizationalUnitOutput)

	UpdatePolicy(*organizations.UpdatePolicyInput) (*organizations.UpdatePolicyOutput, error)
	UpdatePolicyWithContext(aws.Context, *organizations.UpdatePolicyInput, ...request.Option) (*organizations.UpdatePolicyOutput, error)
	UpdatePolicyRequest(*organizations.UpdatePolicyInput) (*request.Request, *organizations.UpdatePolicyOutput)
}

var _ OrganizationsAPI = (*organizations.Organizations)(nil)
