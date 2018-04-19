package units

import (
	"context"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

type PatchIntentUnitsSuite struct {
	sender *send.InternalSender
	env    *mock.Environment
	cancel context.CancelFunc

	repo            string
	headRepo        string
	prNumber        int
	user            string
	hash            string
	diffURL         string
	desc            string
	project         string
	variants        []string
	tasks           []string
	githubPatchData patch.GithubPatch

	suite.Suite
}

func TestPatchIntentUnitsSuite(t *testing.T) {
	suite.Run(t, new(PatchIntentUnitsSuite))
}

func (s *PatchIntentUnitsSuite) SetupSuite() {
	testConfig := testutil.TestConfig()
	db.SetGlobalSessionProvider(testConfig.SessionFactory())
}

func (s *PatchIntentUnitsSuite) SetupTest() {
	s.sender = send.MakeInternalLogger()
	s.env = &mock.Environment{}

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.Require().NoError(s.env.Configure(ctx, filepath.Join(evergreen.FindEvergreenHome(), testutil.TestDir, testutil.TestSettings), nil))
	testutil.ConfigureIntegrationTest(s.T(), s.env.Settings(), "TestPatchIntentUnitsSuite")

	s.NotNil(s.env.Settings())

	s.NoError(db.ClearCollections(evergreen.ConfigCollection, model.ProjectVarsCollection, version.Collection, user.Collection, model.ProjectRefCollection, patch.Collection, patch.IntentCollection))
	s.NoError(db.ClearGridCollections(patch.GridFSPrefix))

	s.NoError((&model.ProjectRef{
		Owner:            "evergreen-ci",
		Repo:             "evergreen",
		Identifier:       "mci",
		Enabled:          true,
		PatchingDisabled: false,
		Branch:           "master",
		RemotePath:       "self-tests.yml",
		RepoKind:         "github",
		PRTestingEnabled: true,
	}).Insert())

	s.NoError((&user.DBUser{
		Id: evergreen.GithubPatchUser,
	}).Insert())

	s.NoError((&model.ProjectVars{
		Id: "mci",
	}).Insert())

	s.NoError((&model.ProjectAlias{
		ProjectID: "mci",
		Alias:     patch.GithubAlias,
		Variant:   "ubuntu.*",
		Task:      "dist.*",
	}).Upsert())
	s.NoError((&model.ProjectAlias{
		ProjectID: "mci",
		Alias:     patch.GithubAlias,
		Variant:   "race.*",
		Task:      "dist.*",
	}).Upsert())
	s.NoError((&model.ProjectAlias{
		ProjectID: "mci",
		Alias:     "doesntexist",
		Variant:   "fake",
		Task:      "fake",
	}).Upsert())

	s.repo = "evergreen-ci/evergreen"
	s.headRepo = "tychoish/evergreen"
	s.prNumber = 448
	s.user = evergreen.GithubPatchUser
	s.hash = "776f608b5b12cd27b8d931c8ee4ca0c13f857299"
	s.diffURL = "https://github.com/evergreen-ci/evergreen/pull/448.diff"
	s.githubPatchData = patch.GithubPatch{
		PRNumber:   448,
		BaseOwner:  "evergreen-ci",
		BaseRepo:   "evergreen",
		BaseBranch: "master",
		HeadOwner:  "richardsamuels",
		HeadRepo:   "evergreen",
		HeadHash:   "something",
		Author:     "richardsamuels",
	}
	s.desc = "Test!"
	s.project = "mci"
	s.variants = []string{"ubuntu1604", "ubuntu1604-arm64", "ubuntu1604-debug", "race-detector"}
	s.tasks = []string{"dist", "dist-test"}

	factory, err := registry.GetJobFactory(patchIntentJobName)
	s.NoError(err)
	s.NotNil(factory)
	s.NotNil(factory())
	s.Equal(factory().Type().Name, patchIntentJobName)
}
func (s *PatchIntentUnitsSuite) TearDownTest() {
	s.cancel()
	evergreen.ResetEnvironment()
}

func (s *PatchIntentUnitsSuite) makeJobAndPatch(intent patch.Intent) *patchIntentProcessor {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	githubOauthToken, err := s.env.Settings().GetGithubOauthToken()
	s.Require().NoError(err)

	j := NewPatchIntentProcessor(bson.NewObjectId(), intent).(*patchIntentProcessor)
	j.env = s.env

	patchDoc := intent.NewPatch()
	s.Require().NoError(j.finishPatch(ctx, patchDoc, githubOauthToken))
	s.Require().NoError(j.Error())
	s.Require().False(j.HasErrors())

	return j
}

func (s *PatchIntentUnitsSuite) TestCantFinalizePatchWithNoTasksAndVariants() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resp, err := http.Get(s.diffURL)
	s.Require().NoError(err)
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	s.Require().NoError(err)

	intent, err := patch.NewCliIntent(s.user, s.project, s.hash, "", string(body), s.desc, true, nil, nil, "doesntexist")
	s.NoError(err)
	s.Require().NotNil(intent)
	s.NoError(intent.Insert())

	githubOauthToken, err := s.env.Settings().GetGithubOauthToken()
	s.Require().NoError(err)

	j := NewPatchIntentProcessor(bson.NewObjectId(), intent).(*patchIntentProcessor)
	j.env = s.env

	patchDoc := intent.NewPatch()
	err = j.finishPatch(ctx, patchDoc, githubOauthToken)
	s.Require().Error(err)
	s.Equal("patch has no build variants or tasks", err.Error())
}

func (s *PatchIntentUnitsSuite) TestProcessCliPatchIntent() {
	githubOauthToken, err := s.env.Settings().GetGithubOauthToken()
	s.Require().NoError(err)

	flags := evergreen.ServiceFlags{
		GithubPRTestingDisabled: true,
	}
	s.NoError(evergreen.SetServiceFlags(flags))

	patchContent, summaries, err := fetchDiffFromGithub(&s.githubPatchData, githubOauthToken)
	s.NoError(err)
	s.NotEmpty(patchContent)
	s.NotEqual("{", patchContent[0])

	s.Equal("cli/host.go", summaries[0].Name)
	s.Equal(2, summaries[0].Additions)
	s.Equal(6, summaries[0].Deletions)

	s.Equal("cli/keys.go", summaries[1].Name)
	s.Equal(1, summaries[1].Additions)
	s.Equal(3, summaries[1].Deletions)

	intent, err := patch.NewCliIntent(s.user, s.project, s.hash, "", patchContent, s.desc, true, s.variants, s.tasks, "")
	s.NoError(err)
	s.Require().NotNil(intent)
	s.NoError(intent.Insert())

	j := s.makeJobAndPatch(intent)

	patchDoc, err := patch.FindOne(patch.ById(j.PatchID))
	s.NoError(err)
	s.Require().NotNil(patchDoc)

	s.verifyPatchDoc(patchDoc, j.PatchID)

	s.Zero(patchDoc.GithubPatchData)

	s.verifyVersionDoc(patchDoc, evergreen.PatchVersionRequester)

	s.gridFSFileExists(patchDoc.Patches[0].PatchSet.PatchFileId)
}

func (s *PatchIntentUnitsSuite) TestProcessGithubPatchIntent() {
	s.Require().NotEmpty(s.env.Settings().GithubPRCreatorOrg)

	dbUser := user.DBUser{
		Id: "testuser",
		Settings: user.UserSettings{
			GithubUser: user.GithubUser{
				UID:         1234,
				LastKnownAs: "somebody",
			},
		},
	}
	s.NoError(dbUser.Insert())
	s.user = dbUser.Id

	intent, err := patch.NewGithubIntent("1", testutil.NewGithubPREvent(s.prNumber, s.repo, s.headRepo, s.hash, "tychoish", ""))
	s.NoError(err)
	s.NotNil(intent)
	s.NoError(intent.Insert())

	j := s.makeJobAndPatch(intent)

	patchDoc, err := patch.FindOne(patch.ById(j.PatchID))
	s.Require().NoError(err)
	s.Require().NotNil(patchDoc)

	s.verifyPatchDoc(patchDoc, j.PatchID)

	s.Equal(s.prNumber, patchDoc.GithubPatchData.PRNumber)
	s.Equal("tychoish", patchDoc.GithubPatchData.Author)
	s.Equal(dbUser.Id, patchDoc.Author)

	repo := strings.Split(s.repo, "/")
	s.Equal(repo[0], patchDoc.GithubPatchData.BaseOwner)
	s.Equal(repo[1], patchDoc.GithubPatchData.BaseRepo)
	headRepo := strings.Split(s.headRepo, "/")
	s.Equal(headRepo[0], patchDoc.GithubPatchData.HeadOwner)
	s.Equal(headRepo[1], patchDoc.GithubPatchData.HeadRepo)
	s.Equal("776f608b5b12cd27b8d931c8ee4ca0c13f857299", patchDoc.Githash)

	s.verifyVersionDoc(patchDoc, evergreen.GithubPRRequester)

	s.gridFSFileExists(patchDoc.Patches[0].PatchSet.PatchFileId)
}

func (s *PatchIntentUnitsSuite) TestFindEvergreenUserForPR() {
	dbUser := user.DBUser{
		Id: "testuser",
		Settings: user.UserSettings{
			GithubUser: user.GithubUser{
				UID:         1234,
				LastKnownAs: "somebody",
			},
		},
	}
	s.NoError(dbUser.Insert())

	u, err := findEvergreenUserForPR(1234)
	s.NoError(err)
	s.Require().NotNil(u)
	s.Equal("testuser", u.Id)

	u, err = findEvergreenUserForPR(123)
	s.NoError(err)
	s.Require().NotNil(u)
	s.Equal(evergreen.GithubPatchUser, u.Id)
}

func (s *PatchIntentUnitsSuite) verifyPatchDoc(patchDoc *patch.Patch, expectedPatchID bson.ObjectId) {
	s.Equal(evergreen.PatchCreated, patchDoc.Status)
	s.Equal(expectedPatchID, patchDoc.Id)
	s.NotEmpty(patchDoc.Patches)
	s.True(patchDoc.Activated)
	s.NotEmpty(patchDoc.PatchedConfig)
	s.NotZero(patchDoc.CreateTime)
	s.Zero(patchDoc.StartTime)
	s.Zero(patchDoc.FinishTime)
	s.NotEqual(0, patchDoc.PatchNumber)
	s.Equal(s.hash, patchDoc.Githash)

	s.Len(patchDoc.Patches, 1)
	s.Empty(patchDoc.Patches[0].ModuleName)
	s.Equal(s.hash, patchDoc.Patches[0].Githash)
	s.Empty(patchDoc.Patches[0].PatchSet.Patch)
	s.NotEmpty(patchDoc.Patches[0].PatchSet.PatchFileId)
	s.Len(patchDoc.Patches[0].PatchSet.Summary, 2)

	s.Len(patchDoc.BuildVariants, 4)
	s.Contains(patchDoc.BuildVariants, "ubuntu1604")
	s.Contains(patchDoc.BuildVariants, "ubuntu1604-arm64")
	s.Contains(patchDoc.BuildVariants, "ubuntu1604-debug")
	s.Contains(patchDoc.BuildVariants, "race-detector")
	s.Len(patchDoc.Tasks, 2)
	s.Contains(patchDoc.Tasks, "dist")
	s.Contains(patchDoc.Tasks, "dist-test")
}

func (s *PatchIntentUnitsSuite) verifyVersionDoc(patchDoc *patch.Patch, expectedRequester string) {
	versionDoc, err := version.FindOne(version.ById(patchDoc.Id.Hex()))
	s.NoError(err)
	s.Require().NotNil(versionDoc)

	s.NotZero(versionDoc.CreateTime)
	s.Zero(versionDoc.StartTime)
	s.Zero(versionDoc.FinishTime)
	s.Equal(s.hash, versionDoc.Revision)
	s.Equal(patchDoc.Description, versionDoc.Message)
	s.NotZero(versionDoc.Config)
	s.Equal(s.user, versionDoc.Author)
	s.Len(versionDoc.BuildIds, 4)

	s.Require().Len(versionDoc.BuildVariants, 4)
	s.True(versionDoc.BuildVariants[0].Activated)
	s.Zero(versionDoc.BuildVariants[0].ActivateAt)
	s.NotEmpty(versionDoc.BuildVariants[0].BuildId)
	s.Contains(versionDoc.BuildIds, versionDoc.BuildVariants[0].BuildId)

	s.True(versionDoc.BuildVariants[1].Activated)
	s.Zero(versionDoc.BuildVariants[1].ActivateAt)
	s.NotEmpty(versionDoc.BuildVariants[1].BuildId)
	s.Contains(versionDoc.BuildIds, versionDoc.BuildVariants[1].BuildId)

	s.True(versionDoc.BuildVariants[2].Activated)
	s.Zero(versionDoc.BuildVariants[2].ActivateAt)
	s.NotEmpty(versionDoc.BuildVariants[2].BuildId)
	s.Contains(versionDoc.BuildIds, versionDoc.BuildVariants[2].BuildId)

	s.True(versionDoc.BuildVariants[3].Activated)
	s.Zero(versionDoc.BuildVariants[3].ActivateAt)
	s.NotEmpty(versionDoc.BuildVariants[3].BuildId)
	s.Contains(versionDoc.BuildIds, versionDoc.BuildVariants[3].BuildId)

	s.Equal(expectedRequester, versionDoc.Requester)
	s.Empty(versionDoc.Errors)
	s.Empty(versionDoc.Warnings)
	s.Empty(versionDoc.Owner)
	s.Empty(versionDoc.Repo)
	s.Equal(versionDoc.Id, patchDoc.Version)
}

func (s *PatchIntentUnitsSuite) gridFSFileExists(patchFileID string) {
	reader, err := db.GetGridFile(patch.GridFSPrefix, patchFileID)
	s.Require().NoError(err)
	s.Require().NotNil(reader)
	defer reader.Close()
	bytes, err := ioutil.ReadAll(reader)
	s.NoError(err)
	s.NotEmpty(bytes)
}

func (s *PatchIntentUnitsSuite) TestRunInDegradedModeWithGithubIntent() {
	flags := evergreen.ServiceFlags{
		GithubPRTestingDisabled: true,
	}
	s.NoError(evergreen.SetServiceFlags(flags))

	intent, err := patch.NewGithubIntent("1", testutil.NewGithubPREvent(s.prNumber, s.repo, s.headRepo, s.hash, "tychoish", ""))
	s.NoError(err)
	s.NotNil(intent)
	s.NoError(intent.Insert())

	patchID := bson.NewObjectId()
	j, ok := NewPatchIntentProcessor(patchID, intent).(*patchIntentProcessor)
	j.env = s.env
	s.True(ok)
	s.NotNil(j)
	j.Run(context.Background())
	s.Error(j.Error())
	s.Contains(j.Error().Error(), "github pr testing is disabled, not processing pull request")

	patchDoc, err := patch.FindOne(patch.ById(patchID))
	s.NoError(err)
	s.Nil(patchDoc)

	unprocessedIntents, err := patch.FindUnprocessedGithubIntents()
	s.NoError(err)
	s.Len(unprocessedIntents, 1)

	s.Equal(intent.ID(), unprocessedIntents[0].ID())
}

func (s *PatchIntentUnitsSuite) TestGithubPRTestFromUnknownUserDoesntCreateVersions() {
	flags := evergreen.ServiceFlags{
		GithubStatusAPIDisabled: true,
	}
	s.Require().NoError(evergreen.SetServiceFlags(flags))

	intent, err := patch.NewGithubIntent("1", testutil.NewGithubPREvent(s.prNumber, s.repo, s.headRepo, s.hash, "octocat", ""))
	s.NoError(err)
	s.NotNil(intent)
	s.NoError(intent.Insert())

	patchID := bson.NewObjectId()
	j, ok := NewPatchIntentProcessor(patchID, intent).(*patchIntentProcessor)
	j.env = s.env
	s.True(ok)
	s.NotNil(j)
	j.Run(context.Background())
	s.Error(j.Error())

	patchDoc, err := patch.FindOne(patch.ById(patchID))
	s.NoError(err)
	if s.NotNil(patchDoc) {
		s.Empty(patchDoc.Version)
	}

	versionDoc, err := version.FindOne(version.ById(patchID.Hex()))
	s.NoError(err)
	s.Nil(versionDoc)

	unprocessedIntents, err := patch.FindUnprocessedGithubIntents()
	s.NoError(err)
	s.Empty(unprocessedIntents)
}

func (s *PatchIntentUnitsSuite) TestBuildPatchURL() {
	s.Equal("https://api.github.com/repos/evergreen-ci/evergreen/pulls/448.diff", buildPatchURL(&s.githubPatchData))
}
