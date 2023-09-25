package command

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type AutoExtractSuite struct {
	ctx    context.Context
	cancel context.CancelFunc
	conf   *internal.TaskConfig
	comm   client.Communicator
	logger client.LoggerProducer

	cmd            *autoExtract
	targetLocation string
	params         map[string]interface{}

	suite.Suite
}

func TestAutoExtractSuite(t *testing.T) {
	suite.Run(t, new(AutoExtractSuite))
}

func (s *AutoExtractSuite) SetupTest() {
	var err error
	s.targetLocation = s.T().TempDir()

	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.comm = client.NewMock("http://localhost.com")
	s.conf = &internal.TaskConfig{
		Expansions: util.Expansions{},
		Task:       task.Task{},
		Project:    model.Project{},
		WorkDir:    s.targetLocation,
	}
	s.logger, err = s.comm.GetLoggerProducer(s.ctx, client.TaskData{ID: s.conf.Task.Id, Secret: s.conf.Task.Secret}, nil)
	s.NoError(err)

	s.cmd = &autoExtract{}
	s.params = map[string]interface{}{}
}

func (s *AutoExtractSuite) TearDownTest() {
	s.cancel()
}

func (s *AutoExtractSuite) TestNilArguments() {
	s.Error(s.cmd.ParseParams(nil))
	s.Error(s.cmd.ParseParams(s.params))
}

func (s *AutoExtractSuite) TestMalformedParams() {
	s.params["exclude_files"] = 1
	s.params["path"] = "foo"
	s.Error(s.cmd.ParseParams(s.params))
}

func (s *AutoExtractSuite) TestCorrectParams() {
	s.params["path"] = "foo"
	s.NoError(s.cmd.ParseParams(s.params))
}

func (s *AutoExtractSuite) TestErrorWhenExcludeFilesExist() {
	s.cmd.ExcludeFiles = []string{"foo"}
	s.cmd.ArchivePath = "bar"
	s.Error(s.cmd.ParseParams(s.params))
}

func (s *AutoExtractSuite) TestErrorsWithMalformedExpansions() {
	s.cmd.TargetDirectory = "${foo"
	s.Error(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
}

func (s *AutoExtractSuite) TestErrorsIfNoTarget() {
	s.Zero(s.cmd.TargetDirectory)
	s.Error(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
}

func (s *AutoExtractSuite) TestErrorsAndNormalizedPath() {
	s.cmd.TargetDirectory = "foo"
	s.cmd.ArchivePath = "bar"

	s.Error(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
	s.Contains(s.cmd.TargetDirectory, s.conf.WorkDir)
	s.Contains(s.cmd.ArchivePath, s.conf.WorkDir)
}

func (s *AutoExtractSuite) TestExtractionArchiveDoesNotExist() {
	s.cmd.TargetDirectory = s.targetLocation
	s.cmd.ArchivePath = filepath.Join(testutil.GetDirectoryOfFile(),
		"testdata", "archive", "artifacts.tar.gauto")

	s.Error(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
}

func (s *AutoExtractSuite) TestExtractionFileExistsAndIsNotArchive() {
	s.cmd.TargetDirectory = s.targetLocation
	_, thisFile, _, _ := runtime.Caller(0)
	s.cmd.ArchivePath = thisFile

	s.Error(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
}

func (s *AutoExtractSuite) TestExtractionZipSucceedsButIsNotIdempotent() {
	s.cmd.TargetDirectory = s.targetLocation
	s.cmd.ArchivePath = filepath.Join(testutil.GetDirectoryOfFile(),
		"testdata", "archive", "artifacts.zip")

	s.NoError(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))

	checkCommonExtractedArchiveContents(s.T(), s.cmd.TargetDirectory)

	// Extracting the same archive contents multiple times to the same directory
	// results in an error because the files already exist.
	s.Error(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
}

func (s *AutoExtractSuite) TestExtractionTarSucceedsButIsNotIdempotent() {
	s.cmd.TargetDirectory = s.targetLocation
	s.cmd.ArchivePath = filepath.Join(testutil.GetDirectoryOfFile(),
		"testdata", "archive", "artifacts.tar.gz")

	s.NoError(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))

	checkCommonExtractedArchiveContents(s.T(), s.cmd.TargetDirectory)

	// Extracting the same archive contents multiple times to the same directory
	// results in an error because the files already exist.
	s.Error(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
}

// checkCommonExtractedArchiveContents checks that the testdata's archive file
// extracted to the expected contents.
func checkCommonExtractedArchiveContents(t *testing.T, targetDirectory string) {
	// There's a single file artifacts/dir1/dir2/testfile.txt.
	expectedContents := map[string]bool{}
	pathParts := []string{"artifacts", "dir1", "dir2", "testfile.txt"}
	for i := range pathParts {
		expectedContents[filepath.Join(pathParts[:i+1]...)] = false
	}

	// Check that the proper directory structure is created and contains the one
	// extracted file.
	err := filepath.Walk(targetDirectory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		assert.True(t, strings.HasPrefix(path, targetDirectory), "path '%s' should be inside target directory '%s'", path, targetDirectory)
		if path == targetDirectory {
			return nil
		}

		relPath, err := filepath.Rel(targetDirectory, path)
		require.NoError(t, err)
		_, ok := expectedContents[relPath]
		assert.True(t, ok, "unexpected file '%s'", relPath)
		expectedContents[relPath] = true
		return nil
	})

	assert.NoError(t, err)

	for path, found := range expectedContents {
		assert.True(t, found, "did not find expected path '%s'", path)
	}
}
