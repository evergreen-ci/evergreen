// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

// Package redshiftiface provides an interface to enable mocking the Amazon Redshift service client
// for testing your code.
//
// It is important to note that this interface will have breaking changes
// when the service model is updated and adds new API operations, paginators,
// and waiters.
package redshiftiface

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/redshift"
)

// RedshiftAPI provides an interface to enable mocking the
// redshift.Redshift service client's API operation,
// paginators, and waiters. This make unit testing your code that calls out
// to the SDK's service client's calls easier.
//
// The best way to use this interface is so the SDK's service client's calls
// can be stubbed out for unit testing your code with the SDK without needing
// to inject custom request handlers into the SDK's request pipeline.
//
//    // myFunc uses an SDK service client to make a request to
//    // Amazon Redshift.
//    func myFunc(svc redshiftiface.RedshiftAPI) bool {
//        // Make svc.AcceptReservedNodeExchange request
//    }
//
//    func main() {
//        sess := session.New()
//        svc := redshift.New(sess)
//
//        myFunc(svc)
//    }
//
// In your _test.go file:
//
//    // Define a mock struct to be used in your unit tests of myFunc.
//    type mockRedshiftClient struct {
//        redshiftiface.RedshiftAPI
//    }
//    func (m *mockRedshiftClient) AcceptReservedNodeExchange(input *redshift.AcceptReservedNodeExchangeInput) (*redshift.AcceptReservedNodeExchangeOutput, error) {
//        // mock response/functionality
//    }
//
//    func TestMyFunc(t *testing.T) {
//        // Setup Test
//        mockSvc := &mockRedshiftClient{}
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
type RedshiftAPI interface {
	AcceptReservedNodeExchange(*redshift.AcceptReservedNodeExchangeInput) (*redshift.AcceptReservedNodeExchangeOutput, error)
	AcceptReservedNodeExchangeWithContext(aws.Context, *redshift.AcceptReservedNodeExchangeInput, ...request.Option) (*redshift.AcceptReservedNodeExchangeOutput, error)
	AcceptReservedNodeExchangeRequest(*redshift.AcceptReservedNodeExchangeInput) (*request.Request, *redshift.AcceptReservedNodeExchangeOutput)

	AuthorizeClusterSecurityGroupIngress(*redshift.AuthorizeClusterSecurityGroupIngressInput) (*redshift.AuthorizeClusterSecurityGroupIngressOutput, error)
	AuthorizeClusterSecurityGroupIngressWithContext(aws.Context, *redshift.AuthorizeClusterSecurityGroupIngressInput, ...request.Option) (*redshift.AuthorizeClusterSecurityGroupIngressOutput, error)
	AuthorizeClusterSecurityGroupIngressRequest(*redshift.AuthorizeClusterSecurityGroupIngressInput) (*request.Request, *redshift.AuthorizeClusterSecurityGroupIngressOutput)

	AuthorizeSnapshotAccess(*redshift.AuthorizeSnapshotAccessInput) (*redshift.AuthorizeSnapshotAccessOutput, error)
	AuthorizeSnapshotAccessWithContext(aws.Context, *redshift.AuthorizeSnapshotAccessInput, ...request.Option) (*redshift.AuthorizeSnapshotAccessOutput, error)
	AuthorizeSnapshotAccessRequest(*redshift.AuthorizeSnapshotAccessInput) (*request.Request, *redshift.AuthorizeSnapshotAccessOutput)

	CopyClusterSnapshot(*redshift.CopyClusterSnapshotInput) (*redshift.CopyClusterSnapshotOutput, error)
	CopyClusterSnapshotWithContext(aws.Context, *redshift.CopyClusterSnapshotInput, ...request.Option) (*redshift.CopyClusterSnapshotOutput, error)
	CopyClusterSnapshotRequest(*redshift.CopyClusterSnapshotInput) (*request.Request, *redshift.CopyClusterSnapshotOutput)

	CreateCluster(*redshift.CreateClusterInput) (*redshift.CreateClusterOutput, error)
	CreateClusterWithContext(aws.Context, *redshift.CreateClusterInput, ...request.Option) (*redshift.CreateClusterOutput, error)
	CreateClusterRequest(*redshift.CreateClusterInput) (*request.Request, *redshift.CreateClusterOutput)

	CreateClusterParameterGroup(*redshift.CreateClusterParameterGroupInput) (*redshift.CreateClusterParameterGroupOutput, error)
	CreateClusterParameterGroupWithContext(aws.Context, *redshift.CreateClusterParameterGroupInput, ...request.Option) (*redshift.CreateClusterParameterGroupOutput, error)
	CreateClusterParameterGroupRequest(*redshift.CreateClusterParameterGroupInput) (*request.Request, *redshift.CreateClusterParameterGroupOutput)

	CreateClusterSecurityGroup(*redshift.CreateClusterSecurityGroupInput) (*redshift.CreateClusterSecurityGroupOutput, error)
	CreateClusterSecurityGroupWithContext(aws.Context, *redshift.CreateClusterSecurityGroupInput, ...request.Option) (*redshift.CreateClusterSecurityGroupOutput, error)
	CreateClusterSecurityGroupRequest(*redshift.CreateClusterSecurityGroupInput) (*request.Request, *redshift.CreateClusterSecurityGroupOutput)

	CreateClusterSnapshot(*redshift.CreateClusterSnapshotInput) (*redshift.CreateClusterSnapshotOutput, error)
	CreateClusterSnapshotWithContext(aws.Context, *redshift.CreateClusterSnapshotInput, ...request.Option) (*redshift.CreateClusterSnapshotOutput, error)
	CreateClusterSnapshotRequest(*redshift.CreateClusterSnapshotInput) (*request.Request, *redshift.CreateClusterSnapshotOutput)

	CreateClusterSubnetGroup(*redshift.CreateClusterSubnetGroupInput) (*redshift.CreateClusterSubnetGroupOutput, error)
	CreateClusterSubnetGroupWithContext(aws.Context, *redshift.CreateClusterSubnetGroupInput, ...request.Option) (*redshift.CreateClusterSubnetGroupOutput, error)
	CreateClusterSubnetGroupRequest(*redshift.CreateClusterSubnetGroupInput) (*request.Request, *redshift.CreateClusterSubnetGroupOutput)

	CreateEventSubscription(*redshift.CreateEventSubscriptionInput) (*redshift.CreateEventSubscriptionOutput, error)
	CreateEventSubscriptionWithContext(aws.Context, *redshift.CreateEventSubscriptionInput, ...request.Option) (*redshift.CreateEventSubscriptionOutput, error)
	CreateEventSubscriptionRequest(*redshift.CreateEventSubscriptionInput) (*request.Request, *redshift.CreateEventSubscriptionOutput)

	CreateHsmClientCertificate(*redshift.CreateHsmClientCertificateInput) (*redshift.CreateHsmClientCertificateOutput, error)
	CreateHsmClientCertificateWithContext(aws.Context, *redshift.CreateHsmClientCertificateInput, ...request.Option) (*redshift.CreateHsmClientCertificateOutput, error)
	CreateHsmClientCertificateRequest(*redshift.CreateHsmClientCertificateInput) (*request.Request, *redshift.CreateHsmClientCertificateOutput)

	CreateHsmConfiguration(*redshift.CreateHsmConfigurationInput) (*redshift.CreateHsmConfigurationOutput, error)
	CreateHsmConfigurationWithContext(aws.Context, *redshift.CreateHsmConfigurationInput, ...request.Option) (*redshift.CreateHsmConfigurationOutput, error)
	CreateHsmConfigurationRequest(*redshift.CreateHsmConfigurationInput) (*request.Request, *redshift.CreateHsmConfigurationOutput)

	CreateSnapshotCopyGrant(*redshift.CreateSnapshotCopyGrantInput) (*redshift.CreateSnapshotCopyGrantOutput, error)
	CreateSnapshotCopyGrantWithContext(aws.Context, *redshift.CreateSnapshotCopyGrantInput, ...request.Option) (*redshift.CreateSnapshotCopyGrantOutput, error)
	CreateSnapshotCopyGrantRequest(*redshift.CreateSnapshotCopyGrantInput) (*request.Request, *redshift.CreateSnapshotCopyGrantOutput)

	CreateTags(*redshift.CreateTagsInput) (*redshift.CreateTagsOutput, error)
	CreateTagsWithContext(aws.Context, *redshift.CreateTagsInput, ...request.Option) (*redshift.CreateTagsOutput, error)
	CreateTagsRequest(*redshift.CreateTagsInput) (*request.Request, *redshift.CreateTagsOutput)

	DeleteCluster(*redshift.DeleteClusterInput) (*redshift.DeleteClusterOutput, error)
	DeleteClusterWithContext(aws.Context, *redshift.DeleteClusterInput, ...request.Option) (*redshift.DeleteClusterOutput, error)
	DeleteClusterRequest(*redshift.DeleteClusterInput) (*request.Request, *redshift.DeleteClusterOutput)

	DeleteClusterParameterGroup(*redshift.DeleteClusterParameterGroupInput) (*redshift.DeleteClusterParameterGroupOutput, error)
	DeleteClusterParameterGroupWithContext(aws.Context, *redshift.DeleteClusterParameterGroupInput, ...request.Option) (*redshift.DeleteClusterParameterGroupOutput, error)
	DeleteClusterParameterGroupRequest(*redshift.DeleteClusterParameterGroupInput) (*request.Request, *redshift.DeleteClusterParameterGroupOutput)

	DeleteClusterSecurityGroup(*redshift.DeleteClusterSecurityGroupInput) (*redshift.DeleteClusterSecurityGroupOutput, error)
	DeleteClusterSecurityGroupWithContext(aws.Context, *redshift.DeleteClusterSecurityGroupInput, ...request.Option) (*redshift.DeleteClusterSecurityGroupOutput, error)
	DeleteClusterSecurityGroupRequest(*redshift.DeleteClusterSecurityGroupInput) (*request.Request, *redshift.DeleteClusterSecurityGroupOutput)

	DeleteClusterSnapshot(*redshift.DeleteClusterSnapshotInput) (*redshift.DeleteClusterSnapshotOutput, error)
	DeleteClusterSnapshotWithContext(aws.Context, *redshift.DeleteClusterSnapshotInput, ...request.Option) (*redshift.DeleteClusterSnapshotOutput, error)
	DeleteClusterSnapshotRequest(*redshift.DeleteClusterSnapshotInput) (*request.Request, *redshift.DeleteClusterSnapshotOutput)

	DeleteClusterSubnetGroup(*redshift.DeleteClusterSubnetGroupInput) (*redshift.DeleteClusterSubnetGroupOutput, error)
	DeleteClusterSubnetGroupWithContext(aws.Context, *redshift.DeleteClusterSubnetGroupInput, ...request.Option) (*redshift.DeleteClusterSubnetGroupOutput, error)
	DeleteClusterSubnetGroupRequest(*redshift.DeleteClusterSubnetGroupInput) (*request.Request, *redshift.DeleteClusterSubnetGroupOutput)

	DeleteEventSubscription(*redshift.DeleteEventSubscriptionInput) (*redshift.DeleteEventSubscriptionOutput, error)
	DeleteEventSubscriptionWithContext(aws.Context, *redshift.DeleteEventSubscriptionInput, ...request.Option) (*redshift.DeleteEventSubscriptionOutput, error)
	DeleteEventSubscriptionRequest(*redshift.DeleteEventSubscriptionInput) (*request.Request, *redshift.DeleteEventSubscriptionOutput)

	DeleteHsmClientCertificate(*redshift.DeleteHsmClientCertificateInput) (*redshift.DeleteHsmClientCertificateOutput, error)
	DeleteHsmClientCertificateWithContext(aws.Context, *redshift.DeleteHsmClientCertificateInput, ...request.Option) (*redshift.DeleteHsmClientCertificateOutput, error)
	DeleteHsmClientCertificateRequest(*redshift.DeleteHsmClientCertificateInput) (*request.Request, *redshift.DeleteHsmClientCertificateOutput)

	DeleteHsmConfiguration(*redshift.DeleteHsmConfigurationInput) (*redshift.DeleteHsmConfigurationOutput, error)
	DeleteHsmConfigurationWithContext(aws.Context, *redshift.DeleteHsmConfigurationInput, ...request.Option) (*redshift.DeleteHsmConfigurationOutput, error)
	DeleteHsmConfigurationRequest(*redshift.DeleteHsmConfigurationInput) (*request.Request, *redshift.DeleteHsmConfigurationOutput)

	DeleteSnapshotCopyGrant(*redshift.DeleteSnapshotCopyGrantInput) (*redshift.DeleteSnapshotCopyGrantOutput, error)
	DeleteSnapshotCopyGrantWithContext(aws.Context, *redshift.DeleteSnapshotCopyGrantInput, ...request.Option) (*redshift.DeleteSnapshotCopyGrantOutput, error)
	DeleteSnapshotCopyGrantRequest(*redshift.DeleteSnapshotCopyGrantInput) (*request.Request, *redshift.DeleteSnapshotCopyGrantOutput)

	DeleteTags(*redshift.DeleteTagsInput) (*redshift.DeleteTagsOutput, error)
	DeleteTagsWithContext(aws.Context, *redshift.DeleteTagsInput, ...request.Option) (*redshift.DeleteTagsOutput, error)
	DeleteTagsRequest(*redshift.DeleteTagsInput) (*request.Request, *redshift.DeleteTagsOutput)

	DescribeClusterDbRevisions(*redshift.DescribeClusterDbRevisionsInput) (*redshift.DescribeClusterDbRevisionsOutput, error)
	DescribeClusterDbRevisionsWithContext(aws.Context, *redshift.DescribeClusterDbRevisionsInput, ...request.Option) (*redshift.DescribeClusterDbRevisionsOutput, error)
	DescribeClusterDbRevisionsRequest(*redshift.DescribeClusterDbRevisionsInput) (*request.Request, *redshift.DescribeClusterDbRevisionsOutput)

	DescribeClusterParameterGroups(*redshift.DescribeClusterParameterGroupsInput) (*redshift.DescribeClusterParameterGroupsOutput, error)
	DescribeClusterParameterGroupsWithContext(aws.Context, *redshift.DescribeClusterParameterGroupsInput, ...request.Option) (*redshift.DescribeClusterParameterGroupsOutput, error)
	DescribeClusterParameterGroupsRequest(*redshift.DescribeClusterParameterGroupsInput) (*request.Request, *redshift.DescribeClusterParameterGroupsOutput)

	DescribeClusterParameterGroupsPages(*redshift.DescribeClusterParameterGroupsInput, func(*redshift.DescribeClusterParameterGroupsOutput, bool) bool) error
	DescribeClusterParameterGroupsPagesWithContext(aws.Context, *redshift.DescribeClusterParameterGroupsInput, func(*redshift.DescribeClusterParameterGroupsOutput, bool) bool, ...request.Option) error

	DescribeClusterParameters(*redshift.DescribeClusterParametersInput) (*redshift.DescribeClusterParametersOutput, error)
	DescribeClusterParametersWithContext(aws.Context, *redshift.DescribeClusterParametersInput, ...request.Option) (*redshift.DescribeClusterParametersOutput, error)
	DescribeClusterParametersRequest(*redshift.DescribeClusterParametersInput) (*request.Request, *redshift.DescribeClusterParametersOutput)

	DescribeClusterParametersPages(*redshift.DescribeClusterParametersInput, func(*redshift.DescribeClusterParametersOutput, bool) bool) error
	DescribeClusterParametersPagesWithContext(aws.Context, *redshift.DescribeClusterParametersInput, func(*redshift.DescribeClusterParametersOutput, bool) bool, ...request.Option) error

	DescribeClusterSecurityGroups(*redshift.DescribeClusterSecurityGroupsInput) (*redshift.DescribeClusterSecurityGroupsOutput, error)
	DescribeClusterSecurityGroupsWithContext(aws.Context, *redshift.DescribeClusterSecurityGroupsInput, ...request.Option) (*redshift.DescribeClusterSecurityGroupsOutput, error)
	DescribeClusterSecurityGroupsRequest(*redshift.DescribeClusterSecurityGroupsInput) (*request.Request, *redshift.DescribeClusterSecurityGroupsOutput)

	DescribeClusterSecurityGroupsPages(*redshift.DescribeClusterSecurityGroupsInput, func(*redshift.DescribeClusterSecurityGroupsOutput, bool) bool) error
	DescribeClusterSecurityGroupsPagesWithContext(aws.Context, *redshift.DescribeClusterSecurityGroupsInput, func(*redshift.DescribeClusterSecurityGroupsOutput, bool) bool, ...request.Option) error

	DescribeClusterSnapshots(*redshift.DescribeClusterSnapshotsInput) (*redshift.DescribeClusterSnapshotsOutput, error)
	DescribeClusterSnapshotsWithContext(aws.Context, *redshift.DescribeClusterSnapshotsInput, ...request.Option) (*redshift.DescribeClusterSnapshotsOutput, error)
	DescribeClusterSnapshotsRequest(*redshift.DescribeClusterSnapshotsInput) (*request.Request, *redshift.DescribeClusterSnapshotsOutput)

	DescribeClusterSnapshotsPages(*redshift.DescribeClusterSnapshotsInput, func(*redshift.DescribeClusterSnapshotsOutput, bool) bool) error
	DescribeClusterSnapshotsPagesWithContext(aws.Context, *redshift.DescribeClusterSnapshotsInput, func(*redshift.DescribeClusterSnapshotsOutput, bool) bool, ...request.Option) error

	DescribeClusterSubnetGroups(*redshift.DescribeClusterSubnetGroupsInput) (*redshift.DescribeClusterSubnetGroupsOutput, error)
	DescribeClusterSubnetGroupsWithContext(aws.Context, *redshift.DescribeClusterSubnetGroupsInput, ...request.Option) (*redshift.DescribeClusterSubnetGroupsOutput, error)
	DescribeClusterSubnetGroupsRequest(*redshift.DescribeClusterSubnetGroupsInput) (*request.Request, *redshift.DescribeClusterSubnetGroupsOutput)

	DescribeClusterSubnetGroupsPages(*redshift.DescribeClusterSubnetGroupsInput, func(*redshift.DescribeClusterSubnetGroupsOutput, bool) bool) error
	DescribeClusterSubnetGroupsPagesWithContext(aws.Context, *redshift.DescribeClusterSubnetGroupsInput, func(*redshift.DescribeClusterSubnetGroupsOutput, bool) bool, ...request.Option) error

	DescribeClusterTracks(*redshift.DescribeClusterTracksInput) (*redshift.DescribeClusterTracksOutput, error)
	DescribeClusterTracksWithContext(aws.Context, *redshift.DescribeClusterTracksInput, ...request.Option) (*redshift.DescribeClusterTracksOutput, error)
	DescribeClusterTracksRequest(*redshift.DescribeClusterTracksInput) (*request.Request, *redshift.DescribeClusterTracksOutput)

	DescribeClusterVersions(*redshift.DescribeClusterVersionsInput) (*redshift.DescribeClusterVersionsOutput, error)
	DescribeClusterVersionsWithContext(aws.Context, *redshift.DescribeClusterVersionsInput, ...request.Option) (*redshift.DescribeClusterVersionsOutput, error)
	DescribeClusterVersionsRequest(*redshift.DescribeClusterVersionsInput) (*request.Request, *redshift.DescribeClusterVersionsOutput)

	DescribeClusterVersionsPages(*redshift.DescribeClusterVersionsInput, func(*redshift.DescribeClusterVersionsOutput, bool) bool) error
	DescribeClusterVersionsPagesWithContext(aws.Context, *redshift.DescribeClusterVersionsInput, func(*redshift.DescribeClusterVersionsOutput, bool) bool, ...request.Option) error

	DescribeClusters(*redshift.DescribeClustersInput) (*redshift.DescribeClustersOutput, error)
	DescribeClustersWithContext(aws.Context, *redshift.DescribeClustersInput, ...request.Option) (*redshift.DescribeClustersOutput, error)
	DescribeClustersRequest(*redshift.DescribeClustersInput) (*request.Request, *redshift.DescribeClustersOutput)

	DescribeClustersPages(*redshift.DescribeClustersInput, func(*redshift.DescribeClustersOutput, bool) bool) error
	DescribeClustersPagesWithContext(aws.Context, *redshift.DescribeClustersInput, func(*redshift.DescribeClustersOutput, bool) bool, ...request.Option) error

	DescribeDefaultClusterParameters(*redshift.DescribeDefaultClusterParametersInput) (*redshift.DescribeDefaultClusterParametersOutput, error)
	DescribeDefaultClusterParametersWithContext(aws.Context, *redshift.DescribeDefaultClusterParametersInput, ...request.Option) (*redshift.DescribeDefaultClusterParametersOutput, error)
	DescribeDefaultClusterParametersRequest(*redshift.DescribeDefaultClusterParametersInput) (*request.Request, *redshift.DescribeDefaultClusterParametersOutput)

	DescribeDefaultClusterParametersPages(*redshift.DescribeDefaultClusterParametersInput, func(*redshift.DescribeDefaultClusterParametersOutput, bool) bool) error
	DescribeDefaultClusterParametersPagesWithContext(aws.Context, *redshift.DescribeDefaultClusterParametersInput, func(*redshift.DescribeDefaultClusterParametersOutput, bool) bool, ...request.Option) error

	DescribeEventCategories(*redshift.DescribeEventCategoriesInput) (*redshift.DescribeEventCategoriesOutput, error)
	DescribeEventCategoriesWithContext(aws.Context, *redshift.DescribeEventCategoriesInput, ...request.Option) (*redshift.DescribeEventCategoriesOutput, error)
	DescribeEventCategoriesRequest(*redshift.DescribeEventCategoriesInput) (*request.Request, *redshift.DescribeEventCategoriesOutput)

	DescribeEventSubscriptions(*redshift.DescribeEventSubscriptionsInput) (*redshift.DescribeEventSubscriptionsOutput, error)
	DescribeEventSubscriptionsWithContext(aws.Context, *redshift.DescribeEventSubscriptionsInput, ...request.Option) (*redshift.DescribeEventSubscriptionsOutput, error)
	DescribeEventSubscriptionsRequest(*redshift.DescribeEventSubscriptionsInput) (*request.Request, *redshift.DescribeEventSubscriptionsOutput)

	DescribeEventSubscriptionsPages(*redshift.DescribeEventSubscriptionsInput, func(*redshift.DescribeEventSubscriptionsOutput, bool) bool) error
	DescribeEventSubscriptionsPagesWithContext(aws.Context, *redshift.DescribeEventSubscriptionsInput, func(*redshift.DescribeEventSubscriptionsOutput, bool) bool, ...request.Option) error

	DescribeEvents(*redshift.DescribeEventsInput) (*redshift.DescribeEventsOutput, error)
	DescribeEventsWithContext(aws.Context, *redshift.DescribeEventsInput, ...request.Option) (*redshift.DescribeEventsOutput, error)
	DescribeEventsRequest(*redshift.DescribeEventsInput) (*request.Request, *redshift.DescribeEventsOutput)

	DescribeEventsPages(*redshift.DescribeEventsInput, func(*redshift.DescribeEventsOutput, bool) bool) error
	DescribeEventsPagesWithContext(aws.Context, *redshift.DescribeEventsInput, func(*redshift.DescribeEventsOutput, bool) bool, ...request.Option) error

	DescribeHsmClientCertificates(*redshift.DescribeHsmClientCertificatesInput) (*redshift.DescribeHsmClientCertificatesOutput, error)
	DescribeHsmClientCertificatesWithContext(aws.Context, *redshift.DescribeHsmClientCertificatesInput, ...request.Option) (*redshift.DescribeHsmClientCertificatesOutput, error)
	DescribeHsmClientCertificatesRequest(*redshift.DescribeHsmClientCertificatesInput) (*request.Request, *redshift.DescribeHsmClientCertificatesOutput)

	DescribeHsmClientCertificatesPages(*redshift.DescribeHsmClientCertificatesInput, func(*redshift.DescribeHsmClientCertificatesOutput, bool) bool) error
	DescribeHsmClientCertificatesPagesWithContext(aws.Context, *redshift.DescribeHsmClientCertificatesInput, func(*redshift.DescribeHsmClientCertificatesOutput, bool) bool, ...request.Option) error

	DescribeHsmConfigurations(*redshift.DescribeHsmConfigurationsInput) (*redshift.DescribeHsmConfigurationsOutput, error)
	DescribeHsmConfigurationsWithContext(aws.Context, *redshift.DescribeHsmConfigurationsInput, ...request.Option) (*redshift.DescribeHsmConfigurationsOutput, error)
	DescribeHsmConfigurationsRequest(*redshift.DescribeHsmConfigurationsInput) (*request.Request, *redshift.DescribeHsmConfigurationsOutput)

	DescribeHsmConfigurationsPages(*redshift.DescribeHsmConfigurationsInput, func(*redshift.DescribeHsmConfigurationsOutput, bool) bool) error
	DescribeHsmConfigurationsPagesWithContext(aws.Context, *redshift.DescribeHsmConfigurationsInput, func(*redshift.DescribeHsmConfigurationsOutput, bool) bool, ...request.Option) error

	DescribeLoggingStatus(*redshift.DescribeLoggingStatusInput) (*redshift.LoggingStatus, error)
	DescribeLoggingStatusWithContext(aws.Context, *redshift.DescribeLoggingStatusInput, ...request.Option) (*redshift.LoggingStatus, error)
	DescribeLoggingStatusRequest(*redshift.DescribeLoggingStatusInput) (*request.Request, *redshift.LoggingStatus)

	DescribeOrderableClusterOptions(*redshift.DescribeOrderableClusterOptionsInput) (*redshift.DescribeOrderableClusterOptionsOutput, error)
	DescribeOrderableClusterOptionsWithContext(aws.Context, *redshift.DescribeOrderableClusterOptionsInput, ...request.Option) (*redshift.DescribeOrderableClusterOptionsOutput, error)
	DescribeOrderableClusterOptionsRequest(*redshift.DescribeOrderableClusterOptionsInput) (*request.Request, *redshift.DescribeOrderableClusterOptionsOutput)

	DescribeOrderableClusterOptionsPages(*redshift.DescribeOrderableClusterOptionsInput, func(*redshift.DescribeOrderableClusterOptionsOutput, bool) bool) error
	DescribeOrderableClusterOptionsPagesWithContext(aws.Context, *redshift.DescribeOrderableClusterOptionsInput, func(*redshift.DescribeOrderableClusterOptionsOutput, bool) bool, ...request.Option) error

	DescribeReservedNodeOfferings(*redshift.DescribeReservedNodeOfferingsInput) (*redshift.DescribeReservedNodeOfferingsOutput, error)
	DescribeReservedNodeOfferingsWithContext(aws.Context, *redshift.DescribeReservedNodeOfferingsInput, ...request.Option) (*redshift.DescribeReservedNodeOfferingsOutput, error)
	DescribeReservedNodeOfferingsRequest(*redshift.DescribeReservedNodeOfferingsInput) (*request.Request, *redshift.DescribeReservedNodeOfferingsOutput)

	DescribeReservedNodeOfferingsPages(*redshift.DescribeReservedNodeOfferingsInput, func(*redshift.DescribeReservedNodeOfferingsOutput, bool) bool) error
	DescribeReservedNodeOfferingsPagesWithContext(aws.Context, *redshift.DescribeReservedNodeOfferingsInput, func(*redshift.DescribeReservedNodeOfferingsOutput, bool) bool, ...request.Option) error

	DescribeReservedNodes(*redshift.DescribeReservedNodesInput) (*redshift.DescribeReservedNodesOutput, error)
	DescribeReservedNodesWithContext(aws.Context, *redshift.DescribeReservedNodesInput, ...request.Option) (*redshift.DescribeReservedNodesOutput, error)
	DescribeReservedNodesRequest(*redshift.DescribeReservedNodesInput) (*request.Request, *redshift.DescribeReservedNodesOutput)

	DescribeReservedNodesPages(*redshift.DescribeReservedNodesInput, func(*redshift.DescribeReservedNodesOutput, bool) bool) error
	DescribeReservedNodesPagesWithContext(aws.Context, *redshift.DescribeReservedNodesInput, func(*redshift.DescribeReservedNodesOutput, bool) bool, ...request.Option) error

	DescribeResize(*redshift.DescribeResizeInput) (*redshift.DescribeResizeOutput, error)
	DescribeResizeWithContext(aws.Context, *redshift.DescribeResizeInput, ...request.Option) (*redshift.DescribeResizeOutput, error)
	DescribeResizeRequest(*redshift.DescribeResizeInput) (*request.Request, *redshift.DescribeResizeOutput)

	DescribeSnapshotCopyGrants(*redshift.DescribeSnapshotCopyGrantsInput) (*redshift.DescribeSnapshotCopyGrantsOutput, error)
	DescribeSnapshotCopyGrantsWithContext(aws.Context, *redshift.DescribeSnapshotCopyGrantsInput, ...request.Option) (*redshift.DescribeSnapshotCopyGrantsOutput, error)
	DescribeSnapshotCopyGrantsRequest(*redshift.DescribeSnapshotCopyGrantsInput) (*request.Request, *redshift.DescribeSnapshotCopyGrantsOutput)

	DescribeTableRestoreStatus(*redshift.DescribeTableRestoreStatusInput) (*redshift.DescribeTableRestoreStatusOutput, error)
	DescribeTableRestoreStatusWithContext(aws.Context, *redshift.DescribeTableRestoreStatusInput, ...request.Option) (*redshift.DescribeTableRestoreStatusOutput, error)
	DescribeTableRestoreStatusRequest(*redshift.DescribeTableRestoreStatusInput) (*request.Request, *redshift.DescribeTableRestoreStatusOutput)

	DescribeTags(*redshift.DescribeTagsInput) (*redshift.DescribeTagsOutput, error)
	DescribeTagsWithContext(aws.Context, *redshift.DescribeTagsInput, ...request.Option) (*redshift.DescribeTagsOutput, error)
	DescribeTagsRequest(*redshift.DescribeTagsInput) (*request.Request, *redshift.DescribeTagsOutput)

	DisableLogging(*redshift.DisableLoggingInput) (*redshift.LoggingStatus, error)
	DisableLoggingWithContext(aws.Context, *redshift.DisableLoggingInput, ...request.Option) (*redshift.LoggingStatus, error)
	DisableLoggingRequest(*redshift.DisableLoggingInput) (*request.Request, *redshift.LoggingStatus)

	DisableSnapshotCopy(*redshift.DisableSnapshotCopyInput) (*redshift.DisableSnapshotCopyOutput, error)
	DisableSnapshotCopyWithContext(aws.Context, *redshift.DisableSnapshotCopyInput, ...request.Option) (*redshift.DisableSnapshotCopyOutput, error)
	DisableSnapshotCopyRequest(*redshift.DisableSnapshotCopyInput) (*request.Request, *redshift.DisableSnapshotCopyOutput)

	EnableLogging(*redshift.EnableLoggingInput) (*redshift.LoggingStatus, error)
	EnableLoggingWithContext(aws.Context, *redshift.EnableLoggingInput, ...request.Option) (*redshift.LoggingStatus, error)
	EnableLoggingRequest(*redshift.EnableLoggingInput) (*request.Request, *redshift.LoggingStatus)

	EnableSnapshotCopy(*redshift.EnableSnapshotCopyInput) (*redshift.EnableSnapshotCopyOutput, error)
	EnableSnapshotCopyWithContext(aws.Context, *redshift.EnableSnapshotCopyInput, ...request.Option) (*redshift.EnableSnapshotCopyOutput, error)
	EnableSnapshotCopyRequest(*redshift.EnableSnapshotCopyInput) (*request.Request, *redshift.EnableSnapshotCopyOutput)

	GetClusterCredentials(*redshift.GetClusterCredentialsInput) (*redshift.GetClusterCredentialsOutput, error)
	GetClusterCredentialsWithContext(aws.Context, *redshift.GetClusterCredentialsInput, ...request.Option) (*redshift.GetClusterCredentialsOutput, error)
	GetClusterCredentialsRequest(*redshift.GetClusterCredentialsInput) (*request.Request, *redshift.GetClusterCredentialsOutput)

	GetReservedNodeExchangeOfferings(*redshift.GetReservedNodeExchangeOfferingsInput) (*redshift.GetReservedNodeExchangeOfferingsOutput, error)
	GetReservedNodeExchangeOfferingsWithContext(aws.Context, *redshift.GetReservedNodeExchangeOfferingsInput, ...request.Option) (*redshift.GetReservedNodeExchangeOfferingsOutput, error)
	GetReservedNodeExchangeOfferingsRequest(*redshift.GetReservedNodeExchangeOfferingsInput) (*request.Request, *redshift.GetReservedNodeExchangeOfferingsOutput)

	ModifyCluster(*redshift.ModifyClusterInput) (*redshift.ModifyClusterOutput, error)
	ModifyClusterWithContext(aws.Context, *redshift.ModifyClusterInput, ...request.Option) (*redshift.ModifyClusterOutput, error)
	ModifyClusterRequest(*redshift.ModifyClusterInput) (*request.Request, *redshift.ModifyClusterOutput)

	ModifyClusterDbRevision(*redshift.ModifyClusterDbRevisionInput) (*redshift.ModifyClusterDbRevisionOutput, error)
	ModifyClusterDbRevisionWithContext(aws.Context, *redshift.ModifyClusterDbRevisionInput, ...request.Option) (*redshift.ModifyClusterDbRevisionOutput, error)
	ModifyClusterDbRevisionRequest(*redshift.ModifyClusterDbRevisionInput) (*request.Request, *redshift.ModifyClusterDbRevisionOutput)

	ModifyClusterIamRoles(*redshift.ModifyClusterIamRolesInput) (*redshift.ModifyClusterIamRolesOutput, error)
	ModifyClusterIamRolesWithContext(aws.Context, *redshift.ModifyClusterIamRolesInput, ...request.Option) (*redshift.ModifyClusterIamRolesOutput, error)
	ModifyClusterIamRolesRequest(*redshift.ModifyClusterIamRolesInput) (*request.Request, *redshift.ModifyClusterIamRolesOutput)

	ModifyClusterParameterGroup(*redshift.ModifyClusterParameterGroupInput) (*redshift.ClusterParameterGroupNameMessage, error)
	ModifyClusterParameterGroupWithContext(aws.Context, *redshift.ModifyClusterParameterGroupInput, ...request.Option) (*redshift.ClusterParameterGroupNameMessage, error)
	ModifyClusterParameterGroupRequest(*redshift.ModifyClusterParameterGroupInput) (*request.Request, *redshift.ClusterParameterGroupNameMessage)

	ModifyClusterSubnetGroup(*redshift.ModifyClusterSubnetGroupInput) (*redshift.ModifyClusterSubnetGroupOutput, error)
	ModifyClusterSubnetGroupWithContext(aws.Context, *redshift.ModifyClusterSubnetGroupInput, ...request.Option) (*redshift.ModifyClusterSubnetGroupOutput, error)
	ModifyClusterSubnetGroupRequest(*redshift.ModifyClusterSubnetGroupInput) (*request.Request, *redshift.ModifyClusterSubnetGroupOutput)

	ModifyEventSubscription(*redshift.ModifyEventSubscriptionInput) (*redshift.ModifyEventSubscriptionOutput, error)
	ModifyEventSubscriptionWithContext(aws.Context, *redshift.ModifyEventSubscriptionInput, ...request.Option) (*redshift.ModifyEventSubscriptionOutput, error)
	ModifyEventSubscriptionRequest(*redshift.ModifyEventSubscriptionInput) (*request.Request, *redshift.ModifyEventSubscriptionOutput)

	ModifySnapshotCopyRetentionPeriod(*redshift.ModifySnapshotCopyRetentionPeriodInput) (*redshift.ModifySnapshotCopyRetentionPeriodOutput, error)
	ModifySnapshotCopyRetentionPeriodWithContext(aws.Context, *redshift.ModifySnapshotCopyRetentionPeriodInput, ...request.Option) (*redshift.ModifySnapshotCopyRetentionPeriodOutput, error)
	ModifySnapshotCopyRetentionPeriodRequest(*redshift.ModifySnapshotCopyRetentionPeriodInput) (*request.Request, *redshift.ModifySnapshotCopyRetentionPeriodOutput)

	PurchaseReservedNodeOffering(*redshift.PurchaseReservedNodeOfferingInput) (*redshift.PurchaseReservedNodeOfferingOutput, error)
	PurchaseReservedNodeOfferingWithContext(aws.Context, *redshift.PurchaseReservedNodeOfferingInput, ...request.Option) (*redshift.PurchaseReservedNodeOfferingOutput, error)
	PurchaseReservedNodeOfferingRequest(*redshift.PurchaseReservedNodeOfferingInput) (*request.Request, *redshift.PurchaseReservedNodeOfferingOutput)

	RebootCluster(*redshift.RebootClusterInput) (*redshift.RebootClusterOutput, error)
	RebootClusterWithContext(aws.Context, *redshift.RebootClusterInput, ...request.Option) (*redshift.RebootClusterOutput, error)
	RebootClusterRequest(*redshift.RebootClusterInput) (*request.Request, *redshift.RebootClusterOutput)

	ResetClusterParameterGroup(*redshift.ResetClusterParameterGroupInput) (*redshift.ClusterParameterGroupNameMessage, error)
	ResetClusterParameterGroupWithContext(aws.Context, *redshift.ResetClusterParameterGroupInput, ...request.Option) (*redshift.ClusterParameterGroupNameMessage, error)
	ResetClusterParameterGroupRequest(*redshift.ResetClusterParameterGroupInput) (*request.Request, *redshift.ClusterParameterGroupNameMessage)

	ResizeCluster(*redshift.ResizeClusterInput) (*redshift.ResizeClusterOutput, error)
	ResizeClusterWithContext(aws.Context, *redshift.ResizeClusterInput, ...request.Option) (*redshift.ResizeClusterOutput, error)
	ResizeClusterRequest(*redshift.ResizeClusterInput) (*request.Request, *redshift.ResizeClusterOutput)

	RestoreFromClusterSnapshot(*redshift.RestoreFromClusterSnapshotInput) (*redshift.RestoreFromClusterSnapshotOutput, error)
	RestoreFromClusterSnapshotWithContext(aws.Context, *redshift.RestoreFromClusterSnapshotInput, ...request.Option) (*redshift.RestoreFromClusterSnapshotOutput, error)
	RestoreFromClusterSnapshotRequest(*redshift.RestoreFromClusterSnapshotInput) (*request.Request, *redshift.RestoreFromClusterSnapshotOutput)

	RestoreTableFromClusterSnapshot(*redshift.RestoreTableFromClusterSnapshotInput) (*redshift.RestoreTableFromClusterSnapshotOutput, error)
	RestoreTableFromClusterSnapshotWithContext(aws.Context, *redshift.RestoreTableFromClusterSnapshotInput, ...request.Option) (*redshift.RestoreTableFromClusterSnapshotOutput, error)
	RestoreTableFromClusterSnapshotRequest(*redshift.RestoreTableFromClusterSnapshotInput) (*request.Request, *redshift.RestoreTableFromClusterSnapshotOutput)

	RevokeClusterSecurityGroupIngress(*redshift.RevokeClusterSecurityGroupIngressInput) (*redshift.RevokeClusterSecurityGroupIngressOutput, error)
	RevokeClusterSecurityGroupIngressWithContext(aws.Context, *redshift.RevokeClusterSecurityGroupIngressInput, ...request.Option) (*redshift.RevokeClusterSecurityGroupIngressOutput, error)
	RevokeClusterSecurityGroupIngressRequest(*redshift.RevokeClusterSecurityGroupIngressInput) (*request.Request, *redshift.RevokeClusterSecurityGroupIngressOutput)

	RevokeSnapshotAccess(*redshift.RevokeSnapshotAccessInput) (*redshift.RevokeSnapshotAccessOutput, error)
	RevokeSnapshotAccessWithContext(aws.Context, *redshift.RevokeSnapshotAccessInput, ...request.Option) (*redshift.RevokeSnapshotAccessOutput, error)
	RevokeSnapshotAccessRequest(*redshift.RevokeSnapshotAccessInput) (*request.Request, *redshift.RevokeSnapshotAccessOutput)

	RotateEncryptionKey(*redshift.RotateEncryptionKeyInput) (*redshift.RotateEncryptionKeyOutput, error)
	RotateEncryptionKeyWithContext(aws.Context, *redshift.RotateEncryptionKeyInput, ...request.Option) (*redshift.RotateEncryptionKeyOutput, error)
	RotateEncryptionKeyRequest(*redshift.RotateEncryptionKeyInput) (*request.Request, *redshift.RotateEncryptionKeyOutput)

	WaitUntilClusterAvailable(*redshift.DescribeClustersInput) error
	WaitUntilClusterAvailableWithContext(aws.Context, *redshift.DescribeClustersInput, ...request.WaiterOption) error

	WaitUntilClusterDeleted(*redshift.DescribeClustersInput) error
	WaitUntilClusterDeletedWithContext(aws.Context, *redshift.DescribeClustersInput, ...request.WaiterOption) error

	WaitUntilClusterRestored(*redshift.DescribeClustersInput) error
	WaitUntilClusterRestoredWithContext(aws.Context, *redshift.DescribeClustersInput, ...request.WaiterOption) error

	WaitUntilSnapshotAvailable(*redshift.DescribeClusterSnapshotsInput) error
	WaitUntilSnapshotAvailableWithContext(aws.Context, *redshift.DescribeClusterSnapshotsInput, ...request.WaiterOption) error
}

var _ RedshiftAPI = (*redshift.Redshift)(nil)
