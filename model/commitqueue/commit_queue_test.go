package commitqueue

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/suite"
	mgo "gopkg.in/mgo.v2"
	yaml "gopkg.in/yaml.v2"
)

type CommitQueueSuite struct {
	suite.Suite
	q *CommitQueue
}

var sampleCommitQueueItem = CommitQueueItem{
	Issue: "c123",
	Modules: []Module{
		Module{
			Module: "test_module",
			Issue:  "d234",
		},
	},
}

func TestCommitQueueSuite(t *testing.T) {
	s := new(CommitQueueSuite)
	suite.Run(t, s)
}

func (s *CommitQueueSuite) SetupTest() {
	dbSessionFactory, err := getDBSessionFactory()
	s.Require().NoError(err)
	db.SetGlobalSessionProvider(dbSessionFactory)
	s.Require().NoError(db.ClearCollections(Collection))

	s.q = &CommitQueue{
		ProjectID: "mci",
	}

	s.NoError(InsertQueue(s.q))
	q, err := FindOneId("mci")
	s.Require().NotNil(q)
	s.Require().NoError(err)
}

func (s *CommitQueueSuite) TestEnqueue() {
	pos, err := s.q.Enqueue(sampleCommitQueueItem)
	s.NoError(err)
	s.Equal(1, pos)
	s.Len(s.q.Queue, 1)
	s.Equal("c123", s.q.Next().Issue)
	s.NotEqual(-1, s.q.findItem("c123"))

	// Persisted to db
	dbq, err := FindOneId("mci")
	s.NoError(err)
	s.Len(dbq.Queue, 1)
	s.Equal(sampleCommitQueueItem, *dbq.Next())
	s.NotEqual(-1, dbq.findItem("c123"))
}

func (s *CommitQueueSuite) TestNext() {
	pos, err := s.q.Enqueue(sampleCommitQueueItem)
	s.NoError(err)
	s.Equal(1, pos)
	s.Len(s.q.Queue, 1)

	s.NoError(s.q.SetProcessing(true))
	s.Nil(s.q.Next())

	s.NoError(s.q.SetProcessing(false))
	s.NotNil(s.q.Next())
	s.Equal(s.q.Next().Issue, "c123")
}

func (s *CommitQueueSuite) TestRemoveOne() {
	item := sampleCommitQueueItem
	pos, err := s.q.Enqueue(item)
	s.Require().NoError(err)
	s.Require().Equal(1, pos)
	item.Issue = "d234"
	pos, err = s.q.Enqueue(item)
	s.Require().NoError(err)
	s.Require().Equal(2, pos)
	item.Issue = "e345"
	pos, err = s.q.Enqueue(item)
	s.Require().NoError(err)
	s.Require().Equal(3, pos)
	s.Require().Len(s.q.Queue, 3)

	found, err := s.q.Remove("not_here")
	s.NoError(err)
	s.False(found)

	found, err = s.q.Remove("d234")
	s.NoError(err)
	s.True(found)
	items := s.q.Queue
	s.Len(items, 2)
	// Still in order
	s.Equal("c123", items[0].Issue)
	s.Equal("e345", items[1].Issue)

	// Persisted to db
	dbq, err := FindOneId("mci")
	s.NoError(err)
	items = dbq.Queue
	s.Len(items, 2)
	s.Equal("c123", items[0].Issue)
	s.Equal("e345", items[1].Issue)

	s.NoError(s.q.SetProcessing(true))
	s.Nil(s.q.Next())
	found, err = s.q.Remove("c123")
	s.True(found)
	s.NoError(err)
	s.NotNil(s.q.Next())
	s.Equal(s.q.Next().Issue, "e345")
}

func (s *CommitQueueSuite) TestClearAll() {
	item := sampleCommitQueueItem
	pos, err := s.q.Enqueue(item)
	s.Require().NoError(err)
	s.Require().Equal(1, pos)
	item.Issue = "d234"
	pos, err = s.q.Enqueue(item)
	s.Require().NoError(err)
	s.Require().Equal(2, pos)
	item.Issue = "e345"
	pos, err = s.q.Enqueue(item)
	s.Require().NoError(err)
	s.Require().Equal(3, pos)
	s.Require().Len(s.q.Queue, 3)

	q := &CommitQueue{
		ProjectID: "logkeeper",
		Queue:     []CommitQueueItem{},
	}
	s.Require().NoError(InsertQueue(q))

	// Only one commit queue has contents
	clearedCount, err := ClearAllCommitQueues()
	s.NoError(err)
	s.Equal(1, clearedCount)

	s.q, err = FindOneId("mci")
	s.NoError(err)
	s.Empty(s.q.Queue)
	q, err = FindOneId("logkeeper")
	s.NoError(err)
	s.Empty(q.Queue)

	// both have contents
	item.Issue = "c1234"
	pos, err = s.q.Enqueue(item)
	s.Require().NoError(err)
	s.Require().Equal(1, pos)
	item.Issue = "d234"
	pos, err = q.Enqueue(item)
	s.Require().NoError(err)
	s.Require().Equal(1, pos)
	clearedCount, err = ClearAllCommitQueues()
	s.NoError(err)
	s.Equal(2, clearedCount)
}

func (s *CommitQueueSuite) TestCommentTrigger() {
	comment := "no dice"
	action := "created"
	s.False(TriggersCommitQueue(action, comment))

	comment = triggerComment
	s.True(TriggersCommitQueue(action, comment))

	action = "deleted"
	s.False(TriggersCommitQueue(action, comment))
}

// Duplicated here from testutil to avoid import cycle
// (evergreen package requires github_pr_sender and testutil requires evergreen)
type settings struct {
	Database dbSettings `yaml:"database"`
}
type dbSettings struct {
	Url                  string       `yaml:"url"`
	SSL                  bool         `yaml:"ssl"`
	DB                   string       `yaml:"db"`
	WriteConcernSettings writeConcern `yaml:"write_concern"`
}
type writeConcern struct {
	W        int    `yaml:"w"`
	WMode    string `yaml:"wmode"`
	WTimeout int    `yaml:"wtimeout"`
	FSync    bool   `yaml:"fsync"`
	J        bool   `yaml:"j"`
}

func getDBSessionFactory() (*db.SessionFactory, error) {
	evgHome := os.Getenv("EVGHOME")
	testDir := "config_test"
	testSettings := "evg_settings.yml"
	configPath := filepath.Join(evgHome, testDir, testSettings)
	configData, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	settings := &settings{}
	err = yaml.Unmarshal(configData, settings)
	if err != nil {
		return nil, err
	}

	safety := mgo.Safe{}
	safety.W = settings.Database.WriteConcernSettings.W
	safety.WMode = settings.Database.WriteConcernSettings.WMode
	safety.WTimeout = settings.Database.WriteConcernSettings.WTimeout
	safety.FSync = settings.Database.WriteConcernSettings.FSync
	safety.J = settings.Database.WriteConcernSettings.J
	return db.NewSessionFactory(settings.Database.Url, settings.Database.DB, settings.Database.SSL, safety, 5*time.Second), nil
}
