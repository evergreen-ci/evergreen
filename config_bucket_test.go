package evergreen

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBucketsConfigLogBucketExpirationDays(t *testing.T) {
	days90 := 90
	days365 := 365
	days180 := 180

	cfg := &BucketsConfig{
		LogBucket:              BucketConfig{Name: "log-bucket", ExpirationDays: &days90},
		LogBucketLongRetention: BucketConfig{Name: "log-bucket-long", ExpirationDays: &days365},
		LogBucketFailedTasks:   BucketConfig{Name: "log-bucket-failed", ExpirationDays: &days180},
	}

	t.Run("EmptyBucketNameShouldReturnNotFound", func(t *testing.T) {
		_, ok := cfg.LogBucketExpirationDays("")
		assert.False(t, ok)
	})

	t.Run("LogBucketShouldReturnDays", func(t *testing.T) {
		days, ok := cfg.LogBucketExpirationDays("log-bucket")
		assert.True(t, ok)
		assert.Equal(t, 90, days)
	})

	t.Run("LogBucketLongRetentionShouldReturnDays", func(t *testing.T) {
		days, ok := cfg.LogBucketExpirationDays("log-bucket-long")
		assert.True(t, ok)
		assert.Equal(t, 365, days)
	})

	t.Run("LogBucketFailedTasksShouldReturnDays", func(t *testing.T) {
		days, ok := cfg.LogBucketExpirationDays("log-bucket-failed")
		assert.True(t, ok)
		assert.Equal(t, 180, days)
	})

	t.Run("UnknownBucketShouldReturnNotFound", func(t *testing.T) {
		_, ok := cfg.LogBucketExpirationDays("artifact-bucket")
		assert.False(t, ok)
	})

	t.Run("MatchingBucketWithNilExpirationShouldReturnNotFound", func(t *testing.T) {
		cfgNoDays := &BucketsConfig{
			LogBucket: BucketConfig{Name: "log-bucket"},
		}
		_, ok := cfgNoDays.LogBucketExpirationDays("log-bucket")
		assert.False(t, ok)
	})
}

func TestBucketsConfigGetSourceCacheBucket(t *testing.T) {
	bucket := BucketConfig{Name: "source-cache", RoleARN: "arn:aws:iam::123:role/source-cache"}

	t.Run("ProjectInListShouldReturnBucket", func(t *testing.T) {
		cfg := &BucketsConfig{SourceCacheBucket: bucket, SourceCacheProjects: []string{"proj-a", "proj-b"}}
		assert.Equal(t, bucket, cfg.GetSourceCacheBucket("proj-b"))
	})

	// Absence from the list is the only "off" state the feature has.
	t.Run("ProjectNotNamedAnywhereShouldReturnZeroBucket", func(t *testing.T) {
		cfg := &BucketsConfig{SourceCacheBucket: bucket, SourceCacheProjects: []string{"proj-a"}}
		assert.Zero(t, cfg.GetSourceCacheBucket("proj-unnamed"))
	})

	t.Run("EmptyProjectListShouldReturnZeroBucketForEveryProject", func(t *testing.T) {
		cfg := &BucketsConfig{SourceCacheBucket: bucket}
		assert.Zero(t, cfg.GetSourceCacheBucket("proj-a"))
	})

	t.Run("ListedProjectWithNoBucketConfiguredShouldReturnZeroBucket", func(t *testing.T) {
		cfg := &BucketsConfig{SourceCacheProjects: []string{"proj-a"}}
		assert.Zero(t, cfg.GetSourceCacheBucket("proj-a"))
	})
}
