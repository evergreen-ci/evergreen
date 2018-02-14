package command

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/stretchr/testify/suite"
)

type ArtifactsSuite struct {
	suite.Suite
	cmd    *attachArtifacts
	conf   *model.TaskConfig
	comm   client.Communicator
	logger client.LoggerProducer
	mock   *client.Mock
	ctx    context.Context
	cancel context.CancelFunc
	tmpdir string
}

func TestArtifactsSuite(t *testing.T) {
	suite.Run(t, new(ArtifactsSuite))
}

func (s *ArtifactsSuite) SetupSuite() {
	var err error
	s.tmpdir, err = ioutil.TempDir("", "evergreen.command.attach_artifacts.test")
	s.Require().NoError(err)

	path := filepath.Join(s.tmpdir, "example.json")
	s.NoError(util.WriteJSONInto(path,
		[]*artifact.File{
			{
				Name: "name_of_artifact",
				Link: "here it is",
			},
		}))

	_, err = os.Stat(path)
	s.Require().False(os.IsNotExist(err))
}

func (s *ArtifactsSuite) TearDownSuite() {
	s.Require().NoError(os.RemoveAll(s.tmpdir))
}

func (s *ArtifactsSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.comm = client.NewMock("http://localhost.com")
	s.conf = &model.TaskConfig{Expansions: &util.Expansions{}, Task: &task.Task{}, Project: &model.Project{}}
	s.logger = s.comm.GetLoggerProducer(s.ctx, client.TaskData{ID: s.conf.Task.Id, Secret: s.conf.Task.Secret})
	s.cmd = attachArtifactsFactory().(*attachArtifacts)
	s.conf.WorkDir = s.tmpdir
	s.mock = s.comm.(*client.Mock)
}

func (s *ArtifactsSuite) TearDownTest() {
	s.cancel()
}

func (s *ArtifactsSuite) TestParseErrorWOrks() {
	s.cmd.Files = []string{"foo"}

	s.NoError(s.cmd.ParseParams(map[string]interface{}{}))
}

func (s *ArtifactsSuite) TestParseErrorsIfTypesDoNotMatch() {
	s.Error(s.cmd.ParseParams(map[string]interface{}{
		"files": 1,
	}))

	s.Error(s.cmd.ParseParams(map[string]interface{}{
		"files": []int{1, 3, 7},
	}))
}

func (s *ArtifactsSuite) TestParseErrorIfNothingIs() {
	s.Len(s.cmd.Files, 0)
	s.Error(s.cmd.ParseParams(map[string]interface{}{}))
}

func (s *ArtifactsSuite) TestArtifactErrorsWithInvalidExpansions() {
	s.Len(s.cmd.Files, 0)
	s.NoError(s.cmd.ParseParams(map[string]interface{}{
		"files": []string{
			"fo${bar",
		},
	}))
	s.Len(s.cmd.Files, 1)
	s.Equal("fo${bar", s.cmd.Files[0])

	s.Error(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
}

func (s *ArtifactsSuite) TestArtifactErrorsIfDoesNotExist() {
	s.cmd.Files = []string{"foo"}
	s.Error(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
	s.Len(s.cmd.Files, 0)
}

func (s *ArtifactsSuite) TestArtifactSkipsErrorWithOptionalArgument() {
	s.cmd.Files = []string{"foo"}
	s.cmd.Optional = true
	s.Error(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
	s.Len(s.cmd.Files, 0)
}

func (s *ArtifactsSuite) TestReadFileFailsIfTasksDoesNotExist() {
	result, err := readArtifactsFile(s.tmpdir, "does-not-exist")
	s.Error(err)
	s.Nil(result)

	result, err = readArtifactsFile(s.tmpdir, "")
	s.Error(err)
	s.Nil(result)
}

func (s *ArtifactsSuite) TestReadFileSucceeds() {
	result, err := readArtifactsFile(s.tmpdir, "example.json")
	s.NoError(err)
	s.Len(result, 1)
}

func (s *ArtifactsSuite) TestCommandParsesFile() {
	s.Len(s.mock.AttachedFiles, 0)
	s.cmd.Files = []string{"example.json"}
	s.NoError(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
	s.Len(s.mock.AttachedFiles, 1)
	s.Len(s.mock.AttachedFiles[s.conf.Task.Id], 1)
}
