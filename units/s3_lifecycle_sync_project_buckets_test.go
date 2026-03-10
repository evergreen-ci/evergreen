package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/s3lifecycle"
	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupProjectBucketsTest(t *testing.T) *mock.Environment {
	env := &mock.Environment{}
	require.NoError(t, env.Configure(t.Context()))
	require.NoError(t, db.ClearCollections(evergreen.ConfigCollection, s3lifecycle.Collection))
	t.Cleanup(func() {
		require.NoError(t, db.ClearCollections(evergreen.ConfigCollection, s3lifecycle.Collection))
	})
	return env
}

func TestProjectBucketsJobFactory(t *testing.T) {
	factory, err := registry.GetJobFactory(s3LifecycleSyncProjectBucketsJobName)
	require.NoError(t, err)
	require.NotNil(t, factory)
	require.NotNil(t, factory())
	require.Equal(t, s3LifecycleSyncProjectBucketsJobName, factory().Type().Name)
}

func TestProjectBucketsJobCreation(t *testing.T) {
	job := NewS3LifecycleSyncProjectBucketsJob("2025-01-16")
	require.NotNil(t, job)
	require.Equal(t, "s3-lifecycle-sync-project-buckets.2025-01-16", job.ID())
	require.Equal(t, s3LifecycleSyncProjectBucketsJobName, job.Type().Name)
}

func TestProjectBucketsRunWithDisabledFlag(t *testing.T) {
	env := setupProjectBucketsTest(t)
	flags := evergreen.ServiceFlags{
		S3LifecycleSyncDisabled: true,
	}
	require.NoError(t, flags.Set(t.Context()))

	job := makeS3LifecycleSyncProjectBucketsJob()
	job.env = env
	job.SetID("test-job-id")

	require.False(t, job.Status().Completed)
	job.Run(t.Context())
	require.True(t, job.Status().Completed)
	require.False(t, job.HasErrors())
}

func TestProjectBucketsRunWithNoBuckets(t *testing.T) {
	env := setupProjectBucketsTest(t)
	flags := evergreen.ServiceFlags{
		S3LifecycleSyncDisabled: false,
	}
	require.NoError(t, flags.Set(t.Context()))

	job := makeS3LifecycleSyncProjectBucketsJob()
	job.env = env
	job.SetID("test-job-id")

	require.False(t, job.Status().Completed)
	job.Run(t.Context())
	require.True(t, job.Status().Completed)
	require.False(t, job.HasErrors())
}

func TestConvertPailRuleToDoc(t *testing.T) {
	bucketName := "test-bucket"
	region := "us-west-2"

	awsRule := pail.LifecycleRule{
		ID:                      "rule-1",
		Prefix:                  "sandbox/",
		Status:                  "Enabled",
		ExpirationDays:          utility.ToInt32Ptr(30),
		TransitionToIADays:      utility.ToInt32Ptr(7),
		TransitionToGlacierDays: utility.ToInt32Ptr(14),
		Transitions: []pail.Transition{
			{Days: utility.ToInt32Ptr(7), StorageClass: "STANDARD_IA"},
			{Days: utility.ToInt32Ptr(14), StorageClass: "GLACIER"},
		},
	}

	existingRules := []s3lifecycle.S3LifecycleRuleDoc{
		{
			FilterPrefix:        "sandbox/",
			ProjectAssociations: []string{"project1", "project2"},
		},
		{
			FilterPrefix:        "other/",
			ProjectAssociations: []string{"project3"},
		},
	}

	doc := convertPailRuleToDoc(bucketName, region, awsRule, existingRules)

	assert.Equal(t, bucketName, doc.BucketName)
	assert.Equal(t, "sandbox/", doc.FilterPrefix)
	assert.Equal(t, s3lifecycle.BucketTypeUserSpecified, doc.BucketType)
	assert.Equal(t, region, doc.Region)
	assert.Equal(t, "rule-1", doc.RuleID)
	assert.Equal(t, "Enabled", doc.RuleStatus)
	require.NotNil(t, doc.ExpirationDays)
	assert.Equal(t, 30, *doc.ExpirationDays)
	require.NotNil(t, doc.TransitionToIADays)
	assert.Equal(t, 7, *doc.TransitionToIADays)
	require.NotNil(t, doc.TransitionToGlacierDays)
	assert.Equal(t, 14, *doc.TransitionToGlacierDays)
	assert.Len(t, doc.Transitions, 2)
	assert.Equal(t, "", doc.SyncError)

	// ProjectAssociations should be preserved from existing rule.
	assert.Equal(t, []string{"project1", "project2"}, doc.ProjectAssociations)
}

func TestConvertPailRuleToDocWithoutExisting(t *testing.T) {
	bucketName := "test-bucket"
	region := "us-west-2"

	awsRule := pail.LifecycleRule{
		ID:             "rule-1",
		Prefix:         "logs/",
		Status:         "Enabled",
		ExpirationDays: utility.ToInt32Ptr(90),
	}

	existingRules := []s3lifecycle.S3LifecycleRuleDoc{
		{
			FilterPrefix:        "other/",
			ProjectAssociations: []string{"project1"},
		},
	}

	doc := convertPailRuleToDoc(bucketName, region, awsRule, existingRules)

	assert.Equal(t, "logs/", doc.FilterPrefix)
	// ProjectAssociations should be nil since no matching existing rule.
	assert.Nil(t, doc.ProjectAssociations)
}

func TestConvertPailTransitions(t *testing.T) {
	pailTransitions := []pail.Transition{
		{Days: utility.ToInt32Ptr(7), StorageClass: "STANDARD_IA"},
		{Days: utility.ToInt32Ptr(14), StorageClass: "GLACIER"},
		{Days: utility.ToInt32Ptr(90), StorageClass: "DEEP_ARCHIVE"},
	}

	result := convertPailTransitions(pailTransitions)

	require.Len(t, result, 3)
	assert.Equal(t, int32(7), *result[0].Days)
	assert.Equal(t, "STANDARD_IA", result[0].StorageClass)
	assert.Equal(t, int32(14), *result[1].Days)
	assert.Equal(t, "GLACIER", result[1].StorageClass)
	assert.Equal(t, int32(90), *result[2].Days)
	assert.Equal(t, "DEEP_ARCHIVE", result[2].StorageClass)
}

func TestConvertPailTransitionsEmpty(t *testing.T) {
	result := convertPailTransitions(nil)
	assert.Nil(t, result)

	result = convertPailTransitions([]pail.Transition{})
	assert.Nil(t, result)
}

func TestPopulateProjectBucketsOperation(t *testing.T) {
	env := setupProjectBucketsTest(t)

	flags := evergreen.ServiceFlags{
		S3LifecycleSyncDisabled: true,
	}
	require.NoError(t, flags.Set(t.Context()))

	op := PopulateS3LifecycleSyncProjectBucketsJob()
	require.NotNil(t, op)
	err := op(t.Context(), env.RemoteQueue())
	require.NoError(t, err)

	flags.S3LifecycleSyncDisabled = false
	require.NoError(t, flags.Set(t.Context()))

	err = op(t.Context(), env.RemoteQueue())
	require.NoError(t, err)
}

// mockS3LifecycleClient is a mock implementation of cloud.S3LifecycleClient for testing.
type mockS3LifecycleClient struct {
	rules map[string][]pail.LifecycleRule
	err   error
}

func (m *mockS3LifecycleClient) GetBucketLifecycleConfiguration(ctx context.Context, bucket, region string, roleARN *string, externalID *string) ([]pail.LifecycleRule, error) {
	if m.err != nil {
		return nil, m.err
	}
	if rules, ok := m.rules[bucket]; ok {
		return rules, nil
	}
	return []pail.LifecycleRule{}, nil
}

func TestSyncBucket(t *testing.T) {
	require.NoError(t, db.Clear(s3lifecycle.Collection))
	defer func() {
		require.NoError(t, db.Clear(s3lifecycle.Collection))
	}()

	ctx := context.Background()

	// Create existing rules in the collection.
	existingRule1 := &s3lifecycle.S3LifecycleRuleDoc{
		BucketName:          "test-bucket",
		FilterPrefix:        "sandbox/",
		BucketType:          s3lifecycle.BucketTypeUserSpecified,
		Region:              "us-west-2",
		RuleID:              "old-rule-1",
		RuleStatus:          "Enabled",
		ExpirationDays:      utility.ToIntPtr(30),
		ProjectAssociations: []string{"project1"},
		LastSyncedAt:        time.Now().Add(-24 * time.Hour),
	}
	require.NoError(t, existingRule1.Upsert(ctx))

	existingRule2 := &s3lifecycle.S3LifecycleRuleDoc{
		BucketName:          "test-bucket",
		FilterPrefix:        "obsolete/",
		BucketType:          s3lifecycle.BucketTypeUserSpecified,
		Region:              "us-west-2",
		RuleID:              "old-rule-2",
		RuleStatus:          "Enabled",
		ExpirationDays:      utility.ToIntPtr(7),
		ProjectAssociations: []string{"project2"},
		LastSyncedAt:        time.Now().Add(-24 * time.Hour),
	}
	require.NoError(t, existingRule2.Upsert(ctx))

	// Mock AWS rules (obsolete/ prefix removed, sandbox/ updated, new logs/ added).
	mockClient := &mockS3LifecycleClient{
		rules: map[string][]pail.LifecycleRule{
			"test-bucket": {
				{
					ID:             "new-rule-1",
					Prefix:         "sandbox/",
					Status:         "Enabled",
					ExpirationDays: utility.ToInt32Ptr(90), // Updated from 30 to 90
				},
				{
					ID:             "new-rule-2",
					Prefix:         "logs/",
					Status:         "Enabled",
					ExpirationDays: utility.ToInt32Ptr(14),
				},
			},
		},
	}

	job := makeS3LifecycleSyncProjectBucketsJob()
	err := job.syncBucket(ctx, mockClient, "test-bucket")
	require.NoError(t, err)

	// Verify sandbox/ rule was updated.
	sandboxRule, err := s3lifecycle.FindByBucketAndPrefix(ctx, "test-bucket", "sandbox/")
	require.NoError(t, err)
	require.NotNil(t, sandboxRule)
	assert.Equal(t, "new-rule-1", sandboxRule.RuleID)
	require.NotNil(t, sandboxRule.ExpirationDays)
	assert.Equal(t, 90, *sandboxRule.ExpirationDays)
	// ProjectAssociations should be preserved.
	assert.Equal(t, []string{"project1"}, sandboxRule.ProjectAssociations)

	// Verify logs/ rule was added.
	logsRule, err := s3lifecycle.FindByBucketAndPrefix(ctx, "test-bucket", "logs/")
	require.NoError(t, err)
	require.NotNil(t, logsRule)
	assert.Equal(t, "new-rule-2", logsRule.RuleID)
	require.NotNil(t, logsRule.ExpirationDays)
	assert.Equal(t, 14, *logsRule.ExpirationDays)

	// Verify obsolete/ rule was removed.
	obsoleteRule, err := s3lifecycle.FindByBucketAndPrefix(ctx, "test-bucket", "obsolete/")
	require.NoError(t, err)
	assert.Nil(t, obsoleteRule)
}

func TestSyncBucketWithNoExistingRules(t *testing.T) {
	require.NoError(t, db.Clear(s3lifecycle.Collection))
	defer func() {
		require.NoError(t, db.Clear(s3lifecycle.Collection))
	}()

	ctx := context.Background()

	mockClient := &mockS3LifecycleClient{
		rules: map[string][]pail.LifecycleRule{
			"test-bucket": {
				{ID: "rule-1", Prefix: "logs/", Status: "Enabled", ExpirationDays: utility.ToInt32Ptr(30)},
			},
		},
	}

	job := makeS3LifecycleSyncProjectBucketsJob()
	err := job.syncBucket(ctx, mockClient, "test-bucket")
	require.NoError(t, err)

	// No error, but also no sync should happen since there are no existing rules.
	logsRule, err := s3lifecycle.FindByBucketAndPrefix(ctx, "test-bucket", "logs/")
	require.NoError(t, err)
	assert.Nil(t, logsRule)
}
