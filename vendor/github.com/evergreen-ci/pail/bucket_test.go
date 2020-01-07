package pail

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	mgo "gopkg.in/mgo.v2"
)

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

type bucketTestCase struct {
	id   string
	test func(*testing.T, Bucket)
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

	mdburl := "mongodb://localhost:27017"
	ses, err := mgo.DialWithTimeout(mdburl, time.Second)
	require.NoError(t, err)
	defer ses.Close()
	defer func() { ses.DB(uuid).DropDatabase() }()

	s3BucketName := "build-test-curator"
	s3Prefix := newUUID() + "-"
	s3Region := "us-east-1"
	defer func() { require.NoError(t, cleanUpS3Bucket(s3BucketName, s3Prefix, s3Region)) }()

	client, err := mongo.NewClient(options.Client().ApplyURI(mdburl))
	require.NoError(t, err)
	connctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	require.NoError(t, client.Connect(connctx))

	for _, impl := range []struct {
		name        string
		constructor func(*testing.T) Bucket
		tests       []bucketTestCase
	}{
		{
			name: "Local",
			constructor: func(t *testing.T) Bucket {
				path := filepath.Join(tempdir, uuid)
				require.NoError(t, os.MkdirAll(path, 0777))
				return &localFileSystem{path: path, prefix: newUUID()}
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
						bucket.prefix = ""
						opts := SyncOptions{Remote: filepath.Dir(file)}
						err := b.Pull(tctx, opts)
						assert.Error(t, err)
					},
				},
				{
					id: "PushErrorsContext",
					test: func(t *testing.T, b Bucket) {
						tctx, cancel := context.WithCancel(ctx)
						cancel()
						opts := SyncOptions{Local: filepath.Dir(file)}
						err := b.Push(tctx, opts)
						assert.Error(t, err)
					},
				},
			},
		},
		{
			name: "GridFS",
			constructor: func(t *testing.T) Bucket {
				require.NoError(t, client.Database(uuid).Drop(ctx))
				b, err := NewGridFSBucketWithClient(ctx, client, GridFSOptions{
					Name:     newUUID(),
					Prefix:   newUUID(),
					Database: uuid,
				})
				require.NoError(t, err)
				return b
			},
		},
		{
			name: "LegacyGridFS",
			constructor: func(t *testing.T) Bucket {
				require.NoError(t, client.Database(uuid).Drop(ctx))
				b, err := NewLegacyGridFSBucketWithSession(ses.Clone(), GridFSOptions{
					Name:     newUUID(),
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
			tests: getS3SmallBucketTests(ctx, tempdir, s3BucketName, s3Prefix, s3Region),
		},
		{
			name: "S3BucketChecksums",
			constructor: func(t *testing.T) Bucket {
				s3Options := S3Options{
					Region:                 s3Region,
					Name:                   s3BucketName,
					Prefix:                 s3Prefix + newUUID(),
					MaxRetries:             20,
					UseSingleFileChecksums: true,
				}
				b, err := NewS3Bucket(s3Options)
				require.NoError(t, err)
				return b
			},
			tests: getS3SmallBucketTests(ctx, tempdir, s3BucketName, s3Prefix, s3Region),
		},
		{
			name: "ParallelLocal",
			constructor: func(t *testing.T) Bucket {
				t.Skip()
				path := filepath.Join(tempdir, uuid, newUUID())
				require.NoError(t, os.MkdirAll(path, 0777))
				bucket := &localFileSystem{path: path}

				return NewParallelSyncBucket(ParallelBucketOptions{Workers: runtime.NumCPU()}, bucket)
			},
		},
		{
			name: "ParallelS3Bucket",
			constructor: func(t *testing.T) Bucket {
				s3Options := S3Options{
					Region:                 s3Region,
					Name:                   s3BucketName,
					Prefix:                 s3Prefix + newUUID(),
					MaxRetries:             20,
					UseSingleFileChecksums: true,
				}
				b, err := NewS3Bucket(s3Options)
				require.NoError(t, err)
				return NewParallelSyncBucket(ParallelBucketOptions{Workers: runtime.NumCPU()}, b)
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
			tests: getS3LargeBucketTests(ctx, tempdir, s3BucketName, s3Prefix, s3Region),
		},
		{
			name: "S3MultiPartBucketChecksum",
			constructor: func(t *testing.T) Bucket {
				s3Options := S3Options{
					Region:                 s3Region,
					Name:                   s3BucketName,
					Prefix:                 s3Prefix + newUUID(),
					MaxRetries:             20,
					UseSingleFileChecksums: true,
				}
				b, err := NewS3MultiPartBucket(s3Options)
				require.NoError(t, err)
				return b
			},
			tests: getS3LargeBucketTests(ctx, tempdir, s3BucketName, s3Prefix, s3Region),
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

				// dry run does not write
				setDryRun(bucket, true)
				assert.NoError(t, writeDataToFile(ctx, bucket, newUUID(), "hello world!"))

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
				setDryRun(bucket, true)
				assert.NoError(t, bucket.Remove(ctx, key))
				setDryRun(bucket, false)

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
				assert.Len(t, keys, 20)
				for i := 0; i < 20; i++ {
					key := newUUID()
					deleteData[key] = strings.Join([]string{newUUID(), newUUID(), newUUID()}, "\n")
					deleteKeys = append(deleteKeys, key)
				}
				assert.Len(t, deleteKeys, 20)

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
				assert.Len(t, keys, 5)
				for i := 0; i < 5; i++ {
					key := prefix + newUUID()
					deleteData[key] = strings.Join([]string{newUUID(), newUUID(), newUUID()}, "\n")
					deleteKeys = append(deleteKeys, key)
				}
				assert.Len(t, deleteKeys, 5)

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
				assert.Len(t, keys, 5)
				for i := 0; i < 5; i++ {
					key := newUUID() + postfix
					deleteData[key] = strings.Join([]string{newUUID(), newUUID(), newUUID()}, "\n")
					deleteKeys = append(deleteKeys, key)
				}
				assert.Len(t, deleteKeys, 5)

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
				setDryRun(bucket, true)
				reader, err = bucket.Get(ctx, key)
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
				bucket := impl.constructor(t)
				setDryRun(bucket, true)
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
				dryRunBucket := impl.constructor(t)
				setDryRun(dryRunBucket, true)
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
				dryRunBucket := impl.constructor(t)
				keyOne := newUUID()
				keyTwo := newUUID()
				assert.NoError(t, writeDataToFile(ctx, dryRunBucket, keyOne, contents))
				setDryRun(dryRunBucket, true)
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
				require.True(t, os.IsNotExist(err))
				require.NoError(t, bucket.Download(ctx, key, path))
				_, err = os.Stat(path)
				require.False(t, os.IsNotExist(err))

				data, err := ioutil.ReadFile(path)
				require.NoError(t, err)
				require.Equal(t, contents, string(data))

				// writes file to disk with dry run bucket
				setDryRun(bucket, true)
				path = filepath.Join(tempdir, uuid, newUUID())
				_, err = os.Stat(path)
				require.True(t, os.IsNotExist(err))
				require.NoError(t, bucket.Download(ctx, key, path))
				_, err = os.Stat(path)
				require.False(t, os.IsNotExist(err))

				data, err = ioutil.ReadFile(path)
				require.NoError(t, err)
				require.Equal(t, contents, string(data))
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
				for i := 0; i < 3; i++ {
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
				for i := 0; i < 50; i++ {
					data[newUUID()] = strings.Join([]string{newUUID(), newUUID(), newUUID()}, "\n")
				}

				bucket := impl.constructor(t)
				for k, v := range data {
					require.NoError(t, writeDataToFile(ctx, bucket, k, v))
				}

				t.Run("BasicPull", func(t *testing.T) {
					mirror := filepath.Join(tempdir, "pull-one", newUUID())
					require.NoError(t, os.MkdirAll(mirror, 0700))
					opts := SyncOptions{Local: mirror}
					assert.NoError(t, bucket.Pull(ctx, opts))
					files, err := walkLocalTree(ctx, mirror)
					require.NoError(t, err)
					require.Len(t, files, 50)

					if !strings.Contains(impl.name, "GridFS") {
						for _, fn := range files {
							_, ok := data[filepath.Base(fn)]
							require.True(t, ok)
						}
					}
				})
				t.Run("DryRunBucketPulls", func(t *testing.T) {
					setDryRun(bucket, true)
					mirror := filepath.Join(tempdir, "pull-one", newUUID(), "")
					require.NoError(t, os.MkdirAll(mirror, 0700))
					opts := SyncOptions{Local: mirror}
					assert.NoError(t, bucket.Pull(ctx, opts))
					files, err := walkLocalTree(ctx, mirror)
					require.NoError(t, err)
					require.Len(t, files, 50)

					if !strings.Contains(impl.name, "GridFS") {
						for _, fn := range files {
							_, ok := data[filepath.Base(fn)]
							require.True(t, ok)
						}
					}
					setDryRun(bucket, false)
				})
				t.Run("PullWithExcludes", func(t *testing.T) {
					require.NoError(t, writeDataToFile(ctx, bucket, "python.py", "exclude"))
					require.NoError(t, writeDataToFile(ctx, bucket, "python2.py", "exclude2"))

					mirror := filepath.Join(tempdir, "not_excludes", newUUID())
					require.NoError(t, os.MkdirAll(mirror, 0700))
					opts := SyncOptions{Local: mirror}
					assert.NoError(t, bucket.Pull(ctx, opts))
					files, err := walkLocalTree(ctx, mirror)
					require.NoError(t, err)
					require.Len(t, files, 52)

					if !strings.Contains(impl.name, "GridFS") {
						for _, fn := range files {
							_, ok := data[filepath.Base(fn)]
							if !ok {
								ok = filepath.Base(fn) == "python.py" || filepath.Base(fn) == "python2.py"
							}
							require.True(t, ok)
						}
					}

					mirror = filepath.Join(tempdir, "excludes", newUUID())
					require.NoError(t, os.MkdirAll(mirror, 0700))
					opts.Local = mirror
					opts.Exclude = ".*\\.py"
					assert.NoError(t, bucket.Pull(ctx, opts))
					files, err = walkLocalTree(ctx, mirror)
					require.NoError(t, err)
					require.Len(t, files, 50)

					if !strings.Contains(impl.name, "GridFS") {
						for _, fn := range files {
							_, ok := data[filepath.Base(fn)]
							require.True(t, ok)
						}
					}

					require.NoError(t, bucket.Remove(ctx, "python.py"))
					require.NoError(t, bucket.Remove(ctx, "python2.py"))
				})
				t.Run("DeleteOnSync", func(t *testing.T) {
					setDeleteOnSync(bucket, true)

					// dry run bucket does not delete
					mirror := filepath.Join(tempdir, "pull-one", newUUID())
					require.NoError(t, os.MkdirAll(mirror, 0700))
					require.NoError(t, writeDataToDisk(mirror, "delete1", "should be deleted"))
					require.NoError(t, writeDataToDisk(mirror, "delete2", "this should also be deleted"))
					setDryRun(bucket, true)
					opts := SyncOptions{Local: mirror}
					require.NoError(t, bucket.Pull(ctx, opts))
					files, err := walkLocalTree(ctx, mirror)
					require.NoError(t, err)
					require.Len(t, files, 52)
					setDryRun(bucket, false)
					require.NoError(t, os.RemoveAll(mirror))

					// with out dry run set
					mirror = filepath.Join(tempdir, "pull-one", newUUID())
					require.NoError(t, os.MkdirAll(mirror, 0700))
					require.NoError(t, writeDataToDisk(mirror, "delete1", "should be deleted"))
					require.NoError(t, writeDataToDisk(mirror, "delete2", "this should also be deleted"))
					opts.Local = mirror
					assert.NoError(t, bucket.Pull(ctx, opts))
					files, err = walkLocalTree(ctx, mirror)
					require.NoError(t, err)
					assert.Len(t, files, 50)
					setDeleteOnSync(bucket, false)
				})
				t.Run("LargePull", func(t *testing.T) {
					prefix := newUUID()
					largeData := map[string]string{}
					for i := 0; i < 1050; i++ {
						largeData[newUUID()] = strings.Join([]string{newUUID(), newUUID(), newUUID()}, "\n")
					}
					for k, v := range largeData {
						require.NoError(t, writeDataToFile(ctx, bucket, prefix+"/"+k, v))
					}

					mirror := filepath.Join(tempdir, "pull-one", newUUID(), "")
					require.NoError(t, os.MkdirAll(mirror, 0700))

					opts := SyncOptions{Local: mirror, Remote: prefix}
					assert.NoError(t, bucket.Pull(ctx, opts))
					files, err := walkLocalTree(ctx, mirror)
					require.NoError(t, err)
					assert.Len(t, files, len(largeData))

					if !strings.Contains(impl.name, "GridFS") {
						for _, fn := range files {
							_, ok := largeData[fn]
							require.True(t, ok)
						}
					}
				})
			})
			t.Run("PushToBucket", func(t *testing.T) {
				prefix := filepath.Join(tempdir, newUUID())
				filenames := map[string]bool{}
				for i := 0; i < 50; i++ {
					fn := newUUID()
					filenames[fn] = true
					require.NoError(t, writeDataToDisk(prefix,
						fn, strings.Join([]string{newUUID(), newUUID(), newUUID()}, "\n")))
				}

				bucket := impl.constructor(t)
				t.Run("NoPrefix", func(t *testing.T) {
					opts := SyncOptions{Local: prefix}
					assert.NoError(t, bucket.Push(ctx, opts))

					iter, err := bucket.List(ctx, "")
					require.NoError(t, err)
					counter := 0
					for iter.Next(ctx) {
						require.True(t, filenames[iter.Item().Name()])
						counter++
					}
					assert.NoError(t, iter.Err())
					assert.Equal(t, 50, counter)
				})
				t.Run("ShortPrefix", func(t *testing.T) {
					remotePrefix := "foo"
					opts := SyncOptions{Local: prefix, Remote: remotePrefix}
					assert.NoError(t, bucket.Push(ctx, opts))

					iter, err := bucket.List(ctx, remotePrefix)
					require.NoError(t, err)
					counter := 0
					for iter.Next(ctx) {
						fn, err := filepath.Rel(remotePrefix, iter.Item().Name())
						require.NoError(t, err)
						require.True(t, filenames[fn])
						counter++
					}
					assert.NoError(t, iter.Err())
					assert.Equal(t, 50, counter)
				})
				t.Run("DryRunBucketDoesNotPush", func(t *testing.T) {
					remotePrefix := "bar"
					setDryRun(bucket, true)
					opts := SyncOptions{Local: prefix, Remote: remotePrefix}
					assert.NoError(t, bucket.Push(ctx, opts))

					iter, err := bucket.List(ctx, remotePrefix)
					require.NoError(t, err)
					counter := 0
					for iter.Next(ctx) {
						counter++
					}
					assert.NoError(t, iter.Err())
					assert.Equal(t, 0, counter)

					setDryRun(bucket, false)
				})
				t.Run("PushWithExcludes", func(t *testing.T) {
					require.NoError(t, writeDataToDisk(prefix, "python.py", "exclude"))
					require.NoError(t, writeDataToDisk(prefix, "python2.py", "exclude2"))

					remotePrefix := "not_excludes"
					opts := SyncOptions{Local: prefix, Remote: remotePrefix}
					assert.NoError(t, bucket.Push(ctx, opts))
					iter, err := bucket.List(ctx, remotePrefix)
					require.NoError(t, err)
					counter := 0
					for iter.Next(ctx) {
						fn, err := filepath.Rel(remotePrefix, iter.Item().Name())
						require.NoError(t, err)
						ok := filenames[fn]
						if !ok {
							ok = fn == "python.py" || fn == "python2.py"
						}
						require.True(t, ok)
						counter++
					}
					assert.NoError(t, iter.Err())
					assert.Equal(t, 52, counter)

					remotePrefix = "excludes"
					opts.Remote = remotePrefix
					opts.Exclude = ".*\\.py"
					assert.NoError(t, bucket.Push(ctx, opts))
					iter, err = bucket.List(ctx, remotePrefix)
					require.NoError(t, err)
					counter = 0
					for iter.Next(ctx) {
						fn, err := filepath.Rel(remotePrefix, iter.Item().Name())
						require.NoError(t, err)
						require.True(t, filenames[fn])
						counter++
					}
					assert.NoError(t, iter.Err())
					assert.Equal(t, 50, counter)

					require.NoError(t, os.RemoveAll(filepath.Join(prefix, "python.py")))
					require.NoError(t, os.RemoveAll(filepath.Join(prefix, "python2.py")))
				})
				t.Run("DeleteOnSync", func(t *testing.T) {
					setDeleteOnSync(bucket, true)

					contents := []byte("should be deleted")
					require.NoError(t, bucket.Put(ctx, filepath.Join("baz", "delete1"), bytes.NewBuffer(contents)))
					contents = []byte("this should also be deleted")
					require.NoError(t, bucket.Put(ctx, filepath.Join("baz", "delete2"), bytes.NewBuffer(contents)))

					// dry run bucket does not push or delete
					setDryRun(bucket, true)
					opts := SyncOptions{Local: prefix, Remote: "baz"}
					assert.NoError(t, bucket.Push(ctx, opts))
					setDryRun(bucket, false)
					iter, err := bucket.List(ctx, "baz")
					require.NoError(t, err)
					count := 0
					for iter.Next(ctx) {
						require.NotNil(t, iter.Item())
						count++
					}
					assert.Equal(t, 2, count)

					assert.NoError(t, bucket.Push(ctx, opts))
					iter, err = bucket.List(ctx, "baz")
					require.NoError(t, err)
					count = 0
					for iter.Next(ctx) {
						require.NotNil(t, iter.Item())
						count++
					}
					assert.Equal(t, 50, count)

					setDeleteOnSync(bucket, false)
				})
			})
			t.Run("UploadWithBadFileName", func(t *testing.T) {
				bucket := impl.constructor(t)
				err := bucket.Upload(ctx, "key", "foo\x00bar")
				require.Error(t, err)
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
			})
			t.Run("DownloadToBadFileName", func(t *testing.T) {
				bucket := impl.constructor(t)
				fn := filepath.Base(file)
				err := bucket.Upload(ctx, "key", fn)
				require.NoError(t, err)

				err = bucket.Download(ctx, "key", "location-\x00-key-name")
				require.Error(t, err)
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

func setDryRun(b Bucket, set bool) {
	switch i := b.(type) {
	case *localFileSystem:
		i.dryRun = set
	case *gridfsLegacyBucket:
		i.opts.DryRun = set
	case *s3BucketSmall:
		i.dryRun = set
	case *s3BucketLarge:
		i.dryRun = set
	case *gridfsBucket:
		i.opts.DryRun = set
	case *parallelBucketImpl:
		i.dryRun = set
		setDryRun(i.Bucket, set)
	}
}

func setDeleteOnSync(b Bucket, set bool) {
	switch i := b.(type) {
	case *localFileSystem:
		i.deleteOnSync = set
	case *gridfsLegacyBucket:
		i.opts.DeleteOnSync = set
	case *s3BucketSmall:
		i.deleteOnSync = set
	case *s3BucketLarge:
		i.deleteOnSync = set
	case *gridfsBucket:
		i.opts.DeleteOnSync = set
	case *parallelBucketImpl:
		i.deleteOnSync = set
		setDeleteOnSync(i.Bucket, set)
	}
}
