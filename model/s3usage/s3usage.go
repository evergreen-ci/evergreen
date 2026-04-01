package s3usage

import (
	"context"
	"os"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

// S3Usage tracks S3 API usage for cost calculation.
type S3Usage struct {
	Artifacts ArtifactMetrics `bson:"artifacts,omitempty" json:"artifacts,omitempty"`
	Logs      S3UploadMetrics `bson:"logs,omitempty" json:"logs,omitempty"`
}

// S3UploadMetrics tracks common S3 upload metrics shared across upload types.
type S3UploadMetrics struct {
	PutRequests int   `bson:"put_requests,omitempty" json:"put_requests,omitempty"`
	UploadBytes int64 `bson:"upload_bytes,omitempty" json:"upload_bytes,omitempty"`
}

// ArtifactMetrics tracks artifact upload metrics with an additional file count.
type ArtifactMetrics struct {
	S3UploadMetrics `bson:",inline"`
	// Count is the total number of artifacts uploaded per task.
	Count int `bson:"count,omitempty" json:"count,omitempty"`
	// ArtifactWithMaxPutRequests is the highest PUT request count for a single artifact across all s3.put invocations per task.
	ArtifactWithMaxPutRequests int `bson:"max_put_requests_per_file,omitempty" json:"max_put_requests_per_file,omitempty"`
	// ArtifactWithMinPutRequests is the lowest PUT request count for a single artifact across all s3.put invocations per task.
	ArtifactWithMinPutRequests int `bson:"min_put_requests_per_file,omitempty" json:"min_put_requests_per_file,omitempty"`
}

// FileMetrics contains metrics for a single uploaded file.
type FileMetrics struct {
	LocalPath     string
	RemotePath    string
	FileSizeBytes int64
	PutRequests   int
}

type S3BucketType string
type S3UploadMethod string

const (
	S3PutRequestCost = 0.000005
	S3PartSize       = 5 * 1024 * 1024 // 5 MB in bytes - S3 multipart upload threshold

	S3BucketTypeSmall S3BucketType = "small"
	S3BucketTypeLarge S3BucketType = "large"

	S3UploadMethodWriter S3UploadMethod = "writer"
	S3UploadMethodPut    S3UploadMethod = "put"
	S3UploadMethodCopy   S3UploadMethod = "copy"

	// S3 Intelligent Tiering pricing and transition thresholds.
	S3StandardPricePerGBMonth = 0.023
	S3IAPricePerGBMonth       = 0.0125
	S3ArchivePricePerGBMonth  = 0.004
	S3TransitionToIADays      = 30
	S3TransitionToArchiveDays = 90
	S3BytesPerGB              = 1024 * 1024 * 1024
	S3DaysPerMonth            = 30.0
)

// CalculateUploadMetrics populates file size and PUT requests for each uploaded file.
// Returns the populated metrics plus aggregate totals.
// If any file stat fails, logs a warning and uses zero values for that file.
func CalculateUploadMetrics(
	logger grip.Journaler,
	files []FileMetrics,
	bucketType S3BucketType,
	method S3UploadMethod,
) (populatedFiles []FileMetrics, totalSize int64, totalPuts int) {
	populatedFiles = make([]FileMetrics, len(files))

	for i, file := range files {
		fileInfo, err := os.Stat(file.LocalPath)
		if err != nil {
			logger.Warningf(context.Background(), "Unable to calculate file size and PUT requests for '%s' after successful upload: %s. Using zero values for metadata.", file.LocalPath, err)
			populatedFiles[i] = FileMetrics{
				LocalPath:     file.LocalPath,
				RemotePath:    file.RemotePath,
				FileSizeBytes: 0,
				PutRequests:   0,
			}
			continue
		}

		fileSize := fileInfo.Size()
		putRequests := CalculatePutRequestsWithContext(bucketType, method, fileSize)

		populatedFiles[i] = FileMetrics{
			LocalPath:     file.LocalPath,
			RemotePath:    file.RemotePath,
			FileSizeBytes: fileSize,
			PutRequests:   putRequests,
		}

		totalSize += fileSize
		totalPuts += putRequests
	}

	return populatedFiles, totalSize, totalPuts
}

// CalculatePutRequestsWithContext returns the number of S3 PUT API calls
// needed to upload a file based on bucket type, upload method, and file size.
func CalculatePutRequestsWithContext(bucketType S3BucketType, method S3UploadMethod, fileSize int64) int {
	if fileSize <= 0 {
		return 0
	}

	switch method {
	case S3UploadMethodCopy:
		return 1

	case S3UploadMethodWriter:
		if bucketType == S3BucketTypeSmall {
			return 1
		}
		// Large bucket Writer uses multipart for all sizes, <= 5MB is simple multipart (3 PUTs)
		if fileSize <= S3PartSize {
			return 3
		}
		numParts := int((fileSize + S3PartSize - 1) / S3PartSize)
		return 1 + numParts + 1

	case S3UploadMethodPut:
		// AWS SDK uses single PUT for < 5MB, multipart for >= 5MB
		if fileSize < S3PartSize {
			return 1
		}
		numParts := int((fileSize + S3PartSize - 1) / S3PartSize)
		return 1 + numParts + 1

	default:
		return 0
	}
}

// CalculateS3PutCostWithConfig calculates the S3 PUT request cost.
// Returns 0 if cost cannot be calculated due to missing or invalid config.
func CalculateS3PutCostWithConfig(putRequests int, costConfig *evergreen.CostConfig) float64 {
	if putRequests <= 0 {
		grip.Warning(context.Background(), message.Fields{
			"message":      "no put requests to calculate cost",
			"put_requests": putRequests,
		})
		return 0.0
	}

	if costConfig == nil {
		grip.Warning(context.Background(), message.Fields{
			"message": "cost config is not available to calculate S3 PUT cost",
		})
		return 0.0
	}

	discount := costConfig.S3Cost.Upload.UploadCostDiscount
	if discount < 0.0 || discount > 1.0 {
		grip.Warning(context.Background(), message.Fields{
			"message":  "invalid S3 upload cost discount",
			"discount": discount,
		})
		return 0.0
	}

	return float64(putRequests) * S3PutRequestCost * (1 - discount)
}

// CalculateS3StorageCostWithConfig calculates the S3 storage cost for uploadBytes over their retention period
// using the bucket's Intelligent Tiering schedule. expirationDays must be positive; buckets without a
// lifecycle expiration policy have no defined retention period and cannot have their cost calculated, so
// this function returns 0 for them. Returns 0 if config is nil.
func CalculateS3StorageCostWithConfig(ctx context.Context, uploadBytes int64, expirationDays int, costConfig *evergreen.CostConfig) float64 {
	if uploadBytes <= 0 {
		return 0.0
	}
	// TODO (DEVPROD-26465): callers must always supply a positive expirationDays. Use artifactExpirationDays
	// as the minimum fallback so this guard is never reached.
	if expirationDays <= 0 {
		grip.Warning(ctx, message.Fields{
			"message": "expiration days not configured, cannot calculate S3 storage cost",
		})
		return 0.0
	}
	if costConfig == nil {
		grip.Warning(ctx, message.Fields{
			"message": "cost config is not available to calculate S3 storage cost",
		})
		return 0.0
	}

	standardDiscount := costConfig.S3Cost.Storage.StandardStorageCostDiscount
	iaDiscount := costConfig.S3Cost.Storage.IAStorageCostDiscount
	archiveDiscount := costConfig.S3Cost.Storage.ArchiveStorageCostDiscount

	// Each variable represents how many days the object spends in that Intelligent Tiering tier:
	// Standard (days 0–30), Infrequent Access (days 30–90), Archive (days 90+).
	daysInStandard := min(expirationDays, S3TransitionToIADays)
	daysInIA := max(0, min(expirationDays, S3TransitionToArchiveDays)-S3TransitionToIADays)
	daysInArchive := max(0, expirationDays-S3TransitionToArchiveDays)

	pricePerBytePerDay := func(pricePerGBMonth float64) float64 {
		return pricePerGBMonth / S3BytesPerGB / S3DaysPerMonth
	}

	standardCost := float64(daysInStandard) * pricePerBytePerDay(S3StandardPricePerGBMonth) * (1 - standardDiscount)
	iaCost := float64(daysInIA) * pricePerBytePerDay(S3IAPricePerGBMonth) * (1 - iaDiscount)
	archiveCost := float64(daysInArchive) * pricePerBytePerDay(S3ArchivePricePerGBMonth) * (1 - archiveDiscount)

	return float64(uploadBytes) * (standardCost + iaCost + archiveCost)
}

// IncrementArtifacts increments the artifact upload metrics (from s3.put commands).
// maxPuts and minPuts are the per-file extremes from this s3.put invocation.
func (s *S3Usage) IncrementArtifacts(putRequests int, uploadBytes int64, fileCount int, maxPuts int, minPuts int) {
	s.Artifacts.PutRequests += putRequests
	s.Artifacts.UploadBytes += uploadBytes
	s.Artifacts.Count += fileCount

	if maxPuts > s.Artifacts.ArtifactWithMaxPutRequests {
		s.Artifacts.ArtifactWithMaxPutRequests = maxPuts
	}
	if s.Artifacts.ArtifactWithMinPutRequests == 0 || minPuts < s.Artifacts.ArtifactWithMinPutRequests {
		s.Artifacts.ArtifactWithMinPutRequests = minPuts
	}
}

// IncrementLogs increments the log chunk upload metrics.
func (s *S3Usage) IncrementLogs(putRequests int, uploadBytes int64) {
	s.Logs.PutRequests += putRequests
	s.Logs.UploadBytes += uploadBytes
}

// IsZero implements bsoncodec.Zeroer for BSON marshalling.
func (s *S3Usage) IsZero() bool {
	if s == nil {
		return true
	}
	return s.Artifacts.PutRequests == 0 && s.Artifacts.UploadBytes == 0 && s.Artifacts.Count == 0 &&
		s.Artifacts.ArtifactWithMaxPutRequests == 0 && s.Artifacts.ArtifactWithMinPutRequests == 0 &&
		s.Logs.PutRequests == 0 && s.Logs.UploadBytes == 0
}
