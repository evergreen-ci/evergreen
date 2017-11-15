package artifact

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

func init() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

type TestArtifactFileSuite struct {
	testEntry Entry
	suite.Suite
}

func TestArtifactFile(t *testing.T) {
	s := &TestArtifactFileSuite{}
	suite.Run(t, s)
}

func (s *TestArtifactFileSuite) SetupTest() {
	s.NoError(db.Clear(Collection))
	s.testEntry = Entry{
		TaskId:          "task1",
		TaskDisplayName: "Task One",
		BuildId:         "build1",
		Files: []File{
			{"cat_pix", "http://placekitten.com/800/600", ""},
			{"fast_download", "https://fastdl.mongodb.org", ""},
		},
		Execution: 1,
	}

	s.NoError(s.testEntry.Upsert())
}

func (s *TestArtifactFileSuite) TestArtifactFieldsArePresent() {
	entryFromDb, err := FindOne(ByTaskId("task1"))
	s.NoError(err)

	s.Equal("task1", entryFromDb.TaskId)
	s.Equal("Task One", entryFromDb.TaskDisplayName)
	s.Equal("build1", entryFromDb.BuildId)
	s.Len(entryFromDb.Files, 2)
	s.Equal("cat_pix", entryFromDb.Files[0].Name)
	s.Equal("http://placekitten.com/800/600", entryFromDb.Files[0].Link)
	s.Equal("fast_download", entryFromDb.Files[1].Name)
	s.Equal("https://fastdl.mongodb.org", entryFromDb.Files[1].Link)
	s.Equal(1, entryFromDb.Execution)
}

func (s *TestArtifactFileSuite) TestArtifactFieldsAfterUpdate() {
	s.testEntry.Files = []File{
		{"cat_pix", "http://placekitten.com/300/400", ""},
		{"the_value_of_four", "4", ""},
	}
	s.NoError(s.testEntry.Upsert())

	count, err := db.Count(Collection, bson.M{})
	s.NoError(err)
	s.Equal(1, count)

	entryFromDb, err := FindOne(ByTaskId("task1"))
	s.NoError(err)
	s.NotNil(entryFromDb)

	s.Equal("task1", entryFromDb.TaskId)
	s.Equal("Task One", entryFromDb.TaskDisplayName)
	s.Equal("build1", entryFromDb.BuildId)
	s.Len(entryFromDb.Files, 4)
	s.Equal("cat_pix", entryFromDb.Files[0].Name)
	s.Equal("http://placekitten.com/800/600", entryFromDb.Files[0].Link)
	s.Equal("fast_download", entryFromDb.Files[1].Name)
	s.Equal("https://fastdl.mongodb.org", entryFromDb.Files[1].Link)
	s.Equal("cat_pix", entryFromDb.Files[2].Name)
	s.Equal("http://placekitten.com/300/400", entryFromDb.Files[2].Link)
	s.Equal("the_value_of_four", entryFromDb.Files[3].Name)
	s.Equal("4", entryFromDb.Files[3].Link)
	s.Equal(1, entryFromDb.Execution)
}
