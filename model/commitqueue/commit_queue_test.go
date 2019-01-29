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
		ProjectID:    "mci",
		MergeAction:  "squash",
		StatusAction: "github",
	}

	s.NoError(InsertQueue(s.q))
	q, err := FindOneId("mci")
	s.Require().NotNil(q)
	s.Require().NoError(err)
}

func (s *CommitQueueSuite) TestEnqueue() {
	s.NoError(s.q.Enqueue("c123"))
	s.False(s.q.IsEmpty())
	s.Equal("c123", s.q.Next())
	s.NotEqual(-1, s.q.findItem("c123"))

	// Persisted to db
	dbq, err := FindOneId("mci")
	s.NoError(err)
	s.False(dbq.IsEmpty())
	s.Equal("c123", dbq.Next())
	s.NotEqual(-1, dbq.findItem("c123"))
}

func (s *CommitQueueSuite) TestAll() {
	s.Require().NoError(s.q.Enqueue("c123"))
	s.Require().NoError(s.q.Enqueue("d234"))
	s.Require().NoError(s.q.Enqueue("e345"))

	items := s.q.All()
	s.Len(items, 3)
	s.Equal("c123", items[0])
	s.Equal("d234", items[1])
	s.Equal("e345", items[2])
}

func (s *CommitQueueSuite) TestRemoveOne() {
	s.Require().NoError(s.q.Enqueue("c123"))
	s.Require().NoError(s.q.Enqueue("d234"))
	s.Require().NoError(s.q.Enqueue("e345"))
	s.Require().Len(s.q.All(), 3)

	s.Error(s.q.Remove("not_here"))

	s.NoError(s.q.Remove("d234"))
	items := s.q.All()
	s.Len(items, 2)
	// Still in order
	s.Equal("c123", items[0])
	s.Equal("e345", items[1])

	// Persisted to db
	dbq, err := FindOneId("mci")
	s.NoError(err)
	items = dbq.All()
	s.Len(items, 2)
	s.Equal("c123", items[0])
	s.Equal("e345", items[1])
}

func (s *CommitQueueSuite) TestRemoveAll() {
	s.Require().NoError(s.q.Enqueue("c123"))
	s.Require().NoError(s.q.Enqueue("d234"))
	s.Require().NoError(s.q.Enqueue("e345"))
	s.Require().Len(s.q.All(), 3)

	s.NoError(s.q.RemoveAll())
	s.Empty(s.q.All())

	// Persisted to db
	dbq, err := FindOneId("mci")
	s.NoError(err)
	s.Empty(dbq.All())
}

func (s *CommitQueueSuite) TestUpdateMerge() {
	s.Equal("squash", s.q.MergeAction)
	s.NoError(s.q.UpdateMergeAction("rebase"))
	s.Equal("rebase", s.q.MergeAction)

	dbq, err := FindOneId("mci")
	s.NoError(err)
	s.Equal("rebase", dbq.MergeAction)
}

func (s *CommitQueueSuite) TestUpdateStatus() {
	s.Equal("github", s.q.StatusAction)
	s.NoError(s.q.UpdateStatusAction("email"))
	s.Equal("email", s.q.StatusAction)

	dbq, err := FindOneId("mci")
	s.NoError(err)
	s.Equal("email", dbq.StatusAction)
}

func (s *CommitQueueSuite) TestCommentTrigger() {
	comment := "no dice"
	action := "create"
	s.False(TriggersCommitQueue(action, comment))

	comment = triggerComment
	s.True(TriggersCommitQueue(action, comment))

	action = "delete"
	s.False(TriggersCommitQueue(action, comment))
}

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

// Duplicated here from testutil to avoid import cycle
// (evergreen package requires github_pr_sender and testutil requires evergreen)
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
