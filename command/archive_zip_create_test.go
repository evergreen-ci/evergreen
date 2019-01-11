package command

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/stretchr/testify/suite"
)

type ZipCreateSuite struct {
	ctx    context.Context
	cancel context.CancelFunc
	conf   *model.TaskConfig
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
	s.targetLocation, err = ioutil.TempDir("", "zip-create-suite")
	s.Require().NoError(err)

	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.comm = client.NewMock("http://localhost.com")
	s.conf = &model.TaskConfig{Expansions: &util.Expansions{}, Task: &task.Task{}, Project: &model.Project{}}
	s.logger = s.comm.GetLoggerProducer(s.ctx, client.TaskData{ID: s.conf.Task.Id, Secret: s.conf.Task.Secret}, nil)

	s.cmd = &zipArchiveCreate{}
	s.params = map[string]interface{}{}
}

func (s *ZipCreateSuite) TearDownTest() {
	s.cancel()
	s.Require().NoError(os.RemoveAll(s.targetLocation))
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
	s.Error(s.cmd.Execute(context.Background(), s.comm, s.logger, s.conf))
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

	s.Error(s.cmd.Execute(context.Background(), s.comm, s.logger, s.conf))
	s.Contains(s.cmd.SourceDir, s.conf.WorkDir)
	s.Contains(s.cmd.Target, s.conf.WorkDir)
}

func (s *ZipCreateSuite) TestCreateZipOfPackage() {
	s.cmd.Target = filepath.Join(s.targetLocation, "test1.zip")
	s.cmd.SourceDir = testutil.GetDirectoryOfFile()
	s.cmd.Include = []string{"*.go"}
	s.NoError(s.cmd.Execute(context.Background(), s.comm, s.logger, s.conf))
}
