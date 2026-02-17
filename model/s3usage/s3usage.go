package s3usage

import (
	"os"

	"github.com/mongodb/grip"
)

// S3Usage tracks S3 API usage for cost calculation.
type S3Usage struct {
	UserFiles UserFilesMetrics `bson:"user_files,omitempty" json:"user_files,omitempty"`
	// NumPutRequests tracks log upload requests (maintained for backward compatibility).
	// Will be migrated to nested structure in as part of DEVPROD-25593.
	NumPutRequests int `bson:"num_put_requests,omitempty" json:"num_put_requests,omitempty"`
}

// UserFilesMetrics tracks artifact upload metrics.
type UserFilesMetrics struct {
	PutRequests int   `bson:"put_requests,omitempty" json:"put_requests,omitempty"`
	UploadBytes int64 `bson:"upload_bytes,omitempty" json:"upload_bytes,omitempty"`
	FileCount   int   `bson:"file_count,omitempty" json:"file_count,omitempty"`
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
	S3PartSize = 5 * 1024 * 1024 // 5 MB in bytes - S3 multipart upload threshold

	S3BucketTypeSmall S3BucketType = "small"
	S3BucketTypeLarge S3BucketType = "large"

	S3UploadMethodWriter S3UploadMethod = "writer"
	S3UploadMethodPut    S3UploadMethod = "put"
	S3UploadMethodCopy   S3UploadMethod = "copy"
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
			logger.Warningf("Unable to calculate file size and PUT requests for '%s' after successful upload: %s. Using zero values for metadata.", file.LocalPath, err)
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
		logger.Infof("Calculated metrics for file '%s': size=%d bytes, put_requests=%d", file.LocalPath, fileSize, putRequests)

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

// IncrementUserFiles increments the user file upload metrics (artifacts from s3.put commands).
func (s *S3Usage) IncrementUserFiles(putRequests int, uploadBytes int64, fileCount int) {
	s.UserFiles.PutRequests += putRequests
	s.UserFiles.UploadBytes += uploadBytes
	s.UserFiles.FileCount += fileCount
}

// IncrementPutRequests increments the total PUT request count.
// Used for log upload tracking. Maintained for backward compatibility.
func (s *S3Usage) IncrementPutRequests(count int) {
	s.NumPutRequests += count
}

// IsZero implements bsoncodec.Zeroer for BSON marshalling.
func (s *S3Usage) IsZero() bool {
	if s == nil {
		return true
	}
	return s.UserFiles.PutRequests == 0 && s.UserFiles.UploadBytes == 0 && s.UserFiles.FileCount == 0 &&
		s.NumPutRequests == 0
}
