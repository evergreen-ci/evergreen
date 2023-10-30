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

type TarballExtractSuite struct {
	ctx    context.Context
	cancel context.CancelFunc
	conf   *internal.TaskConfig
	comm   client.Communicator
	logger client.LoggerProducer

	cmd            *tarballExtract
	targetLocation string
	params         map[string]interface{}

	suite.Suite
}

func TestTarballExtractSuite(t *testing.T) {
	suite.Run(t, new(TarballExtractSuite))
}

func (s *TarballExtractSuite) SetupTest() {
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

	s.cmd = &tarballExtract{}
	s.params = map[string]interface{}{}
}

func (s *TarballExtractSuite) TearDownTest() {
	s.cancel()
}

func (s *TarballExtractSuite) TestNilArguments() {
	s.Error(s.cmd.ParseParams(nil))
	s.Error(s.cmd.ParseParams(s.params))
}

func (s *TarballExtractSuite) TestMalformedParams() {
	s.params["exclude_files"] = 1
	s.params["path"] = "foo"
	s.Error(s.cmd.ParseParams(s.params))
}

func (s *TarballExtractSuite) TestCorrectParams() {
	s.params["path"] = "foo"
	s.NoError(s.cmd.ParseParams(s.params))
}

func (s *TarballExtractSuite) TestErrorsWithMalformedExpansions() {
	s.cmd.TargetDirectory = "${foo"
	s.Error(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
}

func (s *TarballExtractSuite) TestErrorsIfNoTarget() {
	s.Zero(s.cmd.TargetDirectory)
	s.Error(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
}

func (s *TarballExtractSuite) TestErrorsAndNormalizedPath() {
	s.cmd.TargetDirectory = "foo"
	s.cmd.ArchivePath = "bar"

	s.Error(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
	s.Contains(s.cmd.TargetDirectory, s.conf.WorkDir)
	s.Contains(s.cmd.ArchivePath, s.conf.WorkDir)
}

func (s *TarballExtractSuite) TestExtractionArchiveDoesNotExist() {
	s.cmd.TargetDirectory = s.targetLocation
	s.cmd.ArchivePath = filepath.Join(testutil.GetDirectoryOfFile(),
		"testdata", "archive", "artifacts.tar.gzip")

	s.Error(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
}

func (s *TarballExtractSuite) TestExtractionFileExistsAndIsNotArchive() {
	s.cmd.TargetDirectory = s.targetLocation
	_, thisFile, _, _ := runtime.Caller(0)
	s.cmd.ArchivePath = thisFile

	s.Error(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
}

func (s *TarballExtractSuite) TestExtractionSucceedsAndIsIdempotent() {
	s.cmd.TargetDirectory = s.targetLocation
	s.cmd.ArchivePath = filepath.Join(testutil.GetDirectoryOfFile(),
		"testdata", "archive", "artifacts.tar.gz")

	s.NoError(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))

	checkCommonExtractedArchiveContents(s.T(), s.cmd.TargetDirectory)

	// Extracting the same archive contents multiple times to the same directory
	// results results in no error. The command simply ignores duplicate files.
	s.NoError(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
}
