package s3usage

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/stretchr/testify/assert"
)

func TestS3Usage(t *testing.T) {
	t.Run("IsZero", func(t *testing.T) {
		s3Usage := S3Usage{}
		assert.True(t, s3Usage.IsZero())

		s3Usage.UserFiles.PutRequests = 10
		assert.False(t, s3Usage.IsZero())

		s3Usage = S3Usage{}
		s3Usage.UserFiles.UploadBytes = 100
		assert.False(t, s3Usage.IsZero())

		s3Usage = S3Usage{}
		s3Usage.UserFiles.FileCount = 1
		assert.False(t, s3Usage.IsZero())

		s3Usage = S3Usage{}
		s3Usage.NumPutRequests = 5
		assert.False(t, s3Usage.IsZero())

		s3Usage = S3Usage{}
		s3Usage.UserFiles.PutCost = 0.005
		assert.False(t, s3Usage.IsZero())
	})

	t.Run("IncrementUserFiles", func(t *testing.T) {
		s3Usage := S3Usage{}
		assert.Equal(t, 0, s3Usage.UserFiles.PutRequests)
		assert.Equal(t, int64(0), s3Usage.UserFiles.UploadBytes)
		assert.Equal(t, 0, s3Usage.UserFiles.FileCount)

		s3Usage.IncrementUserFiles(5, 1024, 2)
		assert.Equal(t, 5, s3Usage.UserFiles.PutRequests)
		assert.Equal(t, int64(1024), s3Usage.UserFiles.UploadBytes)
		assert.Equal(t, 2, s3Usage.UserFiles.FileCount)

		s3Usage.IncrementUserFiles(10, 2048, 3)
		assert.Equal(t, 15, s3Usage.UserFiles.PutRequests)
		assert.Equal(t, int64(3072), s3Usage.UserFiles.UploadBytes)
		assert.Equal(t, 5, s3Usage.UserFiles.FileCount)
	})

	t.Run("IncrementPutRequests", func(t *testing.T) {
		s3Usage := S3Usage{}
		assert.Equal(t, 0, s3Usage.NumPutRequests)

		s3Usage.IncrementPutRequests(5)
		assert.Equal(t, 5, s3Usage.NumPutRequests)

		s3Usage.IncrementPutRequests(10)
		assert.Equal(t, 15, s3Usage.NumPutRequests)
	})

	t.Run("NilReceiverIsZero", func(t *testing.T) {
		var s3Usage *S3Usage
		assert.True(t, s3Usage.IsZero())
	})
}

func TestCalculatePutRequestsWithContext(t *testing.T) {
	const MB = 1024 * 1024

	t.Run("ZeroOrNegativeSize", func(t *testing.T) {
		assert.Equal(t, 0, CalculatePutRequestsWithContext(S3BucketTypeSmall, S3UploadMethodPut, 0))
		assert.Equal(t, 0, CalculatePutRequestsWithContext(S3BucketTypeLarge, S3UploadMethodPut, -100))
		assert.Equal(t, 0, CalculatePutRequestsWithContext(S3BucketTypeSmall, S3UploadMethodWriter, -1*MB))
		assert.Equal(t, 0, CalculatePutRequestsWithContext(S3BucketTypeSmall, S3UploadMethodWriter, 0))
	})

	t.Run("CopyMethod", func(t *testing.T) {
		assert.Equal(t, 1, CalculatePutRequestsWithContext(S3BucketTypeSmall, S3UploadMethodCopy, 1))
		assert.Equal(t, 1, CalculatePutRequestsWithContext(S3BucketTypeLarge, S3UploadMethodCopy, 1))
		assert.Equal(t, 1, CalculatePutRequestsWithContext(S3BucketTypeSmall, S3UploadMethodCopy, 1*MB))
		assert.Equal(t, 1, CalculatePutRequestsWithContext(S3BucketTypeLarge, S3UploadMethodCopy, 100*MB))
		assert.Equal(t, 1, CalculatePutRequestsWithContext(S3BucketTypeSmall, S3UploadMethodCopy, 1000*MB))
		assert.Equal(t, 1, CalculatePutRequestsWithContext(S3BucketTypeLarge, S3UploadMethodCopy, 1000*MB))
	})

	t.Run("SmallBucketWriter", func(t *testing.T) {
		assert.Equal(t, 1, CalculatePutRequestsWithContext(S3BucketTypeSmall, S3UploadMethodWriter, 1))
		assert.Equal(t, 1, CalculatePutRequestsWithContext(S3BucketTypeSmall, S3UploadMethodWriter, 1*MB))
		assert.Equal(t, 1, CalculatePutRequestsWithContext(S3BucketTypeSmall, S3UploadMethodWriter, 4*MB))
		assert.Equal(t, 1, CalculatePutRequestsWithContext(S3BucketTypeSmall, S3UploadMethodWriter, 5*MB))
		assert.Equal(t, 1, CalculatePutRequestsWithContext(S3BucketTypeSmall, S3UploadMethodWriter, 10*MB))
		assert.Equal(t, 1, CalculatePutRequestsWithContext(S3BucketTypeSmall, S3UploadMethodWriter, 100*MB))
	})

	t.Run("LargeBucketWriter", func(t *testing.T) {
		assert.Equal(t, 3, CalculatePutRequestsWithContext(S3BucketTypeLarge, S3UploadMethodWriter, 1))
		assert.Equal(t, 3, CalculatePutRequestsWithContext(S3BucketTypeLarge, S3UploadMethodWriter, 1*MB))
		assert.Equal(t, 3, CalculatePutRequestsWithContext(S3BucketTypeLarge, S3UploadMethodWriter, 5*MB))
		assert.Equal(t, 4, CalculatePutRequestsWithContext(S3BucketTypeLarge, S3UploadMethodWriter, 10*MB))
		assert.Equal(t, 12, CalculatePutRequestsWithContext(S3BucketTypeLarge, S3UploadMethodWriter, 50*MB))
		assert.Equal(t, 22, CalculatePutRequestsWithContext(S3BucketTypeLarge, S3UploadMethodWriter, 100*MB))
	})

	t.Run("PutMethodSmallBucket", func(t *testing.T) {
		assert.Equal(t, 1, CalculatePutRequestsWithContext(S3BucketTypeSmall, S3UploadMethodPut, 1))
		assert.Equal(t, 1, CalculatePutRequestsWithContext(S3BucketTypeSmall, S3UploadMethodPut, 100*1024))
		assert.Equal(t, 1, CalculatePutRequestsWithContext(S3BucketTypeSmall, S3UploadMethodPut, 1*MB))
		assert.Equal(t, 1, CalculatePutRequestsWithContext(S3BucketTypeSmall, S3UploadMethodPut, 4*MB))
		assert.Equal(t, 1, CalculatePutRequestsWithContext(S3BucketTypeSmall, S3UploadMethodPut, 5*MB-1))
		assert.Equal(t, 3, CalculatePutRequestsWithContext(S3BucketTypeSmall, S3UploadMethodPut, 5*MB))
		assert.Equal(t, 4, CalculatePutRequestsWithContext(S3BucketTypeSmall, S3UploadMethodPut, 5*MB+1))
		assert.Equal(t, 4, CalculatePutRequestsWithContext(S3BucketTypeSmall, S3UploadMethodPut, 10*MB))
		assert.Equal(t, 22, CalculatePutRequestsWithContext(S3BucketTypeSmall, S3UploadMethodPut, 100*MB))
	})

	t.Run("PutMethodLargeBucket", func(t *testing.T) {
		assert.Equal(t, 1, CalculatePutRequestsWithContext(S3BucketTypeLarge, S3UploadMethodPut, 1))
		assert.Equal(t, 1, CalculatePutRequestsWithContext(S3BucketTypeLarge, S3UploadMethodPut, 2*MB))
		assert.Equal(t, 1, CalculatePutRequestsWithContext(S3BucketTypeLarge, S3UploadMethodPut, 4*MB))
		assert.Equal(t, 1, CalculatePutRequestsWithContext(S3BucketTypeLarge, S3UploadMethodPut, 5*MB-1))
		assert.Equal(t, 3, CalculatePutRequestsWithContext(S3BucketTypeLarge, S3UploadMethodPut, 5*MB))
		assert.Equal(t, 4, CalculatePutRequestsWithContext(S3BucketTypeLarge, S3UploadMethodPut, 5*MB+1))
		assert.Equal(t, 5, CalculatePutRequestsWithContext(S3BucketTypeLarge, S3UploadMethodPut, 15*MB))
		assert.Equal(t, 22, CalculatePutRequestsWithContext(S3BucketTypeLarge, S3UploadMethodPut, 100*MB))
	})

	t.Run("RealWorldScenarios", func(t *testing.T) {
		assert.Equal(t, 1, CalculatePutRequestsWithContext(S3BucketTypeLarge, S3UploadMethodPut, 2*MB))
		assert.Equal(t, 1, CalculatePutRequestsWithContext(S3BucketTypeSmall, S3UploadMethodWriter, 500*1024))
		assert.Equal(t, 1, CalculatePutRequestsWithContext(S3BucketTypeLarge, S3UploadMethodCopy, 1000*MB))
		assert.Equal(t, 1, CalculatePutRequestsWithContext(S3BucketTypeLarge, S3UploadMethodPut, 300*1024))
		assert.Equal(t, 1, CalculatePutRequestsWithContext(S3BucketTypeSmall, S3UploadMethodPut, 50*1024))
		assert.Equal(t, 6, CalculatePutRequestsWithContext(S3BucketTypeLarge, S3UploadMethodPut, 20*MB))
	})

	t.Run("BoundaryConditions", func(t *testing.T) {
		assert.Equal(t, 3, CalculatePutRequestsWithContext(S3BucketTypeSmall, S3UploadMethodPut, 5*MB))
		assert.Equal(t, 3, CalculatePutRequestsWithContext(S3BucketTypeLarge, S3UploadMethodPut, 5*MB))
		assert.Equal(t, 1, CalculatePutRequestsWithContext(S3BucketTypeSmall, S3UploadMethodPut, 5*MB-1))
		assert.Equal(t, 1, CalculatePutRequestsWithContext(S3BucketTypeLarge, S3UploadMethodPut, 5*MB-1))
		assert.Equal(t, 4, CalculatePutRequestsWithContext(S3BucketTypeSmall, S3UploadMethodPut, 5*MB+1))
		assert.Equal(t, 4, CalculatePutRequestsWithContext(S3BucketTypeLarge, S3UploadMethodPut, 5*MB+1))
	})
}

func TestCalculateS3PutCostWithConfig(t *testing.T) {
	validConfig := &evergreen.CostConfig{
		S3Cost: evergreen.S3CostConfig{
			Upload: evergreen.S3UploadCostConfig{
				UploadCostDiscount: 0.3,
			},
		},
	}

	t.Run("WithValidConfig", func(t *testing.T) {
		cost := CalculateS3PutCostWithConfig(1000, validConfig)
		// 1000 * 0.000005 * 0.7 = 0.0035
		assert.InDelta(t, 0.0035, cost, 0.000001)
	})

	t.Run("WithNilConfig", func(t *testing.T) {
		cost := CalculateS3PutCostWithConfig(1000, nil)
		assert.Equal(t, 0.0, cost)
	})

	t.Run("WithZeroPutRequests", func(t *testing.T) {
		cost := CalculateS3PutCostWithConfig(0, validConfig)
		assert.Equal(t, 0.0, cost)
	})

	t.Run("WithInvalidDiscount", func(t *testing.T) {
		invalidConfig := &evergreen.CostConfig{
			S3Cost: evergreen.S3CostConfig{
				Upload: evergreen.S3UploadCostConfig{
					UploadCostDiscount: 1.5,
				},
			},
		}
		cost := CalculateS3PutCostWithConfig(1000, invalidConfig)
		assert.Equal(t, 0.0, cost)
	})

	t.Run("WithNegativePutRequests", func(t *testing.T) {
		cost := CalculateS3PutCostWithConfig(-5, validConfig)
		assert.Equal(t, 0.0, cost)
	})
}
