package s3lifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/pail"
	_ "github.com/evergreen-ci/evergreen/testutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptr[T any](v T) *T { return &v }

func TestS3LifecycleRuleDoc_Upsert(t *testing.T) {
	require.NoError(t, db.Clear(Collection))
	now := time.Now().Truncate(time.Second)

	doc := &S3LifecycleRuleDoc{
		BucketName:          "test-bucket",
		FilterPrefix:        "logs/",
		BucketType:          BucketTypeAdminManaged,
		Region:              "us-east-1",
		AdminBucketCategory: AdminBucketCategoryLog,
		ProjectAssociations: []string{"proj1", "proj2"},
		RuleID:              "rule1",
		RuleStatus:          "Enabled",
		ExpirationDays:      ptr(30),
		TransitionToIADays:  ptr(7),
		Transitions:         []Transition{{Days: ptr(int32(30)), StorageClass: "GLACIER"}},
		LastSyncedAt:        now,
	}

	require.NoError(t, doc.Upsert(t.Context()))
	assert.Equal(t, "test-bucket#logs/", doc.ID)

	found, err := FindByBucketAndPrefix(t.Context(), "test-bucket", "logs/")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, doc.BucketName, found.BucketName)
	assert.Equal(t, doc.FilterPrefix, found.FilterPrefix)
	assert.Equal(t, doc.BucketType, found.BucketType)
	assert.Equal(t, doc.RuleID, found.RuleID)
	assert.Equal(t, now.Unix(), found.LastSyncedAt.Unix())

	// Test update
	doc.ProjectAssociations = []string{"proj1", "proj2", "proj3"}
	doc.ExpirationDays = ptr(90)
	require.NoError(t, doc.Upsert(t.Context()))

	found, err = FindByBucketAndPrefix(t.Context(), "test-bucket", "logs/")
	require.NoError(t, err)
	assert.Equal(t, []string{"proj1", "proj2", "proj3"}, found.ProjectAssociations)
	assert.Equal(t, 90, *found.ExpirationDays)
}

func TestS3LifecycleRuleDoc_UpsertEmptyBucketName(t *testing.T) {
	err := (&S3LifecycleRuleDoc{BucketName: "", BucketType: BucketTypeAdminManaged}).Upsert(t.Context())
	assert.ErrorContains(t, err, "bucket name cannot be empty")
}

func TestMakeRuleID(t *testing.T) {
	tests := []struct {
		name         string
		bucketName   string
		filterPrefix string
		expected     string
	}{
		{
			name:         "with prefix",
			bucketName:   "mciuploads",
			filterPrefix: "sandbox/",
			expected:     "mciuploads#sandbox/",
		},
		{
			name:         "empty prefix (default rule)",
			bucketName:   "mciuploads",
			filterPrefix: "",
			expected:     "mciuploads#",
		},
		{
			name:         "multi-level prefix",
			bucketName:   "mciuploads",
			filterPrefix: "lifecycle/sandbox/",
			expected:     "mciuploads#lifecycle/sandbox/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := makeRuleID(tt.bucketName, tt.filterPrefix)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFindByBucketAndPrefix(t *testing.T) {
	require.NoError(t, db.Clear(Collection))
	require.NoError(t, (&S3LifecycleRuleDoc{
		BucketName:   "test-bucket",
		FilterPrefix: "sandbox/",
		BucketType:   BucketTypeUserSpecified,
		Region:       "us-west-2",
		RuleID:       "rule1",
		RuleStatus:   "Enabled",
	}).Upsert(t.Context()))

	found, err := FindByBucketAndPrefix(t.Context(), "test-bucket", "sandbox/")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "test-bucket", found.BucketName)
	assert.Equal(t, "sandbox/", found.FilterPrefix)

	found, err = FindByBucketAndPrefix(t.Context(), "test-bucket", "other/")
	require.NoError(t, err)
	assert.Nil(t, found)

	_, err = FindByBucketAndPrefix(t.Context(), "", "sandbox/")
	assert.ErrorContains(t, err, "bucket name cannot be empty")
}

func TestFindMatchingRuleForFileKey(t *testing.T) {
	require.NoError(t, db.Clear(Collection))

	rules := []S3LifecycleRuleDoc{
		{BucketName: "mciuploads", FilterPrefix: "sandbox/", BucketType: BucketTypeUserSpecified, RuleID: "sandbox-rule", RuleStatus: "Enabled", ExpirationDays: ptr(30)},
		{BucketName: "mciuploads", FilterPrefix: "lifecycle/sandbox/", BucketType: BucketTypeUserSpecified, RuleID: "lifecycle-sandbox-rule", RuleStatus: "Enabled", ExpirationDays: ptr(7)},
		{BucketName: "mciuploads", FilterPrefix: "", BucketType: BucketTypeUserSpecified, RuleID: "default-rule", RuleStatus: "Enabled", ExpirationDays: ptr(90)},
		{BucketName: "mciuploads", FilterPrefix: "disabled/", BucketType: BucketTypeUserSpecified, RuleID: "disabled-rule", RuleStatus: "Disabled", ExpirationDays: ptr(1)},
	}
	for _, rule := range rules {
		r := rule
		require.NoError(t, (&r).Upsert(t.Context()))
	}

	tests := []struct {
		bucket, fileKey, wantPrefix, wantRuleID string
		wantDays                                *int
	}{
		{"mciuploads", "sandbox/myfile.txt", "sandbox/", "sandbox-rule", ptr(30)},
		{"mciuploads", "lifecycle/sandbox/myfile.txt", "lifecycle/sandbox/", "lifecycle-sandbox-rule", ptr(7)},
		{"mciuploads", "other/myfile.txt", "", "default-rule", ptr(90)},
		{"mciuploads", "disabled/myfile.txt", "", "default-rule", ptr(90)}, // skips disabled
		{"non-existent", "myfile.txt", "", "", nil},
	}

	for _, tt := range tests {
		found, err := FindMatchingRuleForFileKey(t.Context(), tt.bucket, tt.fileKey)
		require.NoError(t, err)
		if tt.wantRuleID == "" {
			assert.Nil(t, found)
		} else {
			require.NotNil(t, found)
			assert.Equal(t, tt.wantPrefix, found.FilterPrefix)
			assert.Equal(t, tt.wantRuleID, found.RuleID)
			assert.Equal(t, tt.wantDays, found.ExpirationDays)
		}
	}

	_, err := FindMatchingRuleForFileKey(t.Context(), "", "myfile.txt")
	assert.ErrorContains(t, err, "bucket name cannot be empty")

	_, err = FindMatchingRuleForFileKey(t.Context(), "mciuploads", "")
	assert.ErrorContains(t, err, "file key cannot be empty")
}

func TestFindAllRulesForBucket(t *testing.T) {
	require.NoError(t, db.Clear(Collection))

	rules := []S3LifecycleRuleDoc{
		{BucketName: "bucket1", FilterPrefix: "sandbox/", BucketType: BucketTypeUserSpecified, RuleID: "rule1", RuleStatus: "Enabled"},
		{BucketName: "bucket1", FilterPrefix: "logs/", BucketType: BucketTypeUserSpecified, RuleID: "rule2", RuleStatus: "Enabled"},
		{BucketName: "bucket1", FilterPrefix: "", BucketType: BucketTypeUserSpecified, RuleID: "rule3", RuleStatus: "Enabled"},
		{BucketName: "bucket2", FilterPrefix: "", BucketType: BucketTypeAdminManaged, RuleID: "rule4", RuleStatus: "Enabled"},
	}
	for _, rule := range rules {
		r := rule
		require.NoError(t, (&r).Upsert(t.Context()))
	}

	tests := []struct {
		bucket     string
		wantCount  int
		wantRuleID string
		wantErr    string
	}{
		{"bucket1", 3, "", ""},
		{"bucket2", 1, "rule4", ""},
		{"non-existent", 0, "", ""},
		{"", 0, "", "bucket name cannot be empty"},
	}

	for _, tt := range tests {
		found, err := FindAllRulesForBucket(t.Context(), tt.bucket)
		if tt.wantErr != "" {
			assert.ErrorContains(t, err, tt.wantErr)
		} else {
			require.NoError(t, err)
			assert.Len(t, found, tt.wantCount)
			if tt.wantRuleID != "" {
				assert.Equal(t, tt.wantRuleID, found[0].RuleID)
			}
		}
	}
}

func TestDiscoverAdminManagedBuckets(t *testing.T) {
	roleARN := "arn:aws:iam::123456789012:role/LogRole"
	tests := []struct {
		name        string
		config      evergreen.BucketsConfig
		wantCount   int
		wantErr     bool
		checkBucket func(t *testing.T, buckets map[string]BucketInfo)
	}{
		{
			name: "basic discovery with role ARN",
			config: evergreen.BucketsConfig{
				LogBucket:              evergreen.BucketConfig{Name: "log-bucket", Type: evergreen.BucketTypeS3, RoleARN: roleARN},
				LogBucketLongRetention: evergreen.BucketConfig{Name: "retention-bucket", Type: evergreen.BucketTypeS3},
				LogBucketFailedTasks:   evergreen.BucketConfig{Name: "failed-bucket", Type: evergreen.BucketTypeS3},
				TestResultsBucket:      evergreen.BucketConfig{Name: "test-bucket", Type: evergreen.BucketTypeS3},
			},
			wantCount: 3,
			checkBucket: func(t *testing.T, buckets map[string]BucketInfo) {
				assert.Contains(t, buckets, "log-bucket")
				assert.NotContains(t, buckets, "test-bucket")
				require.NotNil(t, buckets["log-bucket"].RoleARN)
				assert.Equal(t, roleARN, *buckets["log-bucket"].RoleARN)
			},
		},
		{
			name: "skips non-S3 buckets",
			config: evergreen.BucketsConfig{
				LogBucket:              evergreen.BucketConfig{Name: "gridfs", Type: evergreen.BucketTypeGridFS},
				LogBucketLongRetention: evergreen.BucketConfig{Name: "s3-bucket", Type: evergreen.BucketTypeS3},
				LogBucketFailedTasks:   evergreen.BucketConfig{Name: "local", Type: evergreen.BucketTypeLocal},
			},
			wantCount: 1,
			checkBucket: func(t *testing.T, buckets map[string]BucketInfo) {
				assert.Contains(t, buckets, "s3-bucket")
			},
		},
		{
			name: "skips empty names",
			config: evergreen.BucketsConfig{
				LogBucket:              evergreen.BucketConfig{Name: "", Type: evergreen.BucketTypeS3},
				LogBucketLongRetention: evergreen.BucketConfig{Name: "valid-bucket", Type: evergreen.BucketTypeS3},
			},
			wantCount: 1,
			checkBucket: func(t *testing.T, buckets map[string]BucketInfo) {
				assert.Contains(t, buckets, "valid-bucket")
			},
		},
		{
			name: "errors with no valid buckets",
			config: evergreen.BucketsConfig{
				LogBucket:              evergreen.BucketConfig{Name: "", Type: evergreen.BucketTypeS3},
				LogBucketLongRetention: evergreen.BucketConfig{Name: "", Type: evergreen.BucketTypeGridFS},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := &evergreen.Settings{Buckets: tt.config}
			buckets, err := DiscoverAdminManagedBuckets(t.Context(), settings)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Len(t, buckets, tt.wantCount)
			if tt.checkBucket != nil {
				tt.checkBucket(t, buckets)
			}
		})
	}
}

// mockS3LifecycleClient is a mock implementation of cloud.S3LifecycleClient for testing.
type mockS3LifecycleClient struct {
	rules []pail.LifecycleRule
	err   error
}

func (m *mockS3LifecycleClient) GetBucketLifecycleConfiguration(ctx context.Context, bucket, region string, roleARN *string, externalID *string) ([]pail.LifecycleRule, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.rules, nil
}

func TestDiscoverAndCacheProjectBucket(t *testing.T) {
	t.Run("NewBucket", func(t *testing.T) {
		require.NoError(t, db.Clear(Collection))

		mockClient := &mockS3LifecycleClient{
			rules: []pail.LifecycleRule{
				{
					ID:             "rule1",
					Prefix:         "sandbox/",
					Status:         "Enabled",
					ExpirationDays: ptr(int32(30)),
				},
				{
					ID:             "rule2",
					Prefix:         "",
					Status:         "Enabled",
					ExpirationDays: ptr(int32(90)),
				},
			},
		}

		discovered := DiscoverAndCacheProjectBucket(t.Context(), "test-bucket", "us-east-1", nil, nil, "test-project", mockClient)
		assert.True(t, discovered, "should trigger discovery for new bucket")

		rules, err := FindAllRulesForBucket(t.Context(), "test-bucket")
		require.NoError(t, err)
		assert.Len(t, rules, 2)

		sandboxRule, err := FindByBucketAndPrefix(t.Context(), "test-bucket", "sandbox/")
		require.NoError(t, err)
		require.NotNil(t, sandboxRule)
		assert.Equal(t, "test-bucket#sandbox/", sandboxRule.ID)
		assert.Equal(t, BucketTypeUserSpecified, sandboxRule.BucketType)
		assert.Equal(t, []string{"test-project"}, sandboxRule.ProjectAssociations)
		assert.Equal(t, 30, *sandboxRule.ExpirationDays)

		defaultRule, err := FindByBucketAndPrefix(t.Context(), "test-bucket", "")
		require.NoError(t, err)
		require.NotNil(t, defaultRule)
		assert.Equal(t, "test-bucket#", defaultRule.ID)
		assert.Equal(t, 90, *defaultRule.ExpirationDays)
	})

	t.Run("AlreadyCached", func(t *testing.T) {
		require.NoError(t, db.Clear(Collection))

		existingRule := &S3LifecycleRuleDoc{
			BucketName:   "cached-bucket",
			FilterPrefix: "",
			BucketType:   BucketTypeUserSpecified,
			Region:       "us-east-1",
			RuleID:       "existing-rule",
			RuleStatus:   "Enabled",
			LastSyncedAt: time.Now(),
		}
		require.NoError(t, existingRule.Upsert(t.Context()))

		mockClient := &mockS3LifecycleClient{
			rules: []pail.LifecycleRule{{ID: "should-not-be-called"}},
		}

		discovered := DiscoverAndCacheProjectBucket(t.Context(), "cached-bucket", "us-east-1", nil, nil, "test-project", mockClient)
		assert.False(t, discovered, "should skip discovery for cached bucket")

		rules, err := FindAllRulesForBucket(t.Context(), "cached-bucket")
		require.NoError(t, err)
		assert.Len(t, rules, 1)
		assert.Equal(t, "existing-rule", rules[0].RuleID)
	})

	t.Run("AWSError", func(t *testing.T) {
		require.NoError(t, db.Clear(Collection))

		mockClient := &mockS3LifecycleClient{
			err: errors.New("AWS API error"),
		}

		discovered := DiscoverAndCacheProjectBucket(t.Context(), "error-bucket", "us-east-1", nil, nil, "test-project", mockClient)
		assert.True(t, discovered, "should return true even if AWS call fails")

		rules, err := FindAllRulesForBucket(t.Context(), "error-bucket")
		require.NoError(t, err)
		assert.Len(t, rules, 0)
	})

	t.Run("NoLifecycleRules", func(t *testing.T) {
		require.NoError(t, db.Clear(Collection))

		mockClient := &mockS3LifecycleClient{
			rules: []pail.LifecycleRule{},
		}

		discovered := DiscoverAndCacheProjectBucket(t.Context(), "empty-bucket", "us-east-1", nil, nil, "test-project", mockClient)
		assert.True(t, discovered, "should trigger discovery even if no rules returned")

		rules, err := FindAllRulesForBucket(t.Context(), "empty-bucket")
		require.NoError(t, err)
		assert.Len(t, rules, 0)
	})
}
