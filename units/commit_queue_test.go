package units

import (
	"context"
	"io/ioutil"
	"path/filepath"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/github"
	"github.com/stretchr/testify/suite"
	mgobson "gopkg.in/mgo.v2/bson"
)

type commitQueueSuite struct {
	suite.Suite
	env      *mock.Environment
	ctx      context.Context
	settings *evergreen.Settings

	prBody     []byte
	pr         *github.PullRequest
	projectRef *model.ProjectRef
}

func TestCommitQueueJob(t *testing.T) {
	s := &commitQueueSuite{}
	env := testutil.NewEnvironment(context.Background(), t)
	settings := env.Settings()
	testutil.ConfigureIntegrationTest(t, settings, "TestGitGetProjectSuite")
	s.settings = settings

	suite.Run(t, s)
}

func (s *commitQueueSuite) SetupSuite() {
	s.NoError(db.ClearCollections(model.ProjectRefCollection))
	var err error
	s.prBody, err = ioutil.ReadFile(filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "pull_request.json"))
	s.NoError(err)
	s.Require().Len(s.prBody, 24745)

	s.projectRef = &model.ProjectRef{
		Id:    "mci",
		Owner: "baxterthehacker",
		Repo:  "public-repo",
		CommitQueue: model.CommitQueueParams{
			Enabled:     utility.TruePtr(),
			MergeMethod: "squash",
		},
	}
	s.Require().NoError(s.projectRef.Insert())

	s.env = &mock.Environment{}
	s.ctx = context.Background()
	s.NoError(s.env.Configure(s.ctx))
}

func (s *commitQueueSuite) SetupTest() {
	s.Require().NoError(db.ClearCollections(commitqueue.Collection))

	webhookInterface, err := github.ParseWebHook("pull_request", s.prBody)
	s.NoError(err)
	prEvent, ok := webhookInterface.(*github.PullRequestEvent)
	s.True(ok)
	s.pr = prEvent.GetPullRequest()

	cq := &commitqueue.CommitQueue{
		ProjectID: "mci",
		Queue: []commitqueue.CommitQueueItem{
			commitqueue.CommitQueueItem{
				Issue: "1",
			},
			commitqueue.CommitQueueItem{
				Issue: "2",
			},
			commitqueue.CommitQueueItem{
				Issue: "3",
			},
			commitqueue.CommitQueueItem{
				Issue: "4",
			},
		},
	}
	s.Require().NoError(commitqueue.InsertQueue(cq))
}

func (s *commitQueueSuite) TestTryUnstick() {
	job := commitQueueJob{}
	s.NoError(db.Clear(patch.Collection))

	patchID := mgobson.ObjectIdHex("aabbccddeeff112233445566")
	patchDoc := &patch.Patch{
		Id:         patchID,
		Githash:    "abcdef",
		FinishTime: time.Now(),
	}
	s.NoError(patchDoc.Insert())

	var cq *commitqueue.CommitQueue = &commitqueue.CommitQueue{
		ProjectID: "mci",
		Queue: []commitqueue.CommitQueueItem{
			{
				Issue:   "aabbccddeeff112233445566",
				Source:  commitqueue.SourceDiff,
				Version: patchID.Hex(),
			},
		},
	}
	job.TryUnstick(context.Background(), cq, s.projectRef, "")
	s.Len(cq.Queue, 0)
}

func (s *commitQueueSuite) TestNewCommitQueueJob() {
	job := NewCommitQueueJob(s.env, "mci", "job-1")
	s.Equal("commit-queue:mci_job-1", job.ID())
}

func (s *commitQueueSuite) TestValidateBranch() {
	var branch *github.Branch
	s.Error(validateBranch(branch))

	branch = &github.Branch{}
	s.Error(validateBranch(branch))

	branch.Commit = &github.RepositoryCommit{}
	s.Error(validateBranch(branch))

	sha := "abcdef"
	branch.Commit.SHA = &sha

	s.NoError(validateBranch(branch))
}

func (s *commitQueueSuite) TestSetDefaultNotification() {
	s.NoError(db.ClearCollections(user.Collection))

	// User with no configuration for notifications is signed up for email notifications
	u1 := &user.DBUser{
		Id: "u1",
	}
	s.NoError(u1.Insert())

	s.NoError(setDefaultNotification(u1.Id))

	u1, err := user.FindOneById(u1.Id)
	s.NoError(err)

	s.Equal(user.PreferenceEmail, u1.Settings.Notifications.CommitQueue)
	s.NotEqual("", u1.Settings.Notifications.CommitQueueID)

	// User that opted out is not affected
	u2 := &user.DBUser{
		Id: "u2",
		Settings: user.UserSettings{
			Notifications: user.NotificationPreferences{
				CommitQueue: "none",
			},
		},
	}
	s.NoError(u2.Insert())

	s.NoError(setDefaultNotification(u2.Id))

	u2, err = user.FindOneById(u2.Id)
	s.NoError(err)

	s.EqualValues("none", u2.Settings.Notifications.CommitQueue)
	s.Equal("", u2.Settings.Notifications.CommitQueueID)
}

func (s *commitQueueSuite) TestUpdatePatch() {
	githubToken, err := s.settings.GetGithubOauthToken()
	s.NoError(err)

	projectRef := &model.ProjectRef{
		Id:         "evergreen",
		Owner:      "evergreen-ci",
		Repo:       "evergreen",
		Branch:     "main",
		RemotePath: "self-tests.yml",
	}
	s.NoError(projectRef.Insert())

	patchDoc := &patch.Patch{
		Patches: []patch.ModulePatch{
			{ModuleName: "", Githash: "abcdef"},
		},
		PatchedConfig: "asdf",
		Project:       "evergreen",
		BuildVariants: []string{"my-variant"},
		Tasks:         []string{"my-task"},
		VariantsTasks: []patch.VariantTasks{
			{Variant: "my-variant", Tasks: []string{"my-task"}},
		},
	}

	projectConfig, err := updatePatch(context.Background(), githubToken, projectRef, patchDoc)
	s.NoError(err)
	s.NotEqual("abcdef", patchDoc.Patches[0].Githash)
	s.NotEqual(model.Project{}, projectConfig)
	s.NotEqual("asdf", patchDoc.PatchedConfig)

	s.Empty(patchDoc.Tasks)
	s.Empty(patchDoc.VariantsTasks)
	s.Empty(patchDoc.BuildVariants)
}
