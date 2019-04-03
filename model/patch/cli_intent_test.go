package patch

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
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
	alias        string
}

func TestCliIntentSuite(t *testing.T) {
	suite.Run(t, new(CliIntentSuite))
}

func (s *CliIntentSuite) SetupSuite() {
	s.patchContent = "patch"
	s.hash = "67da19930b1b18d346477e99a8e18094a672f48a"
	s.user = "octocat"
	s.module = "module"
	s.tasks = []string{"task1", "Task2"}
	s.variants = []string{"variant1", "variant2"}
	s.projectID = "project"
	s.description = "desc"
	s.alias = "alias"
}

func (s *CliIntentSuite) SetupTest() {
	s.NoError(db.ClearGridCollections(GridFSPrefix))
	s.NoError(db.Clear(IntentCollection))
}

func (s *CliIntentSuite) TestNewCliIntent() {
	intent, err := NewCliIntent(s.user, s.projectID, s.hash, s.module, s.patchContent, s.description, true, s.variants, s.tasks, s.alias)
	s.NotNil(intent)
	s.NoError(err)
	s.Implements((*Intent)(nil), intent)
	s.True(intent.ShouldFinalizePatch())
	s.Equal(CliIntentType, intent.GetType())
	s.False(intent.IsProcessed())
	s.Equal(evergreen.PatchVersionRequester, intent.RequesterIdentity())

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
	s.Equal(cIntent.DocumentID, intent.ID())
	s.Equal(s.alias, cIntent.Alias)

	intent, err = NewCliIntent(s.user, s.projectID, s.hash, "", s.patchContent, "", false, []string{}, []string{}, "")
	s.NotNil(intent)
	s.NoError(err)

	cIntent, ok = intent.(*cliIntent)
	s.True(ok)
	s.Empty(cIntent.BuildVariants)
	s.Empty(cIntent.Tasks)
	s.Empty(cIntent.Description)
	s.Empty(cIntent.Module)
	s.Empty(cIntent.Alias)

	intent, err = NewCliIntent(s.user, s.projectID, s.hash, s.module, "", s.description, true, s.variants, s.tasks, s.alias)
	s.NotNil(intent)
	s.NoError(err)
}

func (s *CliIntentSuite) TestNewCliIntentRejectsInvalidIntents() {
	intent, err := NewCliIntent("", s.projectID, s.hash, s.module, s.patchContent, s.description, true, s.variants, s.tasks, s.alias)
	s.Nil(intent)
	s.Error(err)

	intent, err = NewCliIntent(s.user, "", s.hash, s.module, s.patchContent, s.description, true, s.variants, s.tasks, s.alias)
	s.Nil(intent)
	s.Error(err)

	intent, err = NewCliIntent(s.user, s.projectID, "", s.module, s.patchContent, s.description, true, s.variants, s.tasks, s.alias)
	s.Nil(intent)
	s.Error(err)

	intent, err = NewCliIntent(s.user, s.projectID, s.hash, s.module, s.patchContent, s.description, true, []string{}, s.tasks, "")
	s.Nil(intent)
	s.Error(err)

	intent, err = NewCliIntent(s.user, s.projectID, s.hash, s.module, s.patchContent, s.description, true, s.variants, []string{}, "")
	s.Nil(intent)
	s.Error(err)
}

func (s *CliIntentSuite) TestFindIntentSpecifically() {
	intent, err := NewCliIntent(s.user, s.projectID, s.hash, s.module, "", s.description, true, s.variants, s.tasks, s.alias)
	s.NoError(err)
	s.NotNil(intent)
	s.NoError(intent.Insert())

	found, err := FindIntent(intent.ID(), intent.GetType())
	s.NoError(err)
	s.NotNil(found)

	found.(*cliIntent).ProcessedAt = time.Time{}
	intent.(*cliIntent).ProcessedAt = time.Time{}

	s.Equal(intent, found)
	s.Equal(intent.NewPatch(), found.NewPatch())
}

func (s *CliIntentSuite) TestInsert() {
	intent, err := NewCliIntent(s.user, s.projectID, s.hash, s.module, s.patchContent, s.description, true, s.variants, s.tasks, s.alias)
	s.NoError(err)
	s.NotNil(intent)

	s.NoError(intent.Insert())

	var intents []*cliIntent
	intents, err = findCliIntents(false)
	s.NoError(err)
	s.Len(intents, 1)
	s.Equal(intent.ID(), intents[0].DocumentID)
}

func (s *CliIntentSuite) TestSetProcessed() {
	intent, err := NewCliIntent(s.user, s.projectID, s.hash, s.module, s.patchContent, s.description, true, s.variants, s.tasks, s.alias)
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

func (s *CliIntentSuite) TestNewPatch() {
	intent, err := NewCliIntent(s.user, s.projectID, s.hash, s.module, s.patchContent, s.description, true, s.variants, s.tasks, s.alias)
	s.NoError(err)
	s.NotNil(intent)

	patchDoc := intent.NewPatch()
	s.NotNil(patchDoc)
	s.Zero(patchDoc.Id)
	s.Equal(s.description, patchDoc.Description)
	s.Equal(s.projectID, patchDoc.Project)
	s.Equal(s.hash, patchDoc.Githash)
	s.Zero(patchDoc.PatchNumber)
	s.Empty(patchDoc.Version)
	s.Equal(evergreen.PatchCreated, patchDoc.Status)
	s.Zero(patchDoc.CreateTime)
	s.Zero(patchDoc.StartTime)
	s.Zero(patchDoc.FinishTime)
	s.Equal(s.variants, patchDoc.BuildVariants)
	s.Equal(s.tasks, patchDoc.Tasks)
	s.Empty(patchDoc.VariantsTasks)
	s.Len(patchDoc.Patches, 1)
	s.Equal(s.module, patchDoc.Patches[0].ModuleName)
	s.Equal(s.hash, patchDoc.Patches[0].Githash)
	s.Zero(patchDoc.Patches[0].PatchSet)
	s.False(patchDoc.Activated)
	s.Empty(patchDoc.PatchedConfig)
	s.Equal(s.alias, patchDoc.Alias)
	s.Zero(patchDoc.GithubPatchData)
}
