// THIS FILE IS AUTOMATICALLY GENERATED. DO NOT EDIT.

// Package wafiface provides an interface to enable mocking the AWS WAF service client
// for testing your code.
//
// It is important to note that this interface will have breaking changes
// when the service model is updated and adds new API operations, paginators,
// and waiters.
package wafiface

import (
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/waf"
)

// WAFAPI provides an interface to enable mocking the
// waf.WAF service client's API operation,
// paginators, and waiters. This make unit testing your code that calls out
// to the SDK's service client's calls easier.
//
// The best way to use this interface is so the SDK's service client's calls
// can be stubbed out for unit testing your code with the SDK without needing
// to inject custom request handlers into the the SDK's request pipeline.
//
//    // myFunc uses an SDK service client to make a request to
//    // AWS WAF.
//    func myFunc(svc wafiface.WAFAPI) bool {
//        // Make svc.CreateByteMatchSet request
//    }
//
//    func main() {
//        sess := session.New()
//        svc := waf.New(sess)
//
//        myFunc(svc)
//    }
//
// In your _test.go file:
//
//    // Define a mock struct to be used in your unit tests of myFunc.
//    type mockWAFClient struct {
//        wafiface.WAFAPI
//    }
//    func (m *mockWAFClient) CreateByteMatchSet(input *waf.CreateByteMatchSetInput) (*waf.CreateByteMatchSetOutput, error) {
//        // mock response/functionality
//    }
//
//    TestMyFunc(t *testing.T) {
//        // Setup Test
//        mockSvc := &mockWAFClient{}
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
type WAFAPI interface {
	CreateByteMatchSetRequest(*waf.CreateByteMatchSetInput) (*request.Request, *waf.CreateByteMatchSetOutput)

	CreateByteMatchSet(*waf.CreateByteMatchSetInput) (*waf.CreateByteMatchSetOutput, error)

	CreateIPSetRequest(*waf.CreateIPSetInput) (*request.Request, *waf.CreateIPSetOutput)

	CreateIPSet(*waf.CreateIPSetInput) (*waf.CreateIPSetOutput, error)

	CreateRuleRequest(*waf.CreateRuleInput) (*request.Request, *waf.CreateRuleOutput)

	CreateRule(*waf.CreateRuleInput) (*waf.CreateRuleOutput, error)

	CreateSizeConstraintSetRequest(*waf.CreateSizeConstraintSetInput) (*request.Request, *waf.CreateSizeConstraintSetOutput)

	CreateSizeConstraintSet(*waf.CreateSizeConstraintSetInput) (*waf.CreateSizeConstraintSetOutput, error)

	CreateSqlInjectionMatchSetRequest(*waf.CreateSqlInjectionMatchSetInput) (*request.Request, *waf.CreateSqlInjectionMatchSetOutput)

	CreateSqlInjectionMatchSet(*waf.CreateSqlInjectionMatchSetInput) (*waf.CreateSqlInjectionMatchSetOutput, error)

	CreateWebACLRequest(*waf.CreateWebACLInput) (*request.Request, *waf.CreateWebACLOutput)

	CreateWebACL(*waf.CreateWebACLInput) (*waf.CreateWebACLOutput, error)

	CreateXssMatchSetRequest(*waf.CreateXssMatchSetInput) (*request.Request, *waf.CreateXssMatchSetOutput)

	CreateXssMatchSet(*waf.CreateXssMatchSetInput) (*waf.CreateXssMatchSetOutput, error)

	DeleteByteMatchSetRequest(*waf.DeleteByteMatchSetInput) (*request.Request, *waf.DeleteByteMatchSetOutput)

	DeleteByteMatchSet(*waf.DeleteByteMatchSetInput) (*waf.DeleteByteMatchSetOutput, error)

	DeleteIPSetRequest(*waf.DeleteIPSetInput) (*request.Request, *waf.DeleteIPSetOutput)

	DeleteIPSet(*waf.DeleteIPSetInput) (*waf.DeleteIPSetOutput, error)

	DeleteRuleRequest(*waf.DeleteRuleInput) (*request.Request, *waf.DeleteRuleOutput)

	DeleteRule(*waf.DeleteRuleInput) (*waf.DeleteRuleOutput, error)

	DeleteSizeConstraintSetRequest(*waf.DeleteSizeConstraintSetInput) (*request.Request, *waf.DeleteSizeConstraintSetOutput)

	DeleteSizeConstraintSet(*waf.DeleteSizeConstraintSetInput) (*waf.DeleteSizeConstraintSetOutput, error)

	DeleteSqlInjectionMatchSetRequest(*waf.DeleteSqlInjectionMatchSetInput) (*request.Request, *waf.DeleteSqlInjectionMatchSetOutput)

	DeleteSqlInjectionMatchSet(*waf.DeleteSqlInjectionMatchSetInput) (*waf.DeleteSqlInjectionMatchSetOutput, error)

	DeleteWebACLRequest(*waf.DeleteWebACLInput) (*request.Request, *waf.DeleteWebACLOutput)

	DeleteWebACL(*waf.DeleteWebACLInput) (*waf.DeleteWebACLOutput, error)

	DeleteXssMatchSetRequest(*waf.DeleteXssMatchSetInput) (*request.Request, *waf.DeleteXssMatchSetOutput)

	DeleteXssMatchSet(*waf.DeleteXssMatchSetInput) (*waf.DeleteXssMatchSetOutput, error)

	GetByteMatchSetRequest(*waf.GetByteMatchSetInput) (*request.Request, *waf.GetByteMatchSetOutput)

	GetByteMatchSet(*waf.GetByteMatchSetInput) (*waf.GetByteMatchSetOutput, error)

	GetChangeTokenRequest(*waf.GetChangeTokenInput) (*request.Request, *waf.GetChangeTokenOutput)

	GetChangeToken(*waf.GetChangeTokenInput) (*waf.GetChangeTokenOutput, error)

	GetChangeTokenStatusRequest(*waf.GetChangeTokenStatusInput) (*request.Request, *waf.GetChangeTokenStatusOutput)

	GetChangeTokenStatus(*waf.GetChangeTokenStatusInput) (*waf.GetChangeTokenStatusOutput, error)

	GetIPSetRequest(*waf.GetIPSetInput) (*request.Request, *waf.GetIPSetOutput)

	GetIPSet(*waf.GetIPSetInput) (*waf.GetIPSetOutput, error)

	GetRuleRequest(*waf.GetRuleInput) (*request.Request, *waf.GetRuleOutput)

	GetRule(*waf.GetRuleInput) (*waf.GetRuleOutput, error)

	GetSampledRequestsRequest(*waf.GetSampledRequestsInput) (*request.Request, *waf.GetSampledRequestsOutput)

	GetSampledRequests(*waf.GetSampledRequestsInput) (*waf.GetSampledRequestsOutput, error)

	GetSizeConstraintSetRequest(*waf.GetSizeConstraintSetInput) (*request.Request, *waf.GetSizeConstraintSetOutput)

	GetSizeConstraintSet(*waf.GetSizeConstraintSetInput) (*waf.GetSizeConstraintSetOutput, error)

	GetSqlInjectionMatchSetRequest(*waf.GetSqlInjectionMatchSetInput) (*request.Request, *waf.GetSqlInjectionMatchSetOutput)

	GetSqlInjectionMatchSet(*waf.GetSqlInjectionMatchSetInput) (*waf.GetSqlInjectionMatchSetOutput, error)

	GetWebACLRequest(*waf.GetWebACLInput) (*request.Request, *waf.GetWebACLOutput)

	GetWebACL(*waf.GetWebACLInput) (*waf.GetWebACLOutput, error)

	GetXssMatchSetRequest(*waf.GetXssMatchSetInput) (*request.Request, *waf.GetXssMatchSetOutput)

	GetXssMatchSet(*waf.GetXssMatchSetInput) (*waf.GetXssMatchSetOutput, error)

	ListByteMatchSetsRequest(*waf.ListByteMatchSetsInput) (*request.Request, *waf.ListByteMatchSetsOutput)

	ListByteMatchSets(*waf.ListByteMatchSetsInput) (*waf.ListByteMatchSetsOutput, error)

	ListIPSetsRequest(*waf.ListIPSetsInput) (*request.Request, *waf.ListIPSetsOutput)

	ListIPSets(*waf.ListIPSetsInput) (*waf.ListIPSetsOutput, error)

	ListRulesRequest(*waf.ListRulesInput) (*request.Request, *waf.ListRulesOutput)

	ListRules(*waf.ListRulesInput) (*waf.ListRulesOutput, error)

	ListSizeConstraintSetsRequest(*waf.ListSizeConstraintSetsInput) (*request.Request, *waf.ListSizeConstraintSetsOutput)

	ListSizeConstraintSets(*waf.ListSizeConstraintSetsInput) (*waf.ListSizeConstraintSetsOutput, error)

	ListSqlInjectionMatchSetsRequest(*waf.ListSqlInjectionMatchSetsInput) (*request.Request, *waf.ListSqlInjectionMatchSetsOutput)

	ListSqlInjectionMatchSets(*waf.ListSqlInjectionMatchSetsInput) (*waf.ListSqlInjectionMatchSetsOutput, error)

	ListWebACLsRequest(*waf.ListWebACLsInput) (*request.Request, *waf.ListWebACLsOutput)

	ListWebACLs(*waf.ListWebACLsInput) (*waf.ListWebACLsOutput, error)

	ListXssMatchSetsRequest(*waf.ListXssMatchSetsInput) (*request.Request, *waf.ListXssMatchSetsOutput)

	ListXssMatchSets(*waf.ListXssMatchSetsInput) (*waf.ListXssMatchSetsOutput, error)

	UpdateByteMatchSetRequest(*waf.UpdateByteMatchSetInput) (*request.Request, *waf.UpdateByteMatchSetOutput)

	UpdateByteMatchSet(*waf.UpdateByteMatchSetInput) (*waf.UpdateByteMatchSetOutput, error)

	UpdateIPSetRequest(*waf.UpdateIPSetInput) (*request.Request, *waf.UpdateIPSetOutput)

	UpdateIPSet(*waf.UpdateIPSetInput) (*waf.UpdateIPSetOutput, error)

	UpdateRuleRequest(*waf.UpdateRuleInput) (*request.Request, *waf.UpdateRuleOutput)

	UpdateRule(*waf.UpdateRuleInput) (*waf.UpdateRuleOutput, error)

	UpdateSizeConstraintSetRequest(*waf.UpdateSizeConstraintSetInput) (*request.Request, *waf.UpdateSizeConstraintSetOutput)

	UpdateSizeConstraintSet(*waf.UpdateSizeConstraintSetInput) (*waf.UpdateSizeConstraintSetOutput, error)

	UpdateSqlInjectionMatchSetRequest(*waf.UpdateSqlInjectionMatchSetInput) (*request.Request, *waf.UpdateSqlInjectionMatchSetOutput)

	UpdateSqlInjectionMatchSet(*waf.UpdateSqlInjectionMatchSetInput) (*waf.UpdateSqlInjectionMatchSetOutput, error)

	UpdateWebACLRequest(*waf.UpdateWebACLInput) (*request.Request, *waf.UpdateWebACLOutput)

	UpdateWebACL(*waf.UpdateWebACLInput) (*waf.UpdateWebACLOutput, error)

	UpdateXssMatchSetRequest(*waf.UpdateXssMatchSetInput) (*request.Request, *waf.UpdateXssMatchSetOutput)

	UpdateXssMatchSet(*waf.UpdateXssMatchSetInput) (*waf.UpdateXssMatchSetOutput, error)
}

var _ WAFAPI = (*waf.WAF)(nil)
