package patch

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

type CliIntentSuite struct {
	suite.Suite

	patchContent string
	description  string
	variants     []string
	tasks        []string
	module       string
	user         string
	projectID    string
	hash         string
}

func TestCliIntentSuite(t *testing.T) {
	suite.Run(t, new(CliIntentSuite))
}

func (s *CliIntentSuite) SetupSuite() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
	s.patchContent = "patch"
	s.hash = "67da19930b1b18d346477e99a8e18094a672f48a"
	s.user = "octocat"
	s.module = "module"
	s.tasks = []string{"task1", "Task2"}
	s.variants = []string{"variant1", "variant2"}
	s.projectID = "project"
	s.description = "desc"
}

func (s *CliIntentSuite) SetupTest() {
	s.NoError(db.ClearGridCollections(GridFSPrefix))
	s.NoError(db.Clear(IntentCollection))
}

func (s *CliIntentSuite) TestNewCliIntent() {
	intent, err := NewCliIntent(s.user, s.projectID, s.hash, s.module, s.patchContent, s.description, true, s.variants, s.tasks)
	s.NotNil(intent)
	s.NoError(err)
	s.Implements((*Intent)(nil), intent)
	s.True(intent.ShouldFinalizePatch())
	s.Equal(CliIntentType, intent.GetType())
	s.False(intent.IsProcessed())

	cIntent, ok := intent.(*cliIntent)
	s.True(ok)
	s.Equal(s.user, cIntent.User)
	s.Equal(s.projectID, cIntent.ProjectID)
	s.Equal(s.hash, cIntent.BaseHash)
	s.Equal(s.module, cIntent.Module)
	s.Equal(s.patchContent, cIntent.PatchContent)
	s.Equal(s.description, cIntent.Description)
	s.True(cIntent.Finalize)
	s.Equal(s.variants, cIntent.BuildVariants)
	s.Equal(s.tasks, cIntent.Tasks)
	s.Zero(cIntent.ProcessedAt)
	s.Zero(cIntent.CreatedAt)
	s.Equal(cIntent.DocumentID.Hex(), intent.ID())

	intent, err = NewCliIntent(s.user, s.projectID, s.hash, "", s.patchContent, "", false, []string{}, []string{})
	s.NotNil(intent)
	s.NoError(err)

	cIntent, ok = intent.(*cliIntent)
	s.True(ok)
	s.Empty(cIntent.BuildVariants)
	s.Empty(cIntent.Tasks)
	s.Empty(cIntent.Description)
	s.Empty(cIntent.Module)

	intent, err = NewCliIntent(s.user, s.projectID, s.hash, s.module, "", s.description, true, s.variants, s.tasks)
	s.NotNil(intent)
	s.NoError(err)
}

func (s *CliIntentSuite) TestNewCliIntentRejectsInvalidIntents() {
	intent, err := NewCliIntent("", s.projectID, s.hash, s.module, s.patchContent, s.description, true, s.variants, s.tasks)
	s.Nil(intent)
	s.Error(err)

	intent, err = NewCliIntent(s.user, "", s.hash, s.module, s.patchContent, s.description, true, s.variants, s.tasks)
	s.Nil(intent)
	s.Error(err)

	intent, err = NewCliIntent(s.user, s.projectID, "", s.module, s.patchContent, s.description, true, s.variants, s.tasks)
	s.Nil(intent)
	s.Error(err)

	intent, err = NewCliIntent(s.user, s.projectID, s.hash, s.module, s.patchContent, s.description, true, []string{}, s.tasks)
	s.Nil(intent)
	s.Error(err)

	intent, err = NewCliIntent(s.user, s.projectID, s.hash, s.module, s.patchContent, s.description, true, s.variants, []string{})
	s.Nil(intent)
	s.Error(err)
}

func (s *CliIntentSuite) TestInsert() {
	intent, err := NewCliIntent(s.user, s.projectID, s.hash, s.module, s.patchContent, s.description, true, s.variants, s.tasks)
	s.NoError(err)
	s.NotNil(intent)

	s.NoError(intent.Insert())

	var intents []*cliIntent
	intents, err = findCliIntents(false)
	s.NoError(err)
	s.Len(intents, 1)
	s.Equal(intent.ID(), intents[0].DocumentID.Hex())
}

func (s *CliIntentSuite) TestSetProcessed() {
	intent, err := NewCliIntent(s.user, s.projectID, s.hash, s.module, s.patchContent, s.description, true, s.variants, s.tasks)
	s.NoError(err)
	s.NotNil(intent)
	s.NoError(intent.Insert())

	s.NoError(intent.SetProcessed())
	s.True(intent.IsProcessed())

	var intents []*cliIntent
	intents, err = findCliIntents(true)
	s.NoError(err)
	s.Len(intents, 1)

	s.True(intents[0].Processed)
}

func findCliIntents(processed bool) ([]*cliIntent, error) {
	var intents []*cliIntent
	err := db.FindAllQ(IntentCollection, db.Query(bson.M{cliProcessedKey: processed, cliIntentTypeKey: CliIntentType}), &intents)
	if err != nil {
		return []*cliIntent{}, err
	}
	return intents, nil
}
