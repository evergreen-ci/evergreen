package s3usage

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
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

func putRequestsForFile(metrics []BucketFileMetrics, bucket, fileKey string) int {
	for _, b := range metrics {
		if b.Bucket == bucket {
			for _, f := range b.Files {
				if f.FileKey == fileKey {
					return f.PutRequests
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
			{RemotePath: "path/file1.txt", FileSizeBytes: 600, PutRequests: 2},
			{RemotePath: "path/file2.txt", FileSizeBytes: 424},
		}
		s3Usage.IncrementArtifacts(ArtifactIncrementOptions{PutRequests: 5, UploadBytes: 1024, FileCount: 2, MaxPuts: 3, MinPuts: 2, Bucket: "bucket-a", Files: filesA})
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
		s3Usage.IncrementArtifacts(ArtifactIncrementOptions{PutRequests: 10, UploadBytes: 2048, FileCount: 3, MaxPuts: 8, MinPuts: 1, Bucket: "bucket-b", Files: filesB})
		assert.Equal(t, 15, s3Usage.Artifacts.PutRequests)
		assert.Equal(t, int64(3072), s3Usage.Artifacts.UploadBytes)
		assert.Equal(t, 5, s3Usage.Artifacts.Count)
		assert.Equal(t, 8, s3Usage.Artifacts.ArtifactWithMaxPutRequests)
		assert.Equal(t, 1, s3Usage.Artifacts.ArtifactWithMinPutRequests)
		require.True(t, hasBucket(s3Usage.Artifacts.BytesByBucketAndKey, "bucket-b"))
		assert.Equal(t, int64(600), bytesForFile(s3Usage.Artifacts.BytesByBucketAndKey, "bucket-a", "path/file1.txt"), "bucket-a file bytes should be unchanged")
		assert.Equal(t, int64(2048), bytesForFile(s3Usage.Artifacts.BytesByBucketAndKey, "bucket-b", "other/file3.txt"))

		filesA2 := []FileMetrics{
			{RemotePath: "path/file1.txt", FileSizeBytes: 512, PutRequests: 3},
		}
		s3Usage.IncrementArtifacts(ArtifactIncrementOptions{PutRequests: 3, UploadBytes: 512, FileCount: 1, MaxPuts: 3, MinPuts: 3, Bucket: "bucket-a", Files: filesA2})
		assert.Equal(t, int64(512), bytesForFile(s3Usage.Artifacts.BytesByBucketAndKey, "bucket-a", "path/file1.txt"), "bucket-a file bytes should be replaced by the latest upload's size (S3 overwrite semantics)")
		assert.Equal(t, 5, putRequestsForFile(s3Usage.Artifacts.BytesByBucketAndKey, "bucket-a", "path/file1.txt"))
	})

	t.Run("IncrementArtifactsSkipsWhenDevprodAllowlistSetAndAccountNotOwned", func(t *testing.T) {
		s3Usage := S3Usage{}
		owned := []string{"123456789012"}
		s3Usage.IncrementArtifacts(ArtifactIncrementOptions{
			DevprodOwnedAWSAccountIDs: owned,
			PutRequests:               10,
			UploadBytes:               100,
			FileCount:                 1,
			MaxPuts:                   3,
			MinPuts:                   3,
			Bucket:                    "b",
			AWSRoleARN:                "arn:aws:iam::999999999999:role/r",
			Files:                     []FileMetrics{{RemotePath: "x", FileSizeBytes: 100}},
		})
		assert.Equal(t, 0, s3Usage.Artifacts.PutRequests)
		assert.True(t, s3Usage.IsZero())

		s3Usage.IncrementArtifacts(ArtifactIncrementOptions{
			DevprodOwnedAWSAccountIDs: owned,
			PutRequests:               5,
			UploadBytes:               50,
			FileCount:                 1,
			MaxPuts:                   2,
			MinPuts:                   2,
			Bucket:                    "b2",
			AWSRoleARN:                "arn:aws:iam::123456789012:role/r",
			Files:                     []FileMetrics{{RemotePath: "y", FileSizeBytes: 50}},
		})
		assert.Equal(t, 5, s3Usage.Artifacts.PutRequests)
		assert.False(t, s3Usage.IsZero())
	})

	t.Run("IncrementArtifactsWithAccountIDOwnedShouldRecord", func(t *testing.T) {
		s3Usage := S3Usage{}
		owned := []string{"123456789012"}
		s3Usage.IncrementArtifacts(ArtifactIncrementOptions{
			DevprodOwnedAWSAccountIDs: owned,
			PutRequests:               7,
			UploadBytes:               70,
			FileCount:                 1,
			MaxPuts:                   7,
			MinPuts:                   7,
			Bucket:                    "b",
			AWSAccountID:              "123456789012",
			Files:                     []FileMetrics{{RemotePath: "z", FileSizeBytes: 70}},
		})
		assert.Equal(t, 7, s3Usage.Artifacts.PutRequests)
		assert.False(t, s3Usage.IsZero())
	})

	t.Run("IncrementArtifactsWithAccountIDUnownedShouldSkip", func(t *testing.T) {
		s3Usage := S3Usage{}
		owned := []string{"123456789012"}
		s3Usage.IncrementArtifacts(ArtifactIncrementOptions{
			DevprodOwnedAWSAccountIDs: owned,
			PutRequests:               7,
			UploadBytes:               70,
			FileCount:                 1,
			MaxPuts:                   7,
			MinPuts:                   7,
			Bucket:                    "b",
			AWSAccountID:              "999999999999",
			Files:                     []FileMetrics{{RemotePath: "z", FileSizeBytes: 70}},
		})
		assert.Equal(t, 0, s3Usage.Artifacts.PutRequests)
		assert.True(t, s3Usage.IsZero())
	})

	t.Run("IncrementArtifactsWithNoARNAndNoAccountIDShouldSkip", func(t *testing.T) {
		s3Usage := S3Usage{}
		owned := []string{"123456789012"}
		s3Usage.IncrementArtifacts(ArtifactIncrementOptions{
			DevprodOwnedAWSAccountIDs: owned,
			PutRequests:               5,
			UploadBytes:               50,
			FileCount:                 1,
			MaxPuts:                   5,
			MinPuts:                   5,
			Bucket:                    "b",
			Files:                     []FileMetrics{{RemotePath: "z", FileSizeBytes: 50}},
		})
		assert.Equal(t, 0, s3Usage.Artifacts.PutRequests)
		assert.True(t, s3Usage.IsZero())
	})

	t.Run("IncrementLogs", func(t *testing.T) {
		s3Usage := S3Usage{}
		assert.Equal(t, 0, s3Usage.Logs.PutRequests)
		assert.Equal(t, int64(0), s3Usage.Logs.UploadBytes)

		s3Usage.IncrementLogs(5, 1024, "", "")
		assert.Equal(t, 5, s3Usage.Logs.PutRequests)
		assert.Equal(t, int64(1024), s3Usage.Logs.UploadBytes)

		s3Usage.IncrementLogs(10, 2048, "", "")
		assert.Equal(t, 15, s3Usage.Logs.PutRequests)
		assert.Equal(t, int64(3072), s3Usage.Logs.UploadBytes)
	})

	t.Run("IncrementLogsPerTypeBytesAndKey", func(t *testing.T) {
		s3Usage := S3Usage{}

		s3Usage.IncrementLogs(1, 100, LogTypeTask, "proj/task/0/task_logs/task")
		s3Usage.IncrementLogs(2, 200, LogTypeAgent, "proj/task/0/task_logs/agent")
		s3Usage.IncrementLogs(3, 300, LogTypeSystem, "proj/task/0/task_logs/system")
		s3Usage.IncrementLogs(4, 400, LogTypeTest, "proj/task/0/test_logs/mytest")

		assert.Equal(t, 10, s3Usage.Logs.PutRequests)
		assert.Equal(t, int64(1000), s3Usage.Logs.UploadBytes)

		assert.Equal(t, int64(100), s3Usage.Logs.Task.Bytes)
		assert.Equal(t, 1, s3Usage.Logs.Task.PutRequests)
		assert.Equal(t, "proj/task/0/task_logs/task", s3Usage.Logs.Task.LogKey)
		assert.Equal(t, int64(200), s3Usage.Logs.Agent.Bytes)
		assert.Equal(t, 2, s3Usage.Logs.Agent.PutRequests)
		assert.Equal(t, "proj/task/0/task_logs/agent", s3Usage.Logs.Agent.LogKey)
		assert.Equal(t, int64(300), s3Usage.Logs.System.Bytes)
		assert.Equal(t, 3, s3Usage.Logs.System.PutRequests)
		assert.Equal(t, "proj/task/0/task_logs/system", s3Usage.Logs.System.LogKey)
		assert.Equal(t, int64(400), s3Usage.Logs.Test.Bytes)
		assert.Equal(t, 4, s3Usage.Logs.Test.PutRequests)
		assert.Equal(t, "proj/task/0/test_logs/mytest", s3Usage.Logs.Test.LogKey)

		// Bytes and PUT counts accumulate across multiple calls; key is overwritten only when non-empty.
		s3Usage.IncrementLogs(1, 50, LogTypeTest, "")
		assert.Equal(t, int64(450), s3Usage.Logs.Test.Bytes)
		assert.Equal(t, 5, s3Usage.Logs.Test.PutRequests)
		assert.Equal(t, "proj/task/0/test_logs/mytest", s3Usage.Logs.Test.LogKey, "key should not be overwritten by empty string")
	})

	t.Run("NilReceiverIsZero", func(t *testing.T) {
		var s3Usage *S3Usage
		assert.True(t, s3Usage.IsZero())
	})

	t.Run("ConcurrentIncrementLogsIsRaceFree", func(t *testing.T) {
		s3Usage := S3Usage{}
		s3Usage.Init()

		const goroutines = 8
		const callsPerGoroutine = 100
		var wg sync.WaitGroup
		wg.Add(goroutines)
		for i := 0; i < goroutines; i++ {
			go func() {
				for j := 0; j < callsPerGoroutine; j++ {
					s3Usage.IncrementLogs(1, 10, LogTypeTask, "proj/task/0/task_logs/task")
				}
				wg.Done()
			}()
		}
		wg.Wait()

		expectedPuts := goroutines * callsPerGoroutine
		expectedBytes := int64(expectedPuts * 10)
		assert.Equal(t, expectedPuts, s3Usage.Logs.PutRequests)
		assert.Equal(t, expectedBytes, s3Usage.Logs.UploadBytes)
		assert.Equal(t, expectedBytes, s3Usage.Logs.Task.Bytes)
	})

	t.Run("ConcurrentIncrementArtifactsIsRaceFree", func(t *testing.T) {
		s3Usage := S3Usage{}
		s3Usage.Init()
		owned := []string{"123456789012"}

		const goroutines = 8
		const callsPerGoroutine = 50
		var wg sync.WaitGroup
		wg.Add(goroutines)
		for i := 0; i < goroutines; i++ {
			go func(id int) {
				for j := 0; j < callsPerGoroutine; j++ {
					remotePath := "shared/key.bin"
					if id%2 == 1 {
						remotePath = "unique/" + string(rune('A'+id)) + ".bin"
					}
					s3Usage.IncrementArtifacts(ArtifactIncrementOptions{
						DevprodOwnedAWSAccountIDs: owned,
						AWSAccountID:              "123456789012",
						PutRequests:               1,
						UploadBytes:               100,
						FileCount:                 1,
						MaxPuts:                   1,
						MinPuts:                   1,
						Bucket:                    "bucket-a",
						Files:                     []FileMetrics{{RemotePath: remotePath, FileSizeBytes: 100}},
					})
				}
				wg.Done()
			}(i)
		}
		wg.Wait()

		expectedCalls := goroutines * callsPerGoroutine
		assert.Equal(t, expectedCalls, s3Usage.Artifacts.PutRequests)
		assert.Equal(t, int64(expectedCalls*100), s3Usage.Artifacts.UploadBytes)
		assert.Equal(t, expectedCalls, s3Usage.Artifacts.Count)
	})

	t.Run("SnapshotWithoutInitLogsCritical", func(t *testing.T) {
		s3Usage := S3Usage{}
		s3Usage.IncrementLogs(5, 1024, LogTypeTask, "key")

		sender := grip.GetSender()
		mock := send.MakeInternalLogger()
		require.NoError(t, grip.SetSender(mock))
		t.Cleanup(func() {
			assert.NoError(t, grip.SetSender(sender))
		})

		snap := s3Usage.Snapshot()
		assert.Equal(t, 5, snap.Logs.PutRequests)
		assert.True(t, mock.HasMessage(), "expected critical message when Snapshot called without Init")
	})
}

func TestBuildFileMetrics(t *testing.T) {
	t.Run("FileExistsShouldReturnCorrectMetrics", func(t *testing.T) {
		tmpDir := t.TempDir()
		localPath := filepath.Join(tmpDir, "test.txt")
		require.NoError(t, os.WriteFile(localPath, []byte("hello world"), 0644))
		fi, err := os.Stat(localPath)
		require.NoError(t, err)
		expectedSize := fi.Size()

		metrics, fileSize := BuildFileMetrics(grip.NewJournaler("test"), localPath, "remote/path/test.txt", 3)
		assert.Equal(t, localPath, metrics.LocalPath)
		assert.Equal(t, "remote/path/test.txt", metrics.RemotePath)
		assert.Equal(t, expectedSize, metrics.FileSizeBytes)
		assert.Equal(t, 3, metrics.PutRequests)
		assert.Equal(t, expectedSize, fileSize)
	})
	t.Run("FileNotExistShouldReturnZeroSize", func(t *testing.T) {
		metrics, fileSize := BuildFileMetrics(grip.NewJournaler("test"), "/nonexistent/path/file.txt", "remote/path/file.txt", 1)
		assert.Equal(t, "/nonexistent/path/file.txt", metrics.LocalPath)
		assert.Equal(t, "remote/path/file.txt", metrics.RemotePath)
		assert.Equal(t, int64(0), metrics.FileSizeBytes)
		assert.Equal(t, 1, metrics.PutRequests)
		assert.Equal(t, int64(0), fileSize)
	})
}

func TestComputePerFileExtremes(t *testing.T) {
	t.Run("EmptyInputReturnsZero", func(t *testing.T) {
		maxPuts, minPuts := ComputePerFileExtremes(nil)
		assert.Zero(t, maxPuts)
		assert.Zero(t, minPuts)
	})
	t.Run("SingleFileReturnsSameMaxAndMin", func(t *testing.T) {
		files := []FileMetrics{
			{PutRequests: 5},
		}
		maxPuts, minPuts := ComputePerFileExtremes(files)
		assert.Equal(t, 5, maxPuts)
		assert.Equal(t, 5, minPuts)
	})
	t.Run("MultipleFilesReturnsCorrectExtremes", func(t *testing.T) {
		files := []FileMetrics{
			{PutRequests: 3},
			{PutRequests: 10},
			{PutRequests: 1},
			{PutRequests: 7},
		}
		maxPuts, minPuts := ComputePerFileExtremes(files)
		assert.Equal(t, 10, maxPuts)
		assert.Equal(t, 1, minPuts)
	})
	t.Run("AllSameValueReturnsSameMaxAndMin", func(t *testing.T) {
		files := []FileMetrics{
			{PutRequests: 4},
			{PutRequests: 4},
			{PutRequests: 4},
		}
		maxPuts, minPuts := ComputePerFileExtremes(files)
		assert.Equal(t, 4, maxPuts)
		assert.Equal(t, 4, minPuts)
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
		standard, adjusted := CalculateS3PutCostWithConfig(1000, validConfig)
		assert.InDelta(t, 0.005, standard, 0.000001)
		assert.InDelta(t, 0.0035, adjusted, 0.000001)
		assert.Greater(t, standard, adjusted)
	})

	t.Run("WithNilConfig", func(t *testing.T) {
		standard, adjusted := CalculateS3PutCostWithConfig(1000, nil)
		assert.InDelta(t, 0.005, standard, 0.000001)
		assert.Equal(t, 0.0, adjusted)
	})

	t.Run("WithZeroPutRequests", func(t *testing.T) {
		standard, adjusted := CalculateS3PutCostWithConfig(0, validConfig)
		assert.Equal(t, 0.0, standard)
		assert.Equal(t, 0.0, adjusted)
	})

	t.Run("WithNegativePutRequests", func(t *testing.T) {
		standard, adjusted := CalculateS3PutCostWithConfig(-5, validConfig)
		assert.Equal(t, 0.0, standard)
		assert.Equal(t, 0.0, adjusted)
	})

	t.Run("WithInvalidDiscount", func(t *testing.T) {
		invalidConfig := &evergreen.CostConfig{
			S3Cost: evergreen.S3CostConfig{
				Upload: evergreen.S3UploadCostConfig{
					UploadCostDiscount: 1.5,
				},
			},
		}
		standard, adjusted := CalculateS3PutCostWithConfig(1000, invalidConfig)
		assert.InDelta(t, 0.005, standard, 0.000001)
		assert.Equal(t, 0.0, adjusted)
	})
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

	t.Run("DefaultArtifacts365Days", func(t *testing.T) {
		// ExpirationDays=365: Standard=30, IA=60, Archive=275
		standard, adjusted := CalculateS3StorageCostWithConfig(t.Context(), GB, 365, validConfig)
		assert.Greater(t, standard, 0.0)
		assert.Greater(t, adjusted, 0.0)
		assert.Greater(t, standard, adjusted)
		stdTier := 30.0 * (0.023 / float64(GB) / 30.0) * (1 - 0.37)
		iaTier := 60.0 * (0.0125 / float64(GB) / 30.0) * (1 - 0.312)
		archiveTier := 275.0 * (0.004 / float64(GB) / 30.0) * (1 - 0.265)
		expectedAdj := float64(GB) * (stdTier + iaTier + archiveTier)
		assert.InDelta(t, expectedAdj, adjusted, 0.000001)
	})

	t.Run("MongoDBMongoArtifacts90Days", func(t *testing.T) {
		// ExpirationDays=90: Standard=30, IA=60, Archive=0
		standard, adjusted := CalculateS3StorageCostWithConfig(t.Context(), GB, 90, validConfig)
		assert.Greater(t, standard, adjusted)
		stdTier := 30.0 * (0.023 / float64(GB) / 30.0) * (1 - 0.37)
		iaTier := 60.0 * (0.0125 / float64(GB) / 30.0) * (1 - 0.312)
		expectedAdj := float64(GB) * (stdTier + iaTier)
		assert.InDelta(t, expectedAdj, adjusted, 0.000001)
	})

	t.Run("MongoSyncArtifacts180Days", func(t *testing.T) {
		// ExpirationDays=180: Standard=30, IA=60, Archive=90
		standard, adjusted := CalculateS3StorageCostWithConfig(t.Context(), GB, 180, validConfig)
		assert.Greater(t, standard, adjusted)
		stdTier := 30.0 * (0.023 / float64(GB) / 30.0) * (1 - 0.37)
		iaTier := 60.0 * (0.0125 / float64(GB) / 30.0) * (1 - 0.312)
		archiveTier := 90.0 * (0.004 / float64(GB) / 30.0) * (1 - 0.265)
		expectedAdj := float64(GB) * (stdTier + iaTier + archiveTier)
		assert.InDelta(t, expectedAdj, adjusted, 0.000001)
	})

	t.Run("DefaultLog60Days", func(t *testing.T) {
		// ExpirationDays=60: Standard=30, IA=30, Archive=0
		standard, adjusted := CalculateS3StorageCostWithConfig(t.Context(), GB, 60, validConfig)
		assert.Greater(t, standard, adjusted)
		stdTier := 30.0 * (0.023 / float64(GB) / 30.0) * (1 - 0.37)
		iaTier := 30.0 * (0.0125 / float64(GB) / 30.0) * (1 - 0.312)
		expectedAdj := float64(GB) * (stdTier + iaTier)
		assert.InDelta(t, expectedAdj, adjusted, 0.000001)
	})

	t.Run("FailedLog180Days", func(t *testing.T) {
		// ExpirationDays=180: Standard=30, IA=60, Archive=90
		standard, adjusted := CalculateS3StorageCostWithConfig(t.Context(), GB, 180, validConfig)
		assert.Greater(t, standard, 0.0)
		assert.Greater(t, adjusted, 0.0)
		assert.Greater(t, standard, adjusted)
	})

	t.Run("LongRetentionLog365Days", func(t *testing.T) {
		// ExpirationDays=365: Standard=30, IA=60, Archive=275
		standard, adjusted := CalculateS3StorageCostWithConfig(t.Context(), GB, 365, validConfig)
		assert.Greater(t, standard, 0.0)
		assert.Greater(t, adjusted, 0.0)
		assert.Greater(t, standard, adjusted)
	})

	t.Run("ZeroBytes", func(t *testing.T) {
		standard, adjusted := CalculateS3StorageCostWithConfig(t.Context(), 0, 365, validConfig)
		assert.Equal(t, 0.0, standard)
		assert.Equal(t, 0.0, adjusted)
	})

	t.Run("ZeroExpirationDays", func(t *testing.T) {
		standard, adjusted := CalculateS3StorageCostWithConfig(t.Context(), GB, 0, validConfig)
		assert.Equal(t, 0.0, standard)
		assert.Equal(t, 0.0, adjusted)
	})

	t.Run("NilConfig", func(t *testing.T) {
		standard, adjusted := CalculateS3StorageCostWithConfig(t.Context(), GB, 365, nil)
		assert.Greater(t, standard, 0.0)
		assert.Equal(t, 0.0, adjusted)
	})
}
