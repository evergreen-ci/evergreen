package s3usage

// S3Usage tracks S3 API usage for cost calculation, broken down by usage category.
type S3Usage struct {
	UserFiles UserFilesMetrics `bson:"user_files,omitempty" json:"user_files,omitempty"`
	LogFiles  LogFilesMetrics  `bson:"log_files,omitempty" json:"log_files,omitempty"`
}

// UserFilesMetrics tracks S3 usage for user-uploaded artifact files (s3.put, attach.artifacts).
type UserFilesMetrics struct {
	PutRequests int   `bson:"put_requests,omitempty" json:"put_requests,omitempty"`
	UploadBytes int64 `bson:"upload_bytes,omitempty" json:"upload_bytes,omitempty"`
	FileCount   int   `bson:"file_count,omitempty" json:"file_count,omitempty"`
}

// LogFilesMetrics tracks S3 usage for log file uploads (task logs, system logs).
type LogFilesMetrics struct {
	PutRequests int   `bson:"put_requests,omitempty" json:"put_requests,omitempty"`
	UploadBytes int64 `bson:"upload_bytes,omitempty" json:"upload_bytes,omitempty"`
}

// IsZero implements bsoncodec.Zeroer for BSON marshalling.
func (s *S3Usage) IsZero() bool {
	return s.UserFiles.PutRequests == 0 && s.UserFiles.UploadBytes == 0 && s.UserFiles.FileCount == 0 &&
		s.LogFiles.PutRequests == 0 && s.LogFiles.UploadBytes == 0
}

// IncrementUserFiles increments user file metrics by the given amounts.
func (s *S3Usage) IncrementUserFiles(putRequests int, uploadBytes int64, fileCount int) {
	s.UserFiles.PutRequests += putRequests
	s.UserFiles.UploadBytes += uploadBytes
	s.UserFiles.FileCount += fileCount
}

// IncrementLogFiles increments log file metrics by the given amounts.
func (s *S3Usage) IncrementLogFiles(putRequests int, uploadBytes int64) {
	s.LogFiles.PutRequests += putRequests
	s.LogFiles.UploadBytes += uploadBytes
}
