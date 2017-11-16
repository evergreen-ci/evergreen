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
	testEntries []Entry
	suite.Suite
}

func TestArtifactFile(t *testing.T) {
	s := &TestArtifactFileSuite{}
	suite.Run(t, s)
}

func (s *TestArtifactFileSuite) SetupTest() {
	s.NoError(db.Clear(Collection))
	s.testEntries = []Entry{
		{
			TaskId:          "task1",
			TaskDisplayName: "Task One",
			BuildId:         "build1",
			Files: []File{
				{"cat_pix", "http://placekitten.com/800/600", ""},
				{"fast_download", "https://fastdl.mongodb.org", ""},
			},
			Execution: 1,
		},
		{
			TaskId:          "task2",
			TaskDisplayName: "Task Two",
			BuildId:         "build2",
			Files: []File{
				{"other", "http://example.com/other", ""},
			},
			Execution: 5,
		},
	}

	// hack to insert an entry without execution number (representative of
	// existing data)
	s.NoError(db.Insert(Collection, struct {
		TaskId          string `json:"task" bson:"task"`
		TaskDisplayName string `json:"task_name" bson:"task_name"`
		BuildId         string `json:"build" bson:"build"`
		Files           []File `json:"files" bson:"files"`
	}{
		TaskId:          "task2",
		TaskDisplayName: "Task Two",
		BuildId:         "build2",
		Files: []File{
			{"other", "http://example.com/other", ""},
		},
	}))

	for _, entry := range s.testEntries {
		s.NoError(entry.Upsert())
	}

	count, err := db.Count(Collection, bson.M{})
	s.NoError(err)
	s.Equal(3, count)
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
	s.testEntries[0].Files = []File{
		{"cat_pix", "http://placekitten.com/300/400", ""},
		{"the_value_of_four", "4", ""},
	}
	s.NoError(s.testEntries[0].Upsert())

	count, err := db.Count(Collection, bson.M{})
	s.NoError(err)
	s.Equal(3, count)

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

func (s *TestArtifactFileSuite) TestFindByTaskIdAndExecution() {
	entries, err := FindAll(ByTaskIdAndExecution("task1", 1))
	s.Len(entries, 1)
	s.NoError(err)
	s.NotNil(entries[0])
	s.Equal(1, entries[0].Execution)
	s.Equal("task1", entries[0].TaskId)

	entries, err = FindAll(ByTaskIdAndExecution("task2", 0))
	s.Len(entries, 0)
	s.NoError(err)
	s.Empty(entries)

	entries, err = FindAll(ByTaskIdAndExecution("task2", 5))
	s.Len(entries, 1)
	s.NoError(err)
	s.NotNil(entries[0])
	s.Equal(5, entries[0].Execution)
	s.Equal("task2", entries[0].TaskId)
}

func (s *TestArtifactFileSuite) TestFindByTaskIdWithoutExecution() {
	entries, err := FindAll(ByTaskIdWithoutExecution("task1"))
	s.Len(entries, 0)
	s.NoError(err)

	entries, err = FindAll(ByTaskIdWithoutExecution("task2"))
	s.Len(entries, 1)
	s.NoError(err)
	s.NotNil(entries[0])
	s.Equal(0, entries[0].Execution)
	s.Equal("task2", entries[0].TaskId)
}
