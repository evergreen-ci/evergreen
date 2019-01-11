package pail

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	mgo "gopkg.in/mgo.v2"
)

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func createS3Client(region string) (*s3.S3, error) {
	sess, err := session.NewSession(&aws.Config{Region: aws.String(region)})
	if err != nil {
		return nil, errors.Wrap(err, "problem connecting to AWS")
	}
	svc := s3.New(sess)
	return svc, nil
}

func cleanUpS3Bucket(name, prefix, region string) error {
	svc, err := createS3Client(region)
	if err != nil {
		return errors.Wrap(err, "clean up failed")
	}
	deleteObjectsInput := &s3.DeleteObjectsInput{
		Bucket: aws.String(name),
		Delete: &s3.Delete{},
	}
	listInput := &s3.ListObjectsInput{
		Bucket: aws.String(name),
		Prefix: aws.String(prefix),
	}
	var result *s3.ListObjectsOutput

	for {
		result, err = svc.ListObjects(listInput)
		if err != nil {
			return errors.Wrap(err, "clean up failed")
		}

		for _, object := range result.Contents {
			deleteObjectsInput.Delete.Objects = append(deleteObjectsInput.Delete.Objects, &s3.ObjectIdentifier{
				Key: object.Key,
			})
		}

		if deleteObjectsInput.Delete.Objects != nil {
			_, err = svc.DeleteObjects(deleteObjectsInput)
			if err != nil {
				return errors.Wrap(err, "failed to delete S3 bucket")
			}
			deleteObjectsInput.Delete = &s3.Delete{}
		}

		if *result.IsTruncated {
			listInput.Marker = result.Contents[len(result.Contents)-1].Key
		} else {
			break
		}
	}

	return nil
}

func TestBucket(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	uuid := newUUID()
	_, file, _, _ := runtime.Caller(0)
	tempdir, err := ioutil.TempDir("", "pail-bucket-test")
	require.NoError(t, err)
	defer func() { require.NoError(t, os.RemoveAll(tempdir)) }()
	require.NoError(t, err, os.MkdirAll(filepath.Join(tempdir, uuid), 0700))

	ses, err := mgo.DialWithTimeout("mongodb://localhost:27017", time.Second)
	require.NoError(t, err)
	defer ses.Close()
	defer func() { ses.DB(uuid).DropDatabase() }()

	s3BucketName := "build-test-curator"
	s3Prefix := newUUID() + "-"
	s3Region := "us-east-1"
	defer func() { require.NoError(t, cleanUpS3Bucket(s3BucketName, s3Prefix, s3Region)) }()

	type bucketTestCase struct {
		id   string
		test func(*testing.T, Bucket)
	}

	for _, impl := range []struct {
		name        string
		constructor func(*testing.T) Bucket
		tests       []bucketTestCase
	}{
		{
			name: "Local",
			constructor: func(t *testing.T) Bucket {
				path := filepath.Join(tempdir, uuid, newUUID())
				require.NoError(t, os.MkdirAll(path, 0777))
				return &localFileSystem{path: path}
			},
			tests: []bucketTestCase{
				{
					id: "VerifyBucketType",
					test: func(t *testing.T, b Bucket) {
						bucket, ok := b.(*localFileSystem)
						require.True(t, ok)
						assert.NotNil(t, bucket)
					},
				},
				{
					id: "PathDoesNotExist",
					test: func(t *testing.T, b Bucket) {
						bucket := b.(*localFileSystem)
						bucket.path = "foo"
						assert.Error(t, bucket.Check(ctx))
					},
				},
				{
					id: "WriterErrorFileName",
					test: func(t *testing.T, b Bucket) {
						_, err := b.Writer(ctx, "\x00")
						require.Error(t, err)
						assert.Contains(t, err.Error(), "problem opening")
					},
				},
				{
					id: "ReaderErrorFileName",
					test: func(t *testing.T, b Bucket) {
						_, err := b.Reader(ctx, "\x00")
						require.Error(t, err)
						assert.Contains(t, err.Error(), "problem opening")
					},
				},
				{
					id: "CopyErrorFileNameFrom",
					test: func(t *testing.T, b Bucket) {
						options := CopyOptions{
							SourceKey:         "\x00",
							DestinationKey:    "foo",
							DestinationBucket: b,
						}
						err := b.Copy(ctx, options)
						require.Error(t, err)
						assert.Contains(t, err.Error(), "problem opening")
					},
				},
				{
					id: "CopyErrorFileNameTo",
					test: func(t *testing.T, b Bucket) {
						fn := filepath.Base(file)
						err := b.Upload(ctx, "foo", fn)
						require.NoError(t, err)

						options := CopyOptions{
							SourceKey:         "foo",
							DestinationKey:    "\x00",
							DestinationBucket: b,
						}
						err = b.Copy(ctx, options)
						require.Error(t, err)
						assert.Contains(t, err.Error(), "problem opening")
					},
				},
				{
					id: "PutErrorFileName",
					test: func(t *testing.T, b Bucket) {
						err := b.Put(ctx, "\x00", nil)
						require.Error(t, err)
						assert.Contains(t, err.Error(), "problem opening")
					},
				},
				{
					id: "PutErrorReader",
					test: func(t *testing.T, b Bucket) {
						err := b.Put(ctx, "foo", &brokenWriter{})
						require.Error(t, err)
						assert.Contains(t, err.Error(), "problem copying data to file")
					},
				},
				{
					id: "WriterErrorDirectoryName",
					test: func(t *testing.T, b Bucket) {
						bucket := b.(*localFileSystem)
						bucket.path = "\x00"
						_, err := b.Writer(ctx, "foo")
						require.Error(t, err)
						assert.Contains(t, err.Error(), "problem creating base directories")
					},
				},
				{
					id: "PullErrorsContext",
					test: func(t *testing.T, b Bucket) {
						tctx, cancel := context.WithCancel(ctx)
						cancel()
						bucket := b.(*localFileSystem)
						bucket.path = ""
						err := b.Pull(tctx, "", filepath.Dir(file))
						assert.Error(t, err)
					},
				},
				{
					id: "PushErrorsContext",
					test: func(t *testing.T, b Bucket) {
						tctx, cancel := context.WithCancel(ctx)
						cancel()
						err := b.Push(tctx, filepath.Dir(file), "")
						assert.Error(t, err)
					},
				},
			},
		},
		{
			name: "LegacyGridFS",
			constructor: func(t *testing.T) Bucket {
				b, err := NewLegacyGridFSBucketWithSession(ses.Clone(), GridFSOptions{
					Prefix:   newUUID(),
					Database: uuid,
				})
				require.NoError(t, err)
				return b
			},
			tests: []bucketTestCase{
				{
					id: "VerifyBucketType",
					test: func(t *testing.T, b Bucket) {
						bucket, ok := b.(*gridfsLegacyBucket)
						require.True(t, ok)
						assert.NotNil(t, bucket)
					},
				},
				{
					id: "OpenFailsWithClosedSession",
					test: func(t *testing.T, b Bucket) {
						bucket := b.(*gridfsLegacyBucket)
						go func() {
							time.Sleep(time.Millisecond)
							bucket.session.Close()
						}()
						_, err := bucket.openFile(ctx, "foo", false)
						assert.Error(t, err)
					},
				},
			},
		},
		{
			name: "S3Bucket",
			constructor: func(t *testing.T) Bucket {
				s3Options := S3Options{
					Region:     s3Region,
					Name:       s3BucketName,
					Prefix:     s3Prefix + newUUID(),
					MaxRetries: 20,
				}
				b, err := NewS3Bucket(s3Options)
				require.NoError(t, err)
				return b
			},
			tests: []bucketTestCase{
				{
					id: "VerifyBucketType",
					test: func(t *testing.T, b Bucket) {
						bucket, ok := b.(*s3BucketSmall)
						require.True(t, ok)
						assert.NotNil(t, bucket)
					},
				},
				{
					id: "TestCredentialsOverrideDefaults",
					test: func(t *testing.T, b Bucket) {
						input := &s3.GetBucketLocationInput{
							Bucket: aws.String(s3BucketName),
						}

						rawBucket := b.(*s3BucketSmall)
						_, err := rawBucket.svc.GetBucketLocationWithContext(ctx, input)
						assert.NoError(t, err)

						badOptions := S3Options{
							Credentials: CreateAWSCredentials("asdf", "asdf", "asdf"),
							Region:      s3Region,
							Name:        s3BucketName,
						}
						badBucket, err := NewS3Bucket(badOptions)
						require.NoError(t, err)
						rawBucket = badBucket.(*s3BucketSmall)
						_, err = rawBucket.svc.GetBucketLocationWithContext(ctx, input)
						assert.Error(t, err)
					},
				},
				{
					id: "TestCheckPassesWhenDoNotHaveAccess",
					test: func(t *testing.T, b Bucket) {
						rawBucket := b.(*s3BucketSmall)
						rawBucket.name = "mciuploads"
						assert.NoError(t, rawBucket.Check(ctx))
					},
				},
				{
					id: "TestCheckFailsWhenBucketDNE",
					test: func(t *testing.T, b Bucket) {
						rawBucket := b.(*s3BucketSmall)
						rawBucket.name = newUUID()
						assert.Error(t, rawBucket.Check(ctx))
					},
				},
				{
					id: "TestSharedCredentialsOption",
					test: func(t *testing.T, b Bucket) {
						require.NoError(t, b.Check(ctx))

						newFile, err := os.Create(filepath.Join(tempdir, "creds"))
						require.NoError(t, err)
						defer newFile.Close()
						_, err = newFile.WriteString("[my_profile]\n")
						require.NoError(t, err)
						awsKey := fmt.Sprintf("aws_access_key_id = %s\n", os.Getenv("AWS_KEY"))
						_, err = newFile.WriteString(awsKey)
						require.NoError(t, err)
						awsSecret := fmt.Sprintf("aws_secret_access_key = %s\n", os.Getenv("AWS_SECRET"))
						_, err = newFile.WriteString(awsSecret)
						require.NoError(t, err)

						sharedCredsOptions := S3Options{
							SharedCredentialsFilepath: filepath.Join(tempdir, "creds"),
							SharedCredentialsProfile:  "my_profile",
							Region:                    s3Region,
							Name:                      s3BucketName,
						}
						sharedCredsBucket, err := NewS3Bucket(sharedCredsOptions)
						require.NoError(t, err)
						assert.NoError(t, sharedCredsBucket.Check(ctx))
					},
				},
				{
					id: "TestSharedCredentialsUsesCorrectDefaultFile",
					test: func(t *testing.T, b Bucket) {
						require.NoError(t, b.Check(ctx))

						sharedCredsOptions := S3Options{
							SharedCredentialsProfile: "default",
							Region:                   s3Region,
							Name:                     s3BucketName,
						}
						sharedCredsBucket, err := NewS3Bucket(sharedCredsOptions)
						homeDir, err := homedir.Dir()
						require.NoError(t, err)
						fileName := filepath.Join(homeDir, ".aws", "credentials")
						_, err = os.Stat(fileName)
						if err == nil {
							assert.NoError(t, sharedCredsBucket.Check(ctx))
						} else {
							assert.True(t, os.IsNotExist(err))
						}
					},
				},
				{
					id: "TestSharedCredentialsFailsWhenProfileDNE",
					test: func(t *testing.T, b Bucket) {
						require.NoError(t, b.Check(ctx))

						sharedCredsOptions := S3Options{
							SharedCredentialsProfile: "DNE",
							Region:                   s3Region,
							Name:                     s3BucketName,
						}
						_, err := NewS3Bucket(sharedCredsOptions)
						assert.Error(t, err)
					},
				},

				{
					id: "TestPermissions",
					test: func(t *testing.T, b Bucket) {
						// default permissions
						key := newUUID()
						writer, err := b.Writer(ctx, key)
						require.NoError(t, err)
						_, err = writer.Write([]byte("hello world"))
						require.NoError(t, err)
						require.NoError(t, writer.Close())
						rawBucket := b.(*s3BucketSmall)
						objectAclInput := &s3.GetObjectAclInput{
							Bucket: aws.String(s3BucketName),
							Key:    aws.String(rawBucket.normalizeKey(key)),
						}
						objectAclOutput, err := rawBucket.svc.GetObjectAcl(objectAclInput)
						require.NoError(t, err)
						require.Equal(t, 1, len(objectAclOutput.Grants))
						assert.Equal(t, "FULL_CONTROL", *objectAclOutput.Grants[0].Permission)

						// explicitly set permissions
						openOptions := S3Options{
							Region:     s3Region,
							Name:       s3BucketName,
							Prefix:     s3Prefix + newUUID(),
							Permission: "public-read",
						}
						openBucket, err := NewS3Bucket(openOptions)
						key = newUUID()
						writer, err = openBucket.Writer(ctx, key)
						require.NoError(t, err)
						_, err = writer.Write([]byte("hello world"))
						require.NoError(t, err)
						require.NoError(t, writer.Close())
						rawBucket = openBucket.(*s3BucketSmall)
						objectAclInput = &s3.GetObjectAclInput{
							Bucket: aws.String(s3BucketName),
							Key:    aws.String(rawBucket.normalizeKey(key)),
						}
						objectAclOutput, err = rawBucket.svc.GetObjectAcl(objectAclInput)
						require.NoError(t, err)
						require.Equal(t, 2, len(objectAclOutput.Grants))
						assert.Equal(t, "READ", *objectAclOutput.Grants[1].Permission)
					},
				},
				{
					id: "TestContentType",
					test: func(t *testing.T, b Bucket) {
						// default content type
						key := newUUID()
						writer, err := b.Writer(ctx, key)
						require.NoError(t, err)
						_, err = writer.Write([]byte("hello world"))
						require.NoError(t, err)
						require.NoError(t, writer.Close())
						rawBucket := b.(*s3BucketSmall)
						getObjectInput := &s3.GetObjectInput{
							Bucket: aws.String(s3BucketName),
							Key:    aws.String(rawBucket.normalizeKey(key)),
						}
						getObjectOutput, err := rawBucket.svc.GetObject(getObjectInput)
						require.NoError(t, err)
						assert.Nil(t, getObjectOutput.ContentType)

						// explicitly set content type
						htmlOptions := S3Options{
							Region:      s3Region,
							Name:        s3BucketName,
							Prefix:      s3Prefix + newUUID(),
							ContentType: "html/text",
						}
						htmlBucket, err := NewS3Bucket(htmlOptions)
						require.NoError(t, err)
						key = newUUID()
						writer, err = htmlBucket.Writer(ctx, key)
						require.NoError(t, err)
						_, err = writer.Write([]byte("hello world"))
						require.NoError(t, err)
						require.NoError(t, writer.Close())
						rawBucket = htmlBucket.(*s3BucketSmall)
						getObjectInput = &s3.GetObjectInput{
							Bucket: aws.String(s3BucketName),
							Key:    aws.String(rawBucket.normalizeKey(key)),
						}
						getObjectOutput, err = rawBucket.svc.GetObject(getObjectInput)
						require.NoError(t, err)
						require.NotNil(t, getObjectOutput.ContentType)
						assert.Equal(t, "html/text", *getObjectOutput.ContentType)
					},
				},
			},
		},
		{
			name: "S3MultiPartBucket",
			constructor: func(t *testing.T) Bucket {
				s3Options := S3Options{
					Region:     s3Region,
					Name:       s3BucketName,
					Prefix:     s3Prefix + newUUID(),
					MaxRetries: 20,
				}
				b, err := NewS3MultiPartBucket(s3Options)
				require.NoError(t, err)
				return b
			},
			tests: []bucketTestCase{
				{
					id: "VerifyBucketType",
					test: func(t *testing.T, b Bucket) {
						bucket, ok := b.(*s3BucketLarge)
						require.True(t, ok)
						assert.NotNil(t, bucket)
					},
				},
				{
					id: "TestPermissions",
					test: func(t *testing.T, b Bucket) {
						// default permissions
						key := newUUID()
						writer, err := b.Writer(ctx, key)
						require.NoError(t, err)
						_, err = writer.Write([]byte("hello world"))
						require.NoError(t, err)
						require.NoError(t, writer.Close())
						rawBucket := b.(*s3BucketLarge)
						objectAclInput := &s3.GetObjectAclInput{
							Bucket: aws.String(s3BucketName),
							Key:    aws.String(rawBucket.normalizeKey(key)),
						}
						objectAclOutput, err := rawBucket.svc.GetObjectAcl(objectAclInput)
						require.NoError(t, err)
						require.Equal(t, 1, len(objectAclOutput.Grants))
						assert.Equal(t, "FULL_CONTROL", *objectAclOutput.Grants[0].Permission)

						// explicitly set permissions
						openOptions := S3Options{
							Region:     s3Region,
							Name:       s3BucketName,
							Prefix:     s3Prefix + newUUID(),
							Permission: "public-read",
						}
						openBucket, err := NewS3MultiPartBucket(openOptions)
						key = newUUID()
						writer, err = openBucket.Writer(ctx, key)
						require.NoError(t, err)
						_, err = writer.Write([]byte("hello world"))
						require.NoError(t, err)
						require.NoError(t, writer.Close())
						rawBucket = openBucket.(*s3BucketLarge)
						objectAclInput = &s3.GetObjectAclInput{
							Bucket: aws.String(s3BucketName),
							Key:    aws.String(rawBucket.normalizeKey(key)),
						}
						objectAclOutput, err = rawBucket.svc.GetObjectAcl(objectAclInput)
						require.NoError(t, err)
						require.Equal(t, 2, len(objectAclOutput.Grants))
						assert.Equal(t, "READ", *objectAclOutput.Grants[1].Permission)
					},
				},
				{
					id: "TestContentType",
					test: func(t *testing.T, b Bucket) {
						// default content type
						key := newUUID()
						writer, err := b.Writer(ctx, key)
						require.NoError(t, err)
						_, err = writer.Write([]byte("hello world"))
						require.NoError(t, err)
						require.NoError(t, writer.Close())
						rawBucket := b.(*s3BucketLarge)
						getObjectInput := &s3.GetObjectInput{
							Bucket: aws.String(s3BucketName),
							Key:    aws.String(rawBucket.normalizeKey(key)),
						}
						getObjectOutput, err := rawBucket.svc.GetObject(getObjectInput)
						require.NoError(t, err)
						assert.Nil(t, getObjectOutput.ContentType)

						// explicitly set content type
						htmlOptions := S3Options{
							Region:      s3Region,
							Name:        s3BucketName,
							Prefix:      s3Prefix + newUUID(),
							ContentType: "html/text",
						}
						htmlBucket, err := NewS3MultiPartBucket(htmlOptions)
						require.NoError(t, err)
						key = newUUID()
						writer, err = htmlBucket.Writer(ctx, key)
						require.NoError(t, err)
						_, err = writer.Write([]byte("hello world"))
						require.NoError(t, err)
						require.NoError(t, writer.Close())
						rawBucket = htmlBucket.(*s3BucketLarge)
						getObjectInput = &s3.GetObjectInput{
							Bucket: aws.String(s3BucketName),
							Key:    aws.String(rawBucket.normalizeKey(key)),
						}
						getObjectOutput, err = rawBucket.svc.GetObject(getObjectInput)
						require.NoError(t, err)
						require.NotNil(t, getObjectOutput.ContentType)
						assert.Equal(t, "html/text", *getObjectOutput.ContentType)
					},
				},
				{
					id: "TestLargeFileRoundTrip",
					test: func(t *testing.T, b Bucket) {
						size := int64(10000000)
						key := newUUID()
						bigBuff := make([]byte, size)
						path := filepath.Join(tempdir, "bigfile.test0")

						// upload large empty file
						require.NoError(t, ioutil.WriteFile(path, bigBuff, 0666))
						require.NoError(t, b.Upload(ctx, key, path))

						// check size of empty file
						path = filepath.Join(tempdir, "bigfile.test1")
						require.NoError(t, b.Download(ctx, key, path))
						fi, err := os.Stat(path)
						require.NoError(t, err)
						assert.Equal(t, size, fi.Size())
					},
				},
			},
		},
	} {
		t.Run(impl.name, func(t *testing.T) {
			for _, test := range impl.tests {
				t.Run(test.id, func(t *testing.T) {
					bucket := impl.constructor(t)
					test.test(t, bucket)
				})
			}
			t.Run("ValidateFixture", func(t *testing.T) {
				assert.NotNil(t, impl.constructor(t))
			})
			t.Run("CheckIsValid", func(t *testing.T) {
				assert.NoError(t, impl.constructor(t).Check(ctx))
			})
			t.Run("ListIsEmpty", func(t *testing.T) {
				bucket := impl.constructor(t)
				iter, err := bucket.List(ctx, "")
				require.NoError(t, err)
				assert.False(t, iter.Next(ctx))
				assert.Nil(t, iter.Item())
				assert.NoError(t, iter.Err())
			})
			t.Run("ListErrorsWithCancledContext", func(t *testing.T) {
				bucket := impl.constructor(t)
				tctx, cancel := context.WithCancel(ctx)
				cancel()
				iter, err := bucket.List(tctx, "")
				assert.Error(t, err)
				assert.Nil(t, iter)
			})
			t.Run("WriteOneFile", func(t *testing.T) {
				bucket := impl.constructor(t)
				assert.NoError(t, writeDataToFile(ctx, bucket, newUUID(), "hello world!"))

				// dry run
				dryRunBucket := clone(bucket, true)
				assert.NoError(t, writeDataToFile(ctx, dryRunBucket, newUUID(), "hello world!"))

				// just check that only one key exists in the iterator
				iter, err := bucket.List(ctx, "")
				require.NoError(t, err)
				assert.True(t, iter.Next(ctx))
				assert.False(t, iter.Next(ctx))
				assert.NoError(t, iter.Err())
			})
			t.Run("RemoveOneFile", func(t *testing.T) {
				bucket := impl.constructor(t)
				key := newUUID()
				assert.NoError(t, writeDataToFile(ctx, bucket, key, "hello world!"))

				// dry run does not remove anything
				dryRunBucket := clone(bucket, true)
				assert.NoError(t, dryRunBucket.Remove(ctx, key))

				// just check that it exists in the iterator
				iter, err := bucket.List(ctx, "")
				require.NoError(t, err)
				assert.True(t, iter.Next(ctx))
				assert.False(t, iter.Next(ctx))
				assert.NoError(t, iter.Err())

				assert.NoError(t, bucket.Remove(ctx, key))
				iter, err = bucket.List(ctx, "")
				require.NoError(t, err)
				assert.False(t, iter.Next(ctx))
				assert.Nil(t, iter.Item())
				assert.NoError(t, iter.Err())
			})
			t.Run("RemoveManyFiles", func(t *testing.T) {
				data := map[string]string{}
				keys := []string{}
				deleteData := map[string]string{}
				deleteKeys := []string{}
				for i := 0; i < 20; i++ {
					key := newUUID()
					data[key] = strings.Join([]string{newUUID(), newUUID(), newUUID()}, "\n")
					keys = append(keys, key)
				}
				for i := 0; i < 20; i++ {
					key := newUUID()
					deleteData[key] = strings.Join([]string{newUUID(), newUUID(), newUUID()}, "\n")
					deleteKeys = append(deleteKeys, key)
				}

				bucket := impl.constructor(t)
				for k, v := range data {
					require.NoError(t, writeDataToFile(ctx, bucket, k, v))
				}
				for k, v := range deleteData {
					require.NoError(t, writeDataToFile(ctx, bucket, k, v))
				}

				// smaller s3 batch sizes for testing
				switch i := bucket.(type) {
				case *s3BucketSmall:
					i.batchSize = 20
				case *s3BucketLarge:
					i.batchSize = 20
				}

				// check keys are in bucket
				iter, err := bucket.List(ctx, "")
				require.NoError(t, err)
				for iter.Next(ctx) {
					assert.NoError(t, iter.Err())
					require.NotNil(t, iter.Item())
					_, ok1 := data[iter.Item().Name()]
					_, ok2 := deleteData[iter.Item().Name()]
					assert.True(t, ok1 || ok2)
				}

				assert.NoError(t, bucket.RemoveMany(ctx, deleteKeys...))
				iter, err = bucket.List(ctx, "")
				require.NoError(t, err)
				for iter.Next(ctx) {
					assert.NoError(t, iter.Err())
					require.NotNil(t, iter.Item())
					_, ok := data[iter.Item().Name()]
					assert.True(t, ok)
					_, ok = deleteData[iter.Item().Name()]
					assert.False(t, ok)
				}

			})
			t.Run("RemovePrefix", func(t *testing.T) {
				data := map[string]string{}
				keys := []string{}
				deleteData := map[string]string{}
				deleteKeys := []string{}
				prefix := newUUID()
				for i := 0; i < 5; i++ {
					key := newUUID()
					data[key] = strings.Join([]string{newUUID(), newUUID(), newUUID()}, "\n")
					keys = append(keys, key)
				}
				for i := 0; i < 5; i++ {
					key := prefix + newUUID()
					deleteData[key] = strings.Join([]string{newUUID(), newUUID(), newUUID()}, "\n")
					deleteKeys = append(deleteKeys, key)
				}

				bucket := impl.constructor(t)
				for k, v := range data {
					require.NoError(t, writeDataToFile(ctx, bucket, k, v))
				}
				for k, v := range deleteData {
					require.NoError(t, writeDataToFile(ctx, bucket, k, v))
				}

				// check keys are in bucket
				iter, err := bucket.List(ctx, "")
				require.NoError(t, err)
				for iter.Next(ctx) {
					assert.NoError(t, iter.Err())
					require.NotNil(t, iter.Item())
					_, ok1 := data[iter.Item().Name()]
					_, ok2 := deleteData[iter.Item().Name()]
					assert.True(t, ok1 || ok2)
				}

				assert.NoError(t, bucket.RemoveMany(ctx, deleteKeys...))
				iter, err = bucket.List(ctx, "")
				require.NoError(t, err)
				for iter.Next(ctx) {
					assert.NoError(t, iter.Err())
					require.NotNil(t, iter.Item())
					_, ok := data[iter.Item().Name()]
					assert.True(t, ok)
					_, ok = deleteData[iter.Item().Name()]
					assert.False(t, ok)
				}
			})
			t.Run("RemoveMatching", func(t *testing.T) {
				data := map[string]string{}
				keys := []string{}
				deleteData := map[string]string{}
				deleteKeys := []string{}
				postfix := newUUID()
				for i := 0; i < 5; i++ {
					key := newUUID()
					data[key] = strings.Join([]string{newUUID(), newUUID(), newUUID()}, "\n")
					keys = append(keys, key)
				}
				for i := 0; i < 5; i++ {
					key := newUUID() + postfix
					deleteData[key] = strings.Join([]string{newUUID(), newUUID(), newUUID()}, "\n")
					deleteKeys = append(deleteKeys, key)
				}

				bucket := impl.constructor(t)
				for k, v := range data {
					require.NoError(t, writeDataToFile(ctx, bucket, k, v))
				}
				for k, v := range deleteData {
					require.NoError(t, writeDataToFile(ctx, bucket, k, v))
				}

				// check keys are in bucket
				iter, err := bucket.List(ctx, "")
				require.NoError(t, err)
				for iter.Next(ctx) {
					assert.NoError(t, iter.Err())
					require.NotNil(t, iter.Item())
					_, ok1 := data[iter.Item().Name()]
					_, ok2 := deleteData[iter.Item().Name()]
					assert.True(t, ok1 || ok2)
				}

				assert.NoError(t, bucket.RemoveMatching(ctx, ".*"+postfix))
				iter, err = bucket.List(ctx, "")
				require.NoError(t, err)
				for iter.Next(ctx) {
					assert.NoError(t, iter.Err())
					require.NotNil(t, iter.Item())
					_, ok := data[iter.Item().Name()]
					assert.True(t, ok)
					_, ok = deleteData[iter.Item().Name()]
					assert.False(t, ok)
				}
			})
			t.Run("RemoveMatchingInvalidExpression", func(t *testing.T) {
				bucket := impl.constructor(t)
				assert.Error(t, bucket.RemoveMatching(ctx, "["))
			})
			t.Run("ReadWriteRoundTripSimple", func(t *testing.T) {
				bucket := impl.constructor(t)
				key := newUUID()
				payload := "hello world!"
				require.NoError(t, writeDataToFile(ctx, bucket, key, payload))

				data, err := readDataFromFile(ctx, bucket, key)
				assert.NoError(t, err)
				assert.Equal(t, data, payload)
			})
			t.Run("GetRetrievesData", func(t *testing.T) {
				bucket := impl.constructor(t)
				key := newUUID()
				assert.NoError(t, writeDataToFile(ctx, bucket, key, "hello world!"))

				reader, err := bucket.Get(ctx, key)
				require.NoError(t, err)
				data, err := ioutil.ReadAll(reader)
				require.NoError(t, err)
				assert.Equal(t, "hello world!", string(data))

				// dry run bucket also retrieves data
				dryRunBucket := clone(bucket, true)
				reader, err = dryRunBucket.Get(ctx, key)
				require.NoError(t, err)
				data, err = ioutil.ReadAll(reader)
				require.NoError(t, err)
				assert.Equal(t, "hello world!", string(data))
			})
			t.Run("PutSavesFiles", func(t *testing.T) {
				const contents = "check data"
				bucket := impl.constructor(t)
				key := newUUID()
				assert.NoError(t, bucket.Put(ctx, key, bytes.NewBuffer([]byte(contents))))

				reader, err := bucket.Get(ctx, key)
				require.NoError(t, err)
				data, err := ioutil.ReadAll(reader)
				require.NoError(t, err)
				assert.Equal(t, contents, string(data))
			})
			t.Run("PutWithDryRunDoesNotSaveFiles", func(t *testing.T) {
				const contents = "check data"
				bucket := clone(impl.constructor(t), true)
				key := newUUID()
				assert.NoError(t, bucket.Put(ctx, key, bytes.NewBuffer([]byte(contents))))

				_, err := bucket.Get(ctx, key)
				assert.Error(t, err)
			})
			t.Run("CopyDuplicatesData", func(t *testing.T) {
				const contents = "this one"
				bucket := impl.constructor(t)
				keyOne := newUUID()
				keyTwo := newUUID()
				assert.NoError(t, writeDataToFile(ctx, bucket, keyOne, contents))
				options := CopyOptions{
					SourceKey:         keyOne,
					DestinationKey:    keyTwo,
					DestinationBucket: bucket,
				}
				assert.NoError(t, bucket.Copy(ctx, options))
				data, err := readDataFromFile(ctx, bucket, keyTwo)
				require.NoError(t, err)
				assert.Equal(t, contents, data)
			})
			t.Run("CopyDoesNotDuplicateDataToDryRunBucket", func(t *testing.T) {
				const contents = "this one"
				bucket := impl.constructor(t)
				dryRunBucket := clone(bucket, true)
				keyOne := newUUID()
				keyTwo := newUUID()
				assert.NoError(t, writeDataToFile(ctx, bucket, keyOne, contents))
				options := CopyOptions{
					SourceKey:         keyOne,
					DestinationKey:    keyTwo,
					DestinationBucket: dryRunBucket,
				}
				assert.NoError(t, bucket.Copy(ctx, options))
				_, err := dryRunBucket.Get(ctx, keyTwo)
				assert.Error(t, err)
			})
			t.Run("CopyDuplicatesDataFromDryRunBucket", func(t *testing.T) {
				const contents = "this one"
				bucket := impl.constructor(t)
				dryRunBucket := clone(bucket, true)
				keyOne := newUUID()
				keyTwo := newUUID()
				assert.NoError(t, writeDataToFile(ctx, bucket, keyOne, contents))
				options := CopyOptions{
					SourceKey:         keyOne,
					DestinationKey:    keyTwo,
					DestinationBucket: bucket,
				}
				assert.NoError(t, dryRunBucket.Copy(ctx, options))
				data, err := readDataFromFile(ctx, bucket, keyTwo)
				require.NoError(t, err)
				assert.Equal(t, contents, data)
			})

			t.Run("CopyDuplicatesToDifferentBucket", func(t *testing.T) {
				const contents = "this one"
				srcBucket := impl.constructor(t)
				destBucket := impl.constructor(t)
				keyOne := newUUID()
				keyTwo := newUUID()
				assert.NoError(t, writeDataToFile(ctx, srcBucket, keyOne, contents))
				options := CopyOptions{
					SourceKey:         keyOne,
					DestinationKey:    keyTwo,
					DestinationBucket: destBucket,
				}
				assert.NoError(t, srcBucket.Copy(ctx, options))
				data, err := readDataFromFile(ctx, destBucket, keyTwo)
				require.NoError(t, err)
				assert.Equal(t, contents, data)
			})
			t.Run("DownloadWritesFileToDisk", func(t *testing.T) {
				const contents = "in the file"
				bucket := impl.constructor(t)
				key := newUUID()
				path := filepath.Join(tempdir, uuid, key)
				assert.NoError(t, writeDataToFile(ctx, bucket, key, contents))

				_, err := os.Stat(path)
				assert.True(t, os.IsNotExist(err))
				assert.NoError(t, bucket.Download(ctx, key, path))
				_, err = os.Stat(path)
				assert.False(t, os.IsNotExist(err))

				data, err := ioutil.ReadFile(path)
				require.NoError(t, err)
				assert.Equal(t, contents, string(data))

				// writes file to disk with dry run bucket
				dryRunBucket := clone(bucket, true)
				path = filepath.Join(tempdir, uuid, newUUID())
				_, err = os.Stat(path)
				assert.True(t, os.IsNotExist(err))
				assert.NoError(t, dryRunBucket.Download(ctx, key, path))
				_, err = os.Stat(path)
				assert.False(t, os.IsNotExist(err))

				data, err = ioutil.ReadFile(path)
				require.NoError(t, err)
				assert.Equal(t, contents, string(data))
			})
			t.Run("ListRespectsPrefixes", func(t *testing.T) {
				bucket := impl.constructor(t)
				key := newUUID()

				assert.NoError(t, writeDataToFile(ctx, bucket, key, "foo/bar"))

				// there's one thing in the iterator
				// with the correct prefix
				iter, err := bucket.List(ctx, "")
				require.NoError(t, err)
				assert.True(t, iter.Next(ctx))
				assert.False(t, iter.Next(ctx))
				assert.NoError(t, iter.Err())

				// there's nothing in the iterator
				// with a prefix
				iter, err = bucket.List(ctx, "bar")
				require.NoError(t, err)
				assert.False(t, iter.Next(ctx))
				assert.Nil(t, iter.Item())
				assert.NoError(t, iter.Err())
			})
			t.Run("RoundTripManyFiles", func(t *testing.T) {
				data := map[string]string{}
				for i := 0; i < 300; i++ {
					data[newUUID()] = strings.Join([]string{newUUID(), newUUID(), newUUID()}, "\n")
				}

				bucket := impl.constructor(t)
				for k, v := range data {
					require.NoError(t, writeDataToFile(ctx, bucket, k, v))
				}

				iter, err := bucket.List(ctx, "")
				require.NoError(t, err)
				count := 0
				for iter.Next(ctx) {
					count++
					item := iter.Item()
					require.NotNil(t, item)

					key := item.Name()
					_, ok := data[key]
					require.True(t, ok)
					assert.NotZero(t, item.Bucket())

					reader, err := item.Get(ctx)
					require.NoError(t, err)
					require.NotNil(t, reader)
					out, err := ioutil.ReadAll(reader)
					assert.NoError(t, err)
					assert.NoError(t, reader.Close())
					assert.Equal(t, string(out), data[item.Name()])
				}
				assert.NoError(t, iter.Err())
				assert.Equal(t, len(data), count)
			})
			t.Run("PullFromBucket", func(t *testing.T) {
				data := map[string]string{}
				for i := 0; i < 100; i++ {
					data[newUUID()] = strings.Join([]string{newUUID(), newUUID(), newUUID()}, "\n")
				}

				bucket := impl.constructor(t)
				for k, v := range data {
					assert.NoError(t, writeDataToFile(ctx, bucket, k, v))
				}

				mirror := filepath.Join(tempdir, "pull-one", newUUID())
				require.NoError(t, os.MkdirAll(mirror, 0700))
				for i := 0; i < 3; i++ {
					assert.NoError(t, bucket.Pull(ctx, mirror, ""))
					files, err := walkLocalTree(ctx, mirror)
					require.NoError(t, err)
					assert.Len(t, files, 100)

					if impl.name != "LegacyGridFS" {
						for _, fn := range files {
							_, ok := data[filepath.Base(fn)]
							assert.True(t, ok)
						}
					}
				}

				// should work with dry run bucket
				dryRunBucket := clone(bucket, true)
				mirror = filepath.Join(tempdir, "pull-one", newUUID())
				require.NoError(t, os.MkdirAll(mirror, 0700))
				for i := 0; i < 3; i++ {
					assert.NoError(t, dryRunBucket.Pull(ctx, mirror, ""))
					files, err := walkLocalTree(ctx, mirror)
					require.NoError(t, err)
					assert.Len(t, files, 100)

					if impl.name != "LegacyGridFS" {
						for _, fn := range files {
							_, ok := data[filepath.Base(fn)]
							assert.True(t, ok)
						}
					}
				}
			})
			t.Run("PushToBucket", func(t *testing.T) {
				prefix := filepath.Join(tempdir, newUUID())
				for i := 0; i < 100; i++ {
					require.NoError(t, writeDataToDisk(prefix,
						newUUID(), strings.Join([]string{newUUID(), newUUID(), newUUID()}, "\n")))
				}

				bucket := impl.constructor(t)
				t.Run("NoPrefix", func(t *testing.T) {
					assert.NoError(t, bucket.Push(ctx, prefix, ""))
					assert.NoError(t, bucket.Push(ctx, prefix, ""))
				})
				t.Run("ShortPrefix", func(t *testing.T) {
					assert.NoError(t, bucket.Push(ctx, prefix, "foo"))
					assert.NoError(t, bucket.Push(ctx, prefix, "foo"))
				})
				t.Run("DryRunBucketDoesNotPush", func(t *testing.T) {
					dryRunBucket := clone(bucket, true)
					assert.NoError(t, dryRunBucket.Push(ctx, prefix, "bar"))
					assert.NoError(t, dryRunBucket.Push(ctx, prefix, "bar"))
				})
				t.Run("BucketContents", func(t *testing.T) {
					iter, err := bucket.List(ctx, "")
					require.NoError(t, err)
					counter := 0
					for iter.Next(ctx) {
						counter++
					}
					assert.NoError(t, iter.Err())
					assert.Equal(t, 200, counter)
				})
			})
			t.Run("UploadWithBadFileName", func(t *testing.T) {
				bucket := impl.constructor(t)
				err := bucket.Upload(ctx, "key", "foo\x00bar")
				require.Error(t, err)
				assert.Contains(t, err.Error(), "problem opening file")
			})
			t.Run("DownloadWithBadFileName", func(t *testing.T) {
				bucket := impl.constructor(t)
				err := bucket.Download(ctx, "fileIWant\x00", "loc")
				assert.Error(t, err)
			})
			t.Run("DownloadBadDirectory", func(t *testing.T) {
				bucket := impl.constructor(t)
				fn := filepath.Base(file)
				err := bucket.Upload(ctx, "key", fn)
				require.NoError(t, err)

				err = bucket.Download(ctx, "key", "location-\x00/key-name")
				require.Error(t, err)
				assert.Contains(t, err.Error(), "problem creating enclosing directory")
			})
			t.Run("DownloadToBadFileName", func(t *testing.T) {
				bucket := impl.constructor(t)
				fn := filepath.Base(file)
				err := bucket.Upload(ctx, "key", fn)
				require.NoError(t, err)

				err = bucket.Download(ctx, "key", "location-\x00-key-name")
				require.Error(t, err)
				assert.Contains(t, err.Error(), "problem creating file")
			})
		})
	}
}

func writeDataToDisk(prefix, key, data string) error {
	if err := os.MkdirAll(prefix, 0700); err != nil {
		return errors.WithStack(err)
	}
	path := filepath.Join(prefix, key)
	return errors.WithStack(ioutil.WriteFile(path, []byte(data), 0600))
}

func writeDataToFile(ctx context.Context, bucket Bucket, key, data string) error {
	wctx, cancel := context.WithCancel(ctx)
	defer cancel()

	writer, err := bucket.Writer(wctx, key)
	if err != nil {
		return errors.WithStack(err)
	}

	_, err = writer.Write([]byte(data))
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(writer.Close())
}

func readDataFromFile(ctx context.Context, bucket Bucket, key string) (string, error) {
	rctx, cancel := context.WithCancel(ctx)
	defer cancel()

	reader, err := bucket.Reader(rctx, key)
	if err != nil {
		return "", errors.WithStack(err)
	}
	out, err := ioutil.ReadAll(reader)
	if err != nil {
		return "", errors.WithStack(err)
	}

	err = reader.Close()
	if err != nil {
		return "", errors.WithStack(err)
	}

	return string(out), nil

}

type brokenWriter struct{}

func (*brokenWriter) Write(_ []byte) (int, error) { return -1, errors.New("always") }
func (*brokenWriter) Read(_ []byte) (int, error)  { return -1, errors.New("always") }

func clone(b Bucket, dryRun bool) Bucket {
	switch i := b.(type) {
	case *s3BucketSmall:
		return &s3BucketSmall{
			s3Bucket: s3Bucket{
				name:        i.name,
				prefix:      i.prefix,
				sess:        i.sess,
				svc:         s3.New(i.sess),
				permission:  i.permission,
				contentType: i.contentType,
				dryRun:      dryRun,
			},
		}
	case *s3BucketLarge:
		return &s3BucketLarge{
			s3Bucket: s3Bucket{
				name:        i.name,
				prefix:      i.prefix,
				sess:        i.sess,
				svc:         s3.New(i.sess),
				permission:  i.permission,
				contentType: i.contentType,
				dryRun:      dryRun,
			},
			minPartSize: i.minPartSize,
		}
	case *localFileSystem:
		return &localFileSystem{path: i.path, dryRun: dryRun}
	case *gridfsLegacyBucket:
		opts := i.opts
		opts.DryRun = dryRun
		return &gridfsLegacyBucket{
			opts:    opts,
			session: i.session,
		}
	default:
		return nil
	}
}
