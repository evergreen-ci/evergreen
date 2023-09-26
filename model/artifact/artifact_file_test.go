package artifact

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	_ "github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
)

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
				{
					Name:           "cat_pix",
					Link:           "http://placekitten.com/800/600",
					Visibility:     "signed",
					IgnoreForFetch: false,
					AwsKey:         "key",
					AwsSecret:      "secret",
					Bucket:         "bucket",
					FileKey:        "filekey",
				},
				{
					Name:           "fast_download",
					Link:           "https://fastdl.mongodb.org",
					IgnoreForFetch: false,
				},
			},
			Execution: 1,
		},
		{
			TaskId:          "task2",
			TaskDisplayName: "Task Two",
			BuildId:         "build2",
			Files: []File{
				{
					Name:           "other",
					Link:           "http://example.com/other",
					Visibility:     "",
					IgnoreForFetch: false,
					AwsKey:         "key",
					AwsSecret:      "secret",
				},
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
			{
				Name:           "other",
				Link:           "http://example.com/other",
				Visibility:     "",
				IgnoreForFetch: false,
				AwsKey:         "key",
				AwsSecret:      "secret",
			},
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
		{
			Name:           "cat_pix",
			Link:           "http://placekitten.com/300/400",
			Visibility:     "",
			IgnoreForFetch: false,
			AwsKey:         "key",
			AwsSecret:      "secret",
		},
		{
			Name:           "the_value_of_four",
			Link:           "4",
			Visibility:     "",
			IgnoreForFetch: false,
			AwsKey:         "key",
			AwsSecret:      "secret",
		},
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

func (s *TestArtifactFileSuite) TestFindByIdsAndExecutions() {
	tasks := []TaskIDAndExecution{
		{TaskID: "task1", Execution: 1},
		{TaskID: "task2", Execution: 5},
	}
	entries, err := FindAll(ByTaskIdsAndExecutions(tasks))
	s.NoError(err)
	s.Len(entries, 2)
}

func (s *TestArtifactFileSuite) TestFindByIds() {
	entries, err := FindAll(ByTaskIds([]string{"task1", "task2"}))
	s.NoError(err)
	s.Len(entries, 3)
}

func (s *TestArtifactFileSuite) TestRotateSecret() {
	changes, err := RotateSecrets("secret", "changedSecret", true)
	s.NoError(err)
	s.Len(changes, 3)
	entryFromDb, err := FindOne(ByTaskId("task1"))
	s.NoError(err)
	s.Equal(entryFromDb.Files[0].AwsSecret, "secret")
	changes, err = RotateSecrets("secret", "changedSecret", false)
	s.NoError(err)
	s.Len(changes, 3)
	entryFromDb, err = FindOne(ByTaskId("task1"))
	s.NoError(err)
	s.Equal(entryFromDb.Files[0].AwsSecret, "changedSecret")
}

func (s *TestArtifactFileSuite) TestEscapeFiles() {
	files := []File{
		{
			Name: "cat_picture",
			Link: "https://bucket.s3.amazonaws.com/something/file#1.tar.gz",
		},
		{
			Name: "not_a_cat_picture",
			Link: "https://notacat#0.png",
		},
	}

	escapedFiles := EscapeFiles(files)

	s.Equal("https://bucket.s3.amazonaws.com/something/file%231.tar.gz", escapedFiles[0].Link)
	s.Equal("https://notacat%230.png", escapedFiles[1].Link)

}
