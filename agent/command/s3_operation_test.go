package command

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/pail"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestS3OperationExpandParams(t *testing.T) {
	for tName, tCase := range map[string]func(*testing.T, s3Operation, *internal.TaskConfig){
		"OptionalFails": func(t *testing.T, op s3Operation, conf *internal.TaskConfig) {
			for _, v := range []string{"NOPE", "NONE", "EMPTY", "01", "100", "${foo|wat}"} {
				op.Optional = v
				require.ErrorContains(t, op.expandParams(conf), "expanding optional")
			}
		},
		"OptionalSucceeds": func(t *testing.T, op s3Operation, conf *internal.TaskConfig) {
			for _, v := range []string{"true", "True", "1", "T", "t"} {
				op.Optional = v
				require.NoError(t, op.expandParams(conf))
			}
		},
		"TaskData": func(t *testing.T, op s3Operation, conf *internal.TaskConfig) {
			assert.Empty(t, op.taskData.ID)
			assert.Empty(t, op.taskData.Secret)
			require.NoError(t, op.expandParams(conf))
			assert.Equal(t, conf.Task.Id, op.taskData.ID)
			assert.Equal(t, conf.Task.Secret, op.taskData.Secret)
		},
		"AssumeRoleARNIsNotFound": func(t *testing.T, op s3Operation, conf *internal.TaskConfig) {
			sessionToken := "sessionToken"
			roleARN := "roleARN"
			conf.AssumeRoleRoles[sessionToken] = roleARN
			require.NoError(t, op.expandParams(conf))
			assert.Empty(t, op.assumeRoleARN)
		},
		"AssumeRoleARNIsFound": func(t *testing.T, op s3Operation, conf *internal.TaskConfig) {
			sessionToken := "sessionToken"
			roleARN := "roleARN"
			conf.AssumeRoleRoles[sessionToken] = roleARN
			op.AWSSessionToken = sessionToken
			require.NoError(t, op.expandParams(conf))
			assert.Equal(t, op.assumeRoleARN, roleARN)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			op := s3Operation{}

			conf := &internal.TaskConfig{
				Task: task.Task{
					Id:     "taskid",
					Secret: "tasksecret",
				},
				AssumeRoleRoles: map[string]string{},
			}

			tCase(t, op, conf)
		})
	}
}

func TestS3OperationCreatePailBucket(t *testing.T) {
	bucket, err := pail.NewLocalTemporaryBucket(pail.LocalOptions{})
	require.NoError(t, err)
	require.NotNil(t, bucket)

	mock := client.NewMock("url")

	t.Run("CopiesOverRegionAndBucket", func(t *testing.T) {
		op := s3Operation{}
		op.Region = "region"
		op.Bucket = "bucket"

		foundRegion := false
		foundBucket := false
		require.NoError(t, op.createPailBucket(pail.S3Options{}, mock,
			func(opts pail.S3Options) (pail.Bucket, error) {
				foundRegion = opts.Region == op.Region
				foundBucket = opts.Name == op.Bucket
				return nil, nil
			},
		))
		assert.True(t, foundRegion)
		assert.True(t, foundBucket)
	})

	t.Run("EvergreenCredentialsWithRoleARN", func(t *testing.T) {
		op := s3Operation{}
		op.RoleARN = "roleARN"

		var creds aws.CredentialsProvider
		require.NoError(t, op.createPailBucket(pail.S3Options{}, mock,
			func(opts pail.S3Options) (pail.Bucket, error) {
				creds = opts.Credentials
				return nil, nil
			},
		))
		assert.IsType(t, &evergreenCredentialProvider{}, creds)
	})

	t.Run("EvergreenCredentialsWithAssumeRoleARN", func(t *testing.T) {
		op := s3Operation{}
		op.assumeRoleARN = "roleARN"

		var creds aws.CredentialsProvider
		require.NoError(t, op.createPailBucket(pail.S3Options{}, mock,
			func(opts pail.S3Options) (pail.Bucket, error) {
				creds = opts.Credentials
				return nil, nil
			},
		))
		assert.IsType(t, &evergreenCredentialProvider{}, creds)
	})

	t.Run("EvergreenCredentialsWithInternalBucket", func(t *testing.T) {
		op := s3Operation{}
		op.temporaryUseInternalBucket = true

		var creds aws.CredentialsProvider
		require.NoError(t, op.createPailBucket(pail.S3Options{}, mock,
			func(opts pail.S3Options) (pail.Bucket, error) {
				creds = opts.Credentials
				return nil, nil
			},
		))
		assert.IsType(t, &evergreenCredentialProvider{}, creds)
	})

	t.Run("StaticCredentials", func(t *testing.T) {
		op := s3Operation{}
		op.AWSKey = "test"

		var creds aws.CredentialsProvider
		require.NoError(t, op.createPailBucket(pail.S3Options{}, mock,
			func(opts pail.S3Options) (pail.Bucket, error) {
				creds = opts.Credentials
				return nil, nil
			},
		))
		assert.IsType(t, credentials.StaticCredentialsProvider{}, creds)
	})

	t.Run("EvergreenCredentialsTakesPrecedenceOverStatic", func(t *testing.T) {
		op := s3Operation{}
		op.AWSKey = "test"
		op.RoleARN = "roleARN"

		var creds aws.CredentialsProvider
		require.NoError(t, op.createPailBucket(pail.S3Options{}, mock,
			func(opts pail.S3Options) (pail.Bucket, error) {
				creds = opts.Credentials
				return nil, nil
			},
		))
		assert.IsType(t, &evergreenCredentialProvider{}, creds)
	})
}

func TestS3OperationGetRoleARN(t *testing.T) {
	t.Run("Parameter", func(t *testing.T) {
		op := s3Operation{}
		op.RoleARN = "roleARN"
		assert.Equal(t, op.getRoleARN(), "roleARN")
	})

	t.Run("AssumeRole", func(t *testing.T) {
		op := s3Operation{}
		op.assumeRoleARN = "assumeRoleARN"
		assert.Equal(t, op.getRoleARN(), "assumeRoleARN")
	})
}

func TestS3CredentialsShouldRunForVariant(t *testing.T) {
	t.Run("Empty", func(t *testing.T) {
		op := s3Operation{}
		assert.True(t, op.shouldRunForVariant("bv"))
	})

	t.Run("ExcludesBV", func(t *testing.T) {
		op := s3Operation{}
		op.BuildVariants = []string{"bv1"}
		assert.False(t, op.shouldRunForVariant("bv2"))
	})

	t.Run("IncludesBV", func(t *testing.T) {
		op := s3Operation{}
		op.BuildVariants = []string{"bv1", "bv2"}
		assert.False(t, op.shouldRunForVariant("bv2"))
	})
}

func TestAWSCredentialsValidate(t *testing.T) {
	t.Run("StaticCredentials", func(t *testing.T) {
		t.Run("Fails", func(t *testing.T) {
			creds := awsCredentials{}
			err := errors.Join(creds.validate()...)
			require.Error(t, err)
			assert.ErrorContains(t, err, "AWS key cannot be blank")
			assert.ErrorContains(t, err, "AWS secret cannot be blank")
		})

		t.Run("Succeeds", func(t *testing.T) {
			creds := awsCredentials{
				AWSKey:    "key",
				AWSSecret: "secret",
			}
			assert.Empty(t, creds.validate())
		})
	})

	t.Run("RoleARN", func(t *testing.T) {
		t.Run("Fails", func(t *testing.T) {
			creds := awsCredentials{
				RoleARN:         "roleARN",
				AWSKey:          "key",
				AWSSecret:       "secret",
				AWSSessionToken: "session",
			}
			err := errors.Join(creds.validate()...)
			require.Error(t, err)
			assert.ErrorContains(t, err, "AWS key must be empty when using role ARN")
			assert.ErrorContains(t, err, "AWS secret must be empty when using role ARN")
			assert.ErrorContains(t, err, "AWS session token must be empty when using role ARN")
		})

		t.Run("Succeeds", func(t *testing.T) {
			creds := awsCredentials{
				RoleARN: "roleARN",
			}
			assert.Empty(t, creds.validate())
		})
	})
}

func TestBucketOptionsValidate(t *testing.T) {
	t.Run("InvalidBucket", func(t *testing.T) {
		opts := bucketOptions{
			Bucket:     "1",
			RemoteFile: "file",
		}
		err := errors.Join(opts.validate()...)
		require.Error(t, err)
		assert.ErrorContains(t, err, "must be at leaset 3 characters")
	})

	t.Run("InvalidRemoteFile", func(t *testing.T) {
		opts := bucketOptions{
			Bucket: "bucket",
		}
		err := errors.Join(opts.validate()...)
		require.Error(t, err)
		assert.ErrorContains(t, err, "remote file cannot be blank")
	})

	t.Run("Valid", func(t *testing.T) {
		opts := bucketOptions{
			Bucket:     "bucket",
			RemoteFile: "file",
		}
		assert.Empty(t, opts.validate())
	})
}
