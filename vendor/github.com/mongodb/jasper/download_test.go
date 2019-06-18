package jasper

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/mholt/archiver"
	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tychoish/bond"
	"github.com/tychoish/bond/recall"
	"github.com/tychoish/lru"
)

// Caller is responsible for giving a valid path.
func validMongoDBDownloadOptions() MongoDBDownloadOptions {
	target := runtime.GOOS
	if target == "darwin" {
		target = "osx"
	}

	edition := "enterprise"
	if target == "linux" {
		edition = "base"
	}

	return MongoDBDownloadOptions{
		BuildOpts: bond.BuildOptions{
			Target:  target,
			Arch:    bond.MongoDBArch("x86_64"),
			Edition: bond.MongoDBEdition(edition),
			Debug:   false,
		},
		Releases: []string{"4.0-current"},
	}
}

func addFileToDirectory(dir string, fileName string, fileContents string) error {
	if format := archiver.MatchingFormat(fileName); format != nil {
		tmpFile, err := ioutil.TempFile(dir, "tmp.txt")
		if err != nil {
			return err
		}
		defer os.Remove(tmpFile.Name())
		if _, err := tmpFile.Write([]byte(fileContents)); err != nil {
			return err
		}
		if err := tmpFile.Close(); err != nil {
			return err
		}

		if err := format.Make(filepath.Join(dir, fileName), []string{tmpFile.Name()}); err != nil {
			return err
		}
		return nil
	}

	file, err := os.Create(filepath.Join(dir, fileName))
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write([]byte(fileContents)); err != nil {
		return err
	}
	return nil
}

func TestSetupDownloadMongoDBReleasesFailsWithZeroOptions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), taskTimeout)
	defer cancel()

	opts := MongoDBDownloadOptions{}
	err := SetupDownloadMongoDBReleases(ctx, lru.NewCache(), opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "problem creating enclosing directories")
}

func TestSetupDownloadMongoDBReleasesWithInvalidPath(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), taskTimeout)
	defer cancel()

	opts := validMongoDBDownloadOptions()
	absPath, err := filepath.Abs("download_test.go")
	require.NoError(t, err)
	opts.Path = absPath

	err = SetupDownloadMongoDBReleases(ctx, lru.NewCache(), opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "problem creating enclosing directories")
}

func TestSetupDownloadMongoDBReleasesWithInvalidArtifactsFeed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), taskTimeout)
	defer cancel()

	dir, err := ioutil.TempDir("build", "out")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	opts := validMongoDBDownloadOptions()
	absDir, err := filepath.Abs(dir)
	require.NoError(t, err)
	opts.Path = filepath.Join(absDir, "full.json")

	err = SetupDownloadMongoDBReleases(ctx, lru.NewCache(), opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "problem making artifacts feed")
}

func TestCreateValidDownloadJobs(t *testing.T) {
	dir, err := ioutil.TempDir("build", "out")
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
	dir := "download_test.go"
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
	ctx, cancel := context.WithTimeout(context.Background(), longTaskTimeout)
	defer cancel()

	downloadDir, err := ioutil.TempDir("build", "download_test")
	require.NoError(t, err)
	defer os.RemoveAll(downloadDir)

	serverDir, err := ioutil.TempDir("build", "download_test_server")
	require.NoError(t, err)
	defer os.RemoveAll(serverDir)

	fileName := "foo.zip"
	fileContents := "foo"
	require.NoError(t, addFileToDirectory(serverDir, fileName, fileContents))

	port := getPortNumber()
	serverAddr := fmt.Sprintf("localhost:%d", port)
	server := &http.Server{Addr: serverAddr, Handler: http.FileServer(http.Dir(serverDir))}
	defer func() {
		assert.NoError(t, server.Close())
	}()
	go func() {
		server.ListenAndServe()
	}()

	job, err := recall.NewDownloadJob(fmt.Sprintf("http://%s/%s", serverAddr, fileName), downloadDir, true)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	q := queue.NewLocalUnordered(2)
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

	absPath, err := filepath.Abs("build")
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
		format        ArchiveFormat
	}{
		"Auto": {
			archiveMaker:  archiver.TarGz,
			expectSuccess: true,
			fileExtension: ".tar.gz",
			format:        ArchiveAuto,
		},
		"TarGz": {
			archiveMaker:  archiver.TarGz,
			expectSuccess: true,
			fileExtension: ".tar.gz",
			format:        ArchiveTarGz,
		},
		"Zip": {
			archiveMaker:  archiver.Zip,
			expectSuccess: true,
			fileExtension: ".zip",
			format:        ArchiveZip,
		},
		"InvalidArchiveFormat": {
			archiveMaker:  archiver.TarGz,
			expectSuccess: false,
			fileExtension: ".foo",
			format:        ArchiveFormat("foo"),
		},
		"MismatchedArchiveFileAndFormat": {
			archiveMaker:  archiver.TarGz,
			expectSuccess: false,
			fileExtension: ".tar.gz",
			format:        ArchiveZip,
		},
	} {
		t.Run(testName, func(t *testing.T) {
			file, err := ioutil.TempFile("build", "out.txt")
			require.NoError(t, err)
			defer os.Remove(file.Name())
			archiveFile, err := ioutil.TempFile("build", "out"+testCase.fileExtension)
			require.NoError(t, err)
			defer os.Remove(archiveFile.Name())
			extractDir, err := ioutil.TempDir("build", "out")
			require.NoError(t, err)
			defer os.RemoveAll(extractDir)

			require.NoError(t, testCase.archiveMaker.Make(archiveFile.Name(), []string{file.Name()}))

			info := DownloadInfo{
				Path: archiveFile.Name(),
				ArchiveOpts: ArchiveOptions{
					ShouldExtract: true,
					Format:        testCase.format,
					TargetPath:    extractDir,
				},
			}
			if !testCase.expectSuccess {
				assert.Error(t, doExtract(info))
				return
			}
			assert.NoError(t, doExtract(info))

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
	file, err := ioutil.TempFile("build", "out.txt")
	require.NoError(t, err)
	defer os.Remove(file.Name())

	info := DownloadInfo{
		URL:  "https://example.com",
		Path: file.Name(),
		ArchiveOpts: ArchiveOptions{
			ShouldExtract: true,
			Format:        ArchiveAuto,
			TargetPath:    "build",
		},
	}
	err = doExtract(info)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not detect archive format")
}
