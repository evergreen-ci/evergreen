package command

import (
	"context"
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
	"github.com/mholt/archiver/v3"
	"github.com/stretchr/testify/suite"
)

type ZipCreateSuite struct {
	ctx    context.Context
	cancel context.CancelFunc
	conf   *internal.TaskConfig
	comm   client.Communicator
	logger client.LoggerProducer

	cmd            *zipArchiveCreate
	targetLocation string
	params         map[string]interface{}

	suite.Suite
}

func TestZipCreateSuite(t *testing.T) {
	suite.Run(t, new(ZipCreateSuite))
}

func (s *ZipCreateSuite) SetupTest() {
	var err error
	s.targetLocation = s.T().TempDir()

	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.comm = client.NewMock("http://localhost.com")
	s.conf = &internal.TaskConfig{Expansions: util.Expansions{}, Task: task.Task{}, Project: model.Project{}}
	s.logger, err = s.comm.GetLoggerProducer(s.ctx, client.TaskData{ID: s.conf.Task.Id, Secret: s.conf.Task.Secret}, nil)
	s.NoError(err)

	s.cmd = &zipArchiveCreate{}
	s.params = map[string]interface{}{}
}

func (s *ZipCreateSuite) TearDownTest() {
	s.cancel()
}

func (s *ZipCreateSuite) TestNilArguments() {
	s.Error(s.cmd.ParseParams(nil))
	s.Error(s.cmd.ParseParams(s.params))
}

func (s *ZipCreateSuite) TestMalformedParams() {
	s.params["exclude_files"] = 1
	s.params["path"] = "foo"
	s.Error(s.cmd.ParseParams(s.params))
}

func (s *ZipCreateSuite) TestErrorsWithMalformedExpansions() {
	s.cmd.Target = "${feoo"
	s.Error(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
}

func (s *ZipCreateSuite) TestErrorsWithMissingArguments() {
	s.Error(s.cmd.ParseParams(s.params))
	s.cmd.Target = "foo"
	s.Error(s.cmd.ParseParams(s.params))
	s.cmd.SourceDir = "bar"
	s.Error(s.cmd.ParseParams(s.params))
	s.cmd.Include = []string{"bazzz"}

	s.NoError(s.cmd.ParseParams(s.params))
}

func (s *ZipCreateSuite) TestErrorsAndNormalizedPath() {
	var err error
	s.conf.WorkDir, err = filepath.Abs(filepath.Join("srv", "evergreen"))
	s.Require().NoError(err)
	s.cmd.SourceDir = "foo"
	s.cmd.Target = "bar"

	s.Error(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
	s.Contains(s.cmd.SourceDir, s.conf.WorkDir)
	s.Contains(s.cmd.Target, s.conf.WorkDir)
}

func (s *ZipCreateSuite) TestCreateZipOfPackage() {
	s.cmd.Target = filepath.Join(s.targetLocation, "test1.zip")
	s.cmd.SourceDir = testutil.GetDirectoryOfFile()
	s.cmd.Include = []string{"*.go"}
	s.NoError(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))

	var numFiles int
	var foundThisFile bool
	_, thisFile, _, _ := runtime.Caller(0)
	s.NoError(archiver.NewZip().Walk(s.cmd.Target, func(f archiver.File) error {
		s.True(strings.HasSuffix(f.FileInfo.Name(), ".go"))
		if f.FileInfo.Name() == filepath.Base(thisFile) {
			foundThisFile = true
		}
		numFiles++
		return nil
	}))
	s.NotZero(numFiles)
	s.True(foundThisFile)
}
