package evergreen

import (
	"testing"
	"time"

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

func TestBucketsConfigGetRetryFailedLogMoveLookback(t *testing.T) {
	t.Run("DurationShouldTakePrecedence", func(t *testing.T) {
		cfg := BucketsConfig{
			RetryFailedLogMoveLookback:     30 * time.Minute,
			RetryFailedLogMoveLookbackDays: 7,
		}
		assert.Equal(t, 30*time.Minute, cfg.GetRetryFailedLogMoveLookback())
	})

	t.Run("LegacyDaysShouldConvertToDuration", func(t *testing.T) {
		cfg := BucketsConfig{RetryFailedLogMoveLookbackDays: 7}
		assert.Equal(t, 7*24*time.Hour, cfg.GetRetryFailedLogMoveLookback())
	})

	t.Run("ZeroShouldRemainZero", func(t *testing.T) {
		cfg := BucketsConfig{}
		assert.Zero(t, cfg.GetRetryFailedLogMoveLookback())
	})

	t.Run("NegativeDurationShouldRemainNegative", func(t *testing.T) {
		cfg := BucketsConfig{RetryFailedLogMoveLookback: -time.Second}
		assert.Equal(t, -time.Second, cfg.GetRetryFailedLogMoveLookback())
	})
}
