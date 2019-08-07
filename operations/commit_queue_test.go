package operations

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/yaml.v2"
)

type CommitQueueSuite struct {
	client client.Communicator
	conf   *ClientSettings
	ctx    context.Context
	server *service.TestServer
	suite.Suite
}

func TestCommitQueueSuite(t *testing.T) {
	suite.Run(t, new(CommitQueueSuite))
}

func (s *CommitQueueSuite) SetupSuite() {
	s.ctx = context.Background()

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
	s.conf, err = NewClientSettings(settingsFile.Name())
	s.Require().NoError(err)
	s.client = s.conf.GetRestCommunicator(s.ctx)
}

func (s *CommitQueueSuite) TearDownSuite() {
	s.server.Close()
	s.client.Close()
}

func (s *CommitQueueSuite) TestListContentsForCLI() {
	s.Require().NoError(db.ClearCollections(commitqueue.Collection, patch.Collection, model.ProjectRefCollection))
	now := time.Now()
	p1 := patch.Patch{
		Id:          bson.NewObjectId(),
		Project:     "mci",
		Author:      "annie.black",
		Activated:   true,
		Description: "fix things",
		CreateTime:  now,
		Status:      evergreen.TaskDispatched,
	}
	s.NoError(p1.Insert())
	p2 := patch.Patch{
		Id:          bson.NewObjectId(),
		Author:      "annie.black",
		Project:     "mci",
		Description: "do things",
	}
	s.NoError(p2.Insert())
	p3 := patch.Patch{
		Id:          bson.NewObjectId(),
		Author:      "john.liu",
		Project:     "mci",
		Description: "no things",
	}
	s.NoError(p3.Insert())

	pRef := &model.ProjectRef{
		Identifier:  "mci",
		CommitQueue: model.CommitQueueParams{PatchType: commitqueue.CLIPatchType},
	}
	s.Require().NoError(pRef.Insert())

	cq := &commitqueue.CommitQueue{ProjectID: "mci"}
	s.Require().NoError(commitqueue.InsertQueue(cq))

	pos, err := cq.Enqueue(commitqueue.CommitQueueItem{Issue: p1.Id.Hex()})
	s.NoError(err)
	s.Equal(1, pos)
	pos, err = cq.Enqueue(commitqueue.CommitQueueItem{Issue: p2.Id.Hex()})
	s.NoError(err)
	s.Equal(2, pos)
	pos, err = cq.Enqueue(commitqueue.CommitQueueItem{Issue: p3.Id.Hex()})
	s.NoError(err)
	s.Equal(3, pos)

	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	s.NoError(grip.SetSender(send.MakePlainLogger()))
	ac, _, err := s.conf.getLegacyClients()
	s.NoError(err)

	s.NoError(listCommitQueue(s.ctx, s.client, ac, "mci", s.conf.UIServerHost))
	s.NoError(w.Close())
	os.Stdout = origStdout
	out, _ := ioutil.ReadAll(r)
	stringOut := string(out[:])

	s.Contains(stringOut, "Project: mci")
	s.Contains(stringOut, fmt.Sprintf("Type of queue: %s", commitqueue.CLIPatchType))
	s.Contains(stringOut, "Description : do things")
	s.Contains(stringOut, fmt.Sprintf("ID : %s", p1.Id.Hex()))
	s.Contains(stringOut, fmt.Sprintf("ID : %s", p2.Id.Hex()))
	s.Contains(stringOut, fmt.Sprintf("ID : %s", p3.Id.Hex()))
	s.Contains(stringOut, fmt.Sprintf("Author: %s", p1.Author))
	s.Contains(stringOut, fmt.Sprintf("Author: %s", p3.Author))
	versionURL := fmt.Sprintf("Build : %s/version/%s", s.conf.UIServerHost, p1.Id.Hex())
	patchURL := fmt.Sprintf("Build : %s/patch/%s", s.conf.UIServerHost, p2.Id.Hex())
	s.Contains(stringOut, versionURL)
	s.Contains(stringOut, patchURL)
}

func (s *CommitQueueSuite) TestListContentsForPRs() {
	s.Require().NoError(db.ClearCollections(commitqueue.Collection, model.ProjectRefCollection))
	cq := &commitqueue.CommitQueue{
		ProjectID: "mci",
		Queue: []commitqueue.CommitQueueItem{
			commitqueue.CommitQueueItem{
				Issue: "123",
			},
			commitqueue.CommitQueueItem{
				Issue: "456",
			},
			commitqueue.CommitQueueItem{
				Issue: "789",
			},
		},
	}
	s.Require().NoError(commitqueue.InsertQueue(cq))
	cq.Queue[0].Version = "my_version"
	s.NoError(cq.UpdateVersion(cq.Queue[0]))
	pRef := &model.ProjectRef{
		Identifier:  "mci",
		Owner:       "evergreen-ci",
		Repo:        "evergreen",
		CommitQueue: model.CommitQueueParams{PatchType: commitqueue.PRPatchType},
	}
	s.Require().NoError(pRef.Insert())

	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	s.NoError(grip.SetSender(send.MakePlainLogger()))
	ac, _, err := s.conf.getLegacyClients()
	s.NoError(err)

	s.NoError(listCommitQueue(s.ctx, s.client, ac, "mci", s.conf.UIServerHost))
	s.NoError(w.Close())
	os.Stdout = origStdout
	out, _ := ioutil.ReadAll(r)
	stringOut := string(out[:])

	s.Contains(stringOut, "Project: mci")
	s.Contains(stringOut, "Repo: evergreen")
	s.Contains(stringOut, fmt.Sprintf("Type of queue: %s", commitqueue.PRPatchType))
	s.Contains(stringOut, "Owner: evergreen-ci")
	s.Contains(stringOut, "PR # : 123")
	s.Contains(stringOut, "PR # : 456")
	s.Contains(stringOut, "PR # : 789")
	url := fmt.Sprintf("URL : https://github.com/%s/%s/pull/%s", pRef.Owner, pRef.Repo, "456")
	versionURL := fmt.Sprintf("Build : %s/version/%s", s.conf.UIServerHost, "my_version")
	s.Contains(stringOut, url)
	s.Contains(stringOut, versionURL)
}

func (s *CommitQueueSuite) TestListContentsWithModule() {
	s.Require().NoError(db.ClearCollections(commitqueue.Collection, model.ProjectRefCollection))
	cq := &commitqueue.CommitQueue{
		ProjectID: "mci",
		Queue: []commitqueue.CommitQueueItem{
			commitqueue.CommitQueueItem{
				Issue: "123",
				Modules: []commitqueue.Module{
					commitqueue.Module{
						Module: "test_module",
						Issue:  "1234",
					},
				},
			},
			commitqueue.CommitQueueItem{
				Issue: "456",
			},
			commitqueue.CommitQueueItem{
				Issue: "789",
			},
		},
	}
	s.Require().NoError(commitqueue.InsertQueue(cq))

	pRef := &model.ProjectRef{
		Identifier:  "mci",
		Owner:       "me",
		Repo:        "evergreen",
		CommitQueue: model.CommitQueueParams{PatchType: commitqueue.PRPatchType},
	}
	s.Require().NoError(pRef.Insert())

	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	s.NoError(grip.SetSender(send.MakePlainLogger()))

	ac, _, err := s.conf.getLegacyClients()
	s.NoError(err)
	s.NoError(listCommitQueue(s.ctx, s.client, ac, "mci", s.conf.UIServerHost))
	s.NoError(w.Close())
	os.Stdout = origStdout
	out, _ := ioutil.ReadAll(r)
	stringOut := string(out[:])

	s.Contains(stringOut, "Project: mci")
	s.Contains(stringOut, "PR # : 123")
	s.Contains(stringOut, "Modules :")
	s.Contains(stringOut, "1: test_module (1234)")
	s.Contains(stringOut, "PR # : 456")
	s.Contains(stringOut, "PR # : 789")
}

func (s *CommitQueueSuite) TestDeleteCommitQueueItem() {
	s.Require().NoError(db.ClearCollections(commitqueue.Collection, model.ProjectRefCollection))
	cq := &commitqueue.CommitQueue{
		ProjectID: "mci",
		Queue: []commitqueue.CommitQueueItem{
			commitqueue.CommitQueueItem{
				Issue: "123",
			},
			commitqueue.CommitQueueItem{
				Issue: "456",
			},
			commitqueue.CommitQueueItem{
				Issue: "789",
			},
		},
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
	s.NoError(grip.SetSender(send.MakePlainLogger()))
	s.NoError(deleteCommitQueueItem(s.ctx, s.client, "mci", "123"))
	s.NoError(w.Close())
	os.Stdout = origStdout
	out, _ := ioutil.ReadAll(r)
	stringOut := string(out[:])

	s.Contains(stringOut, "Item '123' deleted")
}
