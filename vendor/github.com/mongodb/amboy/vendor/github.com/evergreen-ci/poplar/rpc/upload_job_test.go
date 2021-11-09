package rpc

import (
	"context"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/poplar"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUploadJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s3Name := "build-test-curator"
	s3Prefix := "poplar-upload-job-test"
	s3Region := "us-east-1"
	s3Opts := pail.S3Options{
		Name:   s3Name,
		Prefix: s3Prefix,
		Region: s3Region,
	}

	client := utility.GetHTTPClient()
	defer utility.PutHTTPClient(client)

	s3Bucket, err := pail.NewS3BucketWithHTTPClient(client, s3Opts)
	require.NoError(t, err)

	for _, test := range []struct {
		name     string
		artifact poplar.TestArtifact
		conf     poplar.BucketConfiguration
		dryRun   bool
		noUpload bool
		hasErr   bool
	}{
		{
			name: "UploadFails",
			artifact: poplar.TestArtifact{
				Prefix:    s3Prefix,
				LocalFile: filepath.Join("..", "testdata", "bson_example.bson"),
				Path:      "DNE",
			},
			conf: poplar.BucketConfiguration{
				Region: s3Region,
			},
			hasErr: true,
		},
		{
			name: "Upload",
			artifact: poplar.TestArtifact{
				Bucket:    s3Name,
				Prefix:    s3Prefix,
				LocalFile: filepath.Join("..", "testdata", "bson_example.bson"),
				Path:      "bsonFile",
			},
			conf: poplar.BucketConfiguration{
				Region: s3Region,
			},
		},
		{
			name: "UploadNoLocalFile",
			artifact: poplar.TestArtifact{
				Bucket: s3Name,
				Prefix: s3Prefix,
				Path:   "bsonFile",
			},
			conf: poplar.BucketConfiguration{
				Region: s3Region,
			},
			noUpload: true,
		},
		{
			name: "UploadDryRun",
			artifact: poplar.TestArtifact{
				Bucket:    s3Name,
				Prefix:    s3Prefix,
				LocalFile: filepath.Join("..", "testdata", "bson_example.bson"),
				Path:      "bsonFile2",
			},
			conf: poplar.BucketConfiguration{
				Region: s3Region,
			},
			dryRun: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			j := NewUploadJob(test.artifact, test.conf, test.dryRun)
			j.Run(ctx)
			defer func() {
				assert.NoError(t, s3Bucket.Remove(ctx, test.artifact.Path))
			}()

			if test.hasErr {
				assert.Error(t, j.Error())
			} else {
				assert.NoError(t, j.Error())
			}

			path := test.artifact.Path
			if path == "" {
				path = filepath.Base(test.artifact.LocalFile)
			}

			r, getErr := s3Bucket.Get(ctx, path)
			if !test.dryRun && !test.hasErr && !test.noUpload {
				require.NoError(t, getErr)
				remoteData, err := ioutil.ReadAll(r)
				require.NoError(t, err)
				localData, err := ioutil.ReadFile(test.artifact.LocalFile)
				require.NoError(t, err)
				assert.Equal(t, localData, remoteData)
			} else {
				assert.Error(t, getErr)
			}
		})
	}
}
