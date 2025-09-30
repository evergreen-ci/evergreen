package patch

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
)

type CliIntentSuite struct {
	suite.Suite

	patchContent               string
	description                string
	variants                   []string
	tasks                      []string
	regexTestSelectionVariants []string
	regexTestSelectionTasks    []string
	module                     string
	user                       string
	projectID                  string
	hash                       string
	alias                      string
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
	s.regexTestSelectionTasks = []string{"task1", ".*2"}
	s.regexTestSelectionVariants = []string{"variant1", ".*2"}
	s.projectID = "project"
	s.description = "desc"
	s.alias = "alias"
}

func (s *CliIntentSuite) SetupTest() {
	s.NoError(db.ClearGridCollections(GridFSPrefix))
	s.NoError(db.Clear(IntentCollection))
}

func (s *CliIntentSuite) TestNewCliIntent() {
	intent, err := NewCliIntent(CLIIntentParams{
		User:                       s.user,
		Project:                    s.projectID,
		BaseGitHash:                s.hash,
		Module:                     s.module,
		PatchContent:               s.patchContent,
		Description:                s.description,
		Finalize:                   true,
		Variants:                   s.variants,
		Tasks:                      s.tasks,
		Alias:                      s.alias,
		RegexTestSelectionVariants: s.regexTestSelectionVariants,
		RegexTestSelectionTasks:    s.regexTestSelectionTasks,
	})
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
	s.Equal(s.regexTestSelectionVariants, cIntent.RegexTestSelectionBuildVariants)
	s.Equal(s.regexTestSelectionTasks, cIntent.RegexTestSelectionTasks)

	intent, err = NewCliIntent(CLIIntentParams{
		User:         s.user,
		Project:      s.projectID,
		BaseGitHash:  s.hash,
		PatchContent: s.patchContent,
	})
	s.NotNil(intent)
	s.NoError(err)

	cIntent, ok = intent.(*cliIntent)
	s.True(ok)
	s.Empty(cIntent.BuildVariants)
	s.Empty(cIntent.Tasks)
	s.Empty(cIntent.Description)
	s.Empty(cIntent.Module)
	s.Empty(cIntent.Alias)

	intent, err = NewCliIntent(CLIIntentParams{
		User:        s.user,
		Project:     s.projectID,
		BaseGitHash: s.hash,
		Module:      s.module,
		Description: s.description,
		Finalize:    true,
		Variants:    s.variants,
		Tasks:       s.tasks,
		Alias:       s.alias,
	})
	s.NotNil(intent)
	s.NoError(err)
}

func (s *CliIntentSuite) TestNewCliIntentRejectsInvalidIntents() {
	intent, err := NewCliIntent(CLIIntentParams{
		Project:      s.projectID,
		BaseGitHash:  s.hash,
		Module:       s.module,
		PatchContent: s.patchContent,
		Description:  s.description,
		Finalize:     true,
		Variants:     s.variants,
		Tasks:        s.tasks,
		Alias:        s.alias,
	})
	s.Nil(intent)
	s.Error(err)

	intent, err = NewCliIntent(CLIIntentParams{
		User:         s.user,
		BaseGitHash:  s.hash,
		Module:       s.module,
		PatchContent: s.patchContent,
		Description:  s.description,
		Finalize:     true,
		Variants:     s.variants,
		Tasks:        s.tasks,
		Alias:        s.alias,
	})
	s.Nil(intent)
	s.Error(err)

	intent, err = NewCliIntent(CLIIntentParams{
		User:         s.user,
		Project:      s.projectID,
		Module:       s.module,
		PatchContent: s.patchContent,
		Description:  s.description,
		Finalize:     true,
		Variants:     s.variants,
		Tasks:        s.tasks,
		Alias:        s.alias,
	})
	s.Nil(intent)
	s.Error(err)

	intent, err = NewCliIntent(CLIIntentParams{
		User:         s.user,
		Project:      s.projectID,
		BaseGitHash:  s.hash,
		Module:       s.module,
		PatchContent: s.patchContent,
		Description:  s.description,
		Finalize:     true,
		Tasks:        s.tasks,
	})
	s.Nil(intent)
	s.Error(err)

	intent, err = NewCliIntent(CLIIntentParams{
		User:         s.user,
		Project:      s.projectID,
		BaseGitHash:  s.hash,
		Module:       s.module,
		PatchContent: s.patchContent,
		Description:  s.description,
		Finalize:     true,
		Variants:     s.variants,
	})
	s.Nil(intent)
	s.Error(err)
}

func (s *CliIntentSuite) TestFindIntentSpecifically() {
	intent, err := NewCliIntent(CLIIntentParams{
		User:        s.user,
		Project:     s.projectID,
		BaseGitHash: s.hash,
		Module:      s.module,
		Description: s.description,
		Finalize:    true,
		Variants:    s.variants,
		Tasks:       s.tasks,
		Alias:       s.alias,
	})
	s.Require().NoError(err)
	s.NotNil(intent)
	s.Require().NoError(intent.Insert(s.T().Context()))

	found, err := FindIntent(s.T().Context(), intent.ID(), intent.GetType())
	s.Require().NoError(err)
	s.NotNil(found)

	found.(*cliIntent).ProcessedAt = time.Time{}
	intent.(*cliIntent).ProcessedAt = time.Time{}

	s.Equal(intent, found)
	s.Equal(intent.NewPatch(), found.NewPatch())
}

func (s *CliIntentSuite) TestInsert() {
	intent, err := NewCliIntent(CLIIntentParams{
		User:         s.user,
		Project:      s.projectID,
		BaseGitHash:  s.hash,
		Module:       s.module,
		PatchContent: s.patchContent,
		Description:  s.description,
		Finalize:     true,
		Variants:     s.variants,
		Tasks:        s.tasks,
		Alias:        s.alias,
	})
	s.Require().NoError(err)
	s.NotNil(intent)

	s.Require().NoError(intent.Insert(s.T().Context()))

	var intents []*cliIntent
	intents, err = findCliIntents(s.T().Context(), false)
	s.NoError(err)
	s.Len(intents, 1)
	s.Equal(intent.ID(), intents[0].DocumentID)
}

func (s *CliIntentSuite) TestSetProcessed() {
	intent, err := NewCliIntent(CLIIntentParams{
		User:         s.user,
		Project:      s.projectID,
		BaseGitHash:  s.hash,
		Module:       s.module,
		PatchContent: s.patchContent,
		Description:  s.description,
		Finalize:     true,
		Variants:     s.variants,
		Tasks:        s.tasks,
		Alias:        s.alias,
	})
	s.Require().NoError(err)
	s.NotNil(intent)
	s.Require().NoError(intent.Insert(s.T().Context()))

	s.Require().NoError(intent.SetProcessed(s.T().Context()))
	s.True(intent.IsProcessed())

	var intents []*cliIntent
	intents, err = findCliIntents(s.T().Context(), true)
	s.NoError(err)
	s.Len(intents, 1)

	s.True(intents[0].Processed)
}

func findCliIntents(ctx context.Context, processed bool) ([]*cliIntent, error) {
	var intents []*cliIntent
	err := db.FindAllQ(ctx, IntentCollection, db.Query(bson.M{cliProcessedKey: processed, cliIntentTypeKey: CliIntentType}), &intents)
	if err != nil {
		return []*cliIntent{}, err
	}
	return intents, nil
}

func (s *CliIntentSuite) TestNewPatch() {
	intent, err := NewCliIntent(CLIIntentParams{
		User:         s.user,
		Project:      s.projectID,
		BaseGitHash:  s.hash,
		Module:       s.module,
		PatchContent: s.patchContent,
		Description:  s.description,
		Finalize:     true,
		Variants:     s.variants,
		Tasks:        s.tasks,
		Alias:        s.alias,
	})
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
	s.Equal(evergreen.VersionCreated, patchDoc.Status)
	s.Zero(patchDoc.CreateTime)
	s.Zero(patchDoc.StartTime)
	s.Zero(patchDoc.FinishTime)
	s.Equal(s.variants, patchDoc.BuildVariants)
	s.Equal(s.tasks, patchDoc.Tasks)
	s.Empty(patchDoc.VariantsTasks)
	s.Empty(patchDoc.Patches)
	s.False(patchDoc.Activated)
	s.Equal(s.alias, patchDoc.Alias)
	s.Zero(patchDoc.GithubPatchData)
}
