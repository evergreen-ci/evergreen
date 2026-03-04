package s3usage

import (
	"testing"

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
		s3Usage.LogFiles.PutRequests = 5
		assert.False(t, s3Usage.IsZero())

		s3Usage = S3Usage{}
		s3Usage.LogFiles.UploadBytes = 200
		assert.False(t, s3Usage.IsZero())
	})

	t.Run("IncrementUserFiles", func(t *testing.T) {
		s3Usage := S3Usage{}

		s3Usage.IncrementUserFiles(3, 1024, 2)
		assert.Equal(t, 3, s3Usage.UserFiles.PutRequests)
		assert.Equal(t, int64(1024), s3Usage.UserFiles.UploadBytes)
		assert.Equal(t, 2, s3Usage.UserFiles.FileCount)

		s3Usage.IncrementUserFiles(5, 2048, 3)
		assert.Equal(t, 8, s3Usage.UserFiles.PutRequests)
		assert.Equal(t, int64(3072), s3Usage.UserFiles.UploadBytes)
		assert.Equal(t, 5, s3Usage.UserFiles.FileCount)

		assert.Equal(t, 0, s3Usage.LogFiles.PutRequests, "IncrementUserFiles should not affect LogFiles")
	})

	t.Run("IncrementLogFiles", func(t *testing.T) {
		s3Usage := S3Usage{}

		s3Usage.IncrementLogFiles(2, 512)
		assert.Equal(t, 2, s3Usage.LogFiles.PutRequests)
		assert.Equal(t, int64(512), s3Usage.LogFiles.UploadBytes)

		s3Usage.IncrementLogFiles(4, 1024)
		assert.Equal(t, 6, s3Usage.LogFiles.PutRequests)
		assert.Equal(t, int64(1536), s3Usage.LogFiles.UploadBytes)

		assert.Equal(t, 0, s3Usage.UserFiles.PutRequests, "IncrementLogFiles should not affect UserFiles")
	})
}
