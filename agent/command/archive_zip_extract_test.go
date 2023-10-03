package command

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/stretchr/testify/suite"
)

type ZipExtractSuite struct {
	ctx    context.Context
	cancel context.CancelFunc
	conf   *internal.TaskConfig
	comm   client.Communicator
	logger client.LoggerProducer

	cmd            *zipExtract
	targetLocation string
	params         map[string]interface{}

	suite.Suite
}

func TestZipExtractSuite(t *testing.T) {
	suite.Run(t, new(ZipExtractSuite))
}

func (s *ZipExtractSuite) SetupTest() {
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

	s.cmd = &zipExtract{}
	s.params = map[string]interface{}{}
}

func (s *ZipExtractSuite) TearDownTest() {
	s.cancel()
}

func (s *ZipExtractSuite) TestNilArguments() {
	s.Error(s.cmd.ParseParams(nil))
	s.Error(s.cmd.ParseParams(s.params))
}

func (s *ZipExtractSuite) TestMalformedParams() {
	s.params["exclude_files"] = 1
	s.params["path"] = "foo"
	s.Error(s.cmd.ParseParams(s.params))
}

func (s *ZipExtractSuite) TestCorrectParams() {
	s.params["path"] = "foo"
	s.NoError(s.cmd.ParseParams(s.params))
}

func (s *ZipExtractSuite) TestErrorWhenExcludeFilesExist() {
	s.cmd.ExcludeFiles = []string{"foo"}
	s.cmd.ArchivePath = "bar"
	s.Error(s.cmd.ParseParams(s.params))
}

func (s *ZipExtractSuite) TestErrorsWithMalformedExpansions() {
	s.cmd.TargetDirectory = "${foo"
	s.Error(s.cmd.Execute(context.Background(), s.comm, s.logger, s.conf))
}

func (s *ZipExtractSuite) TestErrorsIfNoTarget() {
	s.Zero(s.cmd.TargetDirectory)
	s.Error(s.cmd.Execute(context.Background(), s.comm, s.logger, s.conf))
}

func (s *ZipExtractSuite) TestErrorsAndNormalizedPath() {
	s.cmd.TargetDirectory = "foo"
	s.cmd.ArchivePath = "bar"

	s.Error(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
	s.Contains(s.cmd.TargetDirectory, s.conf.WorkDir)
	s.Contains(s.cmd.ArchivePath, s.conf.WorkDir)
}

func (s *ZipExtractSuite) TestExtractionArchiveDoesNotExist() {
	s.cmd.TargetDirectory = s.targetLocation
	s.cmd.ArchivePath = filepath.Join(testutil.GetDirectoryOfFile(),
		"testdata", "archive", "artifacts.tar.gzip")

	s.Error(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
}

func (s *ZipExtractSuite) TestExtractionFileExistsAndIsNotArchive() {
	s.cmd.TargetDirectory = s.targetLocation
	_, thisFile, _, _ := runtime.Caller(0)
	s.cmd.ArchivePath = thisFile

	s.Error(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
}

func (s *ZipExtractSuite) TestExtractionSucceedsButIsNotIdempotent() {
	s.cmd.TargetDirectory = s.targetLocation
	s.cmd.ArchivePath = filepath.Join(testutil.GetDirectoryOfFile(),
		"testdata", "archive", "artifacts.zip")

	s.NoError(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))

	checkCommonExtractedArchiveContents(s.T(), s.cmd.TargetDirectory)

	// Extracting the same archive contents multiple times to the same directory
	// results in an error because the files already exist.
	s.Error(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
}
