package apimodels

// S3CopyRequest holds information necessary for the API server to
// complete an S3 copy request; namely, an S3 key/secret, a source and
// a destination path
type S3CopyRequest struct {
	S3SourceRegion      string `json:"s3_source_region"`
	S3SourceBucket      string `json:"s3_source_bucket"`
	S3SourcePath        string `json:"s3_source_path"`
	S3DestinationRegion string `json:"s3_destination_region"`
	S3DestinationBucket string `json:"s3_destination_bucket"`
	S3DestinationPath   string `json:"s3_destination_path"`
	S3DisplayName       string `json:"display_name"`
	S3Permissions       string `json:"s3_permissions"`
}
