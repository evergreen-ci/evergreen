package units

import (
	"context"
	"io/ioutil"
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
	"github.com/k0kubun/pp"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

var testConfig = testutil.TestConfig()

func init() {
	db.SetGlobalSessionProvider(testConfig.SessionFactory())
}

type PatchIntentUnitsSuite struct {
	sender *send.InternalSender
	env    *mock.Environment
	cancel context.CancelFunc

	repo     string
	prNumber int
	user     string
	hash     string
	patchURL string
	desc     string
	project  string
	variants []string
	tasks    []string

	suite.Suite
}

func TestPatchIntentUnitsSuite(t *testing.T) {
	suite.Run(t, new(PatchIntentUnitsSuite))
}

func (s *PatchIntentUnitsSuite) SetupTest() {
	s.sender = send.MakeInternalLogger()
	s.env = &mock.Environment{}

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.Require().NoError(s.env.Configure(ctx, filepath.Join(evergreen.FindEvergreenHome(), testutil.TestDir, testutil.TestSettings)))
	testutil.ConfigureIntegrationTest(s.T(), s.env.Settings(), "TestPatchIntentUnitsSuite")

	s.NotNil(s.env.Settings())

	s.NoError(db.ClearCollections(version.Collection, user.Collection, model.ProjectRefCollection, patch.Collection, patch.IntentCollection, patch.GridFSPrefix))

	s.NoError((&model.ProjectRef{
		Owner:      "evergreen-ci",
		Repo:       "evergreen",
		Identifier: "mci",
		Enabled:    true,
		Branch:     "master",
		RemotePath: "self-tests.yml",
		RepoKind:   "github",
	}).Insert())

	s.NoError((&user.DBUser{
		Id: "github_patch_user",
	}).Insert())

	s.repo = "evergreen-ci/evergreen"
	s.prNumber = 448
	s.user = "github_patch_user"
	s.hash = "776f608b5b12cd27b8d931c8ee4ca0c13f857299"
	s.patchURL = "https://github.com/evergreen-ci/evergreen/pull/448.patch"
	s.desc = "Test!"
	s.project = "mci"
	s.variants = []string{"ubuntu1604"}
	s.tasks = []string{"dist"}

	factory, err := registry.GetJobFactory(patchIntentJobName)
	s.NoError(err)
	s.NotNil(factory)
	s.NotNil(factory())
	s.Equal(factory().Type().Name, patchIntentJobName)
}

func (s *PatchIntentUnitsSuite) verifyJob(intent patch.Intent) *patchIntentProcessor {
	j := makePatchIntentProcessor()
	j.Intent = intent
	j.env = s.env
	j.PatchID = bson.NewObjectId()
	j.logger = logging.MakeGrip(s.sender)
	s.False(j.Status().Completed)
	s.NotPanics(func() { j.Run() })
	s.True(j.Status().Completed)
	pp.Println(j.Error())
	s.Require().False(j.HasErrors())

	return j
}

func (s *PatchIntentUnitsSuite) TestProcessGithubPatchIntent() {
	intent, err := patch.NewGithubIntent("1", s.repo, s.prNumber, s.user, s.hash, s.patchURL)
	s.NoError(err)
	s.NotNil(intent)
	s.NoError(intent.Insert())

	j := s.verifyJob(intent)
	patchDoc, err := patch.FindOne(patch.ById(j.PatchID))
	s.NoError(err)
	s.NotNil(patchDoc)

	// patchdoc itself
	s.verifyPatchDoc(patchDoc, j.PatchID)
	// TODO
	//s.NotEmpty(patchDoc.BuildVariants)
	//s.NotEmpty(patchDoc.Tasks)

	s.Equal(s.prNumber, patchDoc.GithubPatchData.PRNumber)
	s.Equal(s.user, patchDoc.GithubPatchData.Author)
	s.Equal(s.patchURL, patchDoc.GithubPatchData.PatchURL)
	repo := strings.Split(s.repo, "/")
	s.Equal(repo[0], patchDoc.GithubPatchData.Owner)
	s.Equal(repo[1], patchDoc.GithubPatchData.Repository)
	s.Equal(patchDoc.Patches[0].PatchSet.PatchFileId, patchDoc.Version)

	s.verifyVersionDoc(patchDoc, evergreen.PatchVersionRequester, j.user.Email())

	// Gridfs file
	s.gridFSFileExists(patchDoc.Patches[0].PatchSet.PatchFileId)
}

func (s *PatchIntentUnitsSuite) TestProcessCliPatchIntent() {
	patchContent, err := fetchPatchByURL(s.patchURL)
	s.NoError(err)
	s.NotEmpty(patchContent)

	intent, err := patch.NewCliIntent(s.user, s.project, s.hash, "", patchContent, s.desc, true, s.variants, s.tasks)
	s.NoError(err)
	s.Require().NotNil(intent)
	s.NoError(intent.Insert())

	j := s.verifyJob(intent)

	patchDoc, err := patch.FindOne(patch.ById(j.PatchID))
	s.NoError(err)
	s.Require().NotNil(patchDoc)

	s.verifyPatchDoc(patchDoc, j.PatchID)

	s.Zero(patchDoc.GithubPatchData)

	s.verifyVersionDoc(patchDoc, evergreen.PatchVersionRequester, j.user.Email())

	// Gridfs file
	s.gridFSFileExists(patchDoc.Patches[0].PatchSet.PatchFileId)
}

func (s *PatchIntentUnitsSuite) verifyPatchDoc(patchDoc *patch.Patch, expectedPatchID bson.ObjectId) {
	s.Equal(evergreen.PatchCreated, patchDoc.Status)
	s.Equal(s.variants, patchDoc.BuildVariants)
	s.Equal(s.tasks, patchDoc.Tasks)
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
}

func (s *PatchIntentUnitsSuite) verifyVersionDoc(patchDoc *patch.Patch, expectedRequester, expectedEmail string) {
	versionDoc, err := version.FindOne(version.ById(patchDoc.Id.Hex()))
	s.NoError(err)
	s.Require().NotNil(versionDoc)

	s.NotZero(versionDoc.CreateTime)
	s.Zero(versionDoc.StartTime)
	s.Zero(versionDoc.FinishTime)
	s.Equal(s.hash, versionDoc.Revision)
	s.Equal(expectedEmail, versionDoc.AuthorEmail)
	s.Equal(patchDoc.Description, versionDoc.Message)
	s.NotZero(versionDoc.Config)
	s.Equal(s.user, versionDoc.Author)
	s.Len(versionDoc.BuildIds, 1)

	s.Require().Len(versionDoc.BuildVariants, 1)
	s.True(versionDoc.BuildVariants[0].Activated)
	s.Zero(versionDoc.BuildVariants[0].ActivateAt)
	s.Equal(versionDoc.BuildVariants[0].BuildId, versionDoc.BuildIds[0])
	s.Equal(versionDoc.BuildVariants[0].BuildVariant, s.variants[0])
	s.Equal(expectedRequester, versionDoc.Requester)
	s.Empty(versionDoc.Errors)
	s.Empty(versionDoc.Warnings)
	s.Empty(versionDoc.Owner)
	s.Empty(versionDoc.Repo)
	s.Equal(versionDoc.Id, patchDoc.Version)
	versionDoc.Config = ""
	pp.Println(versionDoc)
}

func (s *PatchIntentUnitsSuite) gridFSFileExists(patchFileID string) {
	reader, err := db.GetGridFile(patch.GridFSPrefix, patchFileID)
	s.NoError(err)
	s.Require().NotNil(reader)
	defer reader.Close()
	bytes, err := ioutil.ReadAll(reader)
	s.NoError(err)
	s.NotEmpty(bytes)
}
