// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

// Package docdbiface provides an interface to enable mocking the Amazon DocumentDB with MongoDB compatibility service client
// for testing your code.
//
// It is important to note that this interface will have breaking changes
// when the service model is updated and adds new API operations, paginators,
// and waiters.
package docdbiface

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/docdb"
)

// DocDBAPI provides an interface to enable mocking the
// docdb.DocDB service client's API operation,
// paginators, and waiters. This make unit testing your code that calls out
// to the SDK's service client's calls easier.
//
// The best way to use this interface is so the SDK's service client's calls
// can be stubbed out for unit testing your code with the SDK without needing
// to inject custom request handlers into the SDK's request pipeline.
//
//    // myFunc uses an SDK service client to make a request to
//    // Amazon DocumentDB with MongoDB compatibility.
//    func myFunc(svc docdbiface.DocDBAPI) bool {
//        // Make svc.AddTagsToResource request
//    }
//
//    func main() {
//        sess := session.New()
//        svc := docdb.New(sess)
//
//        myFunc(svc)
//    }
//
// In your _test.go file:
//
//    // Define a mock struct to be used in your unit tests of myFunc.
//    type mockDocDBClient struct {
//        docdbiface.DocDBAPI
//    }
//    func (m *mockDocDBClient) AddTagsToResource(input *docdb.AddTagsToResourceInput) (*docdb.AddTagsToResourceOutput, error) {
//        // mock response/functionality
//    }
//
//    func TestMyFunc(t *testing.T) {
//        // Setup Test
//        mockSvc := &mockDocDBClient{}
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
type DocDBAPI interface {
	AddTagsToResource(*docdb.AddTagsToResourceInput) (*docdb.AddTagsToResourceOutput, error)
	AddTagsToResourceWithContext(aws.Context, *docdb.AddTagsToResourceInput, ...request.Option) (*docdb.AddTagsToResourceOutput, error)
	AddTagsToResourceRequest(*docdb.AddTagsToResourceInput) (*request.Request, *docdb.AddTagsToResourceOutput)

	ApplyPendingMaintenanceAction(*docdb.ApplyPendingMaintenanceActionInput) (*docdb.ApplyPendingMaintenanceActionOutput, error)
	ApplyPendingMaintenanceActionWithContext(aws.Context, *docdb.ApplyPendingMaintenanceActionInput, ...request.Option) (*docdb.ApplyPendingMaintenanceActionOutput, error)
	ApplyPendingMaintenanceActionRequest(*docdb.ApplyPendingMaintenanceActionInput) (*request.Request, *docdb.ApplyPendingMaintenanceActionOutput)

	CopyDBClusterParameterGroup(*docdb.CopyDBClusterParameterGroupInput) (*docdb.CopyDBClusterParameterGroupOutput, error)
	CopyDBClusterParameterGroupWithContext(aws.Context, *docdb.CopyDBClusterParameterGroupInput, ...request.Option) (*docdb.CopyDBClusterParameterGroupOutput, error)
	CopyDBClusterParameterGroupRequest(*docdb.CopyDBClusterParameterGroupInput) (*request.Request, *docdb.CopyDBClusterParameterGroupOutput)

	CopyDBClusterSnapshot(*docdb.CopyDBClusterSnapshotInput) (*docdb.CopyDBClusterSnapshotOutput, error)
	CopyDBClusterSnapshotWithContext(aws.Context, *docdb.CopyDBClusterSnapshotInput, ...request.Option) (*docdb.CopyDBClusterSnapshotOutput, error)
	CopyDBClusterSnapshotRequest(*docdb.CopyDBClusterSnapshotInput) (*request.Request, *docdb.CopyDBClusterSnapshotOutput)

	CreateDBCluster(*docdb.CreateDBClusterInput) (*docdb.CreateDBClusterOutput, error)
	CreateDBClusterWithContext(aws.Context, *docdb.CreateDBClusterInput, ...request.Option) (*docdb.CreateDBClusterOutput, error)
	CreateDBClusterRequest(*docdb.CreateDBClusterInput) (*request.Request, *docdb.CreateDBClusterOutput)

	CreateDBClusterParameterGroup(*docdb.CreateDBClusterParameterGroupInput) (*docdb.CreateDBClusterParameterGroupOutput, error)
	CreateDBClusterParameterGroupWithContext(aws.Context, *docdb.CreateDBClusterParameterGroupInput, ...request.Option) (*docdb.CreateDBClusterParameterGroupOutput, error)
	CreateDBClusterParameterGroupRequest(*docdb.CreateDBClusterParameterGroupInput) (*request.Request, *docdb.CreateDBClusterParameterGroupOutput)

	CreateDBClusterSnapshot(*docdb.CreateDBClusterSnapshotInput) (*docdb.CreateDBClusterSnapshotOutput, error)
	CreateDBClusterSnapshotWithContext(aws.Context, *docdb.CreateDBClusterSnapshotInput, ...request.Option) (*docdb.CreateDBClusterSnapshotOutput, error)
	CreateDBClusterSnapshotRequest(*docdb.CreateDBClusterSnapshotInput) (*request.Request, *docdb.CreateDBClusterSnapshotOutput)

	CreateDBInstance(*docdb.CreateDBInstanceInput) (*docdb.CreateDBInstanceOutput, error)
	CreateDBInstanceWithContext(aws.Context, *docdb.CreateDBInstanceInput, ...request.Option) (*docdb.CreateDBInstanceOutput, error)
	CreateDBInstanceRequest(*docdb.CreateDBInstanceInput) (*request.Request, *docdb.CreateDBInstanceOutput)

	CreateDBSubnetGroup(*docdb.CreateDBSubnetGroupInput) (*docdb.CreateDBSubnetGroupOutput, error)
	CreateDBSubnetGroupWithContext(aws.Context, *docdb.CreateDBSubnetGroupInput, ...request.Option) (*docdb.CreateDBSubnetGroupOutput, error)
	CreateDBSubnetGroupRequest(*docdb.CreateDBSubnetGroupInput) (*request.Request, *docdb.CreateDBSubnetGroupOutput)

	DeleteDBCluster(*docdb.DeleteDBClusterInput) (*docdb.DeleteDBClusterOutput, error)
	DeleteDBClusterWithContext(aws.Context, *docdb.DeleteDBClusterInput, ...request.Option) (*docdb.DeleteDBClusterOutput, error)
	DeleteDBClusterRequest(*docdb.DeleteDBClusterInput) (*request.Request, *docdb.DeleteDBClusterOutput)

	DeleteDBClusterParameterGroup(*docdb.DeleteDBClusterParameterGroupInput) (*docdb.DeleteDBClusterParameterGroupOutput, error)
	DeleteDBClusterParameterGroupWithContext(aws.Context, *docdb.DeleteDBClusterParameterGroupInput, ...request.Option) (*docdb.DeleteDBClusterParameterGroupOutput, error)
	DeleteDBClusterParameterGroupRequest(*docdb.DeleteDBClusterParameterGroupInput) (*request.Request, *docdb.DeleteDBClusterParameterGroupOutput)

	DeleteDBClusterSnapshot(*docdb.DeleteDBClusterSnapshotInput) (*docdb.DeleteDBClusterSnapshotOutput, error)
	DeleteDBClusterSnapshotWithContext(aws.Context, *docdb.DeleteDBClusterSnapshotInput, ...request.Option) (*docdb.DeleteDBClusterSnapshotOutput, error)
	DeleteDBClusterSnapshotRequest(*docdb.DeleteDBClusterSnapshotInput) (*request.Request, *docdb.DeleteDBClusterSnapshotOutput)

	DeleteDBInstance(*docdb.DeleteDBInstanceInput) (*docdb.DeleteDBInstanceOutput, error)
	DeleteDBInstanceWithContext(aws.Context, *docdb.DeleteDBInstanceInput, ...request.Option) (*docdb.DeleteDBInstanceOutput, error)
	DeleteDBInstanceRequest(*docdb.DeleteDBInstanceInput) (*request.Request, *docdb.DeleteDBInstanceOutput)

	DeleteDBSubnetGroup(*docdb.DeleteDBSubnetGroupInput) (*docdb.DeleteDBSubnetGroupOutput, error)
	DeleteDBSubnetGroupWithContext(aws.Context, *docdb.DeleteDBSubnetGroupInput, ...request.Option) (*docdb.DeleteDBSubnetGroupOutput, error)
	DeleteDBSubnetGroupRequest(*docdb.DeleteDBSubnetGroupInput) (*request.Request, *docdb.DeleteDBSubnetGroupOutput)

	DescribeDBClusterParameterGroups(*docdb.DescribeDBClusterParameterGroupsInput) (*docdb.DescribeDBClusterParameterGroupsOutput, error)
	DescribeDBClusterParameterGroupsWithContext(aws.Context, *docdb.DescribeDBClusterParameterGroupsInput, ...request.Option) (*docdb.DescribeDBClusterParameterGroupsOutput, error)
	DescribeDBClusterParameterGroupsRequest(*docdb.DescribeDBClusterParameterGroupsInput) (*request.Request, *docdb.DescribeDBClusterParameterGroupsOutput)

	DescribeDBClusterParameters(*docdb.DescribeDBClusterParametersInput) (*docdb.DescribeDBClusterParametersOutput, error)
	DescribeDBClusterParametersWithContext(aws.Context, *docdb.DescribeDBClusterParametersInput, ...request.Option) (*docdb.DescribeDBClusterParametersOutput, error)
	DescribeDBClusterParametersRequest(*docdb.DescribeDBClusterParametersInput) (*request.Request, *docdb.DescribeDBClusterParametersOutput)

	DescribeDBClusterSnapshotAttributes(*docdb.DescribeDBClusterSnapshotAttributesInput) (*docdb.DescribeDBClusterSnapshotAttributesOutput, error)
	DescribeDBClusterSnapshotAttributesWithContext(aws.Context, *docdb.DescribeDBClusterSnapshotAttributesInput, ...request.Option) (*docdb.DescribeDBClusterSnapshotAttributesOutput, error)
	DescribeDBClusterSnapshotAttributesRequest(*docdb.DescribeDBClusterSnapshotAttributesInput) (*request.Request, *docdb.DescribeDBClusterSnapshotAttributesOutput)

	DescribeDBClusterSnapshots(*docdb.DescribeDBClusterSnapshotsInput) (*docdb.DescribeDBClusterSnapshotsOutput, error)
	DescribeDBClusterSnapshotsWithContext(aws.Context, *docdb.DescribeDBClusterSnapshotsInput, ...request.Option) (*docdb.DescribeDBClusterSnapshotsOutput, error)
	DescribeDBClusterSnapshotsRequest(*docdb.DescribeDBClusterSnapshotsInput) (*request.Request, *docdb.DescribeDBClusterSnapshotsOutput)

	DescribeDBClusters(*docdb.DescribeDBClustersInput) (*docdb.DescribeDBClustersOutput, error)
	DescribeDBClustersWithContext(aws.Context, *docdb.DescribeDBClustersInput, ...request.Option) (*docdb.DescribeDBClustersOutput, error)
	DescribeDBClustersRequest(*docdb.DescribeDBClustersInput) (*request.Request, *docdb.DescribeDBClustersOutput)

	DescribeDBClustersPages(*docdb.DescribeDBClustersInput, func(*docdb.DescribeDBClustersOutput, bool) bool) error
	DescribeDBClustersPagesWithContext(aws.Context, *docdb.DescribeDBClustersInput, func(*docdb.DescribeDBClustersOutput, bool) bool, ...request.Option) error

	DescribeDBEngineVersions(*docdb.DescribeDBEngineVersionsInput) (*docdb.DescribeDBEngineVersionsOutput, error)
	DescribeDBEngineVersionsWithContext(aws.Context, *docdb.DescribeDBEngineVersionsInput, ...request.Option) (*docdb.DescribeDBEngineVersionsOutput, error)
	DescribeDBEngineVersionsRequest(*docdb.DescribeDBEngineVersionsInput) (*request.Request, *docdb.DescribeDBEngineVersionsOutput)

	DescribeDBEngineVersionsPages(*docdb.DescribeDBEngineVersionsInput, func(*docdb.DescribeDBEngineVersionsOutput, bool) bool) error
	DescribeDBEngineVersionsPagesWithContext(aws.Context, *docdb.DescribeDBEngineVersionsInput, func(*docdb.DescribeDBEngineVersionsOutput, bool) bool, ...request.Option) error

	DescribeDBInstances(*docdb.DescribeDBInstancesInput) (*docdb.DescribeDBInstancesOutput, error)
	DescribeDBInstancesWithContext(aws.Context, *docdb.DescribeDBInstancesInput, ...request.Option) (*docdb.DescribeDBInstancesOutput, error)
	DescribeDBInstancesRequest(*docdb.DescribeDBInstancesInput) (*request.Request, *docdb.DescribeDBInstancesOutput)

	DescribeDBInstancesPages(*docdb.DescribeDBInstancesInput, func(*docdb.DescribeDBInstancesOutput, bool) bool) error
	DescribeDBInstancesPagesWithContext(aws.Context, *docdb.DescribeDBInstancesInput, func(*docdb.DescribeDBInstancesOutput, bool) bool, ...request.Option) error

	DescribeDBSubnetGroups(*docdb.DescribeDBSubnetGroupsInput) (*docdb.DescribeDBSubnetGroupsOutput, error)
	DescribeDBSubnetGroupsWithContext(aws.Context, *docdb.DescribeDBSubnetGroupsInput, ...request.Option) (*docdb.DescribeDBSubnetGroupsOutput, error)
	DescribeDBSubnetGroupsRequest(*docdb.DescribeDBSubnetGroupsInput) (*request.Request, *docdb.DescribeDBSubnetGroupsOutput)

	DescribeDBSubnetGroupsPages(*docdb.DescribeDBSubnetGroupsInput, func(*docdb.DescribeDBSubnetGroupsOutput, bool) bool) error
	DescribeDBSubnetGroupsPagesWithContext(aws.Context, *docdb.DescribeDBSubnetGroupsInput, func(*docdb.DescribeDBSubnetGroupsOutput, bool) bool, ...request.Option) error

	DescribeEngineDefaultClusterParameters(*docdb.DescribeEngineDefaultClusterParametersInput) (*docdb.DescribeEngineDefaultClusterParametersOutput, error)
	DescribeEngineDefaultClusterParametersWithContext(aws.Context, *docdb.DescribeEngineDefaultClusterParametersInput, ...request.Option) (*docdb.DescribeEngineDefaultClusterParametersOutput, error)
	DescribeEngineDefaultClusterParametersRequest(*docdb.DescribeEngineDefaultClusterParametersInput) (*request.Request, *docdb.DescribeEngineDefaultClusterParametersOutput)

	DescribeEventCategories(*docdb.DescribeEventCategoriesInput) (*docdb.DescribeEventCategoriesOutput, error)
	DescribeEventCategoriesWithContext(aws.Context, *docdb.DescribeEventCategoriesInput, ...request.Option) (*docdb.DescribeEventCategoriesOutput, error)
	DescribeEventCategoriesRequest(*docdb.DescribeEventCategoriesInput) (*request.Request, *docdb.DescribeEventCategoriesOutput)

	DescribeEvents(*docdb.DescribeEventsInput) (*docdb.DescribeEventsOutput, error)
	DescribeEventsWithContext(aws.Context, *docdb.DescribeEventsInput, ...request.Option) (*docdb.DescribeEventsOutput, error)
	DescribeEventsRequest(*docdb.DescribeEventsInput) (*request.Request, *docdb.DescribeEventsOutput)

	DescribeEventsPages(*docdb.DescribeEventsInput, func(*docdb.DescribeEventsOutput, bool) bool) error
	DescribeEventsPagesWithContext(aws.Context, *docdb.DescribeEventsInput, func(*docdb.DescribeEventsOutput, bool) bool, ...request.Option) error

	DescribeOrderableDBInstanceOptions(*docdb.DescribeOrderableDBInstanceOptionsInput) (*docdb.DescribeOrderableDBInstanceOptionsOutput, error)
	DescribeOrderableDBInstanceOptionsWithContext(aws.Context, *docdb.DescribeOrderableDBInstanceOptionsInput, ...request.Option) (*docdb.DescribeOrderableDBInstanceOptionsOutput, error)
	DescribeOrderableDBInstanceOptionsRequest(*docdb.DescribeOrderableDBInstanceOptionsInput) (*request.Request, *docdb.DescribeOrderableDBInstanceOptionsOutput)

	DescribeOrderableDBInstanceOptionsPages(*docdb.DescribeOrderableDBInstanceOptionsInput, func(*docdb.DescribeOrderableDBInstanceOptionsOutput, bool) bool) error
	DescribeOrderableDBInstanceOptionsPagesWithContext(aws.Context, *docdb.DescribeOrderableDBInstanceOptionsInput, func(*docdb.DescribeOrderableDBInstanceOptionsOutput, bool) bool, ...request.Option) error

	DescribePendingMaintenanceActions(*docdb.DescribePendingMaintenanceActionsInput) (*docdb.DescribePendingMaintenanceActionsOutput, error)
	DescribePendingMaintenanceActionsWithContext(aws.Context, *docdb.DescribePendingMaintenanceActionsInput, ...request.Option) (*docdb.DescribePendingMaintenanceActionsOutput, error)
	DescribePendingMaintenanceActionsRequest(*docdb.DescribePendingMaintenanceActionsInput) (*request.Request, *docdb.DescribePendingMaintenanceActionsOutput)

	FailoverDBCluster(*docdb.FailoverDBClusterInput) (*docdb.FailoverDBClusterOutput, error)
	FailoverDBClusterWithContext(aws.Context, *docdb.FailoverDBClusterInput, ...request.Option) (*docdb.FailoverDBClusterOutput, error)
	FailoverDBClusterRequest(*docdb.FailoverDBClusterInput) (*request.Request, *docdb.FailoverDBClusterOutput)

	ListTagsForResource(*docdb.ListTagsForResourceInput) (*docdb.ListTagsForResourceOutput, error)
	ListTagsForResourceWithContext(aws.Context, *docdb.ListTagsForResourceInput, ...request.Option) (*docdb.ListTagsForResourceOutput, error)
	ListTagsForResourceRequest(*docdb.ListTagsForResourceInput) (*request.Request, *docdb.ListTagsForResourceOutput)

	ModifyDBCluster(*docdb.ModifyDBClusterInput) (*docdb.ModifyDBClusterOutput, error)
	ModifyDBClusterWithContext(aws.Context, *docdb.ModifyDBClusterInput, ...request.Option) (*docdb.ModifyDBClusterOutput, error)
	ModifyDBClusterRequest(*docdb.ModifyDBClusterInput) (*request.Request, *docdb.ModifyDBClusterOutput)

	ModifyDBClusterParameterGroup(*docdb.ModifyDBClusterParameterGroupInput) (*docdb.ModifyDBClusterParameterGroupOutput, error)
	ModifyDBClusterParameterGroupWithContext(aws.Context, *docdb.ModifyDBClusterParameterGroupInput, ...request.Option) (*docdb.ModifyDBClusterParameterGroupOutput, error)
	ModifyDBClusterParameterGroupRequest(*docdb.ModifyDBClusterParameterGroupInput) (*request.Request, *docdb.ModifyDBClusterParameterGroupOutput)

	ModifyDBClusterSnapshotAttribute(*docdb.ModifyDBClusterSnapshotAttributeInput) (*docdb.ModifyDBClusterSnapshotAttributeOutput, error)
	ModifyDBClusterSnapshotAttributeWithContext(aws.Context, *docdb.ModifyDBClusterSnapshotAttributeInput, ...request.Option) (*docdb.ModifyDBClusterSnapshotAttributeOutput, error)
	ModifyDBClusterSnapshotAttributeRequest(*docdb.ModifyDBClusterSnapshotAttributeInput) (*request.Request, *docdb.ModifyDBClusterSnapshotAttributeOutput)

	ModifyDBInstance(*docdb.ModifyDBInstanceInput) (*docdb.ModifyDBInstanceOutput, error)
	ModifyDBInstanceWithContext(aws.Context, *docdb.ModifyDBInstanceInput, ...request.Option) (*docdb.ModifyDBInstanceOutput, error)
	ModifyDBInstanceRequest(*docdb.ModifyDBInstanceInput) (*request.Request, *docdb.ModifyDBInstanceOutput)

	ModifyDBSubnetGroup(*docdb.ModifyDBSubnetGroupInput) (*docdb.ModifyDBSubnetGroupOutput, error)
	ModifyDBSubnetGroupWithContext(aws.Context, *docdb.ModifyDBSubnetGroupInput, ...request.Option) (*docdb.ModifyDBSubnetGroupOutput, error)
	ModifyDBSubnetGroupRequest(*docdb.ModifyDBSubnetGroupInput) (*request.Request, *docdb.ModifyDBSubnetGroupOutput)

	RebootDBInstance(*docdb.RebootDBInstanceInput) (*docdb.RebootDBInstanceOutput, error)
	RebootDBInstanceWithContext(aws.Context, *docdb.RebootDBInstanceInput, ...request.Option) (*docdb.RebootDBInstanceOutput, error)
	RebootDBInstanceRequest(*docdb.RebootDBInstanceInput) (*request.Request, *docdb.RebootDBInstanceOutput)

	RemoveTagsFromResource(*docdb.RemoveTagsFromResourceInput) (*docdb.RemoveTagsFromResourceOutput, error)
	RemoveTagsFromResourceWithContext(aws.Context, *docdb.RemoveTagsFromResourceInput, ...request.Option) (*docdb.RemoveTagsFromResourceOutput, error)
	RemoveTagsFromResourceRequest(*docdb.RemoveTagsFromResourceInput) (*request.Request, *docdb.RemoveTagsFromResourceOutput)

	ResetDBClusterParameterGroup(*docdb.ResetDBClusterParameterGroupInput) (*docdb.ResetDBClusterParameterGroupOutput, error)
	ResetDBClusterParameterGroupWithContext(aws.Context, *docdb.ResetDBClusterParameterGroupInput, ...request.Option) (*docdb.ResetDBClusterParameterGroupOutput, error)
	ResetDBClusterParameterGroupRequest(*docdb.ResetDBClusterParameterGroupInput) (*request.Request, *docdb.ResetDBClusterParameterGroupOutput)

	RestoreDBClusterFromSnapshot(*docdb.RestoreDBClusterFromSnapshotInput) (*docdb.RestoreDBClusterFromSnapshotOutput, error)
	RestoreDBClusterFromSnapshotWithContext(aws.Context, *docdb.RestoreDBClusterFromSnapshotInput, ...request.Option) (*docdb.RestoreDBClusterFromSnapshotOutput, error)
	RestoreDBClusterFromSnapshotRequest(*docdb.RestoreDBClusterFromSnapshotInput) (*request.Request, *docdb.RestoreDBClusterFromSnapshotOutput)

	RestoreDBClusterToPointInTime(*docdb.RestoreDBClusterToPointInTimeInput) (*docdb.RestoreDBClusterToPointInTimeOutput, error)
	RestoreDBClusterToPointInTimeWithContext(aws.Context, *docdb.RestoreDBClusterToPointInTimeInput, ...request.Option) (*docdb.RestoreDBClusterToPointInTimeOutput, error)
	RestoreDBClusterToPointInTimeRequest(*docdb.RestoreDBClusterToPointInTimeInput) (*request.Request, *docdb.RestoreDBClusterToPointInTimeOutput)

	WaitUntilDBInstanceAvailable(*docdb.DescribeDBInstancesInput) error
	WaitUntilDBInstanceAvailableWithContext(aws.Context, *docdb.DescribeDBInstancesInput, ...request.WaiterOption) error

	WaitUntilDBInstanceDeleted(*docdb.DescribeDBInstancesInput) error
	WaitUntilDBInstanceDeletedWithContext(aws.Context, *docdb.DescribeDBInstancesInput, ...request.WaiterOption) error
}

var _ DocDBAPI = (*docdb.DocDB)(nil)
