package task

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/pkg/errors"
)

// S3Usage tracks S3 API usage for cost calculation
type S3Usage struct {
	// NumPutRequests is the number of S3 PutObject API requests made
	NumPutRequests int `bson:"num_put_requests,omitempty" json:"num_put_requests,omitempty"`
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
)

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

// CalculateS3PutCostWithConfig calculates the total S3 PUT request cost based on usage.
// Caller must provide CostConfig to avoid repeated database fetches in batch operations.
func (s *S3Usage) CalculateS3PutCostWithConfig(costConfig *evergreen.CostConfig) (float64, error) {
	if s.NumPutRequests <= 0 {
		return 0.0, nil
	}

	if costConfig == nil {
		return 0.0, nil
	}

	discount := costConfig.S3Cost.Upload.UploadCostDiscount
	if discount < 0.0 || discount > 1.0 {
		return 0.0, errors.Errorf("invalid S3 upload discount: %f (must be between 0.0 and 1.0)", discount)
	}

	return float64(s.NumPutRequests) * S3PutRequestCost * (1 - discount), nil
}

// IsZero implements bsoncodec.Zeroer for BSON marshalling.
func (s *S3Usage) IsZero() bool {
	return s.NumPutRequests == 0
}

func (s *S3Usage) IncrementPutRequests(count int) {
	s.NumPutRequests += count
}
