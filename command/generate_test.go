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
	"github.com/evergreen-ci/evergreen/util"
	"github.com/stretchr/testify/suite"
	yaml "gopkg.in/yaml.v2"
)

type generateSuite struct {
	cancel     func()
	conf       *model.TaskConfig
	comm       client.Communicator
	logger     client.LoggerProducer
	ctx        context.Context
	g          *generateTask
	tmpDirName string
	json       string

	suite.Suite
}

func TestGenerateSuite(t *testing.T) {
	suite.Run(t, new(generateSuite))
}

func (s *generateSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())

	s.comm = client.NewMock("http://localhost.com")
	s.conf = &model.TaskConfig{
		Expansions: &util.Expansions{},
		Task:       &task.Task{Id: "mock_id", Secret: "mock_secret"},
		Project:    &model.Project{}}
	s.logger = s.comm.GetLoggerProducer(s.ctx, client.TaskData{ID: s.conf.Task.Id, Secret: s.conf.Task.Secret})
	s.g = &generateTask{}
	var err error
	s.tmpDirName, err = ioutil.TempDir("", "generate-suite-")
	s.conf.WorkDir = s.tmpDirName
	s.Require().NoError(err)
	s.json = `
{
    "tasks": [
        {
            "commands": [
                {
                    "command": "git.get_project",
                    "params": {
                        "directory": "src"
                    }
                },
                {
                    "func": "echo-hi"
                }
            ],
            "name": "test"
        }
    ]
}
`

}

func (s *generateSuite) TearDownTest() {
	s.cancel()
	s.Require().NoError(os.RemoveAll(s.tmpDirName))
}

func (s *generateSuite) TestParseParamsWithNoFiles() {
	s.Error(s.g.ParseParams(map[string]interface{}{}))
}

func (s *generateSuite) TestParseParamsWithFiles() {
	s.NoError(s.g.ParseParams(map[string]interface{}{
		"files": []string{"foo", "bar", "baz"},
	}))
	s.Equal([]string{"foo", "bar", "baz"}, s.g.Files)
}

func (s *generateSuite) TestExecuteFileDoesNotExist() {
	c := &generateTask{Files: []string{"file-does-not-exist"}}
	s.Error(c.Execute(s.ctx, s.comm, s.logger, s.conf))
}

func (s *generateSuite) TestExecuteSuccess() {
	f, err := ioutil.TempFile(s.tmpDirName, "")
	s.Require().NoError(err)
	tmpFile := f.Name()
	tmpFileBase := filepath.Base(tmpFile)
	defer os.Remove(tmpFile)

	n, err := f.WriteString(s.json)
	s.NoError(err)
	s.Equal(len(s.json), n)
	s.NoError(f.Close())

	c := &generateTask{Files: []string{tmpFileBase}}
	s.NoError(c.Execute(s.ctx, s.comm, s.logger, s.conf))
}

type Thing struct {
	Thing string `yaml:"thing"`
}

type DrSeuss []Thing

func (s *generateSuite) TestMakeJsonOfAllFiles() {
	thingOne := []byte(`
{
  "thing": "one",
}
`)
	thingTwo := []byte(`
{
  "thing": "two",
}
`)
	var jsonArray DrSeuss
	json := makeJsonOfAllFiles([][]byte{thingOne, thingTwo})
	s.NoError(yaml.Unmarshal(json, &jsonArray))
}
