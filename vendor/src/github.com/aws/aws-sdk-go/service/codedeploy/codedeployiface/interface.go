// THIS FILE IS AUTOMATICALLY GENERATED. DO NOT EDIT.

// Package codedeployiface provides an interface to enable mocking the AWS CodeDeploy service client
// for testing your code.
//
// It is important to note that this interface will have breaking changes
// when the service model is updated and adds new API operations, paginators,
// and waiters.
package codedeployiface

import (
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/codedeploy"
)

// CodeDeployAPI provides an interface to enable mocking the
// codedeploy.CodeDeploy service client's API operation,
// paginators, and waiters. This make unit testing your code that calls out
// to the SDK's service client's calls easier.
//
// The best way to use this interface is so the SDK's service client's calls
// can be stubbed out for unit testing your code with the SDK without needing
// to inject custom request handlers into the the SDK's request pipeline.
//
//    // myFunc uses an SDK service client to make a request to
//    // AWS CodeDeploy.
//    func myFunc(svc codedeployiface.CodeDeployAPI) bool {
//        // Make svc.AddTagsToOnPremisesInstances request
//    }
//
//    func main() {
//        sess := session.New()
//        svc := codedeploy.New(sess)
//
//        myFunc(svc)
//    }
//
// In your _test.go file:
//
//    // Define a mock struct to be used in your unit tests of myFunc.
//    type mockCodeDeployClient struct {
//        codedeployiface.CodeDeployAPI
//    }
//    func (m *mockCodeDeployClient) AddTagsToOnPremisesInstances(input *codedeploy.AddTagsToOnPremisesInstancesInput) (*codedeploy.AddTagsToOnPremisesInstancesOutput, error) {
//        // mock response/functionality
//    }
//
//    TestMyFunc(t *testing.T) {
//        // Setup Test
//        mockSvc := &mockCodeDeployClient{}
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
type CodeDeployAPI interface {
	AddTagsToOnPremisesInstancesRequest(*codedeploy.AddTagsToOnPremisesInstancesInput) (*request.Request, *codedeploy.AddTagsToOnPremisesInstancesOutput)

	AddTagsToOnPremisesInstances(*codedeploy.AddTagsToOnPremisesInstancesInput) (*codedeploy.AddTagsToOnPremisesInstancesOutput, error)

	BatchGetApplicationRevisionsRequest(*codedeploy.BatchGetApplicationRevisionsInput) (*request.Request, *codedeploy.BatchGetApplicationRevisionsOutput)

	BatchGetApplicationRevisions(*codedeploy.BatchGetApplicationRevisionsInput) (*codedeploy.BatchGetApplicationRevisionsOutput, error)

	BatchGetApplicationsRequest(*codedeploy.BatchGetApplicationsInput) (*request.Request, *codedeploy.BatchGetApplicationsOutput)

	BatchGetApplications(*codedeploy.BatchGetApplicationsInput) (*codedeploy.BatchGetApplicationsOutput, error)

	BatchGetDeploymentGroupsRequest(*codedeploy.BatchGetDeploymentGroupsInput) (*request.Request, *codedeploy.BatchGetDeploymentGroupsOutput)

	BatchGetDeploymentGroups(*codedeploy.BatchGetDeploymentGroupsInput) (*codedeploy.BatchGetDeploymentGroupsOutput, error)

	BatchGetDeploymentInstancesRequest(*codedeploy.BatchGetDeploymentInstancesInput) (*request.Request, *codedeploy.BatchGetDeploymentInstancesOutput)

	BatchGetDeploymentInstances(*codedeploy.BatchGetDeploymentInstancesInput) (*codedeploy.BatchGetDeploymentInstancesOutput, error)

	BatchGetDeploymentsRequest(*codedeploy.BatchGetDeploymentsInput) (*request.Request, *codedeploy.BatchGetDeploymentsOutput)

	BatchGetDeployments(*codedeploy.BatchGetDeploymentsInput) (*codedeploy.BatchGetDeploymentsOutput, error)

	BatchGetOnPremisesInstancesRequest(*codedeploy.BatchGetOnPremisesInstancesInput) (*request.Request, *codedeploy.BatchGetOnPremisesInstancesOutput)

	BatchGetOnPremisesInstances(*codedeploy.BatchGetOnPremisesInstancesInput) (*codedeploy.BatchGetOnPremisesInstancesOutput, error)

	CreateApplicationRequest(*codedeploy.CreateApplicationInput) (*request.Request, *codedeploy.CreateApplicationOutput)

	CreateApplication(*codedeploy.CreateApplicationInput) (*codedeploy.CreateApplicationOutput, error)

	CreateDeploymentRequest(*codedeploy.CreateDeploymentInput) (*request.Request, *codedeploy.CreateDeploymentOutput)

	CreateDeployment(*codedeploy.CreateDeploymentInput) (*codedeploy.CreateDeploymentOutput, error)

	CreateDeploymentConfigRequest(*codedeploy.CreateDeploymentConfigInput) (*request.Request, *codedeploy.CreateDeploymentConfigOutput)

	CreateDeploymentConfig(*codedeploy.CreateDeploymentConfigInput) (*codedeploy.CreateDeploymentConfigOutput, error)

	CreateDeploymentGroupRequest(*codedeploy.CreateDeploymentGroupInput) (*request.Request, *codedeploy.CreateDeploymentGroupOutput)

	CreateDeploymentGroup(*codedeploy.CreateDeploymentGroupInput) (*codedeploy.CreateDeploymentGroupOutput, error)

	DeleteApplicationRequest(*codedeploy.DeleteApplicationInput) (*request.Request, *codedeploy.DeleteApplicationOutput)

	DeleteApplication(*codedeploy.DeleteApplicationInput) (*codedeploy.DeleteApplicationOutput, error)

	DeleteDeploymentConfigRequest(*codedeploy.DeleteDeploymentConfigInput) (*request.Request, *codedeploy.DeleteDeploymentConfigOutput)

	DeleteDeploymentConfig(*codedeploy.DeleteDeploymentConfigInput) (*codedeploy.DeleteDeploymentConfigOutput, error)

	DeleteDeploymentGroupRequest(*codedeploy.DeleteDeploymentGroupInput) (*request.Request, *codedeploy.DeleteDeploymentGroupOutput)

	DeleteDeploymentGroup(*codedeploy.DeleteDeploymentGroupInput) (*codedeploy.DeleteDeploymentGroupOutput, error)

	DeregisterOnPremisesInstanceRequest(*codedeploy.DeregisterOnPremisesInstanceInput) (*request.Request, *codedeploy.DeregisterOnPremisesInstanceOutput)

	DeregisterOnPremisesInstance(*codedeploy.DeregisterOnPremisesInstanceInput) (*codedeploy.DeregisterOnPremisesInstanceOutput, error)

	GetApplicationRequest(*codedeploy.GetApplicationInput) (*request.Request, *codedeploy.GetApplicationOutput)

	GetApplication(*codedeploy.GetApplicationInput) (*codedeploy.GetApplicationOutput, error)

	GetApplicationRevisionRequest(*codedeploy.GetApplicationRevisionInput) (*request.Request, *codedeploy.GetApplicationRevisionOutput)

	GetApplicationRevision(*codedeploy.GetApplicationRevisionInput) (*codedeploy.GetApplicationRevisionOutput, error)

	GetDeploymentRequest(*codedeploy.GetDeploymentInput) (*request.Request, *codedeploy.GetDeploymentOutput)

	GetDeployment(*codedeploy.GetDeploymentInput) (*codedeploy.GetDeploymentOutput, error)

	GetDeploymentConfigRequest(*codedeploy.GetDeploymentConfigInput) (*request.Request, *codedeploy.GetDeploymentConfigOutput)

	GetDeploymentConfig(*codedeploy.GetDeploymentConfigInput) (*codedeploy.GetDeploymentConfigOutput, error)

	GetDeploymentGroupRequest(*codedeploy.GetDeploymentGroupInput) (*request.Request, *codedeploy.GetDeploymentGroupOutput)

	GetDeploymentGroup(*codedeploy.GetDeploymentGroupInput) (*codedeploy.GetDeploymentGroupOutput, error)

	GetDeploymentInstanceRequest(*codedeploy.GetDeploymentInstanceInput) (*request.Request, *codedeploy.GetDeploymentInstanceOutput)

	GetDeploymentInstance(*codedeploy.GetDeploymentInstanceInput) (*codedeploy.GetDeploymentInstanceOutput, error)

	GetOnPremisesInstanceRequest(*codedeploy.GetOnPremisesInstanceInput) (*request.Request, *codedeploy.GetOnPremisesInstanceOutput)

	GetOnPremisesInstance(*codedeploy.GetOnPremisesInstanceInput) (*codedeploy.GetOnPremisesInstanceOutput, error)

	ListApplicationRevisionsRequest(*codedeploy.ListApplicationRevisionsInput) (*request.Request, *codedeploy.ListApplicationRevisionsOutput)

	ListApplicationRevisions(*codedeploy.ListApplicationRevisionsInput) (*codedeploy.ListApplicationRevisionsOutput, error)

	ListApplicationRevisionsPages(*codedeploy.ListApplicationRevisionsInput, func(*codedeploy.ListApplicationRevisionsOutput, bool) bool) error

	ListApplicationsRequest(*codedeploy.ListApplicationsInput) (*request.Request, *codedeploy.ListApplicationsOutput)

	ListApplications(*codedeploy.ListApplicationsInput) (*codedeploy.ListApplicationsOutput, error)

	ListApplicationsPages(*codedeploy.ListApplicationsInput, func(*codedeploy.ListApplicationsOutput, bool) bool) error

	ListDeploymentConfigsRequest(*codedeploy.ListDeploymentConfigsInput) (*request.Request, *codedeploy.ListDeploymentConfigsOutput)

	ListDeploymentConfigs(*codedeploy.ListDeploymentConfigsInput) (*codedeploy.ListDeploymentConfigsOutput, error)

	ListDeploymentConfigsPages(*codedeploy.ListDeploymentConfigsInput, func(*codedeploy.ListDeploymentConfigsOutput, bool) bool) error

	ListDeploymentGroupsRequest(*codedeploy.ListDeploymentGroupsInput) (*request.Request, *codedeploy.ListDeploymentGroupsOutput)

	ListDeploymentGroups(*codedeploy.ListDeploymentGroupsInput) (*codedeploy.ListDeploymentGroupsOutput, error)

	ListDeploymentGroupsPages(*codedeploy.ListDeploymentGroupsInput, func(*codedeploy.ListDeploymentGroupsOutput, bool) bool) error

	ListDeploymentInstancesRequest(*codedeploy.ListDeploymentInstancesInput) (*request.Request, *codedeploy.ListDeploymentInstancesOutput)

	ListDeploymentInstances(*codedeploy.ListDeploymentInstancesInput) (*codedeploy.ListDeploymentInstancesOutput, error)

	ListDeploymentInstancesPages(*codedeploy.ListDeploymentInstancesInput, func(*codedeploy.ListDeploymentInstancesOutput, bool) bool) error

	ListDeploymentsRequest(*codedeploy.ListDeploymentsInput) (*request.Request, *codedeploy.ListDeploymentsOutput)

	ListDeployments(*codedeploy.ListDeploymentsInput) (*codedeploy.ListDeploymentsOutput, error)

	ListDeploymentsPages(*codedeploy.ListDeploymentsInput, func(*codedeploy.ListDeploymentsOutput, bool) bool) error

	ListOnPremisesInstancesRequest(*codedeploy.ListOnPremisesInstancesInput) (*request.Request, *codedeploy.ListOnPremisesInstancesOutput)

	ListOnPremisesInstances(*codedeploy.ListOnPremisesInstancesInput) (*codedeploy.ListOnPremisesInstancesOutput, error)

	RegisterApplicationRevisionRequest(*codedeploy.RegisterApplicationRevisionInput) (*request.Request, *codedeploy.RegisterApplicationRevisionOutput)

	RegisterApplicationRevision(*codedeploy.RegisterApplicationRevisionInput) (*codedeploy.RegisterApplicationRevisionOutput, error)

	RegisterOnPremisesInstanceRequest(*codedeploy.RegisterOnPremisesInstanceInput) (*request.Request, *codedeploy.RegisterOnPremisesInstanceOutput)

	RegisterOnPremisesInstance(*codedeploy.RegisterOnPremisesInstanceInput) (*codedeploy.RegisterOnPremisesInstanceOutput, error)

	RemoveTagsFromOnPremisesInstancesRequest(*codedeploy.RemoveTagsFromOnPremisesInstancesInput) (*request.Request, *codedeploy.RemoveTagsFromOnPremisesInstancesOutput)

	RemoveTagsFromOnPremisesInstances(*codedeploy.RemoveTagsFromOnPremisesInstancesInput) (*codedeploy.RemoveTagsFromOnPremisesInstancesOutput, error)

	StopDeploymentRequest(*codedeploy.StopDeploymentInput) (*request.Request, *codedeploy.StopDeploymentOutput)

	StopDeployment(*codedeploy.StopDeploymentInput) (*codedeploy.StopDeploymentOutput, error)

	UpdateApplicationRequest(*codedeploy.UpdateApplicationInput) (*request.Request, *codedeploy.UpdateApplicationOutput)

	UpdateApplication(*codedeploy.UpdateApplicationInput) (*codedeploy.UpdateApplicationOutput, error)

	UpdateDeploymentGroupRequest(*codedeploy.UpdateDeploymentGroupInput) (*request.Request, *codedeploy.UpdateDeploymentGroupOutput)

	UpdateDeploymentGroup(*codedeploy.UpdateDeploymentGroupInput) (*codedeploy.UpdateDeploymentGroupOutput, error)

	WaitUntilDeploymentSuccessful(*codedeploy.GetDeploymentInput) error
}

var _ CodeDeployAPI = (*codedeploy.CodeDeploy)(nil)
