// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

// Package gameliftiface provides an interface to enable mocking the Amazon GameLift service client
// for testing your code.
//
// It is important to note that this interface will have breaking changes
// when the service model is updated and adds new API operations, paginators,
// and waiters.
package gameliftiface

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/gamelift"
)

// GameLiftAPI provides an interface to enable mocking the
// gamelift.GameLift service client's API operation,
// paginators, and waiters. This make unit testing your code that calls out
// to the SDK's service client's calls easier.
//
// The best way to use this interface is so the SDK's service client's calls
// can be stubbed out for unit testing your code with the SDK without needing
// to inject custom request handlers into the SDK's request pipeline.
//
//    // myFunc uses an SDK service client to make a request to
//    // Amazon GameLift.
//    func myFunc(svc gameliftiface.GameLiftAPI) bool {
//        // Make svc.AcceptMatch request
//    }
//
//    func main() {
//        sess := session.New()
//        svc := gamelift.New(sess)
//
//        myFunc(svc)
//    }
//
// In your _test.go file:
//
//    // Define a mock struct to be used in your unit tests of myFunc.
//    type mockGameLiftClient struct {
//        gameliftiface.GameLiftAPI
//    }
//    func (m *mockGameLiftClient) AcceptMatch(input *gamelift.AcceptMatchInput) (*gamelift.AcceptMatchOutput, error) {
//        // mock response/functionality
//    }
//
//    func TestMyFunc(t *testing.T) {
//        // Setup Test
//        mockSvc := &mockGameLiftClient{}
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
type GameLiftAPI interface {
	AcceptMatch(*gamelift.AcceptMatchInput) (*gamelift.AcceptMatchOutput, error)
	AcceptMatchWithContext(aws.Context, *gamelift.AcceptMatchInput, ...request.Option) (*gamelift.AcceptMatchOutput, error)
	AcceptMatchRequest(*gamelift.AcceptMatchInput) (*request.Request, *gamelift.AcceptMatchOutput)

	CreateAlias(*gamelift.CreateAliasInput) (*gamelift.CreateAliasOutput, error)
	CreateAliasWithContext(aws.Context, *gamelift.CreateAliasInput, ...request.Option) (*gamelift.CreateAliasOutput, error)
	CreateAliasRequest(*gamelift.CreateAliasInput) (*request.Request, *gamelift.CreateAliasOutput)

	CreateBuild(*gamelift.CreateBuildInput) (*gamelift.CreateBuildOutput, error)
	CreateBuildWithContext(aws.Context, *gamelift.CreateBuildInput, ...request.Option) (*gamelift.CreateBuildOutput, error)
	CreateBuildRequest(*gamelift.CreateBuildInput) (*request.Request, *gamelift.CreateBuildOutput)

	CreateFleet(*gamelift.CreateFleetInput) (*gamelift.CreateFleetOutput, error)
	CreateFleetWithContext(aws.Context, *gamelift.CreateFleetInput, ...request.Option) (*gamelift.CreateFleetOutput, error)
	CreateFleetRequest(*gamelift.CreateFleetInput) (*request.Request, *gamelift.CreateFleetOutput)

	CreateGameSession(*gamelift.CreateGameSessionInput) (*gamelift.CreateGameSessionOutput, error)
	CreateGameSessionWithContext(aws.Context, *gamelift.CreateGameSessionInput, ...request.Option) (*gamelift.CreateGameSessionOutput, error)
	CreateGameSessionRequest(*gamelift.CreateGameSessionInput) (*request.Request, *gamelift.CreateGameSessionOutput)

	CreateGameSessionQueue(*gamelift.CreateGameSessionQueueInput) (*gamelift.CreateGameSessionQueueOutput, error)
	CreateGameSessionQueueWithContext(aws.Context, *gamelift.CreateGameSessionQueueInput, ...request.Option) (*gamelift.CreateGameSessionQueueOutput, error)
	CreateGameSessionQueueRequest(*gamelift.CreateGameSessionQueueInput) (*request.Request, *gamelift.CreateGameSessionQueueOutput)

	CreateMatchmakingConfiguration(*gamelift.CreateMatchmakingConfigurationInput) (*gamelift.CreateMatchmakingConfigurationOutput, error)
	CreateMatchmakingConfigurationWithContext(aws.Context, *gamelift.CreateMatchmakingConfigurationInput, ...request.Option) (*gamelift.CreateMatchmakingConfigurationOutput, error)
	CreateMatchmakingConfigurationRequest(*gamelift.CreateMatchmakingConfigurationInput) (*request.Request, *gamelift.CreateMatchmakingConfigurationOutput)

	CreateMatchmakingRuleSet(*gamelift.CreateMatchmakingRuleSetInput) (*gamelift.CreateMatchmakingRuleSetOutput, error)
	CreateMatchmakingRuleSetWithContext(aws.Context, *gamelift.CreateMatchmakingRuleSetInput, ...request.Option) (*gamelift.CreateMatchmakingRuleSetOutput, error)
	CreateMatchmakingRuleSetRequest(*gamelift.CreateMatchmakingRuleSetInput) (*request.Request, *gamelift.CreateMatchmakingRuleSetOutput)

	CreatePlayerSession(*gamelift.CreatePlayerSessionInput) (*gamelift.CreatePlayerSessionOutput, error)
	CreatePlayerSessionWithContext(aws.Context, *gamelift.CreatePlayerSessionInput, ...request.Option) (*gamelift.CreatePlayerSessionOutput, error)
	CreatePlayerSessionRequest(*gamelift.CreatePlayerSessionInput) (*request.Request, *gamelift.CreatePlayerSessionOutput)

	CreatePlayerSessions(*gamelift.CreatePlayerSessionsInput) (*gamelift.CreatePlayerSessionsOutput, error)
	CreatePlayerSessionsWithContext(aws.Context, *gamelift.CreatePlayerSessionsInput, ...request.Option) (*gamelift.CreatePlayerSessionsOutput, error)
	CreatePlayerSessionsRequest(*gamelift.CreatePlayerSessionsInput) (*request.Request, *gamelift.CreatePlayerSessionsOutput)

	CreateScript(*gamelift.CreateScriptInput) (*gamelift.CreateScriptOutput, error)
	CreateScriptWithContext(aws.Context, *gamelift.CreateScriptInput, ...request.Option) (*gamelift.CreateScriptOutput, error)
	CreateScriptRequest(*gamelift.CreateScriptInput) (*request.Request, *gamelift.CreateScriptOutput)

	CreateVpcPeeringAuthorization(*gamelift.CreateVpcPeeringAuthorizationInput) (*gamelift.CreateVpcPeeringAuthorizationOutput, error)
	CreateVpcPeeringAuthorizationWithContext(aws.Context, *gamelift.CreateVpcPeeringAuthorizationInput, ...request.Option) (*gamelift.CreateVpcPeeringAuthorizationOutput, error)
	CreateVpcPeeringAuthorizationRequest(*gamelift.CreateVpcPeeringAuthorizationInput) (*request.Request, *gamelift.CreateVpcPeeringAuthorizationOutput)

	CreateVpcPeeringConnection(*gamelift.CreateVpcPeeringConnectionInput) (*gamelift.CreateVpcPeeringConnectionOutput, error)
	CreateVpcPeeringConnectionWithContext(aws.Context, *gamelift.CreateVpcPeeringConnectionInput, ...request.Option) (*gamelift.CreateVpcPeeringConnectionOutput, error)
	CreateVpcPeeringConnectionRequest(*gamelift.CreateVpcPeeringConnectionInput) (*request.Request, *gamelift.CreateVpcPeeringConnectionOutput)

	DeleteAlias(*gamelift.DeleteAliasInput) (*gamelift.DeleteAliasOutput, error)
	DeleteAliasWithContext(aws.Context, *gamelift.DeleteAliasInput, ...request.Option) (*gamelift.DeleteAliasOutput, error)
	DeleteAliasRequest(*gamelift.DeleteAliasInput) (*request.Request, *gamelift.DeleteAliasOutput)

	DeleteBuild(*gamelift.DeleteBuildInput) (*gamelift.DeleteBuildOutput, error)
	DeleteBuildWithContext(aws.Context, *gamelift.DeleteBuildInput, ...request.Option) (*gamelift.DeleteBuildOutput, error)
	DeleteBuildRequest(*gamelift.DeleteBuildInput) (*request.Request, *gamelift.DeleteBuildOutput)

	DeleteFleet(*gamelift.DeleteFleetInput) (*gamelift.DeleteFleetOutput, error)
	DeleteFleetWithContext(aws.Context, *gamelift.DeleteFleetInput, ...request.Option) (*gamelift.DeleteFleetOutput, error)
	DeleteFleetRequest(*gamelift.DeleteFleetInput) (*request.Request, *gamelift.DeleteFleetOutput)

	DeleteGameSessionQueue(*gamelift.DeleteGameSessionQueueInput) (*gamelift.DeleteGameSessionQueueOutput, error)
	DeleteGameSessionQueueWithContext(aws.Context, *gamelift.DeleteGameSessionQueueInput, ...request.Option) (*gamelift.DeleteGameSessionQueueOutput, error)
	DeleteGameSessionQueueRequest(*gamelift.DeleteGameSessionQueueInput) (*request.Request, *gamelift.DeleteGameSessionQueueOutput)

	DeleteMatchmakingConfiguration(*gamelift.DeleteMatchmakingConfigurationInput) (*gamelift.DeleteMatchmakingConfigurationOutput, error)
	DeleteMatchmakingConfigurationWithContext(aws.Context, *gamelift.DeleteMatchmakingConfigurationInput, ...request.Option) (*gamelift.DeleteMatchmakingConfigurationOutput, error)
	DeleteMatchmakingConfigurationRequest(*gamelift.DeleteMatchmakingConfigurationInput) (*request.Request, *gamelift.DeleteMatchmakingConfigurationOutput)

	DeleteMatchmakingRuleSet(*gamelift.DeleteMatchmakingRuleSetInput) (*gamelift.DeleteMatchmakingRuleSetOutput, error)
	DeleteMatchmakingRuleSetWithContext(aws.Context, *gamelift.DeleteMatchmakingRuleSetInput, ...request.Option) (*gamelift.DeleteMatchmakingRuleSetOutput, error)
	DeleteMatchmakingRuleSetRequest(*gamelift.DeleteMatchmakingRuleSetInput) (*request.Request, *gamelift.DeleteMatchmakingRuleSetOutput)

	DeleteScalingPolicy(*gamelift.DeleteScalingPolicyInput) (*gamelift.DeleteScalingPolicyOutput, error)
	DeleteScalingPolicyWithContext(aws.Context, *gamelift.DeleteScalingPolicyInput, ...request.Option) (*gamelift.DeleteScalingPolicyOutput, error)
	DeleteScalingPolicyRequest(*gamelift.DeleteScalingPolicyInput) (*request.Request, *gamelift.DeleteScalingPolicyOutput)

	DeleteScript(*gamelift.DeleteScriptInput) (*gamelift.DeleteScriptOutput, error)
	DeleteScriptWithContext(aws.Context, *gamelift.DeleteScriptInput, ...request.Option) (*gamelift.DeleteScriptOutput, error)
	DeleteScriptRequest(*gamelift.DeleteScriptInput) (*request.Request, *gamelift.DeleteScriptOutput)

	DeleteVpcPeeringAuthorization(*gamelift.DeleteVpcPeeringAuthorizationInput) (*gamelift.DeleteVpcPeeringAuthorizationOutput, error)
	DeleteVpcPeeringAuthorizationWithContext(aws.Context, *gamelift.DeleteVpcPeeringAuthorizationInput, ...request.Option) (*gamelift.DeleteVpcPeeringAuthorizationOutput, error)
	DeleteVpcPeeringAuthorizationRequest(*gamelift.DeleteVpcPeeringAuthorizationInput) (*request.Request, *gamelift.DeleteVpcPeeringAuthorizationOutput)

	DeleteVpcPeeringConnection(*gamelift.DeleteVpcPeeringConnectionInput) (*gamelift.DeleteVpcPeeringConnectionOutput, error)
	DeleteVpcPeeringConnectionWithContext(aws.Context, *gamelift.DeleteVpcPeeringConnectionInput, ...request.Option) (*gamelift.DeleteVpcPeeringConnectionOutput, error)
	DeleteVpcPeeringConnectionRequest(*gamelift.DeleteVpcPeeringConnectionInput) (*request.Request, *gamelift.DeleteVpcPeeringConnectionOutput)

	DescribeAlias(*gamelift.DescribeAliasInput) (*gamelift.DescribeAliasOutput, error)
	DescribeAliasWithContext(aws.Context, *gamelift.DescribeAliasInput, ...request.Option) (*gamelift.DescribeAliasOutput, error)
	DescribeAliasRequest(*gamelift.DescribeAliasInput) (*request.Request, *gamelift.DescribeAliasOutput)

	DescribeBuild(*gamelift.DescribeBuildInput) (*gamelift.DescribeBuildOutput, error)
	DescribeBuildWithContext(aws.Context, *gamelift.DescribeBuildInput, ...request.Option) (*gamelift.DescribeBuildOutput, error)
	DescribeBuildRequest(*gamelift.DescribeBuildInput) (*request.Request, *gamelift.DescribeBuildOutput)

	DescribeEC2InstanceLimits(*gamelift.DescribeEC2InstanceLimitsInput) (*gamelift.DescribeEC2InstanceLimitsOutput, error)
	DescribeEC2InstanceLimitsWithContext(aws.Context, *gamelift.DescribeEC2InstanceLimitsInput, ...request.Option) (*gamelift.DescribeEC2InstanceLimitsOutput, error)
	DescribeEC2InstanceLimitsRequest(*gamelift.DescribeEC2InstanceLimitsInput) (*request.Request, *gamelift.DescribeEC2InstanceLimitsOutput)

	DescribeFleetAttributes(*gamelift.DescribeFleetAttributesInput) (*gamelift.DescribeFleetAttributesOutput, error)
	DescribeFleetAttributesWithContext(aws.Context, *gamelift.DescribeFleetAttributesInput, ...request.Option) (*gamelift.DescribeFleetAttributesOutput, error)
	DescribeFleetAttributesRequest(*gamelift.DescribeFleetAttributesInput) (*request.Request, *gamelift.DescribeFleetAttributesOutput)

	DescribeFleetCapacity(*gamelift.DescribeFleetCapacityInput) (*gamelift.DescribeFleetCapacityOutput, error)
	DescribeFleetCapacityWithContext(aws.Context, *gamelift.DescribeFleetCapacityInput, ...request.Option) (*gamelift.DescribeFleetCapacityOutput, error)
	DescribeFleetCapacityRequest(*gamelift.DescribeFleetCapacityInput) (*request.Request, *gamelift.DescribeFleetCapacityOutput)

	DescribeFleetEvents(*gamelift.DescribeFleetEventsInput) (*gamelift.DescribeFleetEventsOutput, error)
	DescribeFleetEventsWithContext(aws.Context, *gamelift.DescribeFleetEventsInput, ...request.Option) (*gamelift.DescribeFleetEventsOutput, error)
	DescribeFleetEventsRequest(*gamelift.DescribeFleetEventsInput) (*request.Request, *gamelift.DescribeFleetEventsOutput)

	DescribeFleetPortSettings(*gamelift.DescribeFleetPortSettingsInput) (*gamelift.DescribeFleetPortSettingsOutput, error)
	DescribeFleetPortSettingsWithContext(aws.Context, *gamelift.DescribeFleetPortSettingsInput, ...request.Option) (*gamelift.DescribeFleetPortSettingsOutput, error)
	DescribeFleetPortSettingsRequest(*gamelift.DescribeFleetPortSettingsInput) (*request.Request, *gamelift.DescribeFleetPortSettingsOutput)

	DescribeFleetUtilization(*gamelift.DescribeFleetUtilizationInput) (*gamelift.DescribeFleetUtilizationOutput, error)
	DescribeFleetUtilizationWithContext(aws.Context, *gamelift.DescribeFleetUtilizationInput, ...request.Option) (*gamelift.DescribeFleetUtilizationOutput, error)
	DescribeFleetUtilizationRequest(*gamelift.DescribeFleetUtilizationInput) (*request.Request, *gamelift.DescribeFleetUtilizationOutput)

	DescribeGameSessionDetails(*gamelift.DescribeGameSessionDetailsInput) (*gamelift.DescribeGameSessionDetailsOutput, error)
	DescribeGameSessionDetailsWithContext(aws.Context, *gamelift.DescribeGameSessionDetailsInput, ...request.Option) (*gamelift.DescribeGameSessionDetailsOutput, error)
	DescribeGameSessionDetailsRequest(*gamelift.DescribeGameSessionDetailsInput) (*request.Request, *gamelift.DescribeGameSessionDetailsOutput)

	DescribeGameSessionPlacement(*gamelift.DescribeGameSessionPlacementInput) (*gamelift.DescribeGameSessionPlacementOutput, error)
	DescribeGameSessionPlacementWithContext(aws.Context, *gamelift.DescribeGameSessionPlacementInput, ...request.Option) (*gamelift.DescribeGameSessionPlacementOutput, error)
	DescribeGameSessionPlacementRequest(*gamelift.DescribeGameSessionPlacementInput) (*request.Request, *gamelift.DescribeGameSessionPlacementOutput)

	DescribeGameSessionQueues(*gamelift.DescribeGameSessionQueuesInput) (*gamelift.DescribeGameSessionQueuesOutput, error)
	DescribeGameSessionQueuesWithContext(aws.Context, *gamelift.DescribeGameSessionQueuesInput, ...request.Option) (*gamelift.DescribeGameSessionQueuesOutput, error)
	DescribeGameSessionQueuesRequest(*gamelift.DescribeGameSessionQueuesInput) (*request.Request, *gamelift.DescribeGameSessionQueuesOutput)

	DescribeGameSessions(*gamelift.DescribeGameSessionsInput) (*gamelift.DescribeGameSessionsOutput, error)
	DescribeGameSessionsWithContext(aws.Context, *gamelift.DescribeGameSessionsInput, ...request.Option) (*gamelift.DescribeGameSessionsOutput, error)
	DescribeGameSessionsRequest(*gamelift.DescribeGameSessionsInput) (*request.Request, *gamelift.DescribeGameSessionsOutput)

	DescribeInstances(*gamelift.DescribeInstancesInput) (*gamelift.DescribeInstancesOutput, error)
	DescribeInstancesWithContext(aws.Context, *gamelift.DescribeInstancesInput, ...request.Option) (*gamelift.DescribeInstancesOutput, error)
	DescribeInstancesRequest(*gamelift.DescribeInstancesInput) (*request.Request, *gamelift.DescribeInstancesOutput)

	DescribeMatchmaking(*gamelift.DescribeMatchmakingInput) (*gamelift.DescribeMatchmakingOutput, error)
	DescribeMatchmakingWithContext(aws.Context, *gamelift.DescribeMatchmakingInput, ...request.Option) (*gamelift.DescribeMatchmakingOutput, error)
	DescribeMatchmakingRequest(*gamelift.DescribeMatchmakingInput) (*request.Request, *gamelift.DescribeMatchmakingOutput)

	DescribeMatchmakingConfigurations(*gamelift.DescribeMatchmakingConfigurationsInput) (*gamelift.DescribeMatchmakingConfigurationsOutput, error)
	DescribeMatchmakingConfigurationsWithContext(aws.Context, *gamelift.DescribeMatchmakingConfigurationsInput, ...request.Option) (*gamelift.DescribeMatchmakingConfigurationsOutput, error)
	DescribeMatchmakingConfigurationsRequest(*gamelift.DescribeMatchmakingConfigurationsInput) (*request.Request, *gamelift.DescribeMatchmakingConfigurationsOutput)

	DescribeMatchmakingRuleSets(*gamelift.DescribeMatchmakingRuleSetsInput) (*gamelift.DescribeMatchmakingRuleSetsOutput, error)
	DescribeMatchmakingRuleSetsWithContext(aws.Context, *gamelift.DescribeMatchmakingRuleSetsInput, ...request.Option) (*gamelift.DescribeMatchmakingRuleSetsOutput, error)
	DescribeMatchmakingRuleSetsRequest(*gamelift.DescribeMatchmakingRuleSetsInput) (*request.Request, *gamelift.DescribeMatchmakingRuleSetsOutput)

	DescribePlayerSessions(*gamelift.DescribePlayerSessionsInput) (*gamelift.DescribePlayerSessionsOutput, error)
	DescribePlayerSessionsWithContext(aws.Context, *gamelift.DescribePlayerSessionsInput, ...request.Option) (*gamelift.DescribePlayerSessionsOutput, error)
	DescribePlayerSessionsRequest(*gamelift.DescribePlayerSessionsInput) (*request.Request, *gamelift.DescribePlayerSessionsOutput)

	DescribeRuntimeConfiguration(*gamelift.DescribeRuntimeConfigurationInput) (*gamelift.DescribeRuntimeConfigurationOutput, error)
	DescribeRuntimeConfigurationWithContext(aws.Context, *gamelift.DescribeRuntimeConfigurationInput, ...request.Option) (*gamelift.DescribeRuntimeConfigurationOutput, error)
	DescribeRuntimeConfigurationRequest(*gamelift.DescribeRuntimeConfigurationInput) (*request.Request, *gamelift.DescribeRuntimeConfigurationOutput)

	DescribeScalingPolicies(*gamelift.DescribeScalingPoliciesInput) (*gamelift.DescribeScalingPoliciesOutput, error)
	DescribeScalingPoliciesWithContext(aws.Context, *gamelift.DescribeScalingPoliciesInput, ...request.Option) (*gamelift.DescribeScalingPoliciesOutput, error)
	DescribeScalingPoliciesRequest(*gamelift.DescribeScalingPoliciesInput) (*request.Request, *gamelift.DescribeScalingPoliciesOutput)

	DescribeScript(*gamelift.DescribeScriptInput) (*gamelift.DescribeScriptOutput, error)
	DescribeScriptWithContext(aws.Context, *gamelift.DescribeScriptInput, ...request.Option) (*gamelift.DescribeScriptOutput, error)
	DescribeScriptRequest(*gamelift.DescribeScriptInput) (*request.Request, *gamelift.DescribeScriptOutput)

	DescribeVpcPeeringAuthorizations(*gamelift.DescribeVpcPeeringAuthorizationsInput) (*gamelift.DescribeVpcPeeringAuthorizationsOutput, error)
	DescribeVpcPeeringAuthorizationsWithContext(aws.Context, *gamelift.DescribeVpcPeeringAuthorizationsInput, ...request.Option) (*gamelift.DescribeVpcPeeringAuthorizationsOutput, error)
	DescribeVpcPeeringAuthorizationsRequest(*gamelift.DescribeVpcPeeringAuthorizationsInput) (*request.Request, *gamelift.DescribeVpcPeeringAuthorizationsOutput)

	DescribeVpcPeeringConnections(*gamelift.DescribeVpcPeeringConnectionsInput) (*gamelift.DescribeVpcPeeringConnectionsOutput, error)
	DescribeVpcPeeringConnectionsWithContext(aws.Context, *gamelift.DescribeVpcPeeringConnectionsInput, ...request.Option) (*gamelift.DescribeVpcPeeringConnectionsOutput, error)
	DescribeVpcPeeringConnectionsRequest(*gamelift.DescribeVpcPeeringConnectionsInput) (*request.Request, *gamelift.DescribeVpcPeeringConnectionsOutput)

	GetGameSessionLogUrl(*gamelift.GetGameSessionLogUrlInput) (*gamelift.GetGameSessionLogUrlOutput, error)
	GetGameSessionLogUrlWithContext(aws.Context, *gamelift.GetGameSessionLogUrlInput, ...request.Option) (*gamelift.GetGameSessionLogUrlOutput, error)
	GetGameSessionLogUrlRequest(*gamelift.GetGameSessionLogUrlInput) (*request.Request, *gamelift.GetGameSessionLogUrlOutput)

	GetInstanceAccess(*gamelift.GetInstanceAccessInput) (*gamelift.GetInstanceAccessOutput, error)
	GetInstanceAccessWithContext(aws.Context, *gamelift.GetInstanceAccessInput, ...request.Option) (*gamelift.GetInstanceAccessOutput, error)
	GetInstanceAccessRequest(*gamelift.GetInstanceAccessInput) (*request.Request, *gamelift.GetInstanceAccessOutput)

	ListAliases(*gamelift.ListAliasesInput) (*gamelift.ListAliasesOutput, error)
	ListAliasesWithContext(aws.Context, *gamelift.ListAliasesInput, ...request.Option) (*gamelift.ListAliasesOutput, error)
	ListAliasesRequest(*gamelift.ListAliasesInput) (*request.Request, *gamelift.ListAliasesOutput)

	ListBuilds(*gamelift.ListBuildsInput) (*gamelift.ListBuildsOutput, error)
	ListBuildsWithContext(aws.Context, *gamelift.ListBuildsInput, ...request.Option) (*gamelift.ListBuildsOutput, error)
	ListBuildsRequest(*gamelift.ListBuildsInput) (*request.Request, *gamelift.ListBuildsOutput)

	ListFleets(*gamelift.ListFleetsInput) (*gamelift.ListFleetsOutput, error)
	ListFleetsWithContext(aws.Context, *gamelift.ListFleetsInput, ...request.Option) (*gamelift.ListFleetsOutput, error)
	ListFleetsRequest(*gamelift.ListFleetsInput) (*request.Request, *gamelift.ListFleetsOutput)

	ListScripts(*gamelift.ListScriptsInput) (*gamelift.ListScriptsOutput, error)
	ListScriptsWithContext(aws.Context, *gamelift.ListScriptsInput, ...request.Option) (*gamelift.ListScriptsOutput, error)
	ListScriptsRequest(*gamelift.ListScriptsInput) (*request.Request, *gamelift.ListScriptsOutput)

	PutScalingPolicy(*gamelift.PutScalingPolicyInput) (*gamelift.PutScalingPolicyOutput, error)
	PutScalingPolicyWithContext(aws.Context, *gamelift.PutScalingPolicyInput, ...request.Option) (*gamelift.PutScalingPolicyOutput, error)
	PutScalingPolicyRequest(*gamelift.PutScalingPolicyInput) (*request.Request, *gamelift.PutScalingPolicyOutput)

	RequestUploadCredentials(*gamelift.RequestUploadCredentialsInput) (*gamelift.RequestUploadCredentialsOutput, error)
	RequestUploadCredentialsWithContext(aws.Context, *gamelift.RequestUploadCredentialsInput, ...request.Option) (*gamelift.RequestUploadCredentialsOutput, error)
	RequestUploadCredentialsRequest(*gamelift.RequestUploadCredentialsInput) (*request.Request, *gamelift.RequestUploadCredentialsOutput)

	ResolveAlias(*gamelift.ResolveAliasInput) (*gamelift.ResolveAliasOutput, error)
	ResolveAliasWithContext(aws.Context, *gamelift.ResolveAliasInput, ...request.Option) (*gamelift.ResolveAliasOutput, error)
	ResolveAliasRequest(*gamelift.ResolveAliasInput) (*request.Request, *gamelift.ResolveAliasOutput)

	SearchGameSessions(*gamelift.SearchGameSessionsInput) (*gamelift.SearchGameSessionsOutput, error)
	SearchGameSessionsWithContext(aws.Context, *gamelift.SearchGameSessionsInput, ...request.Option) (*gamelift.SearchGameSessionsOutput, error)
	SearchGameSessionsRequest(*gamelift.SearchGameSessionsInput) (*request.Request, *gamelift.SearchGameSessionsOutput)

	StartFleetActions(*gamelift.StartFleetActionsInput) (*gamelift.StartFleetActionsOutput, error)
	StartFleetActionsWithContext(aws.Context, *gamelift.StartFleetActionsInput, ...request.Option) (*gamelift.StartFleetActionsOutput, error)
	StartFleetActionsRequest(*gamelift.StartFleetActionsInput) (*request.Request, *gamelift.StartFleetActionsOutput)

	StartGameSessionPlacement(*gamelift.StartGameSessionPlacementInput) (*gamelift.StartGameSessionPlacementOutput, error)
	StartGameSessionPlacementWithContext(aws.Context, *gamelift.StartGameSessionPlacementInput, ...request.Option) (*gamelift.StartGameSessionPlacementOutput, error)
	StartGameSessionPlacementRequest(*gamelift.StartGameSessionPlacementInput) (*request.Request, *gamelift.StartGameSessionPlacementOutput)

	StartMatchBackfill(*gamelift.StartMatchBackfillInput) (*gamelift.StartMatchBackfillOutput, error)
	StartMatchBackfillWithContext(aws.Context, *gamelift.StartMatchBackfillInput, ...request.Option) (*gamelift.StartMatchBackfillOutput, error)
	StartMatchBackfillRequest(*gamelift.StartMatchBackfillInput) (*request.Request, *gamelift.StartMatchBackfillOutput)

	StartMatchmaking(*gamelift.StartMatchmakingInput) (*gamelift.StartMatchmakingOutput, error)
	StartMatchmakingWithContext(aws.Context, *gamelift.StartMatchmakingInput, ...request.Option) (*gamelift.StartMatchmakingOutput, error)
	StartMatchmakingRequest(*gamelift.StartMatchmakingInput) (*request.Request, *gamelift.StartMatchmakingOutput)

	StopFleetActions(*gamelift.StopFleetActionsInput) (*gamelift.StopFleetActionsOutput, error)
	StopFleetActionsWithContext(aws.Context, *gamelift.StopFleetActionsInput, ...request.Option) (*gamelift.StopFleetActionsOutput, error)
	StopFleetActionsRequest(*gamelift.StopFleetActionsInput) (*request.Request, *gamelift.StopFleetActionsOutput)

	StopGameSessionPlacement(*gamelift.StopGameSessionPlacementInput) (*gamelift.StopGameSessionPlacementOutput, error)
	StopGameSessionPlacementWithContext(aws.Context, *gamelift.StopGameSessionPlacementInput, ...request.Option) (*gamelift.StopGameSessionPlacementOutput, error)
	StopGameSessionPlacementRequest(*gamelift.StopGameSessionPlacementInput) (*request.Request, *gamelift.StopGameSessionPlacementOutput)

	StopMatchmaking(*gamelift.StopMatchmakingInput) (*gamelift.StopMatchmakingOutput, error)
	StopMatchmakingWithContext(aws.Context, *gamelift.StopMatchmakingInput, ...request.Option) (*gamelift.StopMatchmakingOutput, error)
	StopMatchmakingRequest(*gamelift.StopMatchmakingInput) (*request.Request, *gamelift.StopMatchmakingOutput)

	UpdateAlias(*gamelift.UpdateAliasInput) (*gamelift.UpdateAliasOutput, error)
	UpdateAliasWithContext(aws.Context, *gamelift.UpdateAliasInput, ...request.Option) (*gamelift.UpdateAliasOutput, error)
	UpdateAliasRequest(*gamelift.UpdateAliasInput) (*request.Request, *gamelift.UpdateAliasOutput)

	UpdateBuild(*gamelift.UpdateBuildInput) (*gamelift.UpdateBuildOutput, error)
	UpdateBuildWithContext(aws.Context, *gamelift.UpdateBuildInput, ...request.Option) (*gamelift.UpdateBuildOutput, error)
	UpdateBuildRequest(*gamelift.UpdateBuildInput) (*request.Request, *gamelift.UpdateBuildOutput)

	UpdateFleetAttributes(*gamelift.UpdateFleetAttributesInput) (*gamelift.UpdateFleetAttributesOutput, error)
	UpdateFleetAttributesWithContext(aws.Context, *gamelift.UpdateFleetAttributesInput, ...request.Option) (*gamelift.UpdateFleetAttributesOutput, error)
	UpdateFleetAttributesRequest(*gamelift.UpdateFleetAttributesInput) (*request.Request, *gamelift.UpdateFleetAttributesOutput)

	UpdateFleetCapacity(*gamelift.UpdateFleetCapacityInput) (*gamelift.UpdateFleetCapacityOutput, error)
	UpdateFleetCapacityWithContext(aws.Context, *gamelift.UpdateFleetCapacityInput, ...request.Option) (*gamelift.UpdateFleetCapacityOutput, error)
	UpdateFleetCapacityRequest(*gamelift.UpdateFleetCapacityInput) (*request.Request, *gamelift.UpdateFleetCapacityOutput)

	UpdateFleetPortSettings(*gamelift.UpdateFleetPortSettingsInput) (*gamelift.UpdateFleetPortSettingsOutput, error)
	UpdateFleetPortSettingsWithContext(aws.Context, *gamelift.UpdateFleetPortSettingsInput, ...request.Option) (*gamelift.UpdateFleetPortSettingsOutput, error)
	UpdateFleetPortSettingsRequest(*gamelift.UpdateFleetPortSettingsInput) (*request.Request, *gamelift.UpdateFleetPortSettingsOutput)

	UpdateGameSession(*gamelift.UpdateGameSessionInput) (*gamelift.UpdateGameSessionOutput, error)
	UpdateGameSessionWithContext(aws.Context, *gamelift.UpdateGameSessionInput, ...request.Option) (*gamelift.UpdateGameSessionOutput, error)
	UpdateGameSessionRequest(*gamelift.UpdateGameSessionInput) (*request.Request, *gamelift.UpdateGameSessionOutput)

	UpdateGameSessionQueue(*gamelift.UpdateGameSessionQueueInput) (*gamelift.UpdateGameSessionQueueOutput, error)
	UpdateGameSessionQueueWithContext(aws.Context, *gamelift.UpdateGameSessionQueueInput, ...request.Option) (*gamelift.UpdateGameSessionQueueOutput, error)
	UpdateGameSessionQueueRequest(*gamelift.UpdateGameSessionQueueInput) (*request.Request, *gamelift.UpdateGameSessionQueueOutput)

	UpdateMatchmakingConfiguration(*gamelift.UpdateMatchmakingConfigurationInput) (*gamelift.UpdateMatchmakingConfigurationOutput, error)
	UpdateMatchmakingConfigurationWithContext(aws.Context, *gamelift.UpdateMatchmakingConfigurationInput, ...request.Option) (*gamelift.UpdateMatchmakingConfigurationOutput, error)
	UpdateMatchmakingConfigurationRequest(*gamelift.UpdateMatchmakingConfigurationInput) (*request.Request, *gamelift.UpdateMatchmakingConfigurationOutput)

	UpdateRuntimeConfiguration(*gamelift.UpdateRuntimeConfigurationInput) (*gamelift.UpdateRuntimeConfigurationOutput, error)
	UpdateRuntimeConfigurationWithContext(aws.Context, *gamelift.UpdateRuntimeConfigurationInput, ...request.Option) (*gamelift.UpdateRuntimeConfigurationOutput, error)
	UpdateRuntimeConfigurationRequest(*gamelift.UpdateRuntimeConfigurationInput) (*request.Request, *gamelift.UpdateRuntimeConfigurationOutput)

	UpdateScript(*gamelift.UpdateScriptInput) (*gamelift.UpdateScriptOutput, error)
	UpdateScriptWithContext(aws.Context, *gamelift.UpdateScriptInput, ...request.Option) (*gamelift.UpdateScriptOutput, error)
	UpdateScriptRequest(*gamelift.UpdateScriptInput) (*request.Request, *gamelift.UpdateScriptOutput)

	ValidateMatchmakingRuleSet(*gamelift.ValidateMatchmakingRuleSetInput) (*gamelift.ValidateMatchmakingRuleSetOutput, error)
	ValidateMatchmakingRuleSetWithContext(aws.Context, *gamelift.ValidateMatchmakingRuleSetInput, ...request.Option) (*gamelift.ValidateMatchmakingRuleSetOutput, error)
	ValidateMatchmakingRuleSetRequest(*gamelift.ValidateMatchmakingRuleSetInput) (*request.Request, *gamelift.ValidateMatchmakingRuleSetOutput)
}

var _ GameLiftAPI = (*gamelift.GameLift)(nil)
