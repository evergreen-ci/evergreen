// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

// Package robomakeriface provides an interface to enable mocking the AWS RoboMaker service client
// for testing your code.
//
// It is important to note that this interface will have breaking changes
// when the service model is updated and adds new API operations, paginators,
// and waiters.
package robomakeriface

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/robomaker"
)

// RoboMakerAPI provides an interface to enable mocking the
// robomaker.RoboMaker service client's API operation,
// paginators, and waiters. This make unit testing your code that calls out
// to the SDK's service client's calls easier.
//
// The best way to use this interface is so the SDK's service client's calls
// can be stubbed out for unit testing your code with the SDK without needing
// to inject custom request handlers into the SDK's request pipeline.
//
//    // myFunc uses an SDK service client to make a request to
//    // AWS RoboMaker.
//    func myFunc(svc robomakeriface.RoboMakerAPI) bool {
//        // Make svc.BatchDeleteWorlds request
//    }
//
//    func main() {
//        sess := session.New()
//        svc := robomaker.New(sess)
//
//        myFunc(svc)
//    }
//
// In your _test.go file:
//
//    // Define a mock struct to be used in your unit tests of myFunc.
//    type mockRoboMakerClient struct {
//        robomakeriface.RoboMakerAPI
//    }
//    func (m *mockRoboMakerClient) BatchDeleteWorlds(input *robomaker.BatchDeleteWorldsInput) (*robomaker.BatchDeleteWorldsOutput, error) {
//        // mock response/functionality
//    }
//
//    func TestMyFunc(t *testing.T) {
//        // Setup Test
//        mockSvc := &mockRoboMakerClient{}
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
type RoboMakerAPI interface {
	BatchDeleteWorlds(*robomaker.BatchDeleteWorldsInput) (*robomaker.BatchDeleteWorldsOutput, error)
	BatchDeleteWorldsWithContext(aws.Context, *robomaker.BatchDeleteWorldsInput, ...request.Option) (*robomaker.BatchDeleteWorldsOutput, error)
	BatchDeleteWorldsRequest(*robomaker.BatchDeleteWorldsInput) (*request.Request, *robomaker.BatchDeleteWorldsOutput)

	BatchDescribeSimulationJob(*robomaker.BatchDescribeSimulationJobInput) (*robomaker.BatchDescribeSimulationJobOutput, error)
	BatchDescribeSimulationJobWithContext(aws.Context, *robomaker.BatchDescribeSimulationJobInput, ...request.Option) (*robomaker.BatchDescribeSimulationJobOutput, error)
	BatchDescribeSimulationJobRequest(*robomaker.BatchDescribeSimulationJobInput) (*request.Request, *robomaker.BatchDescribeSimulationJobOutput)

	CancelDeploymentJob(*robomaker.CancelDeploymentJobInput) (*robomaker.CancelDeploymentJobOutput, error)
	CancelDeploymentJobWithContext(aws.Context, *robomaker.CancelDeploymentJobInput, ...request.Option) (*robomaker.CancelDeploymentJobOutput, error)
	CancelDeploymentJobRequest(*robomaker.CancelDeploymentJobInput) (*request.Request, *robomaker.CancelDeploymentJobOutput)

	CancelSimulationJob(*robomaker.CancelSimulationJobInput) (*robomaker.CancelSimulationJobOutput, error)
	CancelSimulationJobWithContext(aws.Context, *robomaker.CancelSimulationJobInput, ...request.Option) (*robomaker.CancelSimulationJobOutput, error)
	CancelSimulationJobRequest(*robomaker.CancelSimulationJobInput) (*request.Request, *robomaker.CancelSimulationJobOutput)

	CancelSimulationJobBatch(*robomaker.CancelSimulationJobBatchInput) (*robomaker.CancelSimulationJobBatchOutput, error)
	CancelSimulationJobBatchWithContext(aws.Context, *robomaker.CancelSimulationJobBatchInput, ...request.Option) (*robomaker.CancelSimulationJobBatchOutput, error)
	CancelSimulationJobBatchRequest(*robomaker.CancelSimulationJobBatchInput) (*request.Request, *robomaker.CancelSimulationJobBatchOutput)

	CancelWorldExportJob(*robomaker.CancelWorldExportJobInput) (*robomaker.CancelWorldExportJobOutput, error)
	CancelWorldExportJobWithContext(aws.Context, *robomaker.CancelWorldExportJobInput, ...request.Option) (*robomaker.CancelWorldExportJobOutput, error)
	CancelWorldExportJobRequest(*robomaker.CancelWorldExportJobInput) (*request.Request, *robomaker.CancelWorldExportJobOutput)

	CancelWorldGenerationJob(*robomaker.CancelWorldGenerationJobInput) (*robomaker.CancelWorldGenerationJobOutput, error)
	CancelWorldGenerationJobWithContext(aws.Context, *robomaker.CancelWorldGenerationJobInput, ...request.Option) (*robomaker.CancelWorldGenerationJobOutput, error)
	CancelWorldGenerationJobRequest(*robomaker.CancelWorldGenerationJobInput) (*request.Request, *robomaker.CancelWorldGenerationJobOutput)

	CreateDeploymentJob(*robomaker.CreateDeploymentJobInput) (*robomaker.CreateDeploymentJobOutput, error)
	CreateDeploymentJobWithContext(aws.Context, *robomaker.CreateDeploymentJobInput, ...request.Option) (*robomaker.CreateDeploymentJobOutput, error)
	CreateDeploymentJobRequest(*robomaker.CreateDeploymentJobInput) (*request.Request, *robomaker.CreateDeploymentJobOutput)

	CreateFleet(*robomaker.CreateFleetInput) (*robomaker.CreateFleetOutput, error)
	CreateFleetWithContext(aws.Context, *robomaker.CreateFleetInput, ...request.Option) (*robomaker.CreateFleetOutput, error)
	CreateFleetRequest(*robomaker.CreateFleetInput) (*request.Request, *robomaker.CreateFleetOutput)

	CreateRobot(*robomaker.CreateRobotInput) (*robomaker.CreateRobotOutput, error)
	CreateRobotWithContext(aws.Context, *robomaker.CreateRobotInput, ...request.Option) (*robomaker.CreateRobotOutput, error)
	CreateRobotRequest(*robomaker.CreateRobotInput) (*request.Request, *robomaker.CreateRobotOutput)

	CreateRobotApplication(*robomaker.CreateRobotApplicationInput) (*robomaker.CreateRobotApplicationOutput, error)
	CreateRobotApplicationWithContext(aws.Context, *robomaker.CreateRobotApplicationInput, ...request.Option) (*robomaker.CreateRobotApplicationOutput, error)
	CreateRobotApplicationRequest(*robomaker.CreateRobotApplicationInput) (*request.Request, *robomaker.CreateRobotApplicationOutput)

	CreateRobotApplicationVersion(*robomaker.CreateRobotApplicationVersionInput) (*robomaker.CreateRobotApplicationVersionOutput, error)
	CreateRobotApplicationVersionWithContext(aws.Context, *robomaker.CreateRobotApplicationVersionInput, ...request.Option) (*robomaker.CreateRobotApplicationVersionOutput, error)
	CreateRobotApplicationVersionRequest(*robomaker.CreateRobotApplicationVersionInput) (*request.Request, *robomaker.CreateRobotApplicationVersionOutput)

	CreateSimulationApplication(*robomaker.CreateSimulationApplicationInput) (*robomaker.CreateSimulationApplicationOutput, error)
	CreateSimulationApplicationWithContext(aws.Context, *robomaker.CreateSimulationApplicationInput, ...request.Option) (*robomaker.CreateSimulationApplicationOutput, error)
	CreateSimulationApplicationRequest(*robomaker.CreateSimulationApplicationInput) (*request.Request, *robomaker.CreateSimulationApplicationOutput)

	CreateSimulationApplicationVersion(*robomaker.CreateSimulationApplicationVersionInput) (*robomaker.CreateSimulationApplicationVersionOutput, error)
	CreateSimulationApplicationVersionWithContext(aws.Context, *robomaker.CreateSimulationApplicationVersionInput, ...request.Option) (*robomaker.CreateSimulationApplicationVersionOutput, error)
	CreateSimulationApplicationVersionRequest(*robomaker.CreateSimulationApplicationVersionInput) (*request.Request, *robomaker.CreateSimulationApplicationVersionOutput)

	CreateSimulationJob(*robomaker.CreateSimulationJobInput) (*robomaker.CreateSimulationJobOutput, error)
	CreateSimulationJobWithContext(aws.Context, *robomaker.CreateSimulationJobInput, ...request.Option) (*robomaker.CreateSimulationJobOutput, error)
	CreateSimulationJobRequest(*robomaker.CreateSimulationJobInput) (*request.Request, *robomaker.CreateSimulationJobOutput)

	CreateWorldExportJob(*robomaker.CreateWorldExportJobInput) (*robomaker.CreateWorldExportJobOutput, error)
	CreateWorldExportJobWithContext(aws.Context, *robomaker.CreateWorldExportJobInput, ...request.Option) (*robomaker.CreateWorldExportJobOutput, error)
	CreateWorldExportJobRequest(*robomaker.CreateWorldExportJobInput) (*request.Request, *robomaker.CreateWorldExportJobOutput)

	CreateWorldGenerationJob(*robomaker.CreateWorldGenerationJobInput) (*robomaker.CreateWorldGenerationJobOutput, error)
	CreateWorldGenerationJobWithContext(aws.Context, *robomaker.CreateWorldGenerationJobInput, ...request.Option) (*robomaker.CreateWorldGenerationJobOutput, error)
	CreateWorldGenerationJobRequest(*robomaker.CreateWorldGenerationJobInput) (*request.Request, *robomaker.CreateWorldGenerationJobOutput)

	CreateWorldTemplate(*robomaker.CreateWorldTemplateInput) (*robomaker.CreateWorldTemplateOutput, error)
	CreateWorldTemplateWithContext(aws.Context, *robomaker.CreateWorldTemplateInput, ...request.Option) (*robomaker.CreateWorldTemplateOutput, error)
	CreateWorldTemplateRequest(*robomaker.CreateWorldTemplateInput) (*request.Request, *robomaker.CreateWorldTemplateOutput)

	DeleteFleet(*robomaker.DeleteFleetInput) (*robomaker.DeleteFleetOutput, error)
	DeleteFleetWithContext(aws.Context, *robomaker.DeleteFleetInput, ...request.Option) (*robomaker.DeleteFleetOutput, error)
	DeleteFleetRequest(*robomaker.DeleteFleetInput) (*request.Request, *robomaker.DeleteFleetOutput)

	DeleteRobot(*robomaker.DeleteRobotInput) (*robomaker.DeleteRobotOutput, error)
	DeleteRobotWithContext(aws.Context, *robomaker.DeleteRobotInput, ...request.Option) (*robomaker.DeleteRobotOutput, error)
	DeleteRobotRequest(*robomaker.DeleteRobotInput) (*request.Request, *robomaker.DeleteRobotOutput)

	DeleteRobotApplication(*robomaker.DeleteRobotApplicationInput) (*robomaker.DeleteRobotApplicationOutput, error)
	DeleteRobotApplicationWithContext(aws.Context, *robomaker.DeleteRobotApplicationInput, ...request.Option) (*robomaker.DeleteRobotApplicationOutput, error)
	DeleteRobotApplicationRequest(*robomaker.DeleteRobotApplicationInput) (*request.Request, *robomaker.DeleteRobotApplicationOutput)

	DeleteSimulationApplication(*robomaker.DeleteSimulationApplicationInput) (*robomaker.DeleteSimulationApplicationOutput, error)
	DeleteSimulationApplicationWithContext(aws.Context, *robomaker.DeleteSimulationApplicationInput, ...request.Option) (*robomaker.DeleteSimulationApplicationOutput, error)
	DeleteSimulationApplicationRequest(*robomaker.DeleteSimulationApplicationInput) (*request.Request, *robomaker.DeleteSimulationApplicationOutput)

	DeleteWorldTemplate(*robomaker.DeleteWorldTemplateInput) (*robomaker.DeleteWorldTemplateOutput, error)
	DeleteWorldTemplateWithContext(aws.Context, *robomaker.DeleteWorldTemplateInput, ...request.Option) (*robomaker.DeleteWorldTemplateOutput, error)
	DeleteWorldTemplateRequest(*robomaker.DeleteWorldTemplateInput) (*request.Request, *robomaker.DeleteWorldTemplateOutput)

	DeregisterRobot(*robomaker.DeregisterRobotInput) (*robomaker.DeregisterRobotOutput, error)
	DeregisterRobotWithContext(aws.Context, *robomaker.DeregisterRobotInput, ...request.Option) (*robomaker.DeregisterRobotOutput, error)
	DeregisterRobotRequest(*robomaker.DeregisterRobotInput) (*request.Request, *robomaker.DeregisterRobotOutput)

	DescribeDeploymentJob(*robomaker.DescribeDeploymentJobInput) (*robomaker.DescribeDeploymentJobOutput, error)
	DescribeDeploymentJobWithContext(aws.Context, *robomaker.DescribeDeploymentJobInput, ...request.Option) (*robomaker.DescribeDeploymentJobOutput, error)
	DescribeDeploymentJobRequest(*robomaker.DescribeDeploymentJobInput) (*request.Request, *robomaker.DescribeDeploymentJobOutput)

	DescribeFleet(*robomaker.DescribeFleetInput) (*robomaker.DescribeFleetOutput, error)
	DescribeFleetWithContext(aws.Context, *robomaker.DescribeFleetInput, ...request.Option) (*robomaker.DescribeFleetOutput, error)
	DescribeFleetRequest(*robomaker.DescribeFleetInput) (*request.Request, *robomaker.DescribeFleetOutput)

	DescribeRobot(*robomaker.DescribeRobotInput) (*robomaker.DescribeRobotOutput, error)
	DescribeRobotWithContext(aws.Context, *robomaker.DescribeRobotInput, ...request.Option) (*robomaker.DescribeRobotOutput, error)
	DescribeRobotRequest(*robomaker.DescribeRobotInput) (*request.Request, *robomaker.DescribeRobotOutput)

	DescribeRobotApplication(*robomaker.DescribeRobotApplicationInput) (*robomaker.DescribeRobotApplicationOutput, error)
	DescribeRobotApplicationWithContext(aws.Context, *robomaker.DescribeRobotApplicationInput, ...request.Option) (*robomaker.DescribeRobotApplicationOutput, error)
	DescribeRobotApplicationRequest(*robomaker.DescribeRobotApplicationInput) (*request.Request, *robomaker.DescribeRobotApplicationOutput)

	DescribeSimulationApplication(*robomaker.DescribeSimulationApplicationInput) (*robomaker.DescribeSimulationApplicationOutput, error)
	DescribeSimulationApplicationWithContext(aws.Context, *robomaker.DescribeSimulationApplicationInput, ...request.Option) (*robomaker.DescribeSimulationApplicationOutput, error)
	DescribeSimulationApplicationRequest(*robomaker.DescribeSimulationApplicationInput) (*request.Request, *robomaker.DescribeSimulationApplicationOutput)

	DescribeSimulationJob(*robomaker.DescribeSimulationJobInput) (*robomaker.DescribeSimulationJobOutput, error)
	DescribeSimulationJobWithContext(aws.Context, *robomaker.DescribeSimulationJobInput, ...request.Option) (*robomaker.DescribeSimulationJobOutput, error)
	DescribeSimulationJobRequest(*robomaker.DescribeSimulationJobInput) (*request.Request, *robomaker.DescribeSimulationJobOutput)

	DescribeSimulationJobBatch(*robomaker.DescribeSimulationJobBatchInput) (*robomaker.DescribeSimulationJobBatchOutput, error)
	DescribeSimulationJobBatchWithContext(aws.Context, *robomaker.DescribeSimulationJobBatchInput, ...request.Option) (*robomaker.DescribeSimulationJobBatchOutput, error)
	DescribeSimulationJobBatchRequest(*robomaker.DescribeSimulationJobBatchInput) (*request.Request, *robomaker.DescribeSimulationJobBatchOutput)

	DescribeWorld(*robomaker.DescribeWorldInput) (*robomaker.DescribeWorldOutput, error)
	DescribeWorldWithContext(aws.Context, *robomaker.DescribeWorldInput, ...request.Option) (*robomaker.DescribeWorldOutput, error)
	DescribeWorldRequest(*robomaker.DescribeWorldInput) (*request.Request, *robomaker.DescribeWorldOutput)

	DescribeWorldExportJob(*robomaker.DescribeWorldExportJobInput) (*robomaker.DescribeWorldExportJobOutput, error)
	DescribeWorldExportJobWithContext(aws.Context, *robomaker.DescribeWorldExportJobInput, ...request.Option) (*robomaker.DescribeWorldExportJobOutput, error)
	DescribeWorldExportJobRequest(*robomaker.DescribeWorldExportJobInput) (*request.Request, *robomaker.DescribeWorldExportJobOutput)

	DescribeWorldGenerationJob(*robomaker.DescribeWorldGenerationJobInput) (*robomaker.DescribeWorldGenerationJobOutput, error)
	DescribeWorldGenerationJobWithContext(aws.Context, *robomaker.DescribeWorldGenerationJobInput, ...request.Option) (*robomaker.DescribeWorldGenerationJobOutput, error)
	DescribeWorldGenerationJobRequest(*robomaker.DescribeWorldGenerationJobInput) (*request.Request, *robomaker.DescribeWorldGenerationJobOutput)

	DescribeWorldTemplate(*robomaker.DescribeWorldTemplateInput) (*robomaker.DescribeWorldTemplateOutput, error)
	DescribeWorldTemplateWithContext(aws.Context, *robomaker.DescribeWorldTemplateInput, ...request.Option) (*robomaker.DescribeWorldTemplateOutput, error)
	DescribeWorldTemplateRequest(*robomaker.DescribeWorldTemplateInput) (*request.Request, *robomaker.DescribeWorldTemplateOutput)

	GetWorldTemplateBody(*robomaker.GetWorldTemplateBodyInput) (*robomaker.GetWorldTemplateBodyOutput, error)
	GetWorldTemplateBodyWithContext(aws.Context, *robomaker.GetWorldTemplateBodyInput, ...request.Option) (*robomaker.GetWorldTemplateBodyOutput, error)
	GetWorldTemplateBodyRequest(*robomaker.GetWorldTemplateBodyInput) (*request.Request, *robomaker.GetWorldTemplateBodyOutput)

	ListDeploymentJobs(*robomaker.ListDeploymentJobsInput) (*robomaker.ListDeploymentJobsOutput, error)
	ListDeploymentJobsWithContext(aws.Context, *robomaker.ListDeploymentJobsInput, ...request.Option) (*robomaker.ListDeploymentJobsOutput, error)
	ListDeploymentJobsRequest(*robomaker.ListDeploymentJobsInput) (*request.Request, *robomaker.ListDeploymentJobsOutput)

	ListDeploymentJobsPages(*robomaker.ListDeploymentJobsInput, func(*robomaker.ListDeploymentJobsOutput, bool) bool) error
	ListDeploymentJobsPagesWithContext(aws.Context, *robomaker.ListDeploymentJobsInput, func(*robomaker.ListDeploymentJobsOutput, bool) bool, ...request.Option) error

	ListFleets(*robomaker.ListFleetsInput) (*robomaker.ListFleetsOutput, error)
	ListFleetsWithContext(aws.Context, *robomaker.ListFleetsInput, ...request.Option) (*robomaker.ListFleetsOutput, error)
	ListFleetsRequest(*robomaker.ListFleetsInput) (*request.Request, *robomaker.ListFleetsOutput)

	ListFleetsPages(*robomaker.ListFleetsInput, func(*robomaker.ListFleetsOutput, bool) bool) error
	ListFleetsPagesWithContext(aws.Context, *robomaker.ListFleetsInput, func(*robomaker.ListFleetsOutput, bool) bool, ...request.Option) error

	ListRobotApplications(*robomaker.ListRobotApplicationsInput) (*robomaker.ListRobotApplicationsOutput, error)
	ListRobotApplicationsWithContext(aws.Context, *robomaker.ListRobotApplicationsInput, ...request.Option) (*robomaker.ListRobotApplicationsOutput, error)
	ListRobotApplicationsRequest(*robomaker.ListRobotApplicationsInput) (*request.Request, *robomaker.ListRobotApplicationsOutput)

	ListRobotApplicationsPages(*robomaker.ListRobotApplicationsInput, func(*robomaker.ListRobotApplicationsOutput, bool) bool) error
	ListRobotApplicationsPagesWithContext(aws.Context, *robomaker.ListRobotApplicationsInput, func(*robomaker.ListRobotApplicationsOutput, bool) bool, ...request.Option) error

	ListRobots(*robomaker.ListRobotsInput) (*robomaker.ListRobotsOutput, error)
	ListRobotsWithContext(aws.Context, *robomaker.ListRobotsInput, ...request.Option) (*robomaker.ListRobotsOutput, error)
	ListRobotsRequest(*robomaker.ListRobotsInput) (*request.Request, *robomaker.ListRobotsOutput)

	ListRobotsPages(*robomaker.ListRobotsInput, func(*robomaker.ListRobotsOutput, bool) bool) error
	ListRobotsPagesWithContext(aws.Context, *robomaker.ListRobotsInput, func(*robomaker.ListRobotsOutput, bool) bool, ...request.Option) error

	ListSimulationApplications(*robomaker.ListSimulationApplicationsInput) (*robomaker.ListSimulationApplicationsOutput, error)
	ListSimulationApplicationsWithContext(aws.Context, *robomaker.ListSimulationApplicationsInput, ...request.Option) (*robomaker.ListSimulationApplicationsOutput, error)
	ListSimulationApplicationsRequest(*robomaker.ListSimulationApplicationsInput) (*request.Request, *robomaker.ListSimulationApplicationsOutput)

	ListSimulationApplicationsPages(*robomaker.ListSimulationApplicationsInput, func(*robomaker.ListSimulationApplicationsOutput, bool) bool) error
	ListSimulationApplicationsPagesWithContext(aws.Context, *robomaker.ListSimulationApplicationsInput, func(*robomaker.ListSimulationApplicationsOutput, bool) bool, ...request.Option) error

	ListSimulationJobBatches(*robomaker.ListSimulationJobBatchesInput) (*robomaker.ListSimulationJobBatchesOutput, error)
	ListSimulationJobBatchesWithContext(aws.Context, *robomaker.ListSimulationJobBatchesInput, ...request.Option) (*robomaker.ListSimulationJobBatchesOutput, error)
	ListSimulationJobBatchesRequest(*robomaker.ListSimulationJobBatchesInput) (*request.Request, *robomaker.ListSimulationJobBatchesOutput)

	ListSimulationJobBatchesPages(*robomaker.ListSimulationJobBatchesInput, func(*robomaker.ListSimulationJobBatchesOutput, bool) bool) error
	ListSimulationJobBatchesPagesWithContext(aws.Context, *robomaker.ListSimulationJobBatchesInput, func(*robomaker.ListSimulationJobBatchesOutput, bool) bool, ...request.Option) error

	ListSimulationJobs(*robomaker.ListSimulationJobsInput) (*robomaker.ListSimulationJobsOutput, error)
	ListSimulationJobsWithContext(aws.Context, *robomaker.ListSimulationJobsInput, ...request.Option) (*robomaker.ListSimulationJobsOutput, error)
	ListSimulationJobsRequest(*robomaker.ListSimulationJobsInput) (*request.Request, *robomaker.ListSimulationJobsOutput)

	ListSimulationJobsPages(*robomaker.ListSimulationJobsInput, func(*robomaker.ListSimulationJobsOutput, bool) bool) error
	ListSimulationJobsPagesWithContext(aws.Context, *robomaker.ListSimulationJobsInput, func(*robomaker.ListSimulationJobsOutput, bool) bool, ...request.Option) error

	ListTagsForResource(*robomaker.ListTagsForResourceInput) (*robomaker.ListTagsForResourceOutput, error)
	ListTagsForResourceWithContext(aws.Context, *robomaker.ListTagsForResourceInput, ...request.Option) (*robomaker.ListTagsForResourceOutput, error)
	ListTagsForResourceRequest(*robomaker.ListTagsForResourceInput) (*request.Request, *robomaker.ListTagsForResourceOutput)

	ListWorldExportJobs(*robomaker.ListWorldExportJobsInput) (*robomaker.ListWorldExportJobsOutput, error)
	ListWorldExportJobsWithContext(aws.Context, *robomaker.ListWorldExportJobsInput, ...request.Option) (*robomaker.ListWorldExportJobsOutput, error)
	ListWorldExportJobsRequest(*robomaker.ListWorldExportJobsInput) (*request.Request, *robomaker.ListWorldExportJobsOutput)

	ListWorldExportJobsPages(*robomaker.ListWorldExportJobsInput, func(*robomaker.ListWorldExportJobsOutput, bool) bool) error
	ListWorldExportJobsPagesWithContext(aws.Context, *robomaker.ListWorldExportJobsInput, func(*robomaker.ListWorldExportJobsOutput, bool) bool, ...request.Option) error

	ListWorldGenerationJobs(*robomaker.ListWorldGenerationJobsInput) (*robomaker.ListWorldGenerationJobsOutput, error)
	ListWorldGenerationJobsWithContext(aws.Context, *robomaker.ListWorldGenerationJobsInput, ...request.Option) (*robomaker.ListWorldGenerationJobsOutput, error)
	ListWorldGenerationJobsRequest(*robomaker.ListWorldGenerationJobsInput) (*request.Request, *robomaker.ListWorldGenerationJobsOutput)

	ListWorldGenerationJobsPages(*robomaker.ListWorldGenerationJobsInput, func(*robomaker.ListWorldGenerationJobsOutput, bool) bool) error
	ListWorldGenerationJobsPagesWithContext(aws.Context, *robomaker.ListWorldGenerationJobsInput, func(*robomaker.ListWorldGenerationJobsOutput, bool) bool, ...request.Option) error

	ListWorldTemplates(*robomaker.ListWorldTemplatesInput) (*robomaker.ListWorldTemplatesOutput, error)
	ListWorldTemplatesWithContext(aws.Context, *robomaker.ListWorldTemplatesInput, ...request.Option) (*robomaker.ListWorldTemplatesOutput, error)
	ListWorldTemplatesRequest(*robomaker.ListWorldTemplatesInput) (*request.Request, *robomaker.ListWorldTemplatesOutput)

	ListWorldTemplatesPages(*robomaker.ListWorldTemplatesInput, func(*robomaker.ListWorldTemplatesOutput, bool) bool) error
	ListWorldTemplatesPagesWithContext(aws.Context, *robomaker.ListWorldTemplatesInput, func(*robomaker.ListWorldTemplatesOutput, bool) bool, ...request.Option) error

	ListWorlds(*robomaker.ListWorldsInput) (*robomaker.ListWorldsOutput, error)
	ListWorldsWithContext(aws.Context, *robomaker.ListWorldsInput, ...request.Option) (*robomaker.ListWorldsOutput, error)
	ListWorldsRequest(*robomaker.ListWorldsInput) (*request.Request, *robomaker.ListWorldsOutput)

	ListWorldsPages(*robomaker.ListWorldsInput, func(*robomaker.ListWorldsOutput, bool) bool) error
	ListWorldsPagesWithContext(aws.Context, *robomaker.ListWorldsInput, func(*robomaker.ListWorldsOutput, bool) bool, ...request.Option) error

	RegisterRobot(*robomaker.RegisterRobotInput) (*robomaker.RegisterRobotOutput, error)
	RegisterRobotWithContext(aws.Context, *robomaker.RegisterRobotInput, ...request.Option) (*robomaker.RegisterRobotOutput, error)
	RegisterRobotRequest(*robomaker.RegisterRobotInput) (*request.Request, *robomaker.RegisterRobotOutput)

	RestartSimulationJob(*robomaker.RestartSimulationJobInput) (*robomaker.RestartSimulationJobOutput, error)
	RestartSimulationJobWithContext(aws.Context, *robomaker.RestartSimulationJobInput, ...request.Option) (*robomaker.RestartSimulationJobOutput, error)
	RestartSimulationJobRequest(*robomaker.RestartSimulationJobInput) (*request.Request, *robomaker.RestartSimulationJobOutput)

	StartSimulationJobBatch(*robomaker.StartSimulationJobBatchInput) (*robomaker.StartSimulationJobBatchOutput, error)
	StartSimulationJobBatchWithContext(aws.Context, *robomaker.StartSimulationJobBatchInput, ...request.Option) (*robomaker.StartSimulationJobBatchOutput, error)
	StartSimulationJobBatchRequest(*robomaker.StartSimulationJobBatchInput) (*request.Request, *robomaker.StartSimulationJobBatchOutput)

	SyncDeploymentJob(*robomaker.SyncDeploymentJobInput) (*robomaker.SyncDeploymentJobOutput, error)
	SyncDeploymentJobWithContext(aws.Context, *robomaker.SyncDeploymentJobInput, ...request.Option) (*robomaker.SyncDeploymentJobOutput, error)
	SyncDeploymentJobRequest(*robomaker.SyncDeploymentJobInput) (*request.Request, *robomaker.SyncDeploymentJobOutput)

	TagResource(*robomaker.TagResourceInput) (*robomaker.TagResourceOutput, error)
	TagResourceWithContext(aws.Context, *robomaker.TagResourceInput, ...request.Option) (*robomaker.TagResourceOutput, error)
	TagResourceRequest(*robomaker.TagResourceInput) (*request.Request, *robomaker.TagResourceOutput)

	UntagResource(*robomaker.UntagResourceInput) (*robomaker.UntagResourceOutput, error)
	UntagResourceWithContext(aws.Context, *robomaker.UntagResourceInput, ...request.Option) (*robomaker.UntagResourceOutput, error)
	UntagResourceRequest(*robomaker.UntagResourceInput) (*request.Request, *robomaker.UntagResourceOutput)

	UpdateRobotApplication(*robomaker.UpdateRobotApplicationInput) (*robomaker.UpdateRobotApplicationOutput, error)
	UpdateRobotApplicationWithContext(aws.Context, *robomaker.UpdateRobotApplicationInput, ...request.Option) (*robomaker.UpdateRobotApplicationOutput, error)
	UpdateRobotApplicationRequest(*robomaker.UpdateRobotApplicationInput) (*request.Request, *robomaker.UpdateRobotApplicationOutput)

	UpdateSimulationApplication(*robomaker.UpdateSimulationApplicationInput) (*robomaker.UpdateSimulationApplicationOutput, error)
	UpdateSimulationApplicationWithContext(aws.Context, *robomaker.UpdateSimulationApplicationInput, ...request.Option) (*robomaker.UpdateSimulationApplicationOutput, error)
	UpdateSimulationApplicationRequest(*robomaker.UpdateSimulationApplicationInput) (*request.Request, *robomaker.UpdateSimulationApplicationOutput)

	UpdateWorldTemplate(*robomaker.UpdateWorldTemplateInput) (*robomaker.UpdateWorldTemplateOutput, error)
	UpdateWorldTemplateWithContext(aws.Context, *robomaker.UpdateWorldTemplateInput, ...request.Option) (*robomaker.UpdateWorldTemplateOutput, error)
	UpdateWorldTemplateRequest(*robomaker.UpdateWorldTemplateInput) (*request.Request, *robomaker.UpdateWorldTemplateOutput)
}

var _ RoboMakerAPI = (*robomaker.RoboMaker)(nil)
