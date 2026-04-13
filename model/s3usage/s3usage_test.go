package s3usage

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bytesForFile returns the stored bytes for a specific file in a specific bucket, or 0 if not found.
func bytesForFile(metrics []BucketFileMetrics, bucket, fileKey string) int64 {
	for _, b := range metrics {
		if b.Bucket == bucket {
			for _, f := range b.Files {
				if f.FileKey == fileKey {
					return f.Bytes
				}
			}
		}
	}
	return 0
}

// hasBucket returns true if the given bucket exists in the metrics slice.
func hasBucket(metrics []BucketFileMetrics, bucket string) bool {
	for _, b := range metrics {
		if b.Bucket == bucket {
			return true
		}
	}
	return false
}

func TestS3Usage(t *testing.T) {
	t.Run("IsZero", func(t *testing.T) {
		s3Usage := S3Usage{}
		assert.True(t, s3Usage.IsZero())

		s3Usage.Artifacts.PutRequests = 10
		assert.False(t, s3Usage.IsZero())

		s3Usage = S3Usage{}
		s3Usage.Artifacts.UploadBytes = 100
		assert.False(t, s3Usage.IsZero())

		s3Usage = S3Usage{}
		s3Usage.Artifacts.Count = 1
		assert.False(t, s3Usage.IsZero())

		s3Usage = S3Usage{}
		s3Usage.Logs.PutRequests = 5
		assert.False(t, s3Usage.IsZero())

		s3Usage = S3Usage{}
		s3Usage.Logs.UploadBytes = 100
		assert.False(t, s3Usage.IsZero())

	})

	t.Run("IncrementArtifacts", func(t *testing.T) {
		s3Usage := S3Usage{}
		assert.Equal(t, 0, s3Usage.Artifacts.PutRequests)
		assert.Equal(t, int64(0), s3Usage.Artifacts.UploadBytes)
		assert.Equal(t, 0, s3Usage.Artifacts.Count)
		assert.Equal(t, 0, s3Usage.Artifacts.ArtifactWithMaxPutRequests)
		assert.Equal(t, 0, s3Usage.Artifacts.ArtifactWithMinPutRequests)

		filesA := []FileMetrics{
			{RemotePath: "path/file1.txt", FileSizeBytes: 600},
			{RemotePath: "path/file2.txt", FileSizeBytes: 424},
		}
		s3Usage.IncrementArtifacts(ArtifactUpload{PutRequests: 5, UploadBytes: 1024, FileCount: 2, MaxPuts: 3, MinPuts: 2, Bucket: "bucket-a", Files: filesA})
		assert.Equal(t, 5, s3Usage.Artifacts.PutRequests)
		assert.Equal(t, int64(1024), s3Usage.Artifacts.UploadBytes)
		assert.Equal(t, 2, s3Usage.Artifacts.Count)
		assert.Equal(t, 3, s3Usage.Artifacts.ArtifactWithMaxPutRequests)
		assert.Equal(t, 2, s3Usage.Artifacts.ArtifactWithMinPutRequests)
		require.NotEmpty(t, s3Usage.Artifacts.BytesByBucketAndKey)
		require.True(t, hasBucket(s3Usage.Artifacts.BytesByBucketAndKey, "bucket-a"))
		assert.Equal(t, int64(600), bytesForFile(s3Usage.Artifacts.BytesByBucketAndKey, "bucket-a", "path/file1.txt"))
		assert.Equal(t, int64(424), bytesForFile(s3Usage.Artifacts.BytesByBucketAndKey, "bucket-a", "path/file2.txt"))

		filesB := []FileMetrics{
			{RemotePath: "other/file3.txt", FileSizeBytes: 2048},
		}
		s3Usage.IncrementArtifacts(ArtifactUpload{PutRequests: 10, UploadBytes: 2048, FileCount: 3, MaxPuts: 8, MinPuts: 1, Bucket: "bucket-b", Files: filesB})
		assert.Equal(t, 15, s3Usage.Artifacts.PutRequests)
		assert.Equal(t, int64(3072), s3Usage.Artifacts.UploadBytes)
		assert.Equal(t, 5, s3Usage.Artifacts.Count)
		assert.Equal(t, 8, s3Usage.Artifacts.ArtifactWithMaxPutRequests)
		assert.Equal(t, 1, s3Usage.Artifacts.ArtifactWithMinPutRequests)
		require.True(t, hasBucket(s3Usage.Artifacts.BytesByBucketAndKey, "bucket-b"))
		assert.Equal(t, int64(600), bytesForFile(s3Usage.Artifacts.BytesByBucketAndKey, "bucket-a", "path/file1.txt"), "bucket-a file bytes should be unchanged")
		assert.Equal(t, int64(2048), bytesForFile(s3Usage.Artifacts.BytesByBucketAndKey, "bucket-b", "other/file3.txt"))

		filesA2 := []FileMetrics{
			{RemotePath: "path/file1.txt", FileSizeBytes: 512},
		}
		s3Usage.IncrementArtifacts(ArtifactUpload{PutRequests: 3, UploadBytes: 512, FileCount: 1, MaxPuts: 3, MinPuts: 3, Bucket: "bucket-a", Files: filesA2})
		assert.Equal(t, int64(1112), bytesForFile(s3Usage.Artifacts.BytesByBucketAndKey, "bucket-a", "path/file1.txt"), "bucket-a file bytes should accumulate across invocations")
	})

	t.Run("IncrementLogs", func(t *testing.T) {
		s3Usage := S3Usage{}
		assert.Equal(t, 0, s3Usage.Logs.PutRequests)
		assert.Equal(t, int64(0), s3Usage.Logs.UploadBytes)

		s3Usage.IncrementLogs(5, 1024, LogTypeTask, "project/task1/0/task.log")
		assert.Equal(t, 5, s3Usage.Logs.PutRequests)
		assert.Equal(t, int64(1024), s3Usage.Logs.UploadBytes)
		assert.Equal(t, int64(1024), s3Usage.Logs.Task.Bytes)
		assert.Equal(t, "project/task1/0/task.log", s3Usage.Logs.Task.LogKey)

		s3Usage.IncrementLogs(10, 2048, LogTypeAgent, "project/task1/0/agent.log")
		assert.Equal(t, 15, s3Usage.Logs.PutRequests)
		assert.Equal(t, int64(3072), s3Usage.Logs.UploadBytes)
		assert.Equal(t, int64(2048), s3Usage.Logs.Agent.Bytes)
		assert.Equal(t, "project/task1/0/agent.log", s3Usage.Logs.Agent.LogKey)
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

	t.Run("WithNegativePutRequests", func(t *testing.T) {
		cost := CalculateS3PutCostWithConfig(-5, validConfig)
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

}

func TestStorageTierDays(t *testing.T) {
	for _, tc := range []struct {
		name         string
		tierInfo     StorageTierInfo
		wantStandard int
		wantIA       int
		wantArchive  int
	}{
		{
			name:         "ExpirationBeforeIAThresholdShouldBeAllStandard",
			tierInfo:     StorageTierInfo{ExpirationDays: 20},
			wantStandard: 20,
			wantIA:       0,
			wantArchive:  0,
		},
		{
			name:         "ExpirationBetweenThresholdsShouldSplitStandardAndIA",
			tierInfo:     StorageTierInfo{ExpirationDays: 60},
			wantStandard: 30,
			wantIA:       30,
			wantArchive:  0,
		},
		{
			name:         "ExpirationAtGlacierThresholdShouldHaveNoArchiveDays",
			tierInfo:     StorageTierInfo{ExpirationDays: 90},
			wantStandard: 30,
			wantIA:       60,
			wantArchive:  0,
		},
		{
			name:         "ExpirationAfterGlacierThresholdShouldSplitAllThreeTiers",
			tierInfo:     StorageTierInfo{ExpirationDays: 365},
			wantStandard: 30,
			wantIA:       60,
			wantArchive:  275,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			standard, ia, archive := storageTierDays(tc.tierInfo)
			assert.Equal(t, tc.wantStandard, standard)
			assert.Equal(t, tc.wantIA, ia)
			assert.Equal(t, tc.wantArchive, archive)
		})
	}
}

func TestCalculateS3StorageCostWithConfig(t *testing.T) {
	validConfig := &evergreen.CostConfig{
		S3Cost: evergreen.S3CostConfig{
			Storage: evergreen.S3StorageCostConfig{
				StandardStorageCostDiscount: 0.37,
				IAStorageCostDiscount:       0.312,
				ArchiveStorageCostDiscount:  0.265,
			},
		},
	}

	const GB = 1024 * 1024 * 1024

	t.Run("365DaysShouldSplitAcrossAllThreeTiers", func(t *testing.T) {
		cost := CalculateS3StorageCostWithConfig(t.Context(), GB, StorageTierInfo{ExpirationDays: 365}, validConfig)
		assert.Greater(t, cost, 0.0)
		standard := 30.0 * (0.023 / float64(GB) / 30.0) * (1 - 0.37)
		ia := 60.0 * (0.0125 / float64(GB) / 30.0) * (1 - 0.312)
		archive := 275.0 * (0.004 / float64(GB) / 30.0) * (1 - 0.265)
		expected := float64(GB) * (standard + ia + archive)
		assert.InDelta(t, expected, cost, 0.000001)
	})

	t.Run("180DaysShouldSplitAcrossAllThreeTiers", func(t *testing.T) {
		cost := CalculateS3StorageCostWithConfig(t.Context(), GB, StorageTierInfo{ExpirationDays: 180}, validConfig)
		standard := 30.0 * (0.023 / float64(GB) / 30.0) * (1 - 0.37)
		ia := 60.0 * (0.0125 / float64(GB) / 30.0) * (1 - 0.312)
		archive := 90.0 * (0.004 / float64(GB) / 30.0) * (1 - 0.265)
		expected := float64(GB) * (standard + ia + archive)
		assert.InDelta(t, expected, cost, 0.000001)
	})

	t.Run("90DaysShouldExcludeArchiveTier", func(t *testing.T) {
		cost := CalculateS3StorageCostWithConfig(t.Context(), GB, StorageTierInfo{ExpirationDays: 90}, validConfig)
		standard := 30.0 * (0.023 / float64(GB) / 30.0) * (1 - 0.37)
		ia := 60.0 * (0.0125 / float64(GB) / 30.0) * (1 - 0.312)
		expected := float64(GB) * (standard + ia)
		assert.InDelta(t, expected, cost, 0.000001)
	})

	t.Run("60DaysShouldExcludeArchiveTier", func(t *testing.T) {
		cost := CalculateS3StorageCostWithConfig(t.Context(), GB, StorageTierInfo{ExpirationDays: 60}, validConfig)
		standard := 30.0 * (0.023 / float64(GB) / 30.0) * (1 - 0.37)
		ia := 30.0 * (0.0125 / float64(GB) / 30.0) * (1 - 0.312)
		expected := float64(GB) * (standard + ia)
		assert.InDelta(t, expected, cost, 0.000001)
	})

	t.Run("ZeroBytesShouldReturnZero", func(t *testing.T) {
		cost := CalculateS3StorageCostWithConfig(t.Context(), 0, StorageTierInfo{ExpirationDays: 365}, validConfig)
		assert.Equal(t, 0.0, cost)
	})

	t.Run("ZeroExpirationDaysShouldReturnZero", func(t *testing.T) {
		cost := CalculateS3StorageCostWithConfig(t.Context(), GB, StorageTierInfo{}, validConfig)
		assert.Equal(t, 0.0, cost)
	})

	t.Run("NilConfigShouldReturnZero", func(t *testing.T) {
		cost := CalculateS3StorageCostWithConfig(t.Context(), GB, StorageTierInfo{ExpirationDays: 365}, nil)
		assert.Equal(t, 0.0, cost)
	})
}

func TestCalculateArtifactPutCosts(t *testing.T) {
	validConfig := &evergreen.CostConfig{
		S3Cost: evergreen.S3CostConfig{
			Upload: evergreen.S3UploadCostConfig{
				UploadCostDiscount: 0.0,
			},
		},
	}

	t.Run("ZeroArtifactsShouldReturnZero", func(t *testing.T) {
		var costs S3Costs
		calculateArtifactPutCosts(&costs, S3Usage{}, validConfig)
		assert.Equal(t, 0.0, costs.ArtifactPutCost)
		assert.Equal(t, 0.0, costs.AvgFilePutCost)
		assert.Equal(t, 0.0, costs.MaxFilePutCost)
		assert.Equal(t, 0.0, costs.MinFilePutCost)
	})

	t.Run("SingleFileShouldHaveEqualAvgMinMax", func(t *testing.T) {
		var costs S3Costs
		usage := S3Usage{
			Artifacts: ArtifactMetrics{
				S3UploadMetrics:            S3UploadMetrics{PutRequests: 3},
				Count:                      1,
				ArtifactWithMaxPutRequests: 3,
				ArtifactWithMinPutRequests: 3,
			},
		}
		calculateArtifactPutCosts(&costs, usage, validConfig)
		assert.Greater(t, costs.ArtifactPutCost, 0.0)
		assert.Equal(t, costs.AvgFilePutCost, costs.MaxFilePutCost)
		assert.Equal(t, costs.AvgFilePutCost, costs.MinFilePutCost)
	})

	t.Run("MultipleFilesWithDifferentPutsShouldComputeExtremes", func(t *testing.T) {
		var costs S3Costs
		usage := S3Usage{
			Artifacts: ArtifactMetrics{
				S3UploadMetrics:            S3UploadMetrics{PutRequests: 10},
				Count:                      3,
				ArtifactWithMaxPutRequests: 6,
				ArtifactWithMinPutRequests: 1,
			},
		}
		calculateArtifactPutCosts(&costs, usage, validConfig)
		expectedPutCost := float64(10) * S3PutRequestCost
		assert.InDelta(t, expectedPutCost, costs.ArtifactPutCost, 0.000001)
		assert.InDelta(t, expectedPutCost/3, costs.AvgFilePutCost, 0.000001)
		assert.Greater(t, costs.MaxFilePutCost, costs.MinFilePutCost)
		costPerPut := expectedPutCost / 10
		assert.InDelta(t, costPerPut*6, costs.MaxFilePutCost, 0.000001)
		assert.InDelta(t, costPerPut*1, costs.MinFilePutCost, 0.000001)
	})

	t.Run("NilConfigShouldReturnZero", func(t *testing.T) {
		var costs S3Costs
		usage := S3Usage{
			Artifacts: ArtifactMetrics{
				S3UploadMetrics: S3UploadMetrics{PutRequests: 10},
				Count:           3,
			},
		}
		calculateArtifactPutCosts(&costs, usage, nil)
		assert.Equal(t, 0.0, costs.ArtifactPutCost)
		assert.Equal(t, 0.0, costs.AvgFilePutCost)
		assert.Equal(t, 0.0, costs.MaxFilePutCost)
		assert.Equal(t, 0.0, costs.MinFilePutCost)
	})
}

func TestCalculateAllCosts(t *testing.T) {
	validConfig := &evergreen.CostConfig{
		S3Cost: evergreen.S3CostConfig{
			Upload: evergreen.S3UploadCostConfig{UploadCostDiscount: 0.0},
			Storage: evergreen.S3StorageCostConfig{
				DefaultMaxArtifactExpirationDays: 365,
			},
		},
	}
	const MB = 1024 * 1024

	t.Run("PutCostsCalculatedIndependentlyFromStorageCosts", func(t *testing.T) {
		usage := S3Usage{
			Artifacts: ArtifactMetrics{
				S3UploadMetrics:            S3UploadMetrics{PutRequests: 10, UploadBytes: int64(5 * MB)},
				Count:                      2,
				ArtifactWithMaxPutRequests: 7,
				ArtifactWithMinPutRequests: 3,
			},
			Logs: LogMetrics{S3UploadMetrics: S3UploadMetrics{PutRequests: 5}},
		}
		costs := CalculateAllCosts(t.Context(), usage, nil, validConfig)
		assert.Greater(t, costs.ArtifactPutCost, 0.0)
		assert.Greater(t, costs.LogPutCost, 0.0)
		assert.Greater(t, costs.MaxFilePutCost, costs.MinFilePutCost)
		assert.Equal(t, 0.0, costs.ArtifactStorageCost)
		assert.Equal(t, 0.0, costs.LogStorageCost)
	})

	t.Run("StorageCostsCalculatedWithTierInfo", func(t *testing.T) {
		fileKey := "project/task1/0/artifacts/binary.tar.gz"
		usage := S3Usage{
			Artifacts: ArtifactMetrics{
				S3UploadMetrics: S3UploadMetrics{PutRequests: 1},
				Count:           1,
				BytesByBucketAndKey: []BucketFileMetrics{
					{Bucket: "mciuploads", Files: []FileBytes{{FileKey: fileKey, Bytes: int64(5 * MB)}}},
				},
				ArtifactWithMaxPutRequests: 1,
				ArtifactWithMinPutRequests: 1,
			},
		}
		tierInfoByKey := map[string]StorageTierInfo{
			fileKey: {ExpirationDays: 90},
		}
		costs := CalculateAllCosts(t.Context(), usage, tierInfoByKey, validConfig)
		assert.Greater(t, costs.ArtifactPutCost, 0.0)
		assert.Greater(t, costs.ArtifactStorageCost, 0.0)
	})

	t.Run("NilConfigShouldReturnZeroCosts", func(t *testing.T) {
		usage := S3Usage{
			Artifacts: ArtifactMetrics{
				S3UploadMetrics: S3UploadMetrics{PutRequests: 10},
				Count:           1,
			},
		}
		costs := CalculateAllCosts(t.Context(), usage, nil, nil)
		assert.Equal(t, 0.0, costs.ArtifactPutCost)
		assert.Equal(t, 0.0, costs.LogPutCost)
		assert.Equal(t, 0.0, costs.ArtifactStorageCost)
		assert.Equal(t, 0.0, costs.LogStorageCost)
	})
}
