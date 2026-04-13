package s3usage

import (
	"context"
	"math"
	"os"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

// S3Usage tracks S3 API usage for cost calculation.
type S3Usage struct {
	Artifacts ArtifactMetrics `bson:"artifacts,omitempty" json:"artifacts,omitempty"`
	Logs      LogMetrics      `bson:"logs,omitempty" json:"logs,omitempty"`
}

// S3UploadMetrics tracks common S3 upload metrics shared across upload types.
type S3UploadMetrics struct {
	PutRequests int   `bson:"put_requests,omitempty" json:"put_requests,omitempty"`
	UploadBytes int64 `bson:"upload_bytes,omitempty" json:"upload_bytes,omitempty"`
}

// LogMetrics tracks aggregate log upload metrics and per-type breakdown.
type LogMetrics struct {
	S3UploadMetrics `bson:",inline"`
	Task            LogTypeMetrics `bson:"task_log,omitempty" json:"task_log,omitempty"`
	Agent           LogTypeMetrics `bson:"agent_log,omitempty" json:"agent_log,omitempty"`
	System          LogTypeMetrics `bson:"system_log,omitempty" json:"system_log,omitempty"`
}

// LogTypeMetrics tracks upload bytes and S3 key for a single log type.
type LogTypeMetrics struct {
	LogKey string `bson:"log_key,omitempty" json:"log_key,omitempty"`
	Bytes  int64  `bson:"bytes,omitempty" json:"bytes,omitempty"`
}

// BucketFileMetrics groups per-file byte metrics for a single S3 bucket.
type BucketFileMetrics struct {
	Bucket string      `bson:"bucket" json:"bucket"`
	Files  []FileBytes `bson:"files" json:"files"`
}

// FileBytes tracks bytes uploaded for a single S3 file key.
type FileBytes struct {
	FileKey string `bson:"file_key" json:"file_key"`
	Bytes   int64  `bson:"bytes" json:"bytes"`
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
	// BytesByBucketAndKey groups per-file byte metrics by S3 bucket.
	BytesByBucketAndKey []BucketFileMetrics `bson:"bytes_by_bucket_and_key,omitempty" json:"bytes_by_bucket_and_key,omitempty"`
}

// FileMetrics contains metrics for a single uploaded file.
type FileMetrics struct {
	LocalPath     string
	RemotePath    string
	FileSizeBytes int64
	PutRequests   int
}

// BucketFile is a flat representation of a single artifact file with its bucket.
type BucketFile struct {
	Bucket  string
	FileKey string
	Bytes   int64
}

// ArtifactUpload holds the metrics for a single artifact upload event.
type ArtifactUpload struct {
	PutRequests int
	UploadBytes int64
	FileCount   int
	MaxPuts     int
	MinPuts     int
	Bucket      string
	Files       []FileMetrics
}

// StorageTierInfo holds the S3 lifecycle expiration configuration for a bucket rule.
type StorageTierInfo struct {
	ExpirationDays int
}

// S3Costs holds the calculated S3 cost breakdown for a task.
type S3Costs struct {
	ArtifactPutCost        float64
	LogPutCost             float64
	ArtifactStorageCost    float64
	LogStorageCost         float64
	AvgFilePutCost         float64
	MaxFilePutCost         float64
	MinFilePutCost         float64
	AvgArtifactStorageCost float64
	MinArtifactStorageCost float64
	MaxArtifactStorageCost float64
	AvgLogStorageCost      float64
	MinLogStorageCost      float64
	MaxLogStorageCost      float64
}

type S3BucketType string
type S3UploadMethod string

const (
	LogTypeTask   = "task_log"
	LogTypeAgent  = "agent_log"
	LogTypeSystem = "system_log"

	S3PutRequestCost = 0.000005
	S3PartSize       = 5 * 1024 * 1024 // 5 MB in bytes - S3 multipart upload threshold

	S3BucketTypeSmall S3BucketType = "small"
	S3BucketTypeLarge S3BucketType = "large"

	S3UploadMethodWriter S3UploadMethod = "writer"
	S3UploadMethodPut    S3UploadMethod = "put"
	S3UploadMethodCopy   S3UploadMethod = "copy"

	// S3 Intelligent Tiering pricing constants and tier transition thresholds.
	// Transition days (30, 90) are defined by AWS S3 Intelligent Tiering:
	// https://aws.amazon.com/s3/storage-classes/intelligent-tiering/
	S3StandardPricePerGBMonth = 0.023
	S3IAPricePerGBMonth       = 0.0125
	S3ArchivePricePerGBMonth  = 0.004
	S3BytesPerGB              = 1024 * 1024 * 1024
	S3DaysPerMonth            = 30.0

	// S3ITDefaultTransitionToIADays and S3ITDefaultTransitionToGlacierDays are the implicit
	// tier transition thresholds for S3 Intelligent Tiering. All Evergreen S3 uploads use
	// the IT storage class, so these defaults always apply regardless of lifecycle rule configuration.
	S3ITDefaultTransitionToIADays      = 30
	S3ITDefaultTransitionToGlacierDays = 90
)

// CalculateAllCosts calculates all S3 PUT and storage costs for the given usage.
func CalculateAllCosts(ctx context.Context, usage S3Usage, tierInfoByKey map[string]StorageTierInfo, costConfig *evergreen.CostConfig) S3Costs {
	var costs S3Costs
	calculateArtifactPutCosts(&costs, usage, costConfig)
	costs.LogPutCost = CalculateS3PutCostWithConfig(usage.Logs.PutRequests, costConfig)
	calculateArtifactStorageCosts(ctx, &costs, usage, tierInfoByKey, costConfig)
	calculateLogStorageCosts(ctx, &costs, usage, tierInfoByKey, costConfig)
	return costs
}

func calculateArtifactPutCosts(costs *S3Costs, usage S3Usage, costConfig *evergreen.CostConfig) {
	costs.ArtifactPutCost = CalculateS3PutCostWithConfig(usage.Artifacts.PutRequests, costConfig)
	if usage.Artifacts.Count > 0 && usage.Artifacts.PutRequests > 0 {
		costPerPut := costs.ArtifactPutCost / float64(usage.Artifacts.PutRequests)
		costs.AvgFilePutCost = costs.ArtifactPutCost / float64(usage.Artifacts.Count)
		costs.MaxFilePutCost = costPerPut * float64(usage.Artifacts.ArtifactWithMaxPutRequests)
		costs.MinFilePutCost = costPerPut * float64(usage.Artifacts.ArtifactWithMinPutRequests)
	}
}

func calculateArtifactStorageCosts(ctx context.Context, costs *S3Costs, usage S3Usage, tierInfoByKey map[string]StorageTierInfo, costConfig *evergreen.CostConfig) {
	costs.MinArtifactStorageCost = math.MaxFloat64
	costs.MaxArtifactStorageCost = -math.MaxFloat64
	for _, f := range usage.Artifacts.AllFiles() {
		fileCost := CalculateS3StorageCostWithConfig(ctx, f.Bytes, tierInfoByKey[f.FileKey], costConfig)
		costs.ArtifactStorageCost += fileCost
		if fileCost < costs.MinArtifactStorageCost {
			costs.MinArtifactStorageCost = fileCost
		}
		if fileCost > costs.MaxArtifactStorageCost {
			costs.MaxArtifactStorageCost = fileCost
		}
	}
	if usage.Artifacts.Count == 0 {
		costs.MinArtifactStorageCost = 0
		costs.MaxArtifactStorageCost = 0
	} else {
		costs.AvgArtifactStorageCost = costs.ArtifactStorageCost / float64(usage.Artifacts.Count)
	}
}

func calculateLogStorageCosts(ctx context.Context, costs *S3Costs, usage S3Usage, tierInfoByKey map[string]StorageTierInfo, costConfig *evergreen.CostConfig) {
	costs.MinLogStorageCost = math.MaxFloat64
	costs.MaxLogStorageCost = -math.MaxFloat64
	count := 0
	for _, lm := range []LogTypeMetrics{usage.Logs.Task, usage.Logs.Agent, usage.Logs.System} {
		if lm.LogKey == "" {
			continue
		}
		logCost := CalculateS3StorageCostWithConfig(ctx, lm.Bytes, tierInfoByKey[lm.LogKey], costConfig)
		costs.LogStorageCost += logCost
		if logCost < costs.MinLogStorageCost {
			costs.MinLogStorageCost = logCost
		}
		if logCost > costs.MaxLogStorageCost {
			costs.MaxLogStorageCost = logCost
		}
		count++
	}
	if count == 0 {
		costs.MinLogStorageCost = 0
		costs.MaxLogStorageCost = 0
	} else {
		costs.AvgLogStorageCost = costs.LogStorageCost / float64(count)
	}
}

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

// CalculateS3StorageCostWithConfig calculates the S3 storage cost for uploadBytes over their retention period.
func CalculateS3StorageCostWithConfig(ctx context.Context, uploadBytes int64, tierInfo StorageTierInfo, costConfig *evergreen.CostConfig) float64 {
	if uploadBytes <= 0 {
		return 0.0
	}
	if tierInfo.ExpirationDays <= 0 {
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

	daysInStandard, daysInIA, daysInArchive := storageTierDays(tierInfo)

	pricePerBytePerDay := func(pricePerGBMonth float64) float64 {
		return pricePerGBMonth / S3BytesPerGB / S3DaysPerMonth
	}

	standardCost := float64(daysInStandard) * pricePerBytePerDay(S3StandardPricePerGBMonth) * (1 - standardDiscount)
	iaCost := float64(daysInIA) * pricePerBytePerDay(S3IAPricePerGBMonth) * (1 - iaDiscount)
	archiveCost := float64(daysInArchive) * pricePerBytePerDay(S3ArchivePricePerGBMonth) * (1 - archiveDiscount)

	return float64(uploadBytes) * (standardCost + iaCost + archiveCost)
}

// storageTierDays returns how many days an object spends in each S3 storage tier.
// All Evergreen uploads use IT, which always transitions at S3ITDefaultTransitionToIADays and S3ITDefaultTransitionToGlacierDays.
func storageTierDays(tierInfo StorageTierInfo) (daysInStandard, daysInIA, daysInArchive int) {
	exp := tierInfo.ExpirationDays
	daysInStandard = min(exp, S3ITDefaultTransitionToIADays)
	daysInIA = max(0, min(exp, S3ITDefaultTransitionToGlacierDays)-S3ITDefaultTransitionToIADays)
	daysInArchive = max(0, exp-S3ITDefaultTransitionToGlacierDays)
	return daysInStandard, daysInIA, daysInArchive
}

// AllFiles returns a flat list of all artifact files across all buckets.
func (a *ArtifactMetrics) AllFiles() []BucketFile {
	var files []BucketFile
	for _, b := range a.BytesByBucketAndKey {
		for _, f := range b.Files {
			files = append(files, BucketFile{Bucket: b.Bucket, FileKey: f.FileKey, Bytes: f.Bytes})
		}
	}
	return files
}

// IncrementArtifacts updates aggregate artifact upload metrics after an s3.put command.
func (s *S3Usage) IncrementArtifacts(upload ArtifactUpload) {
	s.Artifacts.PutRequests += upload.PutRequests
	s.Artifacts.UploadBytes += upload.UploadBytes

	if upload.MaxPuts > s.Artifacts.ArtifactWithMaxPutRequests {
		s.Artifacts.ArtifactWithMaxPutRequests = upload.MaxPuts
	}
	if s.Artifacts.Count == 0 || upload.MinPuts < s.Artifacts.ArtifactWithMinPutRequests {
		s.Artifacts.ArtifactWithMinPutRequests = upload.MinPuts
	}
	s.Artifacts.Count += upload.FileCount

	var bucketEntry *BucketFileMetrics
	for i := range s.Artifacts.BytesByBucketAndKey {
		if s.Artifacts.BytesByBucketAndKey[i].Bucket == upload.Bucket {
			bucketEntry = &s.Artifacts.BytesByBucketAndKey[i]
			break
		}
	}
	if bucketEntry == nil {
		s.Artifacts.BytesByBucketAndKey = append(s.Artifacts.BytesByBucketAndKey, BucketFileMetrics{Bucket: upload.Bucket})
		bucketEntry = &s.Artifacts.BytesByBucketAndKey[len(s.Artifacts.BytesByBucketAndKey)-1]
	}
	for _, f := range upload.Files {
		j := 0
		for ; j < len(bucketEntry.Files); j++ {
			if bucketEntry.Files[j].FileKey == f.RemotePath {
				bucketEntry.Files[j].Bytes += f.FileSizeBytes
				break
			}
		}
		if j == len(bucketEntry.Files) {
			bucketEntry.Files = append(bucketEntry.Files, FileBytes{FileKey: f.RemotePath, Bytes: f.FileSizeBytes})
		}
	}
}

// IncrementLogs increments the log chunk upload metrics and accumulates bytes per log type.
func (s *S3Usage) IncrementLogs(putRequests int, uploadBytes int64, logType, logKey string) {
	s.Logs.PutRequests += putRequests
	s.Logs.UploadBytes += uploadBytes
	switch logType {
	case LogTypeTask:
		s.Logs.Task.Bytes += uploadBytes
		s.Logs.Task.LogKey = logKey
	case LogTypeAgent:
		s.Logs.Agent.Bytes += uploadBytes
		s.Logs.Agent.LogKey = logKey
	case LogTypeSystem:
		s.Logs.System.Bytes += uploadBytes
		s.Logs.System.LogKey = logKey
	}
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
