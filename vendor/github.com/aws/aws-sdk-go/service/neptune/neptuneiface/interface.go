// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

// Package neptuneiface provides an interface to enable mocking the Amazon Neptune service client
// for testing your code.
//
// It is important to note that this interface will have breaking changes
// when the service model is updated and adds new API operations, paginators,
// and waiters.
package neptuneiface

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/neptune"
)

// NeptuneAPI provides an interface to enable mocking the
// neptune.Neptune service client's API operation,
// paginators, and waiters. This make unit testing your code that calls out
// to the SDK's service client's calls easier.
//
// The best way to use this interface is so the SDK's service client's calls
// can be stubbed out for unit testing your code with the SDK without needing
// to inject custom request handlers into the SDK's request pipeline.
//
//    // myFunc uses an SDK service client to make a request to
//    // Amazon Neptune.
//    func myFunc(svc neptuneiface.NeptuneAPI) bool {
//        // Make svc.AddRoleToDBCluster request
//    }
//
//    func main() {
//        sess := session.New()
//        svc := neptune.New(sess)
//
//        myFunc(svc)
//    }
//
// In your _test.go file:
//
//    // Define a mock struct to be used in your unit tests of myFunc.
//    type mockNeptuneClient struct {
//        neptuneiface.NeptuneAPI
//    }
//    func (m *mockNeptuneClient) AddRoleToDBCluster(input *neptune.AddRoleToDBClusterInput) (*neptune.AddRoleToDBClusterOutput, error) {
//        // mock response/functionality
//    }
//
//    func TestMyFunc(t *testing.T) {
//        // Setup Test
//        mockSvc := &mockNeptuneClient{}
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
type NeptuneAPI interface {
	AddRoleToDBCluster(*neptune.AddRoleToDBClusterInput) (*neptune.AddRoleToDBClusterOutput, error)
	AddRoleToDBClusterWithContext(aws.Context, *neptune.AddRoleToDBClusterInput, ...request.Option) (*neptune.AddRoleToDBClusterOutput, error)
	AddRoleToDBClusterRequest(*neptune.AddRoleToDBClusterInput) (*request.Request, *neptune.AddRoleToDBClusterOutput)

	AddSourceIdentifierToSubscription(*neptune.AddSourceIdentifierToSubscriptionInput) (*neptune.AddSourceIdentifierToSubscriptionOutput, error)
	AddSourceIdentifierToSubscriptionWithContext(aws.Context, *neptune.AddSourceIdentifierToSubscriptionInput, ...request.Option) (*neptune.AddSourceIdentifierToSubscriptionOutput, error)
	AddSourceIdentifierToSubscriptionRequest(*neptune.AddSourceIdentifierToSubscriptionInput) (*request.Request, *neptune.AddSourceIdentifierToSubscriptionOutput)

	AddTagsToResource(*neptune.AddTagsToResourceInput) (*neptune.AddTagsToResourceOutput, error)
	AddTagsToResourceWithContext(aws.Context, *neptune.AddTagsToResourceInput, ...request.Option) (*neptune.AddTagsToResourceOutput, error)
	AddTagsToResourceRequest(*neptune.AddTagsToResourceInput) (*request.Request, *neptune.AddTagsToResourceOutput)

	ApplyPendingMaintenanceAction(*neptune.ApplyPendingMaintenanceActionInput) (*neptune.ApplyPendingMaintenanceActionOutput, error)
	ApplyPendingMaintenanceActionWithContext(aws.Context, *neptune.ApplyPendingMaintenanceActionInput, ...request.Option) (*neptune.ApplyPendingMaintenanceActionOutput, error)
	ApplyPendingMaintenanceActionRequest(*neptune.ApplyPendingMaintenanceActionInput) (*request.Request, *neptune.ApplyPendingMaintenanceActionOutput)

	CopyDBClusterParameterGroup(*neptune.CopyDBClusterParameterGroupInput) (*neptune.CopyDBClusterParameterGroupOutput, error)
	CopyDBClusterParameterGroupWithContext(aws.Context, *neptune.CopyDBClusterParameterGroupInput, ...request.Option) (*neptune.CopyDBClusterParameterGroupOutput, error)
	CopyDBClusterParameterGroupRequest(*neptune.CopyDBClusterParameterGroupInput) (*request.Request, *neptune.CopyDBClusterParameterGroupOutput)

	CopyDBClusterSnapshot(*neptune.CopyDBClusterSnapshotInput) (*neptune.CopyDBClusterSnapshotOutput, error)
	CopyDBClusterSnapshotWithContext(aws.Context, *neptune.CopyDBClusterSnapshotInput, ...request.Option) (*neptune.CopyDBClusterSnapshotOutput, error)
	CopyDBClusterSnapshotRequest(*neptune.CopyDBClusterSnapshotInput) (*request.Request, *neptune.CopyDBClusterSnapshotOutput)

	CopyDBParameterGroup(*neptune.CopyDBParameterGroupInput) (*neptune.CopyDBParameterGroupOutput, error)
	CopyDBParameterGroupWithContext(aws.Context, *neptune.CopyDBParameterGroupInput, ...request.Option) (*neptune.CopyDBParameterGroupOutput, error)
	CopyDBParameterGroupRequest(*neptune.CopyDBParameterGroupInput) (*request.Request, *neptune.CopyDBParameterGroupOutput)

	CreateDBCluster(*neptune.CreateDBClusterInput) (*neptune.CreateDBClusterOutput, error)
	CreateDBClusterWithContext(aws.Context, *neptune.CreateDBClusterInput, ...request.Option) (*neptune.CreateDBClusterOutput, error)
	CreateDBClusterRequest(*neptune.CreateDBClusterInput) (*request.Request, *neptune.CreateDBClusterOutput)

	CreateDBClusterParameterGroup(*neptune.CreateDBClusterParameterGroupInput) (*neptune.CreateDBClusterParameterGroupOutput, error)
	CreateDBClusterParameterGroupWithContext(aws.Context, *neptune.CreateDBClusterParameterGroupInput, ...request.Option) (*neptune.CreateDBClusterParameterGroupOutput, error)
	CreateDBClusterParameterGroupRequest(*neptune.CreateDBClusterParameterGroupInput) (*request.Request, *neptune.CreateDBClusterParameterGroupOutput)

	CreateDBClusterSnapshot(*neptune.CreateDBClusterSnapshotInput) (*neptune.CreateDBClusterSnapshotOutput, error)
	CreateDBClusterSnapshotWithContext(aws.Context, *neptune.CreateDBClusterSnapshotInput, ...request.Option) (*neptune.CreateDBClusterSnapshotOutput, error)
	CreateDBClusterSnapshotRequest(*neptune.CreateDBClusterSnapshotInput) (*request.Request, *neptune.CreateDBClusterSnapshotOutput)

	CreateDBInstance(*neptune.CreateDBInstanceInput) (*neptune.CreateDBInstanceOutput, error)
	CreateDBInstanceWithContext(aws.Context, *neptune.CreateDBInstanceInput, ...request.Option) (*neptune.CreateDBInstanceOutput, error)
	CreateDBInstanceRequest(*neptune.CreateDBInstanceInput) (*request.Request, *neptune.CreateDBInstanceOutput)

	CreateDBParameterGroup(*neptune.CreateDBParameterGroupInput) (*neptune.CreateDBParameterGroupOutput, error)
	CreateDBParameterGroupWithContext(aws.Context, *neptune.CreateDBParameterGroupInput, ...request.Option) (*neptune.CreateDBParameterGroupOutput, error)
	CreateDBParameterGroupRequest(*neptune.CreateDBParameterGroupInput) (*request.Request, *neptune.CreateDBParameterGroupOutput)

	CreateDBSubnetGroup(*neptune.CreateDBSubnetGroupInput) (*neptune.CreateDBSubnetGroupOutput, error)
	CreateDBSubnetGroupWithContext(aws.Context, *neptune.CreateDBSubnetGroupInput, ...request.Option) (*neptune.CreateDBSubnetGroupOutput, error)
	CreateDBSubnetGroupRequest(*neptune.CreateDBSubnetGroupInput) (*request.Request, *neptune.CreateDBSubnetGroupOutput)

	CreateEventSubscription(*neptune.CreateEventSubscriptionInput) (*neptune.CreateEventSubscriptionOutput, error)
	CreateEventSubscriptionWithContext(aws.Context, *neptune.CreateEventSubscriptionInput, ...request.Option) (*neptune.CreateEventSubscriptionOutput, error)
	CreateEventSubscriptionRequest(*neptune.CreateEventSubscriptionInput) (*request.Request, *neptune.CreateEventSubscriptionOutput)

	DeleteDBCluster(*neptune.DeleteDBClusterInput) (*neptune.DeleteDBClusterOutput, error)
	DeleteDBClusterWithContext(aws.Context, *neptune.DeleteDBClusterInput, ...request.Option) (*neptune.DeleteDBClusterOutput, error)
	DeleteDBClusterRequest(*neptune.DeleteDBClusterInput) (*request.Request, *neptune.DeleteDBClusterOutput)

	DeleteDBClusterParameterGroup(*neptune.DeleteDBClusterParameterGroupInput) (*neptune.DeleteDBClusterParameterGroupOutput, error)
	DeleteDBClusterParameterGroupWithContext(aws.Context, *neptune.DeleteDBClusterParameterGroupInput, ...request.Option) (*neptune.DeleteDBClusterParameterGroupOutput, error)
	DeleteDBClusterParameterGroupRequest(*neptune.DeleteDBClusterParameterGroupInput) (*request.Request, *neptune.DeleteDBClusterParameterGroupOutput)

	DeleteDBClusterSnapshot(*neptune.DeleteDBClusterSnapshotInput) (*neptune.DeleteDBClusterSnapshotOutput, error)
	DeleteDBClusterSnapshotWithContext(aws.Context, *neptune.DeleteDBClusterSnapshotInput, ...request.Option) (*neptune.DeleteDBClusterSnapshotOutput, error)
	DeleteDBClusterSnapshotRequest(*neptune.DeleteDBClusterSnapshotInput) (*request.Request, *neptune.DeleteDBClusterSnapshotOutput)

	DeleteDBInstance(*neptune.DeleteDBInstanceInput) (*neptune.DeleteDBInstanceOutput, error)
	DeleteDBInstanceWithContext(aws.Context, *neptune.DeleteDBInstanceInput, ...request.Option) (*neptune.DeleteDBInstanceOutput, error)
	DeleteDBInstanceRequest(*neptune.DeleteDBInstanceInput) (*request.Request, *neptune.DeleteDBInstanceOutput)

	DeleteDBParameterGroup(*neptune.DeleteDBParameterGroupInput) (*neptune.DeleteDBParameterGroupOutput, error)
	DeleteDBParameterGroupWithContext(aws.Context, *neptune.DeleteDBParameterGroupInput, ...request.Option) (*neptune.DeleteDBParameterGroupOutput, error)
	DeleteDBParameterGroupRequest(*neptune.DeleteDBParameterGroupInput) (*request.Request, *neptune.DeleteDBParameterGroupOutput)

	DeleteDBSubnetGroup(*neptune.DeleteDBSubnetGroupInput) (*neptune.DeleteDBSubnetGroupOutput, error)
	DeleteDBSubnetGroupWithContext(aws.Context, *neptune.DeleteDBSubnetGroupInput, ...request.Option) (*neptune.DeleteDBSubnetGroupOutput, error)
	DeleteDBSubnetGroupRequest(*neptune.DeleteDBSubnetGroupInput) (*request.Request, *neptune.DeleteDBSubnetGroupOutput)

	DeleteEventSubscription(*neptune.DeleteEventSubscriptionInput) (*neptune.DeleteEventSubscriptionOutput, error)
	DeleteEventSubscriptionWithContext(aws.Context, *neptune.DeleteEventSubscriptionInput, ...request.Option) (*neptune.DeleteEventSubscriptionOutput, error)
	DeleteEventSubscriptionRequest(*neptune.DeleteEventSubscriptionInput) (*request.Request, *neptune.DeleteEventSubscriptionOutput)

	DescribeDBClusterParameterGroups(*neptune.DescribeDBClusterParameterGroupsInput) (*neptune.DescribeDBClusterParameterGroupsOutput, error)
	DescribeDBClusterParameterGroupsWithContext(aws.Context, *neptune.DescribeDBClusterParameterGroupsInput, ...request.Option) (*neptune.DescribeDBClusterParameterGroupsOutput, error)
	DescribeDBClusterParameterGroupsRequest(*neptune.DescribeDBClusterParameterGroupsInput) (*request.Request, *neptune.DescribeDBClusterParameterGroupsOutput)

	DescribeDBClusterParameters(*neptune.DescribeDBClusterParametersInput) (*neptune.DescribeDBClusterParametersOutput, error)
	DescribeDBClusterParametersWithContext(aws.Context, *neptune.DescribeDBClusterParametersInput, ...request.Option) (*neptune.DescribeDBClusterParametersOutput, error)
	DescribeDBClusterParametersRequest(*neptune.DescribeDBClusterParametersInput) (*request.Request, *neptune.DescribeDBClusterParametersOutput)

	DescribeDBClusterSnapshotAttributes(*neptune.DescribeDBClusterSnapshotAttributesInput) (*neptune.DescribeDBClusterSnapshotAttributesOutput, error)
	DescribeDBClusterSnapshotAttributesWithContext(aws.Context, *neptune.DescribeDBClusterSnapshotAttributesInput, ...request.Option) (*neptune.DescribeDBClusterSnapshotAttributesOutput, error)
	DescribeDBClusterSnapshotAttributesRequest(*neptune.DescribeDBClusterSnapshotAttributesInput) (*request.Request, *neptune.DescribeDBClusterSnapshotAttributesOutput)

	DescribeDBClusterSnapshots(*neptune.DescribeDBClusterSnapshotsInput) (*neptune.DescribeDBClusterSnapshotsOutput, error)
	DescribeDBClusterSnapshotsWithContext(aws.Context, *neptune.DescribeDBClusterSnapshotsInput, ...request.Option) (*neptune.DescribeDBClusterSnapshotsOutput, error)
	DescribeDBClusterSnapshotsRequest(*neptune.DescribeDBClusterSnapshotsInput) (*request.Request, *neptune.DescribeDBClusterSnapshotsOutput)

	DescribeDBClusters(*neptune.DescribeDBClustersInput) (*neptune.DescribeDBClustersOutput, error)
	DescribeDBClustersWithContext(aws.Context, *neptune.DescribeDBClustersInput, ...request.Option) (*neptune.DescribeDBClustersOutput, error)
	DescribeDBClustersRequest(*neptune.DescribeDBClustersInput) (*request.Request, *neptune.DescribeDBClustersOutput)

	DescribeDBEngineVersions(*neptune.DescribeDBEngineVersionsInput) (*neptune.DescribeDBEngineVersionsOutput, error)
	DescribeDBEngineVersionsWithContext(aws.Context, *neptune.DescribeDBEngineVersionsInput, ...request.Option) (*neptune.DescribeDBEngineVersionsOutput, error)
	DescribeDBEngineVersionsRequest(*neptune.DescribeDBEngineVersionsInput) (*request.Request, *neptune.DescribeDBEngineVersionsOutput)

	DescribeDBEngineVersionsPages(*neptune.DescribeDBEngineVersionsInput, func(*neptune.DescribeDBEngineVersionsOutput, bool) bool) error
	DescribeDBEngineVersionsPagesWithContext(aws.Context, *neptune.DescribeDBEngineVersionsInput, func(*neptune.DescribeDBEngineVersionsOutput, bool) bool, ...request.Option) error

	DescribeDBInstances(*neptune.DescribeDBInstancesInput) (*neptune.DescribeDBInstancesOutput, error)
	DescribeDBInstancesWithContext(aws.Context, *neptune.DescribeDBInstancesInput, ...request.Option) (*neptune.DescribeDBInstancesOutput, error)
	DescribeDBInstancesRequest(*neptune.DescribeDBInstancesInput) (*request.Request, *neptune.DescribeDBInstancesOutput)

	DescribeDBInstancesPages(*neptune.DescribeDBInstancesInput, func(*neptune.DescribeDBInstancesOutput, bool) bool) error
	DescribeDBInstancesPagesWithContext(aws.Context, *neptune.DescribeDBInstancesInput, func(*neptune.DescribeDBInstancesOutput, bool) bool, ...request.Option) error

	DescribeDBParameterGroups(*neptune.DescribeDBParameterGroupsInput) (*neptune.DescribeDBParameterGroupsOutput, error)
	DescribeDBParameterGroupsWithContext(aws.Context, *neptune.DescribeDBParameterGroupsInput, ...request.Option) (*neptune.DescribeDBParameterGroupsOutput, error)
	DescribeDBParameterGroupsRequest(*neptune.DescribeDBParameterGroupsInput) (*request.Request, *neptune.DescribeDBParameterGroupsOutput)

	DescribeDBParameterGroupsPages(*neptune.DescribeDBParameterGroupsInput, func(*neptune.DescribeDBParameterGroupsOutput, bool) bool) error
	DescribeDBParameterGroupsPagesWithContext(aws.Context, *neptune.DescribeDBParameterGroupsInput, func(*neptune.DescribeDBParameterGroupsOutput, bool) bool, ...request.Option) error

	DescribeDBParameters(*neptune.DescribeDBParametersInput) (*neptune.DescribeDBParametersOutput, error)
	DescribeDBParametersWithContext(aws.Context, *neptune.DescribeDBParametersInput, ...request.Option) (*neptune.DescribeDBParametersOutput, error)
	DescribeDBParametersRequest(*neptune.DescribeDBParametersInput) (*request.Request, *neptune.DescribeDBParametersOutput)

	DescribeDBParametersPages(*neptune.DescribeDBParametersInput, func(*neptune.DescribeDBParametersOutput, bool) bool) error
	DescribeDBParametersPagesWithContext(aws.Context, *neptune.DescribeDBParametersInput, func(*neptune.DescribeDBParametersOutput, bool) bool, ...request.Option) error

	DescribeDBSubnetGroups(*neptune.DescribeDBSubnetGroupsInput) (*neptune.DescribeDBSubnetGroupsOutput, error)
	DescribeDBSubnetGroupsWithContext(aws.Context, *neptune.DescribeDBSubnetGroupsInput, ...request.Option) (*neptune.DescribeDBSubnetGroupsOutput, error)
	DescribeDBSubnetGroupsRequest(*neptune.DescribeDBSubnetGroupsInput) (*request.Request, *neptune.DescribeDBSubnetGroupsOutput)

	DescribeDBSubnetGroupsPages(*neptune.DescribeDBSubnetGroupsInput, func(*neptune.DescribeDBSubnetGroupsOutput, bool) bool) error
	DescribeDBSubnetGroupsPagesWithContext(aws.Context, *neptune.DescribeDBSubnetGroupsInput, func(*neptune.DescribeDBSubnetGroupsOutput, bool) bool, ...request.Option) error

	DescribeEngineDefaultClusterParameters(*neptune.DescribeEngineDefaultClusterParametersInput) (*neptune.DescribeEngineDefaultClusterParametersOutput, error)
	DescribeEngineDefaultClusterParametersWithContext(aws.Context, *neptune.DescribeEngineDefaultClusterParametersInput, ...request.Option) (*neptune.DescribeEngineDefaultClusterParametersOutput, error)
	DescribeEngineDefaultClusterParametersRequest(*neptune.DescribeEngineDefaultClusterParametersInput) (*request.Request, *neptune.DescribeEngineDefaultClusterParametersOutput)

	DescribeEngineDefaultParameters(*neptune.DescribeEngineDefaultParametersInput) (*neptune.DescribeEngineDefaultParametersOutput, error)
	DescribeEngineDefaultParametersWithContext(aws.Context, *neptune.DescribeEngineDefaultParametersInput, ...request.Option) (*neptune.DescribeEngineDefaultParametersOutput, error)
	DescribeEngineDefaultParametersRequest(*neptune.DescribeEngineDefaultParametersInput) (*request.Request, *neptune.DescribeEngineDefaultParametersOutput)

	DescribeEngineDefaultParametersPages(*neptune.DescribeEngineDefaultParametersInput, func(*neptune.DescribeEngineDefaultParametersOutput, bool) bool) error
	DescribeEngineDefaultParametersPagesWithContext(aws.Context, *neptune.DescribeEngineDefaultParametersInput, func(*neptune.DescribeEngineDefaultParametersOutput, bool) bool, ...request.Option) error

	DescribeEventCategories(*neptune.DescribeEventCategoriesInput) (*neptune.DescribeEventCategoriesOutput, error)
	DescribeEventCategoriesWithContext(aws.Context, *neptune.DescribeEventCategoriesInput, ...request.Option) (*neptune.DescribeEventCategoriesOutput, error)
	DescribeEventCategoriesRequest(*neptune.DescribeEventCategoriesInput) (*request.Request, *neptune.DescribeEventCategoriesOutput)

	DescribeEventSubscriptions(*neptune.DescribeEventSubscriptionsInput) (*neptune.DescribeEventSubscriptionsOutput, error)
	DescribeEventSubscriptionsWithContext(aws.Context, *neptune.DescribeEventSubscriptionsInput, ...request.Option) (*neptune.DescribeEventSubscriptionsOutput, error)
	DescribeEventSubscriptionsRequest(*neptune.DescribeEventSubscriptionsInput) (*request.Request, *neptune.DescribeEventSubscriptionsOutput)

	DescribeEventSubscriptionsPages(*neptune.DescribeEventSubscriptionsInput, func(*neptune.DescribeEventSubscriptionsOutput, bool) bool) error
	DescribeEventSubscriptionsPagesWithContext(aws.Context, *neptune.DescribeEventSubscriptionsInput, func(*neptune.DescribeEventSubscriptionsOutput, bool) bool, ...request.Option) error

	DescribeEvents(*neptune.DescribeEventsInput) (*neptune.DescribeEventsOutput, error)
	DescribeEventsWithContext(aws.Context, *neptune.DescribeEventsInput, ...request.Option) (*neptune.DescribeEventsOutput, error)
	DescribeEventsRequest(*neptune.DescribeEventsInput) (*request.Request, *neptune.DescribeEventsOutput)

	DescribeEventsPages(*neptune.DescribeEventsInput, func(*neptune.DescribeEventsOutput, bool) bool) error
	DescribeEventsPagesWithContext(aws.Context, *neptune.DescribeEventsInput, func(*neptune.DescribeEventsOutput, bool) bool, ...request.Option) error

	DescribeOrderableDBInstanceOptions(*neptune.DescribeOrderableDBInstanceOptionsInput) (*neptune.DescribeOrderableDBInstanceOptionsOutput, error)
	DescribeOrderableDBInstanceOptionsWithContext(aws.Context, *neptune.DescribeOrderableDBInstanceOptionsInput, ...request.Option) (*neptune.DescribeOrderableDBInstanceOptionsOutput, error)
	DescribeOrderableDBInstanceOptionsRequest(*neptune.DescribeOrderableDBInstanceOptionsInput) (*request.Request, *neptune.DescribeOrderableDBInstanceOptionsOutput)

	DescribeOrderableDBInstanceOptionsPages(*neptune.DescribeOrderableDBInstanceOptionsInput, func(*neptune.DescribeOrderableDBInstanceOptionsOutput, bool) bool) error
	DescribeOrderableDBInstanceOptionsPagesWithContext(aws.Context, *neptune.DescribeOrderableDBInstanceOptionsInput, func(*neptune.DescribeOrderableDBInstanceOptionsOutput, bool) bool, ...request.Option) error

	DescribePendingMaintenanceActions(*neptune.DescribePendingMaintenanceActionsInput) (*neptune.DescribePendingMaintenanceActionsOutput, error)
	DescribePendingMaintenanceActionsWithContext(aws.Context, *neptune.DescribePendingMaintenanceActionsInput, ...request.Option) (*neptune.DescribePendingMaintenanceActionsOutput, error)
	DescribePendingMaintenanceActionsRequest(*neptune.DescribePendingMaintenanceActionsInput) (*request.Request, *neptune.DescribePendingMaintenanceActionsOutput)

	DescribeValidDBInstanceModifications(*neptune.DescribeValidDBInstanceModificationsInput) (*neptune.DescribeValidDBInstanceModificationsOutput, error)
	DescribeValidDBInstanceModificationsWithContext(aws.Context, *neptune.DescribeValidDBInstanceModificationsInput, ...request.Option) (*neptune.DescribeValidDBInstanceModificationsOutput, error)
	DescribeValidDBInstanceModificationsRequest(*neptune.DescribeValidDBInstanceModificationsInput) (*request.Request, *neptune.DescribeValidDBInstanceModificationsOutput)

	FailoverDBCluster(*neptune.FailoverDBClusterInput) (*neptune.FailoverDBClusterOutput, error)
	FailoverDBClusterWithContext(aws.Context, *neptune.FailoverDBClusterInput, ...request.Option) (*neptune.FailoverDBClusterOutput, error)
	FailoverDBClusterRequest(*neptune.FailoverDBClusterInput) (*request.Request, *neptune.FailoverDBClusterOutput)

	ListTagsForResource(*neptune.ListTagsForResourceInput) (*neptune.ListTagsForResourceOutput, error)
	ListTagsForResourceWithContext(aws.Context, *neptune.ListTagsForResourceInput, ...request.Option) (*neptune.ListTagsForResourceOutput, error)
	ListTagsForResourceRequest(*neptune.ListTagsForResourceInput) (*request.Request, *neptune.ListTagsForResourceOutput)

	ModifyDBCluster(*neptune.ModifyDBClusterInput) (*neptune.ModifyDBClusterOutput, error)
	ModifyDBClusterWithContext(aws.Context, *neptune.ModifyDBClusterInput, ...request.Option) (*neptune.ModifyDBClusterOutput, error)
	ModifyDBClusterRequest(*neptune.ModifyDBClusterInput) (*request.Request, *neptune.ModifyDBClusterOutput)

	ModifyDBClusterParameterGroup(*neptune.ModifyDBClusterParameterGroupInput) (*neptune.ResetDBClusterParameterGroupOutput, error)
	ModifyDBClusterParameterGroupWithContext(aws.Context, *neptune.ModifyDBClusterParameterGroupInput, ...request.Option) (*neptune.ResetDBClusterParameterGroupOutput, error)
	ModifyDBClusterParameterGroupRequest(*neptune.ModifyDBClusterParameterGroupInput) (*request.Request, *neptune.ResetDBClusterParameterGroupOutput)

	ModifyDBClusterSnapshotAttribute(*neptune.ModifyDBClusterSnapshotAttributeInput) (*neptune.ModifyDBClusterSnapshotAttributeOutput, error)
	ModifyDBClusterSnapshotAttributeWithContext(aws.Context, *neptune.ModifyDBClusterSnapshotAttributeInput, ...request.Option) (*neptune.ModifyDBClusterSnapshotAttributeOutput, error)
	ModifyDBClusterSnapshotAttributeRequest(*neptune.ModifyDBClusterSnapshotAttributeInput) (*request.Request, *neptune.ModifyDBClusterSnapshotAttributeOutput)

	ModifyDBInstance(*neptune.ModifyDBInstanceInput) (*neptune.ModifyDBInstanceOutput, error)
	ModifyDBInstanceWithContext(aws.Context, *neptune.ModifyDBInstanceInput, ...request.Option) (*neptune.ModifyDBInstanceOutput, error)
	ModifyDBInstanceRequest(*neptune.ModifyDBInstanceInput) (*request.Request, *neptune.ModifyDBInstanceOutput)

	ModifyDBParameterGroup(*neptune.ModifyDBParameterGroupInput) (*neptune.ResetDBParameterGroupOutput, error)
	ModifyDBParameterGroupWithContext(aws.Context, *neptune.ModifyDBParameterGroupInput, ...request.Option) (*neptune.ResetDBParameterGroupOutput, error)
	ModifyDBParameterGroupRequest(*neptune.ModifyDBParameterGroupInput) (*request.Request, *neptune.ResetDBParameterGroupOutput)

	ModifyDBSubnetGroup(*neptune.ModifyDBSubnetGroupInput) (*neptune.ModifyDBSubnetGroupOutput, error)
	ModifyDBSubnetGroupWithContext(aws.Context, *neptune.ModifyDBSubnetGroupInput, ...request.Option) (*neptune.ModifyDBSubnetGroupOutput, error)
	ModifyDBSubnetGroupRequest(*neptune.ModifyDBSubnetGroupInput) (*request.Request, *neptune.ModifyDBSubnetGroupOutput)

	ModifyEventSubscription(*neptune.ModifyEventSubscriptionInput) (*neptune.ModifyEventSubscriptionOutput, error)
	ModifyEventSubscriptionWithContext(aws.Context, *neptune.ModifyEventSubscriptionInput, ...request.Option) (*neptune.ModifyEventSubscriptionOutput, error)
	ModifyEventSubscriptionRequest(*neptune.ModifyEventSubscriptionInput) (*request.Request, *neptune.ModifyEventSubscriptionOutput)

	PromoteReadReplicaDBCluster(*neptune.PromoteReadReplicaDBClusterInput) (*neptune.PromoteReadReplicaDBClusterOutput, error)
	PromoteReadReplicaDBClusterWithContext(aws.Context, *neptune.PromoteReadReplicaDBClusterInput, ...request.Option) (*neptune.PromoteReadReplicaDBClusterOutput, error)
	PromoteReadReplicaDBClusterRequest(*neptune.PromoteReadReplicaDBClusterInput) (*request.Request, *neptune.PromoteReadReplicaDBClusterOutput)

	RebootDBInstance(*neptune.RebootDBInstanceInput) (*neptune.RebootDBInstanceOutput, error)
	RebootDBInstanceWithContext(aws.Context, *neptune.RebootDBInstanceInput, ...request.Option) (*neptune.RebootDBInstanceOutput, error)
	RebootDBInstanceRequest(*neptune.RebootDBInstanceInput) (*request.Request, *neptune.RebootDBInstanceOutput)

	RemoveRoleFromDBCluster(*neptune.RemoveRoleFromDBClusterInput) (*neptune.RemoveRoleFromDBClusterOutput, error)
	RemoveRoleFromDBClusterWithContext(aws.Context, *neptune.RemoveRoleFromDBClusterInput, ...request.Option) (*neptune.RemoveRoleFromDBClusterOutput, error)
	RemoveRoleFromDBClusterRequest(*neptune.RemoveRoleFromDBClusterInput) (*request.Request, *neptune.RemoveRoleFromDBClusterOutput)

	RemoveSourceIdentifierFromSubscription(*neptune.RemoveSourceIdentifierFromSubscriptionInput) (*neptune.RemoveSourceIdentifierFromSubscriptionOutput, error)
	RemoveSourceIdentifierFromSubscriptionWithContext(aws.Context, *neptune.RemoveSourceIdentifierFromSubscriptionInput, ...request.Option) (*neptune.RemoveSourceIdentifierFromSubscriptionOutput, error)
	RemoveSourceIdentifierFromSubscriptionRequest(*neptune.RemoveSourceIdentifierFromSubscriptionInput) (*request.Request, *neptune.RemoveSourceIdentifierFromSubscriptionOutput)

	RemoveTagsFromResource(*neptune.RemoveTagsFromResourceInput) (*neptune.RemoveTagsFromResourceOutput, error)
	RemoveTagsFromResourceWithContext(aws.Context, *neptune.RemoveTagsFromResourceInput, ...request.Option) (*neptune.RemoveTagsFromResourceOutput, error)
	RemoveTagsFromResourceRequest(*neptune.RemoveTagsFromResourceInput) (*request.Request, *neptune.RemoveTagsFromResourceOutput)

	ResetDBClusterParameterGroup(*neptune.ResetDBClusterParameterGroupInput) (*neptune.ResetDBClusterParameterGroupOutput, error)
	ResetDBClusterParameterGroupWithContext(aws.Context, *neptune.ResetDBClusterParameterGroupInput, ...request.Option) (*neptune.ResetDBClusterParameterGroupOutput, error)
	ResetDBClusterParameterGroupRequest(*neptune.ResetDBClusterParameterGroupInput) (*request.Request, *neptune.ResetDBClusterParameterGroupOutput)

	ResetDBParameterGroup(*neptune.ResetDBParameterGroupInput) (*neptune.ResetDBParameterGroupOutput, error)
	ResetDBParameterGroupWithContext(aws.Context, *neptune.ResetDBParameterGroupInput, ...request.Option) (*neptune.ResetDBParameterGroupOutput, error)
	ResetDBParameterGroupRequest(*neptune.ResetDBParameterGroupInput) (*request.Request, *neptune.ResetDBParameterGroupOutput)

	RestoreDBClusterFromSnapshot(*neptune.RestoreDBClusterFromSnapshotInput) (*neptune.RestoreDBClusterFromSnapshotOutput, error)
	RestoreDBClusterFromSnapshotWithContext(aws.Context, *neptune.RestoreDBClusterFromSnapshotInput, ...request.Option) (*neptune.RestoreDBClusterFromSnapshotOutput, error)
	RestoreDBClusterFromSnapshotRequest(*neptune.RestoreDBClusterFromSnapshotInput) (*request.Request, *neptune.RestoreDBClusterFromSnapshotOutput)

	RestoreDBClusterToPointInTime(*neptune.RestoreDBClusterToPointInTimeInput) (*neptune.RestoreDBClusterToPointInTimeOutput, error)
	RestoreDBClusterToPointInTimeWithContext(aws.Context, *neptune.RestoreDBClusterToPointInTimeInput, ...request.Option) (*neptune.RestoreDBClusterToPointInTimeOutput, error)
	RestoreDBClusterToPointInTimeRequest(*neptune.RestoreDBClusterToPointInTimeInput) (*request.Request, *neptune.RestoreDBClusterToPointInTimeOutput)

	StartDBCluster(*neptune.StartDBClusterInput) (*neptune.StartDBClusterOutput, error)
	StartDBClusterWithContext(aws.Context, *neptune.StartDBClusterInput, ...request.Option) (*neptune.StartDBClusterOutput, error)
	StartDBClusterRequest(*neptune.StartDBClusterInput) (*request.Request, *neptune.StartDBClusterOutput)

	StopDBCluster(*neptune.StopDBClusterInput) (*neptune.StopDBClusterOutput, error)
	StopDBClusterWithContext(aws.Context, *neptune.StopDBClusterInput, ...request.Option) (*neptune.StopDBClusterOutput, error)
	StopDBClusterRequest(*neptune.StopDBClusterInput) (*request.Request, *neptune.StopDBClusterOutput)

	WaitUntilDBInstanceAvailable(*neptune.DescribeDBInstancesInput) error
	WaitUntilDBInstanceAvailableWithContext(aws.Context, *neptune.DescribeDBInstancesInput, ...request.WaiterOption) error

	WaitUntilDBInstanceDeleted(*neptune.DescribeDBInstancesInput) error
	WaitUntilDBInstanceDeletedWithContext(aws.Context, *neptune.DescribeDBInstancesInput, ...request.WaiterOption) error
}

var _ NeptuneAPI = (*neptune.Neptune)(nil)
