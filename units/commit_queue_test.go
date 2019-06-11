package units

import (
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
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
	s.Require().Len(s.prBody, 24757)

	s.projectRef = &model.ProjectRef{
		Identifier: "mci",
		Owner:      "baxterthehacker",
		Repo:       "public-repo",
		CommitQueue: model.CommitQueueParams{
			Enabled:     true,
			MergeMethod: "squash",
		},
	}
	s.Require().NoError(s.projectRef.Insert())

	s.env = &mock.Environment{}
	s.ctx = context.Background()
	s.NoError(s.env.Configure(s.ctx, filepath.Join(evergreen.FindEvergreenHome(), testutil.TestDir, testutil.TestSettings), nil))
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

func (s *commitQueueSuite) TestNewCommitQueueJob() {
	job := NewCommitQueueJob(s.env, "mci", "job-1")
	s.Equal("commit-queue:mci_job-1", job.ID())
}

func (s *commitQueueSuite) TestSubscribeMerge() {
	s.NoError(db.ClearCollections(event.SubscriptionsCollection))
	s.NoError(subscribeMerge(s.projectRef.Identifier, s.projectRef.Owner, s.projectRef.Repo, "squash", "abcdef", s.pr))

	selectors := []event.Selector{
		event.Selector{
			Type: "id",
			Data: "abcdef",
		},
	}
	subscriptions, err := event.FindSubscriptions(event.ResourceTypePatch, selectors)
	s.NoError(err)
	s.Require().Len(subscriptions, 1)

	subscription := subscriptions[0]

	s.Equal(event.GithubMergeSubscriberType, subscription.Subscriber.Type)
	s.Equal(event.ResourceTypePatch, subscription.ResourceType)
	target, ok := subscription.Subscriber.Target.(*event.GithubMergeSubscriber)
	s.True(ok)
	s.Equal(s.projectRef.Owner, target.Owner)
	s.Equal(s.projectRef.Repo, target.Repo)
	s.Equal(s.pr.GetTitle()+fmt.Sprintf(" (#%d)", *s.pr.Number), target.CommitTitle)
}

func (s *commitQueueSuite) TestWritePatchInfo() {
	s.NoError(db.ClearGridCollections(patch.GridFSPrefix))

	patchDoc := &patch.Patch{
		Id:      mgobson.ObjectIdHex("aabbccddeeff112233445566"),
		Githash: "abcdef",
	}

	patchSummaries := []patch.Summary{
		patch.Summary{
			Name:      "myfile.go",
			Additions: 1,
			Deletions: 0,
		},
	}

	patchContent := `diff --git a/myfile.go b/myfile.go
	index abcdef..123456 100644
	--- a/myfile.go
	+++ b/myfile.go
	@@ +2,1 @@ func myfunc {
	+				fmt.Print(\"hello world\")
			}
	`

	s.NoError(writePatchInfo(patchDoc, patchSummaries, patchContent))
	s.Len(patchDoc.Patches, 1)
	s.Equal(patchSummaries, patchDoc.Patches[0].PatchSet.Summary)
	reader, err := db.GetGridFile(patch.GridFSPrefix, patchDoc.Patches[0].PatchSet.PatchFileId)
	s.NoError(err)
	defer reader.Close()
	bytes, err := ioutil.ReadAll(reader)
	s.NoError(err)
	s.Equal(patchContent, string(bytes))
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

func (s *commitQueueSuite) TestAddMergeTaskAndVariant() {
	config, err := evergreen.GetConfig()
	s.NoError(err)
	s.NoError(db.ClearCollections(distro.Collection))
	s.NoError((&distro.Distro{
		Id: config.CommitQueue.MergeTaskDistro,
	}).Insert())

	project := &model.Project{}
	patchDoc := &patch.Patch{}

	s.NoError(addMergeTaskAndVariant(patchDoc, project))

	s.Require().Len(patchDoc.BuildVariants, 1)
	s.Equal("commit-queue-merge", patchDoc.BuildVariants[0])
	s.Require().Len(patchDoc.Tasks, 1)
	s.Equal("merge-patch", patchDoc.Tasks[0])

	s.Require().Len(project.BuildVariants, 1)
	s.Equal("commit-queue-merge", project.BuildVariants[0].Name)
	s.Require().Len(project.Tasks, 1)
	s.Equal("merge-patch", project.Tasks[0].Name)
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

func (s *commitQueueSuite) TestUpdateGithashes() {
	githubToken, err := s.settings.GetGithubOauthToken()
	s.NoError(err)

	projectRef := &model.ProjectRef{
		Owner:  "evergreen-ci",
		Repo:   "evergreen",
		Branch: "master",
	}

	project := &model.Project{
		Modules: model.ModuleList{
			{Name: "evergreen-module", Repo: "git@github.com:evergreen-ci/evergreen.git", Branch: "master"},
		},
	}

	patchDoc := &patch.Patch{
		Patches: []patch.ModulePatch{
			{ModuleName: "", Githash: "abcdef"},
			{ModuleName: "evergreen-module", Githash: "abcdef"},
		},
	}

	err = updateGithashes(context.Background(), githubToken, projectRef, project, patchDoc)
	s.NoError(err)

	for _, mod := range patchDoc.Patches {
		s.NotEqual("abcdef", mod.Githash)
	}
}
