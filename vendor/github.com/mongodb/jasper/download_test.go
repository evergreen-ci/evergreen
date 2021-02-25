package jasper

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/evergreen-ci/bond/recall"
	"github.com/evergreen-ci/lru"
	"github.com/evergreen-ci/utility"
	"github.com/mholt/archiver"
	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/grip"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/testutil"
	testoptions "github.com/mongodb/jasper/testutil/options"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupDownloadMongoDBReleasesFailsWithZeroOptions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testutil.TestTimeout)
	defer cancel()

	opts := options.MongoDBDownload{}
	err := SetupDownloadMongoDBReleases(ctx, lru.NewCache(), opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "problem creating enclosing directories")
}

func TestSetupDownloadMongoDBReleasesWithInvalidPath(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testutil.TestTimeout)
	defer cancel()

	opts := testoptions.ValidMongoDBDownloadOptions()
	_, path, _, ok := runtime.Caller(0)
	require.True(t, ok)
	absPath, err := filepath.Abs(path)
	require.NoError(t, err)
	opts.Path = absPath

	err = SetupDownloadMongoDBReleases(ctx, lru.NewCache(), opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "problem creating enclosing directories")
}

func TestSetupDownloadMongoDBReleasesWithInvalidArtifactsFeed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testutil.TestTimeout)
	defer cancel()

	dir, err := ioutil.TempDir(testutil.BuildDirectory(), "out")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	opts := testoptions.ValidMongoDBDownloadOptions()
	absDir, err := filepath.Abs(dir)
	require.NoError(t, err)
	opts.Path = filepath.Join(absDir, "full.json")

	err = SetupDownloadMongoDBReleases(ctx, lru.NewCache(), opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "problem making artifacts feed")
}

func TestCreateValidDownloadJobs(t *testing.T) {
	dir, err := ioutil.TempDir(testutil.BuildDirectory(), "out")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	urls := make(chan string)
	go func() {
		urls <- "https://example.com"
		close(urls)
	}()

	catcher := grip.NewBasicCatcher()
	jobs := createDownloadJobs(dir, urls, catcher)

	count := 0
	for job := range jobs {
		count++
		assert.Equal(t, 1, count)
		assert.NotNil(t, job)
	}

	assert.NoError(t, catcher.Resolve())
}

func TestCreateDownloadJobsWithInvalidPath(t *testing.T) {
	_, dir, _, ok := runtime.Caller(0)
	require.True(t, ok)
	urls := make(chan string)
	testURL := "https://example.com"

	catcher := grip.NewBasicCatcher()
	go func() {
		urls <- testURL
		close(urls)
	}()
	jobs := createDownloadJobs(dir, urls, catcher)

	for range jobs {
		assert.Fail(t, "should not create job for bad url")
	}
	err := catcher.Resolve()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "problem creating download job for "+testURL)
}

func TestProcessDownloadJobs(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testutil.LongTestTimeout)
	defer cancel()

	downloadDir, err := ioutil.TempDir(testutil.BuildDirectory(), "download_test")
	require.NoError(t, err)
	defer os.RemoveAll(downloadDir)

	fileServerDir, err := ioutil.TempDir(testutil.BuildDirectory(), "download_test_server")
	require.NoError(t, err)
	defer os.RemoveAll(fileServerDir)

	fileName := "foo.zip"
	fileContents := "foo"
	require.NoError(t, testutil.AddFileToDirectory(fileServerDir, fileName, fileContents))

	port := testutil.GetPortNumber()
	fileServerAddr := fmt.Sprintf("localhost:%d", port)
	fileServer := &http.Server{Addr: fileServerAddr, Handler: http.FileServer(http.Dir(fileServerDir))}
	defer func() {
		assert.NoError(t, fileServer.Close())
	}()
	listener, err := net.Listen("tcp", fileServerAddr)
	require.NoError(t, err)
	go func() {
		grip.Info(fileServer.Serve(listener))
	}()

	baseURL := fmt.Sprintf("http://%s", fileServerAddr)
	httpClient := utility.GetHTTPClient()
	defer utility.PutHTTPClient(httpClient)
	require.NoError(t, testutil.WaitForHTTPService(ctx, baseURL, httpClient))

	job, err := recall.NewDownloadJob(fmt.Sprintf("%s/%s", baseURL, fileName), downloadDir, true)
	require.NoError(t, err)

	q := queue.NewLocalLimitedSize(2, 1048)
	require.NoError(t, q.Start(ctx))
	require.NoError(t, q.Put(ctx, job))

	checkFileNonempty := func(fileName string) error {
		info, err := os.Stat(fileName)
		if err != nil {
			return err
		}
		if info.Size() == 0 {
			return errors.New("expected file to be non-empty")
		}
		return nil
	}
	assert.NoError(t, processDownloadJobs(ctx, checkFileNonempty)(q))
}

func TestAddMongoDBFilesToCacheWithInvalidPath(t *testing.T) {
	fileName := "foo.txt"
	_, err := os.Stat(fileName)
	require.True(t, os.IsNotExist(err))

	absPath, err := filepath.Abs(testutil.BuildDirectory())
	require.NoError(t, err)

	err = addMongoDBFilesToCache(lru.NewCache(), absPath)(fileName)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "problem adding file "+filepath.Join(absPath, fileName)+" to cache")
}

func TestDoExtract(t *testing.T) {
	for testName, testCase := range map[string]struct {
		archiveMaker  archiver.Archiver
		expectSuccess bool
		fileExtension string
		format        options.ArchiveFormat
	}{
		"Auto": {
			archiveMaker:  archiver.TarGz,
			expectSuccess: true,
			fileExtension: ".tar.gz",
			format:        options.ArchiveAuto,
		},
		"TarGz": {
			archiveMaker:  archiver.TarGz,
			expectSuccess: true,
			fileExtension: ".tar.gz",
			format:        options.ArchiveTarGz,
		},
		"Zip": {
			archiveMaker:  archiver.Zip,
			expectSuccess: true,
			fileExtension: ".zip",
			format:        options.ArchiveZip,
		},
		"InvalidArchiveFormat": {
			archiveMaker:  archiver.TarGz,
			expectSuccess: false,
			fileExtension: ".foo",
			format:        options.ArchiveFormat("foo"),
		},
		"MismatchedArchiveFileAndFormat": {
			archiveMaker:  archiver.TarGz,
			expectSuccess: false,
			fileExtension: ".tar.gz",
			format:        options.ArchiveZip,
		},
	} {
		t.Run(testName, func(t *testing.T) {
			file, err := ioutil.TempFile(testutil.BuildDirectory(), "out.txt")
			require.NoError(t, err)
			defer os.Remove(file.Name())
			archiveFile, err := ioutil.TempFile(testutil.BuildDirectory(), "out"+testCase.fileExtension)
			require.NoError(t, err)
			defer os.Remove(archiveFile.Name())
			extractDir, err := ioutil.TempDir(testutil.BuildDirectory(), "out")
			require.NoError(t, err)
			defer os.RemoveAll(extractDir)

			require.NoError(t, testCase.archiveMaker.Make(archiveFile.Name(), []string{file.Name()}))

			opts := options.Download{
				Path: archiveFile.Name(),
				ArchiveOpts: options.Archive{
					ShouldExtract: true,
					Format:        testCase.format,
					TargetPath:    extractDir,
				},
			}
			if !testCase.expectSuccess {
				assert.Error(t, opts.Extract())
				return
			}
			assert.NoError(t, opts.Extract())

			fileInfo, err := os.Stat(archiveFile.Name())
			require.NoError(t, err)
			assert.NotZero(t, fileInfo.Size())

			fileInfos, err := ioutil.ReadDir(extractDir)
			require.NoError(t, err)
			assert.Equal(t, 1, len(fileInfos))
		})
	}
}

func TestDoExtractUnarchivedFile(t *testing.T) {
	file, err := ioutil.TempFile(testutil.BuildDirectory(), "out.txt")
	require.NoError(t, err)
	defer os.Remove(file.Name())

	opts := options.Download{
		URL:  "https://example.com",
		Path: file.Name(),
		ArchiveOpts: options.Archive{
			ShouldExtract: true,
			Format:        options.ArchiveAuto,
			TargetPath:    "build",
		},
	}
	err = opts.Extract()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not detect archive format")
}
