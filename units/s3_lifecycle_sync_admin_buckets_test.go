package units

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/mongodb/amboy/registry"
	"github.com/stretchr/testify/require"
)

func setupTest(t *testing.T) *mock.Environment {
	env := &mock.Environment{}
	require.NoError(t, env.Configure(t.Context()))
	require.NoError(t, db.ClearCollections(evergreen.ConfigCollection))
	t.Cleanup(func() {
		require.NoError(t, db.ClearCollections(evergreen.ConfigCollection))
	})
	return env
}

func TestJobFactory(t *testing.T) {
	factory, err := registry.GetJobFactory(s3LifecycleSyncAdminBucketsJobName)
	require.NoError(t, err)
	require.NotNil(t, factory)
	require.NotNil(t, factory())
	require.Equal(t, s3LifecycleSyncAdminBucketsJobName, factory().Type().Name)
}

func TestJobCreation(t *testing.T) {
	job := NewS3LifecycleSyncAdminBucketsJob("2025-01-16")
	require.NotNil(t, job)
	require.Equal(t, "s3-lifecycle-sync-admin-buckets.2025-01-16", job.ID())
	require.Equal(t, s3LifecycleSyncAdminBucketsJobName, job.Type().Name)
}

func TestRunWithDisabledFlag(t *testing.T) {
	env := setupTest(t)
	flags := evergreen.ServiceFlags{
		S3LifecycleSyncDisabled: true,
	}
	require.NoError(t, flags.Set(t.Context()))

	job := makeS3LifecycleSyncAdminBucketsJob()
	job.env = env
	job.SetID("test-job-id")

	require.False(t, job.Status().Completed)
	job.Run(t.Context())
	require.True(t, job.Status().Completed)
	require.False(t, job.HasErrors())
}

func TestGetBucketFieldForName(t *testing.T) {
	bucketsConfig := evergreen.BucketsConfig{
		LogBucket: evergreen.BucketConfig{
			Name: "my-log-bucket",
		},
		LogBucketLongRetention: evergreen.BucketConfig{
			Name: "my-long-retention-bucket",
		},
		LogBucketFailedTasks: evergreen.BucketConfig{
			Name: "my-failed-tasks-bucket",
		},
	}

	require.Equal(t, "log_bucket", getBucketFieldForName(bucketsConfig, "my-log-bucket"))
	require.Equal(t, "log_bucket_long_retention", getBucketFieldForName(bucketsConfig, "my-long-retention-bucket"))
	require.Equal(t, "log_bucket_failed_tasks", getBucketFieldForName(bucketsConfig, "my-failed-tasks-bucket"))
	require.Equal(t, "", getBucketFieldForName(bucketsConfig, "unknown-bucket"))
}

func TestPopulateOperation(t *testing.T) {
	env := setupTest(t)

	flags := evergreen.ServiceFlags{
		S3LifecycleSyncDisabled: true,
	}
	require.NoError(t, flags.Set(t.Context()))

	op := PopulateS3LifecycleSyncAdminBucketsJob()
	require.NotNil(t, op)
	err := op(t.Context(), env.RemoteQueue())
	require.NoError(t, err)

	flags.S3LifecycleSyncDisabled = false
	require.NoError(t, flags.Set(t.Context()))

	err = op(t.Context(), env.RemoteQueue())
	require.NoError(t, err)
}
