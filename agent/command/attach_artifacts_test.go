package command

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/suite"
)

type ArtifactsSuite struct {
	suite.Suite
	cmd    *attachArtifacts
	conf   *internal.TaskConfig
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
	s.tmpdir = s.T().TempDir()

	path := filepath.Join(s.tmpdir, "example.json")
	s.NoError(utility.WriteJSONFile(path,
		[]*artifact.File{
			{
				Name: "name_of_artifact",
				Link: "here it is",
			},
		}))

	_, err := os.Stat(path)
	s.Require().False(os.IsNotExist(err))

	path = filepath.Join(s.tmpdir, "exactmatch.json")
	s.NoError(utility.WriteJSONFile(path,
		[]*artifact.File{
			{
				Name: "name_of_artifact",
				Link: "here it is",
			},
		}))
	_, err = os.Stat(path)
	s.Require().False(os.IsNotExist(err))
}

func (s *ArtifactsSuite) SetupTest() {
	var err error
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.comm = client.NewMock("http://localhost.com")
	s.conf = &internal.TaskConfig{Expansions: util.Expansions{}, Task: task.Task{}, Project: model.Project{}}
	s.logger, err = s.comm.GetLoggerProducer(s.ctx, &s.conf.Task, nil)
	s.Require().NoError(err)
	s.cmd = attachArtifactsFactory().(*attachArtifacts)
	s.conf.WorkDir = s.tmpdir
	s.mock = s.comm.(*client.Mock)
}

func (s *ArtifactsSuite) TearDownTest() {
	s.cancel()
}

func (s *ArtifactsSuite) TestParseErrorWorks() {
	s.cmd.Files = []string{"foo"}

	s.NoError(s.cmd.ParseParams(map[string]any{}))
}

func (s *ArtifactsSuite) TestParseErrorsIfTypesDoNotMatch() {
	s.Error(s.cmd.ParseParams(map[string]any{
		"files": 1,
	}))

	s.Error(s.cmd.ParseParams(map[string]any{
		"files": []int{1, 3, 7},
	}))
}

func (s *ArtifactsSuite) TestParseErrorIfNothingIsSet() {
	s.Empty(s.cmd.Files)
	s.Error(s.cmd.ParseParams(map[string]any{}))
}

func (s *ArtifactsSuite) TestArtifactErrorsWithInvalidExpansions() {
	s.Empty(s.cmd.Files)
	s.NoError(s.cmd.ParseParams(map[string]any{
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
	s.Empty(s.cmd.Files)
	s.Empty(s.mock.AttachedFiles)
	s.Empty(s.mock.AttachedFiles[s.conf.Task.Id])
}

func (s *ArtifactsSuite) TestArtifactNoErrorIfDoesNotExistWithExactNames() {
	s.cmd.Files = []string{"foo"}
	s.cmd.ExactFileNames = true
	s.Error(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
	s.Len(s.cmd.Files, 1)
	s.Empty(s.mock.AttachedFiles)
	s.Empty(s.mock.AttachedFiles[s.conf.Task.Id])
}

func (s *ArtifactsSuite) TestArtifactSkipsErrorWithOptionalArgument() {
	s.cmd.Files = []string{"foo"}
	s.cmd.Optional = true
	s.NoError(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
	s.Empty(s.cmd.Files)
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
	s.Empty(s.mock.AttachedFiles)
	s.cmd.Files = []string{"example*"}
	s.NoError(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
	s.Len(s.mock.AttachedFiles, 1)
	s.Len(s.mock.AttachedFiles[s.conf.Task.Id], 1)
}

func (s *ArtifactsSuite) TestCommandParsesExactFileNames() {
	s.cmd.ExactFileNames = true
	s.Empty(s.mock.AttachedFiles)
	s.cmd.Files = []string{"exactmatch.json", "example.json"}
	s.NoError(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
	s.Len(s.mock.AttachedFiles, 1)
	s.Len(s.mock.AttachedFiles[s.conf.Task.Id], 2)
}

func (s *ArtifactsSuite) TestPrefixectoryEmptySubDir() {
	dir := s.T().TempDir()
	err := os.WriteFile(filepath.Join(dir, "foo"), []byte("[{}]"), 0644)
	s.Require().NoError(err)
	s.Require().NoError(os.Mkdir(filepath.Join(dir, "subDir"), 0755))
	err = os.WriteFile(filepath.Join(dir, "subDir", "bar"), []byte("[{}]"), 0644)
	s.Require().NoError(err)
	s.conf.WorkDir = dir
	s.cmd.Files = []string{"*"}
	s.NoError(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
	s.Len(s.cmd.Files, 2)
}

func (s *ArtifactsSuite) TestPrefixectoryWithSubDir() {
	dir := s.T().TempDir()
	err := os.WriteFile(filepath.Join(dir, "foo"), []byte("[{}]"), 0644)
	s.Require().NoError(err)
	s.Require().NoError(os.Mkdir(filepath.Join(dir, "subDir"), 0755))
	err = os.WriteFile(filepath.Join(dir, "subDir", "bar"), []byte("[{}]"), 0644)
	s.Require().NoError(err)
	s.conf.WorkDir = dir
	s.cmd.Files = []string{"*"}
	s.cmd.Prefix = "subDir"
	s.NoError(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
	s.Len(s.cmd.Files, 1)
}
