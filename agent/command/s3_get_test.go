package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestS3GetValidate(t *testing.T) {
	t.Run("RequireLocalFileOrExtractTo", func(t *testing.T) {
		cmd := s3get{
			s3Operation: s3Operation{
				awsCredentials: awsCredentials{
					AWSKey:    "key",
					AWSSecret: "secret",
				},
				bucketOptions: bucketOptions{
					Bucket:     "bucket",
					RemoteFile: "remote",
				},
			},
		}

		assert.ErrorContains(t, cmd.validate(), "must specify either local file path or directory to extract to")
	})

	t.Run("HavingBothLocalFileOrExtractToErrors", func(t *testing.T) {
		cmd := s3get{
			s3Operation: s3Operation{
				awsCredentials: awsCredentials{
					AWSKey:    "key",
					AWSSecret: "secret",
				},
				bucketOptions: bucketOptions{
					Bucket:     "bucket",
					RemoteFile: "remote",
				},
			},
			LocalFile: "local",
			ExtractTo: "extract",
		}

		assert.ErrorContains(t, cmd.validate(), "cannot specify both local file path and directory to extract to")
	})

	t.Run("Succeeds", func(t *testing.T) {
		cmd := s3get{
			s3Operation: s3Operation{
				awsCredentials: awsCredentials{
					AWSKey:    "key",
					AWSSecret: "secret",
				},
				bucketOptions: bucketOptions{
					Bucket:     "bucket",
					RemoteFile: "remote",
				},
			},
			LocalFile: "local",
		}

		assert.NoError(t, cmd.validate())
	})
}
