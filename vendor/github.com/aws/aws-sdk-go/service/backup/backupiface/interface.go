// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

// Package backupiface provides an interface to enable mocking the AWS Backup service client
// for testing your code.
//
// It is important to note that this interface will have breaking changes
// when the service model is updated and adds new API operations, paginators,
// and waiters.
package backupiface

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/backup"
)

// BackupAPI provides an interface to enable mocking the
// backup.Backup service client's API operation,
// paginators, and waiters. This make unit testing your code that calls out
// to the SDK's service client's calls easier.
//
// The best way to use this interface is so the SDK's service client's calls
// can be stubbed out for unit testing your code with the SDK without needing
// to inject custom request handlers into the SDK's request pipeline.
//
//    // myFunc uses an SDK service client to make a request to
//    // AWS Backup.
//    func myFunc(svc backupiface.BackupAPI) bool {
//        // Make svc.CreateBackupPlan request
//    }
//
//    func main() {
//        sess := session.New()
//        svc := backup.New(sess)
//
//        myFunc(svc)
//    }
//
// In your _test.go file:
//
//    // Define a mock struct to be used in your unit tests of myFunc.
//    type mockBackupClient struct {
//        backupiface.BackupAPI
//    }
//    func (m *mockBackupClient) CreateBackupPlan(input *backup.CreateBackupPlanInput) (*backup.CreateBackupPlanOutput, error) {
//        // mock response/functionality
//    }
//
//    func TestMyFunc(t *testing.T) {
//        // Setup Test
//        mockSvc := &mockBackupClient{}
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
type BackupAPI interface {
	CreateBackupPlan(*backup.CreateBackupPlanInput) (*backup.CreateBackupPlanOutput, error)
	CreateBackupPlanWithContext(aws.Context, *backup.CreateBackupPlanInput, ...request.Option) (*backup.CreateBackupPlanOutput, error)
	CreateBackupPlanRequest(*backup.CreateBackupPlanInput) (*request.Request, *backup.CreateBackupPlanOutput)

	CreateBackupSelection(*backup.CreateBackupSelectionInput) (*backup.CreateBackupSelectionOutput, error)
	CreateBackupSelectionWithContext(aws.Context, *backup.CreateBackupSelectionInput, ...request.Option) (*backup.CreateBackupSelectionOutput, error)
	CreateBackupSelectionRequest(*backup.CreateBackupSelectionInput) (*request.Request, *backup.CreateBackupSelectionOutput)

	CreateBackupVault(*backup.CreateBackupVaultInput) (*backup.CreateBackupVaultOutput, error)
	CreateBackupVaultWithContext(aws.Context, *backup.CreateBackupVaultInput, ...request.Option) (*backup.CreateBackupVaultOutput, error)
	CreateBackupVaultRequest(*backup.CreateBackupVaultInput) (*request.Request, *backup.CreateBackupVaultOutput)

	DeleteBackupPlan(*backup.DeleteBackupPlanInput) (*backup.DeleteBackupPlanOutput, error)
	DeleteBackupPlanWithContext(aws.Context, *backup.DeleteBackupPlanInput, ...request.Option) (*backup.DeleteBackupPlanOutput, error)
	DeleteBackupPlanRequest(*backup.DeleteBackupPlanInput) (*request.Request, *backup.DeleteBackupPlanOutput)

	DeleteBackupSelection(*backup.DeleteBackupSelectionInput) (*backup.DeleteBackupSelectionOutput, error)
	DeleteBackupSelectionWithContext(aws.Context, *backup.DeleteBackupSelectionInput, ...request.Option) (*backup.DeleteBackupSelectionOutput, error)
	DeleteBackupSelectionRequest(*backup.DeleteBackupSelectionInput) (*request.Request, *backup.DeleteBackupSelectionOutput)

	DeleteBackupVault(*backup.DeleteBackupVaultInput) (*backup.DeleteBackupVaultOutput, error)
	DeleteBackupVaultWithContext(aws.Context, *backup.DeleteBackupVaultInput, ...request.Option) (*backup.DeleteBackupVaultOutput, error)
	DeleteBackupVaultRequest(*backup.DeleteBackupVaultInput) (*request.Request, *backup.DeleteBackupVaultOutput)

	DeleteBackupVaultAccessPolicy(*backup.DeleteBackupVaultAccessPolicyInput) (*backup.DeleteBackupVaultAccessPolicyOutput, error)
	DeleteBackupVaultAccessPolicyWithContext(aws.Context, *backup.DeleteBackupVaultAccessPolicyInput, ...request.Option) (*backup.DeleteBackupVaultAccessPolicyOutput, error)
	DeleteBackupVaultAccessPolicyRequest(*backup.DeleteBackupVaultAccessPolicyInput) (*request.Request, *backup.DeleteBackupVaultAccessPolicyOutput)

	DeleteBackupVaultNotifications(*backup.DeleteBackupVaultNotificationsInput) (*backup.DeleteBackupVaultNotificationsOutput, error)
	DeleteBackupVaultNotificationsWithContext(aws.Context, *backup.DeleteBackupVaultNotificationsInput, ...request.Option) (*backup.DeleteBackupVaultNotificationsOutput, error)
	DeleteBackupVaultNotificationsRequest(*backup.DeleteBackupVaultNotificationsInput) (*request.Request, *backup.DeleteBackupVaultNotificationsOutput)

	DeleteRecoveryPoint(*backup.DeleteRecoveryPointInput) (*backup.DeleteRecoveryPointOutput, error)
	DeleteRecoveryPointWithContext(aws.Context, *backup.DeleteRecoveryPointInput, ...request.Option) (*backup.DeleteRecoveryPointOutput, error)
	DeleteRecoveryPointRequest(*backup.DeleteRecoveryPointInput) (*request.Request, *backup.DeleteRecoveryPointOutput)

	DescribeBackupJob(*backup.DescribeBackupJobInput) (*backup.DescribeBackupJobOutput, error)
	DescribeBackupJobWithContext(aws.Context, *backup.DescribeBackupJobInput, ...request.Option) (*backup.DescribeBackupJobOutput, error)
	DescribeBackupJobRequest(*backup.DescribeBackupJobInput) (*request.Request, *backup.DescribeBackupJobOutput)

	DescribeBackupVault(*backup.DescribeBackupVaultInput) (*backup.DescribeBackupVaultOutput, error)
	DescribeBackupVaultWithContext(aws.Context, *backup.DescribeBackupVaultInput, ...request.Option) (*backup.DescribeBackupVaultOutput, error)
	DescribeBackupVaultRequest(*backup.DescribeBackupVaultInput) (*request.Request, *backup.DescribeBackupVaultOutput)

	DescribeCopyJob(*backup.DescribeCopyJobInput) (*backup.DescribeCopyJobOutput, error)
	DescribeCopyJobWithContext(aws.Context, *backup.DescribeCopyJobInput, ...request.Option) (*backup.DescribeCopyJobOutput, error)
	DescribeCopyJobRequest(*backup.DescribeCopyJobInput) (*request.Request, *backup.DescribeCopyJobOutput)

	DescribeProtectedResource(*backup.DescribeProtectedResourceInput) (*backup.DescribeProtectedResourceOutput, error)
	DescribeProtectedResourceWithContext(aws.Context, *backup.DescribeProtectedResourceInput, ...request.Option) (*backup.DescribeProtectedResourceOutput, error)
	DescribeProtectedResourceRequest(*backup.DescribeProtectedResourceInput) (*request.Request, *backup.DescribeProtectedResourceOutput)

	DescribeRecoveryPoint(*backup.DescribeRecoveryPointInput) (*backup.DescribeRecoveryPointOutput, error)
	DescribeRecoveryPointWithContext(aws.Context, *backup.DescribeRecoveryPointInput, ...request.Option) (*backup.DescribeRecoveryPointOutput, error)
	DescribeRecoveryPointRequest(*backup.DescribeRecoveryPointInput) (*request.Request, *backup.DescribeRecoveryPointOutput)

	DescribeRestoreJob(*backup.DescribeRestoreJobInput) (*backup.DescribeRestoreJobOutput, error)
	DescribeRestoreJobWithContext(aws.Context, *backup.DescribeRestoreJobInput, ...request.Option) (*backup.DescribeRestoreJobOutput, error)
	DescribeRestoreJobRequest(*backup.DescribeRestoreJobInput) (*request.Request, *backup.DescribeRestoreJobOutput)

	ExportBackupPlanTemplate(*backup.ExportBackupPlanTemplateInput) (*backup.ExportBackupPlanTemplateOutput, error)
	ExportBackupPlanTemplateWithContext(aws.Context, *backup.ExportBackupPlanTemplateInput, ...request.Option) (*backup.ExportBackupPlanTemplateOutput, error)
	ExportBackupPlanTemplateRequest(*backup.ExportBackupPlanTemplateInput) (*request.Request, *backup.ExportBackupPlanTemplateOutput)

	GetBackupPlan(*backup.GetBackupPlanInput) (*backup.GetBackupPlanOutput, error)
	GetBackupPlanWithContext(aws.Context, *backup.GetBackupPlanInput, ...request.Option) (*backup.GetBackupPlanOutput, error)
	GetBackupPlanRequest(*backup.GetBackupPlanInput) (*request.Request, *backup.GetBackupPlanOutput)

	GetBackupPlanFromJSON(*backup.GetBackupPlanFromJSONInput) (*backup.GetBackupPlanFromJSONOutput, error)
	GetBackupPlanFromJSONWithContext(aws.Context, *backup.GetBackupPlanFromJSONInput, ...request.Option) (*backup.GetBackupPlanFromJSONOutput, error)
	GetBackupPlanFromJSONRequest(*backup.GetBackupPlanFromJSONInput) (*request.Request, *backup.GetBackupPlanFromJSONOutput)

	GetBackupPlanFromTemplate(*backup.GetBackupPlanFromTemplateInput) (*backup.GetBackupPlanFromTemplateOutput, error)
	GetBackupPlanFromTemplateWithContext(aws.Context, *backup.GetBackupPlanFromTemplateInput, ...request.Option) (*backup.GetBackupPlanFromTemplateOutput, error)
	GetBackupPlanFromTemplateRequest(*backup.GetBackupPlanFromTemplateInput) (*request.Request, *backup.GetBackupPlanFromTemplateOutput)

	GetBackupSelection(*backup.GetBackupSelectionInput) (*backup.GetBackupSelectionOutput, error)
	GetBackupSelectionWithContext(aws.Context, *backup.GetBackupSelectionInput, ...request.Option) (*backup.GetBackupSelectionOutput, error)
	GetBackupSelectionRequest(*backup.GetBackupSelectionInput) (*request.Request, *backup.GetBackupSelectionOutput)

	GetBackupVaultAccessPolicy(*backup.GetBackupVaultAccessPolicyInput) (*backup.GetBackupVaultAccessPolicyOutput, error)
	GetBackupVaultAccessPolicyWithContext(aws.Context, *backup.GetBackupVaultAccessPolicyInput, ...request.Option) (*backup.GetBackupVaultAccessPolicyOutput, error)
	GetBackupVaultAccessPolicyRequest(*backup.GetBackupVaultAccessPolicyInput) (*request.Request, *backup.GetBackupVaultAccessPolicyOutput)

	GetBackupVaultNotifications(*backup.GetBackupVaultNotificationsInput) (*backup.GetBackupVaultNotificationsOutput, error)
	GetBackupVaultNotificationsWithContext(aws.Context, *backup.GetBackupVaultNotificationsInput, ...request.Option) (*backup.GetBackupVaultNotificationsOutput, error)
	GetBackupVaultNotificationsRequest(*backup.GetBackupVaultNotificationsInput) (*request.Request, *backup.GetBackupVaultNotificationsOutput)

	GetRecoveryPointRestoreMetadata(*backup.GetRecoveryPointRestoreMetadataInput) (*backup.GetRecoveryPointRestoreMetadataOutput, error)
	GetRecoveryPointRestoreMetadataWithContext(aws.Context, *backup.GetRecoveryPointRestoreMetadataInput, ...request.Option) (*backup.GetRecoveryPointRestoreMetadataOutput, error)
	GetRecoveryPointRestoreMetadataRequest(*backup.GetRecoveryPointRestoreMetadataInput) (*request.Request, *backup.GetRecoveryPointRestoreMetadataOutput)

	GetSupportedResourceTypes(*backup.GetSupportedResourceTypesInput) (*backup.GetSupportedResourceTypesOutput, error)
	GetSupportedResourceTypesWithContext(aws.Context, *backup.GetSupportedResourceTypesInput, ...request.Option) (*backup.GetSupportedResourceTypesOutput, error)
	GetSupportedResourceTypesRequest(*backup.GetSupportedResourceTypesInput) (*request.Request, *backup.GetSupportedResourceTypesOutput)

	ListBackupJobs(*backup.ListBackupJobsInput) (*backup.ListBackupJobsOutput, error)
	ListBackupJobsWithContext(aws.Context, *backup.ListBackupJobsInput, ...request.Option) (*backup.ListBackupJobsOutput, error)
	ListBackupJobsRequest(*backup.ListBackupJobsInput) (*request.Request, *backup.ListBackupJobsOutput)

	ListBackupJobsPages(*backup.ListBackupJobsInput, func(*backup.ListBackupJobsOutput, bool) bool) error
	ListBackupJobsPagesWithContext(aws.Context, *backup.ListBackupJobsInput, func(*backup.ListBackupJobsOutput, bool) bool, ...request.Option) error

	ListBackupPlanTemplates(*backup.ListBackupPlanTemplatesInput) (*backup.ListBackupPlanTemplatesOutput, error)
	ListBackupPlanTemplatesWithContext(aws.Context, *backup.ListBackupPlanTemplatesInput, ...request.Option) (*backup.ListBackupPlanTemplatesOutput, error)
	ListBackupPlanTemplatesRequest(*backup.ListBackupPlanTemplatesInput) (*request.Request, *backup.ListBackupPlanTemplatesOutput)

	ListBackupPlanTemplatesPages(*backup.ListBackupPlanTemplatesInput, func(*backup.ListBackupPlanTemplatesOutput, bool) bool) error
	ListBackupPlanTemplatesPagesWithContext(aws.Context, *backup.ListBackupPlanTemplatesInput, func(*backup.ListBackupPlanTemplatesOutput, bool) bool, ...request.Option) error

	ListBackupPlanVersions(*backup.ListBackupPlanVersionsInput) (*backup.ListBackupPlanVersionsOutput, error)
	ListBackupPlanVersionsWithContext(aws.Context, *backup.ListBackupPlanVersionsInput, ...request.Option) (*backup.ListBackupPlanVersionsOutput, error)
	ListBackupPlanVersionsRequest(*backup.ListBackupPlanVersionsInput) (*request.Request, *backup.ListBackupPlanVersionsOutput)

	ListBackupPlanVersionsPages(*backup.ListBackupPlanVersionsInput, func(*backup.ListBackupPlanVersionsOutput, bool) bool) error
	ListBackupPlanVersionsPagesWithContext(aws.Context, *backup.ListBackupPlanVersionsInput, func(*backup.ListBackupPlanVersionsOutput, bool) bool, ...request.Option) error

	ListBackupPlans(*backup.ListBackupPlansInput) (*backup.ListBackupPlansOutput, error)
	ListBackupPlansWithContext(aws.Context, *backup.ListBackupPlansInput, ...request.Option) (*backup.ListBackupPlansOutput, error)
	ListBackupPlansRequest(*backup.ListBackupPlansInput) (*request.Request, *backup.ListBackupPlansOutput)

	ListBackupPlansPages(*backup.ListBackupPlansInput, func(*backup.ListBackupPlansOutput, bool) bool) error
	ListBackupPlansPagesWithContext(aws.Context, *backup.ListBackupPlansInput, func(*backup.ListBackupPlansOutput, bool) bool, ...request.Option) error

	ListBackupSelections(*backup.ListBackupSelectionsInput) (*backup.ListBackupSelectionsOutput, error)
	ListBackupSelectionsWithContext(aws.Context, *backup.ListBackupSelectionsInput, ...request.Option) (*backup.ListBackupSelectionsOutput, error)
	ListBackupSelectionsRequest(*backup.ListBackupSelectionsInput) (*request.Request, *backup.ListBackupSelectionsOutput)

	ListBackupSelectionsPages(*backup.ListBackupSelectionsInput, func(*backup.ListBackupSelectionsOutput, bool) bool) error
	ListBackupSelectionsPagesWithContext(aws.Context, *backup.ListBackupSelectionsInput, func(*backup.ListBackupSelectionsOutput, bool) bool, ...request.Option) error

	ListBackupVaults(*backup.ListBackupVaultsInput) (*backup.ListBackupVaultsOutput, error)
	ListBackupVaultsWithContext(aws.Context, *backup.ListBackupVaultsInput, ...request.Option) (*backup.ListBackupVaultsOutput, error)
	ListBackupVaultsRequest(*backup.ListBackupVaultsInput) (*request.Request, *backup.ListBackupVaultsOutput)

	ListBackupVaultsPages(*backup.ListBackupVaultsInput, func(*backup.ListBackupVaultsOutput, bool) bool) error
	ListBackupVaultsPagesWithContext(aws.Context, *backup.ListBackupVaultsInput, func(*backup.ListBackupVaultsOutput, bool) bool, ...request.Option) error

	ListCopyJobs(*backup.ListCopyJobsInput) (*backup.ListCopyJobsOutput, error)
	ListCopyJobsWithContext(aws.Context, *backup.ListCopyJobsInput, ...request.Option) (*backup.ListCopyJobsOutput, error)
	ListCopyJobsRequest(*backup.ListCopyJobsInput) (*request.Request, *backup.ListCopyJobsOutput)

	ListCopyJobsPages(*backup.ListCopyJobsInput, func(*backup.ListCopyJobsOutput, bool) bool) error
	ListCopyJobsPagesWithContext(aws.Context, *backup.ListCopyJobsInput, func(*backup.ListCopyJobsOutput, bool) bool, ...request.Option) error

	ListProtectedResources(*backup.ListProtectedResourcesInput) (*backup.ListProtectedResourcesOutput, error)
	ListProtectedResourcesWithContext(aws.Context, *backup.ListProtectedResourcesInput, ...request.Option) (*backup.ListProtectedResourcesOutput, error)
	ListProtectedResourcesRequest(*backup.ListProtectedResourcesInput) (*request.Request, *backup.ListProtectedResourcesOutput)

	ListProtectedResourcesPages(*backup.ListProtectedResourcesInput, func(*backup.ListProtectedResourcesOutput, bool) bool) error
	ListProtectedResourcesPagesWithContext(aws.Context, *backup.ListProtectedResourcesInput, func(*backup.ListProtectedResourcesOutput, bool) bool, ...request.Option) error

	ListRecoveryPointsByBackupVault(*backup.ListRecoveryPointsByBackupVaultInput) (*backup.ListRecoveryPointsByBackupVaultOutput, error)
	ListRecoveryPointsByBackupVaultWithContext(aws.Context, *backup.ListRecoveryPointsByBackupVaultInput, ...request.Option) (*backup.ListRecoveryPointsByBackupVaultOutput, error)
	ListRecoveryPointsByBackupVaultRequest(*backup.ListRecoveryPointsByBackupVaultInput) (*request.Request, *backup.ListRecoveryPointsByBackupVaultOutput)

	ListRecoveryPointsByBackupVaultPages(*backup.ListRecoveryPointsByBackupVaultInput, func(*backup.ListRecoveryPointsByBackupVaultOutput, bool) bool) error
	ListRecoveryPointsByBackupVaultPagesWithContext(aws.Context, *backup.ListRecoveryPointsByBackupVaultInput, func(*backup.ListRecoveryPointsByBackupVaultOutput, bool) bool, ...request.Option) error

	ListRecoveryPointsByResource(*backup.ListRecoveryPointsByResourceInput) (*backup.ListRecoveryPointsByResourceOutput, error)
	ListRecoveryPointsByResourceWithContext(aws.Context, *backup.ListRecoveryPointsByResourceInput, ...request.Option) (*backup.ListRecoveryPointsByResourceOutput, error)
	ListRecoveryPointsByResourceRequest(*backup.ListRecoveryPointsByResourceInput) (*request.Request, *backup.ListRecoveryPointsByResourceOutput)

	ListRecoveryPointsByResourcePages(*backup.ListRecoveryPointsByResourceInput, func(*backup.ListRecoveryPointsByResourceOutput, bool) bool) error
	ListRecoveryPointsByResourcePagesWithContext(aws.Context, *backup.ListRecoveryPointsByResourceInput, func(*backup.ListRecoveryPointsByResourceOutput, bool) bool, ...request.Option) error

	ListRestoreJobs(*backup.ListRestoreJobsInput) (*backup.ListRestoreJobsOutput, error)
	ListRestoreJobsWithContext(aws.Context, *backup.ListRestoreJobsInput, ...request.Option) (*backup.ListRestoreJobsOutput, error)
	ListRestoreJobsRequest(*backup.ListRestoreJobsInput) (*request.Request, *backup.ListRestoreJobsOutput)

	ListRestoreJobsPages(*backup.ListRestoreJobsInput, func(*backup.ListRestoreJobsOutput, bool) bool) error
	ListRestoreJobsPagesWithContext(aws.Context, *backup.ListRestoreJobsInput, func(*backup.ListRestoreJobsOutput, bool) bool, ...request.Option) error

	ListTags(*backup.ListTagsInput) (*backup.ListTagsOutput, error)
	ListTagsWithContext(aws.Context, *backup.ListTagsInput, ...request.Option) (*backup.ListTagsOutput, error)
	ListTagsRequest(*backup.ListTagsInput) (*request.Request, *backup.ListTagsOutput)

	ListTagsPages(*backup.ListTagsInput, func(*backup.ListTagsOutput, bool) bool) error
	ListTagsPagesWithContext(aws.Context, *backup.ListTagsInput, func(*backup.ListTagsOutput, bool) bool, ...request.Option) error

	PutBackupVaultAccessPolicy(*backup.PutBackupVaultAccessPolicyInput) (*backup.PutBackupVaultAccessPolicyOutput, error)
	PutBackupVaultAccessPolicyWithContext(aws.Context, *backup.PutBackupVaultAccessPolicyInput, ...request.Option) (*backup.PutBackupVaultAccessPolicyOutput, error)
	PutBackupVaultAccessPolicyRequest(*backup.PutBackupVaultAccessPolicyInput) (*request.Request, *backup.PutBackupVaultAccessPolicyOutput)

	PutBackupVaultNotifications(*backup.PutBackupVaultNotificationsInput) (*backup.PutBackupVaultNotificationsOutput, error)
	PutBackupVaultNotificationsWithContext(aws.Context, *backup.PutBackupVaultNotificationsInput, ...request.Option) (*backup.PutBackupVaultNotificationsOutput, error)
	PutBackupVaultNotificationsRequest(*backup.PutBackupVaultNotificationsInput) (*request.Request, *backup.PutBackupVaultNotificationsOutput)

	StartBackupJob(*backup.StartBackupJobInput) (*backup.StartBackupJobOutput, error)
	StartBackupJobWithContext(aws.Context, *backup.StartBackupJobInput, ...request.Option) (*backup.StartBackupJobOutput, error)
	StartBackupJobRequest(*backup.StartBackupJobInput) (*request.Request, *backup.StartBackupJobOutput)

	StartCopyJob(*backup.StartCopyJobInput) (*backup.StartCopyJobOutput, error)
	StartCopyJobWithContext(aws.Context, *backup.StartCopyJobInput, ...request.Option) (*backup.StartCopyJobOutput, error)
	StartCopyJobRequest(*backup.StartCopyJobInput) (*request.Request, *backup.StartCopyJobOutput)

	StartRestoreJob(*backup.StartRestoreJobInput) (*backup.StartRestoreJobOutput, error)
	StartRestoreJobWithContext(aws.Context, *backup.StartRestoreJobInput, ...request.Option) (*backup.StartRestoreJobOutput, error)
	StartRestoreJobRequest(*backup.StartRestoreJobInput) (*request.Request, *backup.StartRestoreJobOutput)

	StopBackupJob(*backup.StopBackupJobInput) (*backup.StopBackupJobOutput, error)
	StopBackupJobWithContext(aws.Context, *backup.StopBackupJobInput, ...request.Option) (*backup.StopBackupJobOutput, error)
	StopBackupJobRequest(*backup.StopBackupJobInput) (*request.Request, *backup.StopBackupJobOutput)

	TagResource(*backup.TagResourceInput) (*backup.TagResourceOutput, error)
	TagResourceWithContext(aws.Context, *backup.TagResourceInput, ...request.Option) (*backup.TagResourceOutput, error)
	TagResourceRequest(*backup.TagResourceInput) (*request.Request, *backup.TagResourceOutput)

	UntagResource(*backup.UntagResourceInput) (*backup.UntagResourceOutput, error)
	UntagResourceWithContext(aws.Context, *backup.UntagResourceInput, ...request.Option) (*backup.UntagResourceOutput, error)
	UntagResourceRequest(*backup.UntagResourceInput) (*request.Request, *backup.UntagResourceOutput)

	UpdateBackupPlan(*backup.UpdateBackupPlanInput) (*backup.UpdateBackupPlanOutput, error)
	UpdateBackupPlanWithContext(aws.Context, *backup.UpdateBackupPlanInput, ...request.Option) (*backup.UpdateBackupPlanOutput, error)
	UpdateBackupPlanRequest(*backup.UpdateBackupPlanInput) (*request.Request, *backup.UpdateBackupPlanOutput)

	UpdateRecoveryPointLifecycle(*backup.UpdateRecoveryPointLifecycleInput) (*backup.UpdateRecoveryPointLifecycleOutput, error)
	UpdateRecoveryPointLifecycleWithContext(aws.Context, *backup.UpdateRecoveryPointLifecycleInput, ...request.Option) (*backup.UpdateRecoveryPointLifecycleOutput, error)
	UpdateRecoveryPointLifecycleRequest(*backup.UpdateRecoveryPointLifecycleInput) (*request.Request, *backup.UpdateRecoveryPointLifecycleOutput)
}

var _ BackupAPI = (*backup.Backup)(nil)
