// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

// Package cognitoidentityiface provides an interface to enable mocking the Amazon Cognito Identity service client
// for testing your code.
//
// It is important to note that this interface will have breaking changes
// when the service model is updated and adds new API operations, paginators,
// and waiters.
package cognitoidentityiface

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/cognitoidentity"
)

// CognitoIdentityAPI provides an interface to enable mocking the
// cognitoidentity.CognitoIdentity service client's API operation,
// paginators, and waiters. This make unit testing your code that calls out
// to the SDK's service client's calls easier.
//
// The best way to use this interface is so the SDK's service client's calls
// can be stubbed out for unit testing your code with the SDK without needing
// to inject custom request handlers into the SDK's request pipeline.
//
//    // myFunc uses an SDK service client to make a request to
//    // Amazon Cognito Identity.
//    func myFunc(svc cognitoidentityiface.CognitoIdentityAPI) bool {
//        // Make svc.CreateIdentityPool request
//    }
//
//    func main() {
//        sess := session.New()
//        svc := cognitoidentity.New(sess)
//
//        myFunc(svc)
//    }
//
// In your _test.go file:
//
//    // Define a mock struct to be used in your unit tests of myFunc.
//    type mockCognitoIdentityClient struct {
//        cognitoidentityiface.CognitoIdentityAPI
//    }
//    func (m *mockCognitoIdentityClient) CreateIdentityPool(input *cognitoidentity.CreateIdentityPoolInput) (*cognitoidentity.IdentityPool, error) {
//        // mock response/functionality
//    }
//
//    func TestMyFunc(t *testing.T) {
//        // Setup Test
//        mockSvc := &mockCognitoIdentityClient{}
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
type CognitoIdentityAPI interface {
	CreateIdentityPool(*cognitoidentity.CreateIdentityPoolInput) (*cognitoidentity.IdentityPool, error)
	CreateIdentityPoolWithContext(aws.Context, *cognitoidentity.CreateIdentityPoolInput, ...request.Option) (*cognitoidentity.IdentityPool, error)
	CreateIdentityPoolRequest(*cognitoidentity.CreateIdentityPoolInput) (*request.Request, *cognitoidentity.IdentityPool)

	DeleteIdentities(*cognitoidentity.DeleteIdentitiesInput) (*cognitoidentity.DeleteIdentitiesOutput, error)
	DeleteIdentitiesWithContext(aws.Context, *cognitoidentity.DeleteIdentitiesInput, ...request.Option) (*cognitoidentity.DeleteIdentitiesOutput, error)
	DeleteIdentitiesRequest(*cognitoidentity.DeleteIdentitiesInput) (*request.Request, *cognitoidentity.DeleteIdentitiesOutput)

	DeleteIdentityPool(*cognitoidentity.DeleteIdentityPoolInput) (*cognitoidentity.DeleteIdentityPoolOutput, error)
	DeleteIdentityPoolWithContext(aws.Context, *cognitoidentity.DeleteIdentityPoolInput, ...request.Option) (*cognitoidentity.DeleteIdentityPoolOutput, error)
	DeleteIdentityPoolRequest(*cognitoidentity.DeleteIdentityPoolInput) (*request.Request, *cognitoidentity.DeleteIdentityPoolOutput)

	DescribeIdentity(*cognitoidentity.DescribeIdentityInput) (*cognitoidentity.IdentityDescription, error)
	DescribeIdentityWithContext(aws.Context, *cognitoidentity.DescribeIdentityInput, ...request.Option) (*cognitoidentity.IdentityDescription, error)
	DescribeIdentityRequest(*cognitoidentity.DescribeIdentityInput) (*request.Request, *cognitoidentity.IdentityDescription)

	DescribeIdentityPool(*cognitoidentity.DescribeIdentityPoolInput) (*cognitoidentity.IdentityPool, error)
	DescribeIdentityPoolWithContext(aws.Context, *cognitoidentity.DescribeIdentityPoolInput, ...request.Option) (*cognitoidentity.IdentityPool, error)
	DescribeIdentityPoolRequest(*cognitoidentity.DescribeIdentityPoolInput) (*request.Request, *cognitoidentity.IdentityPool)

	GetCredentialsForIdentity(*cognitoidentity.GetCredentialsForIdentityInput) (*cognitoidentity.GetCredentialsForIdentityOutput, error)
	GetCredentialsForIdentityWithContext(aws.Context, *cognitoidentity.GetCredentialsForIdentityInput, ...request.Option) (*cognitoidentity.GetCredentialsForIdentityOutput, error)
	GetCredentialsForIdentityRequest(*cognitoidentity.GetCredentialsForIdentityInput) (*request.Request, *cognitoidentity.GetCredentialsForIdentityOutput)

	GetId(*cognitoidentity.GetIdInput) (*cognitoidentity.GetIdOutput, error)
	GetIdWithContext(aws.Context, *cognitoidentity.GetIdInput, ...request.Option) (*cognitoidentity.GetIdOutput, error)
	GetIdRequest(*cognitoidentity.GetIdInput) (*request.Request, *cognitoidentity.GetIdOutput)

	GetIdentityPoolRoles(*cognitoidentity.GetIdentityPoolRolesInput) (*cognitoidentity.GetIdentityPoolRolesOutput, error)
	GetIdentityPoolRolesWithContext(aws.Context, *cognitoidentity.GetIdentityPoolRolesInput, ...request.Option) (*cognitoidentity.GetIdentityPoolRolesOutput, error)
	GetIdentityPoolRolesRequest(*cognitoidentity.GetIdentityPoolRolesInput) (*request.Request, *cognitoidentity.GetIdentityPoolRolesOutput)

	GetOpenIdToken(*cognitoidentity.GetOpenIdTokenInput) (*cognitoidentity.GetOpenIdTokenOutput, error)
	GetOpenIdTokenWithContext(aws.Context, *cognitoidentity.GetOpenIdTokenInput, ...request.Option) (*cognitoidentity.GetOpenIdTokenOutput, error)
	GetOpenIdTokenRequest(*cognitoidentity.GetOpenIdTokenInput) (*request.Request, *cognitoidentity.GetOpenIdTokenOutput)

	GetOpenIdTokenForDeveloperIdentity(*cognitoidentity.GetOpenIdTokenForDeveloperIdentityInput) (*cognitoidentity.GetOpenIdTokenForDeveloperIdentityOutput, error)
	GetOpenIdTokenForDeveloperIdentityWithContext(aws.Context, *cognitoidentity.GetOpenIdTokenForDeveloperIdentityInput, ...request.Option) (*cognitoidentity.GetOpenIdTokenForDeveloperIdentityOutput, error)
	GetOpenIdTokenForDeveloperIdentityRequest(*cognitoidentity.GetOpenIdTokenForDeveloperIdentityInput) (*request.Request, *cognitoidentity.GetOpenIdTokenForDeveloperIdentityOutput)

	GetPrincipalTagAttributeMap(*cognitoidentity.GetPrincipalTagAttributeMapInput) (*cognitoidentity.GetPrincipalTagAttributeMapOutput, error)
	GetPrincipalTagAttributeMapWithContext(aws.Context, *cognitoidentity.GetPrincipalTagAttributeMapInput, ...request.Option) (*cognitoidentity.GetPrincipalTagAttributeMapOutput, error)
	GetPrincipalTagAttributeMapRequest(*cognitoidentity.GetPrincipalTagAttributeMapInput) (*request.Request, *cognitoidentity.GetPrincipalTagAttributeMapOutput)

	ListIdentities(*cognitoidentity.ListIdentitiesInput) (*cognitoidentity.ListIdentitiesOutput, error)
	ListIdentitiesWithContext(aws.Context, *cognitoidentity.ListIdentitiesInput, ...request.Option) (*cognitoidentity.ListIdentitiesOutput, error)
	ListIdentitiesRequest(*cognitoidentity.ListIdentitiesInput) (*request.Request, *cognitoidentity.ListIdentitiesOutput)

	ListIdentityPools(*cognitoidentity.ListIdentityPoolsInput) (*cognitoidentity.ListIdentityPoolsOutput, error)
	ListIdentityPoolsWithContext(aws.Context, *cognitoidentity.ListIdentityPoolsInput, ...request.Option) (*cognitoidentity.ListIdentityPoolsOutput, error)
	ListIdentityPoolsRequest(*cognitoidentity.ListIdentityPoolsInput) (*request.Request, *cognitoidentity.ListIdentityPoolsOutput)

	ListIdentityPoolsPages(*cognitoidentity.ListIdentityPoolsInput, func(*cognitoidentity.ListIdentityPoolsOutput, bool) bool) error
	ListIdentityPoolsPagesWithContext(aws.Context, *cognitoidentity.ListIdentityPoolsInput, func(*cognitoidentity.ListIdentityPoolsOutput, bool) bool, ...request.Option) error

	ListTagsForResource(*cognitoidentity.ListTagsForResourceInput) (*cognitoidentity.ListTagsForResourceOutput, error)
	ListTagsForResourceWithContext(aws.Context, *cognitoidentity.ListTagsForResourceInput, ...request.Option) (*cognitoidentity.ListTagsForResourceOutput, error)
	ListTagsForResourceRequest(*cognitoidentity.ListTagsForResourceInput) (*request.Request, *cognitoidentity.ListTagsForResourceOutput)

	LookupDeveloperIdentity(*cognitoidentity.LookupDeveloperIdentityInput) (*cognitoidentity.LookupDeveloperIdentityOutput, error)
	LookupDeveloperIdentityWithContext(aws.Context, *cognitoidentity.LookupDeveloperIdentityInput, ...request.Option) (*cognitoidentity.LookupDeveloperIdentityOutput, error)
	LookupDeveloperIdentityRequest(*cognitoidentity.LookupDeveloperIdentityInput) (*request.Request, *cognitoidentity.LookupDeveloperIdentityOutput)

	MergeDeveloperIdentities(*cognitoidentity.MergeDeveloperIdentitiesInput) (*cognitoidentity.MergeDeveloperIdentitiesOutput, error)
	MergeDeveloperIdentitiesWithContext(aws.Context, *cognitoidentity.MergeDeveloperIdentitiesInput, ...request.Option) (*cognitoidentity.MergeDeveloperIdentitiesOutput, error)
	MergeDeveloperIdentitiesRequest(*cognitoidentity.MergeDeveloperIdentitiesInput) (*request.Request, *cognitoidentity.MergeDeveloperIdentitiesOutput)

	SetIdentityPoolRoles(*cognitoidentity.SetIdentityPoolRolesInput) (*cognitoidentity.SetIdentityPoolRolesOutput, error)
	SetIdentityPoolRolesWithContext(aws.Context, *cognitoidentity.SetIdentityPoolRolesInput, ...request.Option) (*cognitoidentity.SetIdentityPoolRolesOutput, error)
	SetIdentityPoolRolesRequest(*cognitoidentity.SetIdentityPoolRolesInput) (*request.Request, *cognitoidentity.SetIdentityPoolRolesOutput)

	SetPrincipalTagAttributeMap(*cognitoidentity.SetPrincipalTagAttributeMapInput) (*cognitoidentity.SetPrincipalTagAttributeMapOutput, error)
	SetPrincipalTagAttributeMapWithContext(aws.Context, *cognitoidentity.SetPrincipalTagAttributeMapInput, ...request.Option) (*cognitoidentity.SetPrincipalTagAttributeMapOutput, error)
	SetPrincipalTagAttributeMapRequest(*cognitoidentity.SetPrincipalTagAttributeMapInput) (*request.Request, *cognitoidentity.SetPrincipalTagAttributeMapOutput)

	TagResource(*cognitoidentity.TagResourceInput) (*cognitoidentity.TagResourceOutput, error)
	TagResourceWithContext(aws.Context, *cognitoidentity.TagResourceInput, ...request.Option) (*cognitoidentity.TagResourceOutput, error)
	TagResourceRequest(*cognitoidentity.TagResourceInput) (*request.Request, *cognitoidentity.TagResourceOutput)

	UnlinkDeveloperIdentity(*cognitoidentity.UnlinkDeveloperIdentityInput) (*cognitoidentity.UnlinkDeveloperIdentityOutput, error)
	UnlinkDeveloperIdentityWithContext(aws.Context, *cognitoidentity.UnlinkDeveloperIdentityInput, ...request.Option) (*cognitoidentity.UnlinkDeveloperIdentityOutput, error)
	UnlinkDeveloperIdentityRequest(*cognitoidentity.UnlinkDeveloperIdentityInput) (*request.Request, *cognitoidentity.UnlinkDeveloperIdentityOutput)

	UnlinkIdentity(*cognitoidentity.UnlinkIdentityInput) (*cognitoidentity.UnlinkIdentityOutput, error)
	UnlinkIdentityWithContext(aws.Context, *cognitoidentity.UnlinkIdentityInput, ...request.Option) (*cognitoidentity.UnlinkIdentityOutput, error)
	UnlinkIdentityRequest(*cognitoidentity.UnlinkIdentityInput) (*request.Request, *cognitoidentity.UnlinkIdentityOutput)

	UntagResource(*cognitoidentity.UntagResourceInput) (*cognitoidentity.UntagResourceOutput, error)
	UntagResourceWithContext(aws.Context, *cognitoidentity.UntagResourceInput, ...request.Option) (*cognitoidentity.UntagResourceOutput, error)
	UntagResourceRequest(*cognitoidentity.UntagResourceInput) (*request.Request, *cognitoidentity.UntagResourceOutput)

	UpdateIdentityPool(*cognitoidentity.IdentityPool) (*cognitoidentity.IdentityPool, error)
	UpdateIdentityPoolWithContext(aws.Context, *cognitoidentity.IdentityPool, ...request.Option) (*cognitoidentity.IdentityPool, error)
	UpdateIdentityPoolRequest(*cognitoidentity.IdentityPool) (*request.Request, *cognitoidentity.IdentityPool)
}

var _ CognitoIdentityAPI = (*cognitoidentity.CognitoIdentity)(nil)
