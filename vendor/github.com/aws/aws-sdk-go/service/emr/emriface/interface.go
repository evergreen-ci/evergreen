// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

// Package emriface provides an interface to enable mocking the Amazon Elastic MapReduce service client
// for testing your code.
//
// It is important to note that this interface will have breaking changes
// when the service model is updated and adds new API operations, paginators,
// and waiters.
package emriface

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/emr"
)

// EMRAPI provides an interface to enable mocking the
// emr.EMR service client's API operation,
// paginators, and waiters. This make unit testing your code that calls out
// to the SDK's service client's calls easier.
//
// The best way to use this interface is so the SDK's service client's calls
// can be stubbed out for unit testing your code with the SDK without needing
// to inject custom request handlers into the SDK's request pipeline.
//
//    // myFunc uses an SDK service client to make a request to
//    // Amazon Elastic MapReduce.
//    func myFunc(svc emriface.EMRAPI) bool {
//        // Make svc.AddInstanceFleet request
//    }
//
//    func main() {
//        sess := session.New()
//        svc := emr.New(sess)
//
//        myFunc(svc)
//    }
//
// In your _test.go file:
//
//    // Define a mock struct to be used in your unit tests of myFunc.
//    type mockEMRClient struct {
//        emriface.EMRAPI
//    }
//    func (m *mockEMRClient) AddInstanceFleet(input *emr.AddInstanceFleetInput) (*emr.AddInstanceFleetOutput, error) {
//        // mock response/functionality
//    }
//
//    func TestMyFunc(t *testing.T) {
//        // Setup Test
//        mockSvc := &mockEMRClient{}
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
type EMRAPI interface {
	AddInstanceFleet(*emr.AddInstanceFleetInput) (*emr.AddInstanceFleetOutput, error)
	AddInstanceFleetWithContext(aws.Context, *emr.AddInstanceFleetInput, ...request.Option) (*emr.AddInstanceFleetOutput, error)
	AddInstanceFleetRequest(*emr.AddInstanceFleetInput) (*request.Request, *emr.AddInstanceFleetOutput)

	AddInstanceGroups(*emr.AddInstanceGroupsInput) (*emr.AddInstanceGroupsOutput, error)
	AddInstanceGroupsWithContext(aws.Context, *emr.AddInstanceGroupsInput, ...request.Option) (*emr.AddInstanceGroupsOutput, error)
	AddInstanceGroupsRequest(*emr.AddInstanceGroupsInput) (*request.Request, *emr.AddInstanceGroupsOutput)

	AddJobFlowSteps(*emr.AddJobFlowStepsInput) (*emr.AddJobFlowStepsOutput, error)
	AddJobFlowStepsWithContext(aws.Context, *emr.AddJobFlowStepsInput, ...request.Option) (*emr.AddJobFlowStepsOutput, error)
	AddJobFlowStepsRequest(*emr.AddJobFlowStepsInput) (*request.Request, *emr.AddJobFlowStepsOutput)

	AddTags(*emr.AddTagsInput) (*emr.AddTagsOutput, error)
	AddTagsWithContext(aws.Context, *emr.AddTagsInput, ...request.Option) (*emr.AddTagsOutput, error)
	AddTagsRequest(*emr.AddTagsInput) (*request.Request, *emr.AddTagsOutput)

	CancelSteps(*emr.CancelStepsInput) (*emr.CancelStepsOutput, error)
	CancelStepsWithContext(aws.Context, *emr.CancelStepsInput, ...request.Option) (*emr.CancelStepsOutput, error)
	CancelStepsRequest(*emr.CancelStepsInput) (*request.Request, *emr.CancelStepsOutput)

	CreateSecurityConfiguration(*emr.CreateSecurityConfigurationInput) (*emr.CreateSecurityConfigurationOutput, error)
	CreateSecurityConfigurationWithContext(aws.Context, *emr.CreateSecurityConfigurationInput, ...request.Option) (*emr.CreateSecurityConfigurationOutput, error)
	CreateSecurityConfigurationRequest(*emr.CreateSecurityConfigurationInput) (*request.Request, *emr.CreateSecurityConfigurationOutput)

	CreateStudio(*emr.CreateStudioInput) (*emr.CreateStudioOutput, error)
	CreateStudioWithContext(aws.Context, *emr.CreateStudioInput, ...request.Option) (*emr.CreateStudioOutput, error)
	CreateStudioRequest(*emr.CreateStudioInput) (*request.Request, *emr.CreateStudioOutput)

	CreateStudioSessionMapping(*emr.CreateStudioSessionMappingInput) (*emr.CreateStudioSessionMappingOutput, error)
	CreateStudioSessionMappingWithContext(aws.Context, *emr.CreateStudioSessionMappingInput, ...request.Option) (*emr.CreateStudioSessionMappingOutput, error)
	CreateStudioSessionMappingRequest(*emr.CreateStudioSessionMappingInput) (*request.Request, *emr.CreateStudioSessionMappingOutput)

	DeleteSecurityConfiguration(*emr.DeleteSecurityConfigurationInput) (*emr.DeleteSecurityConfigurationOutput, error)
	DeleteSecurityConfigurationWithContext(aws.Context, *emr.DeleteSecurityConfigurationInput, ...request.Option) (*emr.DeleteSecurityConfigurationOutput, error)
	DeleteSecurityConfigurationRequest(*emr.DeleteSecurityConfigurationInput) (*request.Request, *emr.DeleteSecurityConfigurationOutput)

	DeleteStudio(*emr.DeleteStudioInput) (*emr.DeleteStudioOutput, error)
	DeleteStudioWithContext(aws.Context, *emr.DeleteStudioInput, ...request.Option) (*emr.DeleteStudioOutput, error)
	DeleteStudioRequest(*emr.DeleteStudioInput) (*request.Request, *emr.DeleteStudioOutput)

	DeleteStudioSessionMapping(*emr.DeleteStudioSessionMappingInput) (*emr.DeleteStudioSessionMappingOutput, error)
	DeleteStudioSessionMappingWithContext(aws.Context, *emr.DeleteStudioSessionMappingInput, ...request.Option) (*emr.DeleteStudioSessionMappingOutput, error)
	DeleteStudioSessionMappingRequest(*emr.DeleteStudioSessionMappingInput) (*request.Request, *emr.DeleteStudioSessionMappingOutput)

	DescribeCluster(*emr.DescribeClusterInput) (*emr.DescribeClusterOutput, error)
	DescribeClusterWithContext(aws.Context, *emr.DescribeClusterInput, ...request.Option) (*emr.DescribeClusterOutput, error)
	DescribeClusterRequest(*emr.DescribeClusterInput) (*request.Request, *emr.DescribeClusterOutput)

	DescribeJobFlows(*emr.DescribeJobFlowsInput) (*emr.DescribeJobFlowsOutput, error)
	DescribeJobFlowsWithContext(aws.Context, *emr.DescribeJobFlowsInput, ...request.Option) (*emr.DescribeJobFlowsOutput, error)
	DescribeJobFlowsRequest(*emr.DescribeJobFlowsInput) (*request.Request, *emr.DescribeJobFlowsOutput)

	DescribeNotebookExecution(*emr.DescribeNotebookExecutionInput) (*emr.DescribeNotebookExecutionOutput, error)
	DescribeNotebookExecutionWithContext(aws.Context, *emr.DescribeNotebookExecutionInput, ...request.Option) (*emr.DescribeNotebookExecutionOutput, error)
	DescribeNotebookExecutionRequest(*emr.DescribeNotebookExecutionInput) (*request.Request, *emr.DescribeNotebookExecutionOutput)

	DescribeSecurityConfiguration(*emr.DescribeSecurityConfigurationInput) (*emr.DescribeSecurityConfigurationOutput, error)
	DescribeSecurityConfigurationWithContext(aws.Context, *emr.DescribeSecurityConfigurationInput, ...request.Option) (*emr.DescribeSecurityConfigurationOutput, error)
	DescribeSecurityConfigurationRequest(*emr.DescribeSecurityConfigurationInput) (*request.Request, *emr.DescribeSecurityConfigurationOutput)

	DescribeStep(*emr.DescribeStepInput) (*emr.DescribeStepOutput, error)
	DescribeStepWithContext(aws.Context, *emr.DescribeStepInput, ...request.Option) (*emr.DescribeStepOutput, error)
	DescribeStepRequest(*emr.DescribeStepInput) (*request.Request, *emr.DescribeStepOutput)

	DescribeStudio(*emr.DescribeStudioInput) (*emr.DescribeStudioOutput, error)
	DescribeStudioWithContext(aws.Context, *emr.DescribeStudioInput, ...request.Option) (*emr.DescribeStudioOutput, error)
	DescribeStudioRequest(*emr.DescribeStudioInput) (*request.Request, *emr.DescribeStudioOutput)

	GetBlockPublicAccessConfiguration(*emr.GetBlockPublicAccessConfigurationInput) (*emr.GetBlockPublicAccessConfigurationOutput, error)
	GetBlockPublicAccessConfigurationWithContext(aws.Context, *emr.GetBlockPublicAccessConfigurationInput, ...request.Option) (*emr.GetBlockPublicAccessConfigurationOutput, error)
	GetBlockPublicAccessConfigurationRequest(*emr.GetBlockPublicAccessConfigurationInput) (*request.Request, *emr.GetBlockPublicAccessConfigurationOutput)

	GetManagedScalingPolicy(*emr.GetManagedScalingPolicyInput) (*emr.GetManagedScalingPolicyOutput, error)
	GetManagedScalingPolicyWithContext(aws.Context, *emr.GetManagedScalingPolicyInput, ...request.Option) (*emr.GetManagedScalingPolicyOutput, error)
	GetManagedScalingPolicyRequest(*emr.GetManagedScalingPolicyInput) (*request.Request, *emr.GetManagedScalingPolicyOutput)

	GetStudioSessionMapping(*emr.GetStudioSessionMappingInput) (*emr.GetStudioSessionMappingOutput, error)
	GetStudioSessionMappingWithContext(aws.Context, *emr.GetStudioSessionMappingInput, ...request.Option) (*emr.GetStudioSessionMappingOutput, error)
	GetStudioSessionMappingRequest(*emr.GetStudioSessionMappingInput) (*request.Request, *emr.GetStudioSessionMappingOutput)

	ListBootstrapActions(*emr.ListBootstrapActionsInput) (*emr.ListBootstrapActionsOutput, error)
	ListBootstrapActionsWithContext(aws.Context, *emr.ListBootstrapActionsInput, ...request.Option) (*emr.ListBootstrapActionsOutput, error)
	ListBootstrapActionsRequest(*emr.ListBootstrapActionsInput) (*request.Request, *emr.ListBootstrapActionsOutput)

	ListBootstrapActionsPages(*emr.ListBootstrapActionsInput, func(*emr.ListBootstrapActionsOutput, bool) bool) error
	ListBootstrapActionsPagesWithContext(aws.Context, *emr.ListBootstrapActionsInput, func(*emr.ListBootstrapActionsOutput, bool) bool, ...request.Option) error

	ListClusters(*emr.ListClustersInput) (*emr.ListClustersOutput, error)
	ListClustersWithContext(aws.Context, *emr.ListClustersInput, ...request.Option) (*emr.ListClustersOutput, error)
	ListClustersRequest(*emr.ListClustersInput) (*request.Request, *emr.ListClustersOutput)

	ListClustersPages(*emr.ListClustersInput, func(*emr.ListClustersOutput, bool) bool) error
	ListClustersPagesWithContext(aws.Context, *emr.ListClustersInput, func(*emr.ListClustersOutput, bool) bool, ...request.Option) error

	ListInstanceFleets(*emr.ListInstanceFleetsInput) (*emr.ListInstanceFleetsOutput, error)
	ListInstanceFleetsWithContext(aws.Context, *emr.ListInstanceFleetsInput, ...request.Option) (*emr.ListInstanceFleetsOutput, error)
	ListInstanceFleetsRequest(*emr.ListInstanceFleetsInput) (*request.Request, *emr.ListInstanceFleetsOutput)

	ListInstanceFleetsPages(*emr.ListInstanceFleetsInput, func(*emr.ListInstanceFleetsOutput, bool) bool) error
	ListInstanceFleetsPagesWithContext(aws.Context, *emr.ListInstanceFleetsInput, func(*emr.ListInstanceFleetsOutput, bool) bool, ...request.Option) error

	ListInstanceGroups(*emr.ListInstanceGroupsInput) (*emr.ListInstanceGroupsOutput, error)
	ListInstanceGroupsWithContext(aws.Context, *emr.ListInstanceGroupsInput, ...request.Option) (*emr.ListInstanceGroupsOutput, error)
	ListInstanceGroupsRequest(*emr.ListInstanceGroupsInput) (*request.Request, *emr.ListInstanceGroupsOutput)

	ListInstanceGroupsPages(*emr.ListInstanceGroupsInput, func(*emr.ListInstanceGroupsOutput, bool) bool) error
	ListInstanceGroupsPagesWithContext(aws.Context, *emr.ListInstanceGroupsInput, func(*emr.ListInstanceGroupsOutput, bool) bool, ...request.Option) error

	ListInstances(*emr.ListInstancesInput) (*emr.ListInstancesOutput, error)
	ListInstancesWithContext(aws.Context, *emr.ListInstancesInput, ...request.Option) (*emr.ListInstancesOutput, error)
	ListInstancesRequest(*emr.ListInstancesInput) (*request.Request, *emr.ListInstancesOutput)

	ListInstancesPages(*emr.ListInstancesInput, func(*emr.ListInstancesOutput, bool) bool) error
	ListInstancesPagesWithContext(aws.Context, *emr.ListInstancesInput, func(*emr.ListInstancesOutput, bool) bool, ...request.Option) error

	ListNotebookExecutions(*emr.ListNotebookExecutionsInput) (*emr.ListNotebookExecutionsOutput, error)
	ListNotebookExecutionsWithContext(aws.Context, *emr.ListNotebookExecutionsInput, ...request.Option) (*emr.ListNotebookExecutionsOutput, error)
	ListNotebookExecutionsRequest(*emr.ListNotebookExecutionsInput) (*request.Request, *emr.ListNotebookExecutionsOutput)

	ListNotebookExecutionsPages(*emr.ListNotebookExecutionsInput, func(*emr.ListNotebookExecutionsOutput, bool) bool) error
	ListNotebookExecutionsPagesWithContext(aws.Context, *emr.ListNotebookExecutionsInput, func(*emr.ListNotebookExecutionsOutput, bool) bool, ...request.Option) error

	ListSecurityConfigurations(*emr.ListSecurityConfigurationsInput) (*emr.ListSecurityConfigurationsOutput, error)
	ListSecurityConfigurationsWithContext(aws.Context, *emr.ListSecurityConfigurationsInput, ...request.Option) (*emr.ListSecurityConfigurationsOutput, error)
	ListSecurityConfigurationsRequest(*emr.ListSecurityConfigurationsInput) (*request.Request, *emr.ListSecurityConfigurationsOutput)

	ListSecurityConfigurationsPages(*emr.ListSecurityConfigurationsInput, func(*emr.ListSecurityConfigurationsOutput, bool) bool) error
	ListSecurityConfigurationsPagesWithContext(aws.Context, *emr.ListSecurityConfigurationsInput, func(*emr.ListSecurityConfigurationsOutput, bool) bool, ...request.Option) error

	ListSteps(*emr.ListStepsInput) (*emr.ListStepsOutput, error)
	ListStepsWithContext(aws.Context, *emr.ListStepsInput, ...request.Option) (*emr.ListStepsOutput, error)
	ListStepsRequest(*emr.ListStepsInput) (*request.Request, *emr.ListStepsOutput)

	ListStepsPages(*emr.ListStepsInput, func(*emr.ListStepsOutput, bool) bool) error
	ListStepsPagesWithContext(aws.Context, *emr.ListStepsInput, func(*emr.ListStepsOutput, bool) bool, ...request.Option) error

	ListStudioSessionMappings(*emr.ListStudioSessionMappingsInput) (*emr.ListStudioSessionMappingsOutput, error)
	ListStudioSessionMappingsWithContext(aws.Context, *emr.ListStudioSessionMappingsInput, ...request.Option) (*emr.ListStudioSessionMappingsOutput, error)
	ListStudioSessionMappingsRequest(*emr.ListStudioSessionMappingsInput) (*request.Request, *emr.ListStudioSessionMappingsOutput)

	ListStudioSessionMappingsPages(*emr.ListStudioSessionMappingsInput, func(*emr.ListStudioSessionMappingsOutput, bool) bool) error
	ListStudioSessionMappingsPagesWithContext(aws.Context, *emr.ListStudioSessionMappingsInput, func(*emr.ListStudioSessionMappingsOutput, bool) bool, ...request.Option) error

	ListStudios(*emr.ListStudiosInput) (*emr.ListStudiosOutput, error)
	ListStudiosWithContext(aws.Context, *emr.ListStudiosInput, ...request.Option) (*emr.ListStudiosOutput, error)
	ListStudiosRequest(*emr.ListStudiosInput) (*request.Request, *emr.ListStudiosOutput)

	ListStudiosPages(*emr.ListStudiosInput, func(*emr.ListStudiosOutput, bool) bool) error
	ListStudiosPagesWithContext(aws.Context, *emr.ListStudiosInput, func(*emr.ListStudiosOutput, bool) bool, ...request.Option) error

	ModifyCluster(*emr.ModifyClusterInput) (*emr.ModifyClusterOutput, error)
	ModifyClusterWithContext(aws.Context, *emr.ModifyClusterInput, ...request.Option) (*emr.ModifyClusterOutput, error)
	ModifyClusterRequest(*emr.ModifyClusterInput) (*request.Request, *emr.ModifyClusterOutput)

	ModifyInstanceFleet(*emr.ModifyInstanceFleetInput) (*emr.ModifyInstanceFleetOutput, error)
	ModifyInstanceFleetWithContext(aws.Context, *emr.ModifyInstanceFleetInput, ...request.Option) (*emr.ModifyInstanceFleetOutput, error)
	ModifyInstanceFleetRequest(*emr.ModifyInstanceFleetInput) (*request.Request, *emr.ModifyInstanceFleetOutput)

	ModifyInstanceGroups(*emr.ModifyInstanceGroupsInput) (*emr.ModifyInstanceGroupsOutput, error)
	ModifyInstanceGroupsWithContext(aws.Context, *emr.ModifyInstanceGroupsInput, ...request.Option) (*emr.ModifyInstanceGroupsOutput, error)
	ModifyInstanceGroupsRequest(*emr.ModifyInstanceGroupsInput) (*request.Request, *emr.ModifyInstanceGroupsOutput)

	PutAutoScalingPolicy(*emr.PutAutoScalingPolicyInput) (*emr.PutAutoScalingPolicyOutput, error)
	PutAutoScalingPolicyWithContext(aws.Context, *emr.PutAutoScalingPolicyInput, ...request.Option) (*emr.PutAutoScalingPolicyOutput, error)
	PutAutoScalingPolicyRequest(*emr.PutAutoScalingPolicyInput) (*request.Request, *emr.PutAutoScalingPolicyOutput)

	PutBlockPublicAccessConfiguration(*emr.PutBlockPublicAccessConfigurationInput) (*emr.PutBlockPublicAccessConfigurationOutput, error)
	PutBlockPublicAccessConfigurationWithContext(aws.Context, *emr.PutBlockPublicAccessConfigurationInput, ...request.Option) (*emr.PutBlockPublicAccessConfigurationOutput, error)
	PutBlockPublicAccessConfigurationRequest(*emr.PutBlockPublicAccessConfigurationInput) (*request.Request, *emr.PutBlockPublicAccessConfigurationOutput)

	PutManagedScalingPolicy(*emr.PutManagedScalingPolicyInput) (*emr.PutManagedScalingPolicyOutput, error)
	PutManagedScalingPolicyWithContext(aws.Context, *emr.PutManagedScalingPolicyInput, ...request.Option) (*emr.PutManagedScalingPolicyOutput, error)
	PutManagedScalingPolicyRequest(*emr.PutManagedScalingPolicyInput) (*request.Request, *emr.PutManagedScalingPolicyOutput)

	RemoveAutoScalingPolicy(*emr.RemoveAutoScalingPolicyInput) (*emr.RemoveAutoScalingPolicyOutput, error)
	RemoveAutoScalingPolicyWithContext(aws.Context, *emr.RemoveAutoScalingPolicyInput, ...request.Option) (*emr.RemoveAutoScalingPolicyOutput, error)
	RemoveAutoScalingPolicyRequest(*emr.RemoveAutoScalingPolicyInput) (*request.Request, *emr.RemoveAutoScalingPolicyOutput)

	RemoveManagedScalingPolicy(*emr.RemoveManagedScalingPolicyInput) (*emr.RemoveManagedScalingPolicyOutput, error)
	RemoveManagedScalingPolicyWithContext(aws.Context, *emr.RemoveManagedScalingPolicyInput, ...request.Option) (*emr.RemoveManagedScalingPolicyOutput, error)
	RemoveManagedScalingPolicyRequest(*emr.RemoveManagedScalingPolicyInput) (*request.Request, *emr.RemoveManagedScalingPolicyOutput)

	RemoveTags(*emr.RemoveTagsInput) (*emr.RemoveTagsOutput, error)
	RemoveTagsWithContext(aws.Context, *emr.RemoveTagsInput, ...request.Option) (*emr.RemoveTagsOutput, error)
	RemoveTagsRequest(*emr.RemoveTagsInput) (*request.Request, *emr.RemoveTagsOutput)

	RunJobFlow(*emr.RunJobFlowInput) (*emr.RunJobFlowOutput, error)
	RunJobFlowWithContext(aws.Context, *emr.RunJobFlowInput, ...request.Option) (*emr.RunJobFlowOutput, error)
	RunJobFlowRequest(*emr.RunJobFlowInput) (*request.Request, *emr.RunJobFlowOutput)

	SetTerminationProtection(*emr.SetTerminationProtectionInput) (*emr.SetTerminationProtectionOutput, error)
	SetTerminationProtectionWithContext(aws.Context, *emr.SetTerminationProtectionInput, ...request.Option) (*emr.SetTerminationProtectionOutput, error)
	SetTerminationProtectionRequest(*emr.SetTerminationProtectionInput) (*request.Request, *emr.SetTerminationProtectionOutput)

	SetVisibleToAllUsers(*emr.SetVisibleToAllUsersInput) (*emr.SetVisibleToAllUsersOutput, error)
	SetVisibleToAllUsersWithContext(aws.Context, *emr.SetVisibleToAllUsersInput, ...request.Option) (*emr.SetVisibleToAllUsersOutput, error)
	SetVisibleToAllUsersRequest(*emr.SetVisibleToAllUsersInput) (*request.Request, *emr.SetVisibleToAllUsersOutput)

	StartNotebookExecution(*emr.StartNotebookExecutionInput) (*emr.StartNotebookExecutionOutput, error)
	StartNotebookExecutionWithContext(aws.Context, *emr.StartNotebookExecutionInput, ...request.Option) (*emr.StartNotebookExecutionOutput, error)
	StartNotebookExecutionRequest(*emr.StartNotebookExecutionInput) (*request.Request, *emr.StartNotebookExecutionOutput)

	StopNotebookExecution(*emr.StopNotebookExecutionInput) (*emr.StopNotebookExecutionOutput, error)
	StopNotebookExecutionWithContext(aws.Context, *emr.StopNotebookExecutionInput, ...request.Option) (*emr.StopNotebookExecutionOutput, error)
	StopNotebookExecutionRequest(*emr.StopNotebookExecutionInput) (*request.Request, *emr.StopNotebookExecutionOutput)

	TerminateJobFlows(*emr.TerminateJobFlowsInput) (*emr.TerminateJobFlowsOutput, error)
	TerminateJobFlowsWithContext(aws.Context, *emr.TerminateJobFlowsInput, ...request.Option) (*emr.TerminateJobFlowsOutput, error)
	TerminateJobFlowsRequest(*emr.TerminateJobFlowsInput) (*request.Request, *emr.TerminateJobFlowsOutput)

	UpdateStudio(*emr.UpdateStudioInput) (*emr.UpdateStudioOutput, error)
	UpdateStudioWithContext(aws.Context, *emr.UpdateStudioInput, ...request.Option) (*emr.UpdateStudioOutput, error)
	UpdateStudioRequest(*emr.UpdateStudioInput) (*request.Request, *emr.UpdateStudioOutput)

	UpdateStudioSessionMapping(*emr.UpdateStudioSessionMappingInput) (*emr.UpdateStudioSessionMappingOutput, error)
	UpdateStudioSessionMappingWithContext(aws.Context, *emr.UpdateStudioSessionMappingInput, ...request.Option) (*emr.UpdateStudioSessionMappingOutput, error)
	UpdateStudioSessionMappingRequest(*emr.UpdateStudioSessionMappingInput) (*request.Request, *emr.UpdateStudioSessionMappingOutput)

	WaitUntilClusterRunning(*emr.DescribeClusterInput) error
	WaitUntilClusterRunningWithContext(aws.Context, *emr.DescribeClusterInput, ...request.WaiterOption) error

	WaitUntilClusterTerminated(*emr.DescribeClusterInput) error
	WaitUntilClusterTerminatedWithContext(aws.Context, *emr.DescribeClusterInput, ...request.WaiterOption) error

	WaitUntilStepComplete(*emr.DescribeStepInput) error
	WaitUntilStepCompleteWithContext(aws.Context, *emr.DescribeStepInput, ...request.WaiterOption) error
}

var _ EMRAPI = (*emr.EMR)(nil)
