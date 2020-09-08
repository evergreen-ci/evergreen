package benchmarks

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/pail/testutil"
	"github.com/evergreen-ci/poplar"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

func buildDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filepath.Dir(file)), "build")
}

// RunSyncBucket runs the bucket benchmark suite.
func RunSyncBucket(ctx context.Context) error {
	dir := buildDir()
	prefix := filepath.Join(dir, fmt.Sprintf("sync-bucket-benchmarks-%d", time.Now().Unix()))
	if err := os.MkdirAll(prefix, os.ModePerm); err != nil {
		return errors.Wrap(err, "creating benchmark directory")
	}

	resultFile, err := os.Create(filepath.Join(prefix, "results.txt"))
	if err != nil {
		return errors.Wrap(err, "creating result file")
	}

	var resultText string
	s := syncBucketBenchmarkSuite()
	res, err := s.Run(ctx, prefix)
	if err != nil {
		resultText = err.Error()
	} else {
		resultText = res.Report()
	}

	catcher := grip.NewBasicCatcher()
	_, err = resultFile.WriteString(resultText)
	catcher.Add(errors.Wrap(err, "writing benchmark results to file"))
	catcher.Add(resultFile.Close())

	return catcher.Resolve()
}

type syncBucketConstructor func(pail.S3Options) (pail.SyncBucket, error)
type payloadConstructor func(numFiles int, bytesPerFile int) makePayload
type makePayload func(context.Context, pail.SyncBucket, pail.S3Options) (dir string, err error)

func basicPullIteration(makeBucket syncBucketConstructor, makePayload makePayload, opts pail.S3Options) poplar.Benchmark {
	return func(ctx context.Context, r poplar.Recorder, count int) error {
		for i := 0; i < count; i++ {
			if err := func() error {
				b, err := makeBucket(opts)
				if err != nil {
					return errors.Wrap(err, "making bucket")
				}
				defer func() {
					grip.Error(errors.Wrap(testutil.CleanupS3Bucket(opts.Credentials, opts.Name, opts.Prefix, opts.Region), "cleaning up remote store"))
				}()

				dir, err := makePayload(ctx, b, opts)
				defer func() {
					grip.Error(errors.Wrap(os.RemoveAll(dir), "cleaning up payload"))
				}()
				if err != nil {
					return errors.Wrap(err, "setting up benchmark test case payload")
				}

				if err := runBasicPullIteration(ctx, r, b, opts); err != nil {
					return errors.Wrapf(err, "iteration %d", i)
				}

				return nil
			}(); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}
}

func runBasicPullIteration(ctx context.Context, r poplar.Recorder, b pail.SyncBucket, opts pail.S3Options) error {
	bctx, cancel := context.WithCancel(ctx)
	defer cancel()

	local, err := ioutil.TempDir(buildDir(), "sync-bucket-benchmarks-pull")
	if err != nil {
		return errors.Wrap(err, "making local directory for pull")
	}
	defer func() {
		grip.Error(errors.Wrap(os.RemoveAll(local), "cleaning up local pull directory"))
	}()

	startAt := time.Now()
	r.BeginIteration()

	syncOpts := pail.SyncOptions{
		Local:  local,
		Remote: opts.Prefix,
	}
	errChan := make(chan error)
	go func() {
		select {
		case errChan <- b.Pull(bctx, syncOpts):
		case <-bctx.Done():
		}
	}()

	catcher := grip.NewBasicCatcher()
	select {
	case err := <-errChan:
		r.EndIteration(time.Since(startAt))
		catcher.Wrap(err, "pulling directory from remote store")
	case <-bctx.Done():
		r.EndIteration(time.Since(startAt))
		catcher.Wrap(bctx.Err(), "cancelled pulling directory")
	}

	totalBytes, err := getDirTotalSize(local)
	if err != nil {
		catcher.Add(err)
	} else {
		r.IncSize(totalBytes)
	}

	return catcher.Resolve()
}

func basicPushIteration(makeBucket syncBucketConstructor, makePayload makePayload, opts pail.S3Options) poplar.Benchmark {
	return func(ctx context.Context, r poplar.Recorder, count int) error {
		for i := 0; i < count; i++ {
			if err := func() error {
				b, err := makeBucket(opts)
				if err != nil {
					return errors.Wrap(err, "making bucket")
				}
				defer func() {
					grip.Error(errors.Wrap(testutil.CleanupS3Bucket(opts.Credentials, opts.Name, opts.Prefix, opts.Region), "cleaning up remote store"))
				}()
				dir, err := makePayload(ctx, b, opts)
				defer func() {
					grip.Error(errors.Wrap(os.RemoveAll(dir), "cleaning up payload"))
				}()
				if err != nil {
					return errors.Wrap(err, "setting up benchmark test case payload")
				}
				if err := runBasicPushIteration(ctx, r, b, opts, dir); err != nil {
					return errors.Wrapf(err, "iteration %d", i)
				}
				return nil
			}(); err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}
}

func runBasicPushIteration(ctx context.Context, r poplar.Recorder, b pail.SyncBucket, opts pail.S3Options, dir string) error {
	bctx, cancel := context.WithCancel(ctx)
	defer cancel()

	startAt := time.Now()
	r.BeginIteration()

	syncOpts := pail.SyncOptions{
		Local:  dir,
		Remote: opts.Prefix,
	}
	errChan := make(chan error)
	go func() {
		select {
		case errChan <- b.Push(bctx, syncOpts):
		case <-bctx.Done():
		}
	}()

	catcher := grip.NewBasicCatcher()
	select {
	case err := <-errChan:
		r.EndIteration(time.Since(startAt))
		catcher.Wrap(err, "pushing directory to remote store")
		totalBytes, err := getDirTotalSize(dir)
		if err != nil {
			catcher.Add(err)
		} else {
			r.IncSize(totalBytes)
		}
	case <-bctx.Done():
		r.EndIteration(time.Since(startAt))
		catcher.Wrap(bctx.Err(), "cancelled pushing directory")
		// If the context is done (e.g. due to timeout) before the push
		// finishes, this won't increment the total bytes pushed since it would
		// require extra work to sum the size of all the objects pushed to S3.
	}

	return catcher.Resolve()
}

func getDirTotalSize(dir string) (int64, error) {
	var size int64
	if err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	}); err != nil {
		return -1, errors.Wrap(err, "summing total directory size")
	}
	return size, nil
}

func syncBucketBenchmarkSuite() poplar.BenchmarkSuite {
	var suite poplar.BenchmarkSuite
	bucketCases := map[string]syncBucketConstructor{
		"Small":   smallBucketConstructor,
		"Large":   largeBucketConstructor,
		"Archive": archiveBucketConstructor,
	}
	benchCases := map[string]struct {
		numFiles     int
		bytesPerFile int
		timeout      time.Duration
	}{
		"FewFilesLargeSize": {
			numFiles:     1,
			bytesPerFile: 1024 * 1024,
			timeout:      time.Hour,
		},
		"ManyFilesSmallSize": {
			numFiles:     1000,
			bytesPerFile: 10,
			timeout:      time.Hour,
		},
	}
	for bucketName, makeBucket := range bucketCases {
		for caseName, benchCase := range benchCases {
			suite = append(suite,
				&poplar.BenchmarkCase{
					CaseName:      fmt.Sprintf("%sBucket-Pull-%s-%dFilesEachWith%dBytes", bucketName, caseName, benchCase.numFiles, benchCase.bytesPerFile),
					Bench:         basicPullIteration(makeBucket, uploadLocalTree(benchCase.numFiles, benchCase.bytesPerFile), s3Opts()),
					Count:         1,
					MinRuntime:    1 * time.Nanosecond, // We have to set this to be non-zero even though the test does not use it.
					MaxRuntime:    benchCase.timeout,
					MinIterations: 1,
					MaxIterations: 2,
					Recorder:      poplar.RecorderPerf,
				},
				&poplar.BenchmarkCase{
					CaseName:      fmt.Sprintf("%sBucket-Push-%s-%dFilesEachWith%dBytes", bucketName, caseName, benchCase.numFiles, benchCase.bytesPerFile),
					Bench:         basicPushIteration(makeBucket, makeLocalTree(benchCase.numFiles, benchCase.bytesPerFile), s3Opts()),
					Count:         1,
					MinRuntime:    1 * time.Nanosecond, // We have to set this to be non-zero even though the test does not use it.
					MaxRuntime:    benchCase.timeout,
					MinIterations: 1,
					MaxIterations: 2,
					Recorder:      poplar.RecorderPerf,
				},
			)
		}
	}
	return suite
}

func uploadLocalTree(numFiles int, bytesPerFile int) makePayload {
	return func(ctx context.Context, b pail.SyncBucket, opts pail.S3Options) (dir string, err error) {
		local, err := ioutil.TempDir(buildDir(), "sync-bucket-benchmarks-setup-upload")
		if err != nil {
			return "", errors.Wrap(err, "setting up local setup test data")
		}
		defer func() {
			if err != nil {
				grip.Error(errors.Wrapf(os.RemoveAll(local), "cleaning up local directory '%s'", local))
			}
		}()

		for i := 0; i < numFiles; i++ {
			file := testutil.NewUUID()
			content := utility.MakeRandomString(bytesPerFile)
			if err := ioutil.WriteFile(filepath.Join(local, file), []byte(content), 0777); err != nil {
				return "", errors.Wrap(err, "writing local setup test data file")
			}
		}

		if err := b.Push(ctx, pail.SyncOptions{
			Local:  local,
			Remote: opts.Prefix,
		}); err != nil {
			return "", errors.Wrap(err, "uploading setup test data")
		}
		return local, nil
	}
}

func makeLocalTree(numFiles int, bytesPerFile int) makePayload {
	return func(ctx context.Context, b pail.SyncBucket, opts pail.S3Options) (dir string, err error) {
		local, err := ioutil.TempDir(buildDir(), "sync-bucket-benchmarks-setup-upload")
		if err != nil {
			return "", errors.Wrap(err, "setting up local setup test data")
		}
		defer func() {
			if err != nil {
				grip.Error(errors.Wrapf(os.RemoveAll(local), "cleaning up local directory '%s'", local))
			}
		}()

		for i := 0; i < numFiles; i++ {
			file := testutil.NewUUID()
			content := utility.MakeRandomString(bytesPerFile)
			if err := ioutil.WriteFile(filepath.Join(local, file), []byte(content), 0777); err != nil {
				return "", errors.Wrap(err, "writing local setup test data file")
			}
		}

		return local, nil
	}
}

func s3Opts() pail.S3Options {
	return pail.S3Options{
		Credentials: pail.CreateAWSCredentials(os.Getenv("AWS_KEY"), os.Getenv("AWS_SECRET"), ""),
		Region:      "us-east-1",
		Name:        "build-test-curator",
		Prefix:      testutil.NewUUID(),
		MaxRetries:  20,
	}
}

func smallBucketConstructor(opts pail.S3Options) (pail.SyncBucket, error) {
	b, err := pail.NewS3Bucket(opts)
	if err != nil {
		return nil, errors.Wrap(err, "making small bucket")
	}
	return b, nil
}

func largeBucketConstructor(opts pail.S3Options) (pail.SyncBucket, error) {
	b, err := pail.NewS3MultiPartBucket(opts)
	if err != nil {
		return nil, errors.Wrap(err, "making large bucket")
	}
	return b, nil
}

func archiveBucketConstructor(opts pail.S3Options) (pail.SyncBucket, error) {
	b, err := pail.NewS3ArchiveBucket(opts)
	if err != nil {
		return nil, errors.Wrap(err, "making archive bucket")
	}
	return b, nil
}
