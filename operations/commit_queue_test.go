package operations

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/suite"
	yaml "gopkg.in/yaml.v2"
)

type CommitQueueSuite struct {
	client client.Communicator
	ctx    context.Context
	server *service.TestServer
	suite.Suite
}

func TestCommitQueueSuite(t *testing.T) {
	suite.Run(t, new(CommitQueueSuite))
}

func (s *CommitQueueSuite) SetupSuite() {
	s.ctx = context.Background()
	db.SetGlobalSessionProvider(testConfig.SessionFactory())

	env := evergreen.GetEnvironment()
	if env.Settings() == nil {
		s.NoError(env.Configure(s.ctx, filepath.Join(evergreen.FindEvergreenHome(), testutil.TestDir, testutil.TestSettings), nil))
	}

	var err error
	s.server, err = service.CreateTestServer(testConfig, nil)
	s.Require().NoError(err)

	settings := ClientSettings{
		APIServerHost: s.server.URL + "/api",
		UIServerHost:  "http://dev-evg.mongodb.com",
		APIKey:        "testapikey",
		User:          "testuser",
	}
	settingsFile, err := ioutil.TempFile("", "settings")
	s.Require().NoError(err)
	settingsBytes, err := yaml.Marshal(settings)
	s.Require().NoError(err)
	_, err = settingsFile.Write(settingsBytes)
	s.Require().NoError(err)
	s.Require().NoError(settingsFile.Close())
	conf, err := NewClientSettings(settingsFile.Name())
	s.Require().NoError(err)
	s.client = conf.GetRestCommunicator(s.ctx)
}

func (s *CommitQueueSuite) TearDownSuite() {
	s.server.Close()
}

func (s *CommitQueueSuite) TestListContents() {
	s.Require().NoError(db.ClearCollections(commitqueue.Collection))
	cq := &commitqueue.CommitQueue{
		ProjectID: "mci",
		Queue:     []string{"123", "456", "789"},
	}
	s.Require().NoError(commitqueue.InsertQueue(cq))

	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	grip.SetSender(send.MakePlainLogger())
	s.NoError(listCommitQueue(s.ctx, s.client, "mci"))
	s.NoError(w.Close())
	os.Stdout = origStdout
	out, _ := ioutil.ReadAll(r)
	stringOut := string(out[:])

	s.Contains(stringOut, "Project: mci")
	s.Contains(stringOut, "1: 123")
	s.Contains(stringOut, "2: 456")
	s.Contains(stringOut, "3: 789")
}

func (s *CommitQueueSuite) TestDeleteCommitQueueItem() {
	s.Require().NoError(db.ClearCollections(commitqueue.Collection, model.ProjectRefCollection))
	cq := &commitqueue.CommitQueue{
		ProjectID: "mci",
		Queue:     []string{"123", "456", "789"},
	}
	s.Require().NoError(commitqueue.InsertQueue(cq))
	projectRef := model.ProjectRef{
		Identifier: "mci",
		Admins:     []string{"testuser"},
	}
	s.NoError(projectRef.Insert())

	s.Error(deleteCommitQueueItem(s.ctx, s.client, "mci", "not_here"))

	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	grip.SetSender(send.MakePlainLogger())
	s.NoError(deleteCommitQueueItem(s.ctx, s.client, "mci", "123"))
	s.NoError(w.Close())
	os.Stdout = origStdout
	out, _ := ioutil.ReadAll(r)
	stringOut := string(out[:])

	s.Contains(stringOut, "Item '123' deleted")
}
