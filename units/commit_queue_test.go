package units

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v52/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type commitQueueSuite struct {
	suite.Suite
	env      *mock.Environment
	suiteCtx context.Context
	cancel   context.CancelFunc
	ctx      context.Context
	settings *evergreen.Settings

	prBody     []byte
	pr         *github.PullRequest
	projectRef *model.ProjectRef
}

func TestCommitQueueJob(t *testing.T) {
	s := &commitQueueSuite{}

	s.suiteCtx, s.cancel = context.WithCancel(context.Background())
	s.suiteCtx = testutil.TestSpan(s.suiteCtx, t)

	env := testutil.NewEnvironment(s.suiteCtx, t)
	settings := env.Settings()
	testutil.ConfigureIntegrationTest(t, settings, t.Name())
	s.settings = settings

	suite.Run(t, s)
}

func (s *commitQueueSuite) SetupSuite() {
	s.NoError(db.ClearCollections(model.ProjectRefCollection))
	var err error
	s.prBody, err = os.ReadFile(filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "pull_request.json"))
	s.NoError(err)
	s.Require().Len(s.prBody, 24706)

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
	s.NoError(s.env.Configure(s.suiteCtx))
}

func (s *commitQueueSuite) TearDownSuite() {
	s.cancel()
}

func (s *commitQueueSuite) SetupTest() {
	s.Require().NoError(db.ClearCollections(commitqueue.Collection))
	s.ctx = testutil.TestSpan(s.suiteCtx, s.T())

	webhookInterface, err := github.ParseWebHook("pull_request", s.prBody)
	s.NoError(err)
	prEvent, ok := webhookInterface.(*github.PullRequestEvent)
	s.True(ok)
	s.pr = prEvent.GetPullRequest()

	cq := &commitqueue.CommitQueue{
		ProjectID: "mci",
		Queue: []commitqueue.CommitQueueItem{
			{
				Issue: "1",
			},
			{
				Issue: "2",
			},
			{
				Issue: "3",
			},
			{
				Issue: "4",
			},
		},
	}
	s.Require().NoError(commitqueue.InsertQueue(cq))
}

func (s *commitQueueSuite) TestTryUnstickDequeuesAlreadyFinishedCommitQueueItem() {
	job := commitQueueJob{}
	s.NoError(db.ClearCollections(task.Collection, patch.Collection, model.VersionCollection, commitqueue.Collection))

	patchID := mgobson.NewObjectId()
	patchDoc := &patch.Patch{
		Id:         patchID,
		Status:     evergreen.VersionFailed,
		Githash:    "abcdef",
		FinishTime: time.Now(),
	}
	s.Require().NoError(patchDoc.Insert())
	v := model.Version{
		Id: patchID.Hex(),
	}
	s.Require().NoError(v.Insert())
	mergeTask := task.Task{
		Id:               "merge_task",
		Status:           evergreen.TaskSucceeded,
		Activated:        true,
		CommitQueueMerge: true,
		Version:          v.Id,
		DependsOn: []task.Dependency{
			{
				TaskId:       "some_dependency",
				Unattainable: true,
			},
		},
	}
	s.Require().NoError(mergeTask.Insert())

	cq := &commitqueue.CommitQueue{
		ProjectID: "mci",
		Queue: []commitqueue.CommitQueueItem{
			{
				Issue:   patchID.Hex(),
				Source:  commitqueue.SourceDiff,
				Version: v.Id,
			},
		},
	}
	s.Require().NoError(commitqueue.InsertQueue(cq))
	job.TryUnstick(s.ctx, cq, s.projectRef, "")

	dbCQ, err := commitqueue.FindOneId(cq.ProjectID)
	s.Require().NoError(err)
	s.Require().NotZero(dbCQ)
	s.Empty(dbCQ.Queue, "commit queue item should be removed because it is already finished")
}

func (s *commitQueueSuite) TestTryUnstickFixesBlockedMergeTask() {
	j := commitQueueJob{}
	s.Require().NoError(db.ClearCollections(task.Collection, patch.Collection, model.VersionCollection, commitqueue.Collection))

	p := &patch.Patch{
		Id:     mgobson.NewObjectId(),
		Status: evergreen.VersionStarted,
	}
	s.Require().NoError(p.Insert())
	v := model.Version{
		Id: p.Id.Hex(),
	}
	s.Require().NoError(v.Insert())
	mergeTask := task.Task{
		Id:               "merge_task",
		Activated:        true,
		CommitQueueMerge: true,
		Version:          v.Id,
		DependsOn: []task.Dependency{
			{
				TaskId:       "some_dependency",
				Unattainable: true,
			},
		},
	}
	s.Require().NoError(mergeTask.Insert())
	s.True(mergeTask.Blocked())

	cq := &commitqueue.CommitQueue{
		ProjectID: s.projectRef.Id,
		Queue: []commitqueue.CommitQueueItem{
			{
				Issue:   p.Id.Hex(),
				Source:  commitqueue.SourceDiff,
				Version: v.Id,
			},
		},
	}
	s.Require().NoError(commitqueue.InsertQueue(cq))

	j.TryUnstick(s.ctx, cq, s.projectRef, "")
	s.NoError(j.Error())

	dbCQ, err := commitqueue.FindOneId(cq.ProjectID)
	s.Require().NoError(err)
	s.Require().NotZero(dbCQ)
	s.Empty(dbCQ.Queue, "commit queue should be blocked due to merge task")
}

func (s *commitQueueSuite) TestTryUnstickDoesNotUnstickMergeTaskBlockedByResettingDependencies() {
	j := commitQueueJob{}
	s.Require().NoError(db.ClearCollections(task.Collection, patch.Collection, model.VersionCollection, commitqueue.Collection))

	p := &patch.Patch{
		Id:     mgobson.NewObjectId(),
		Status: evergreen.VersionStarted,
	}
	s.Require().NoError(p.Insert())
	v := model.Version{
		Id: p.Id.Hex(),
	}
	s.Require().NoError(v.Insert())

	resettingTask := task.Task{
		Id:                "resetting_task",
		Status:            evergreen.TaskFailed,
		Version:           v.Id,
		Activated:         true,
		Aborted:           true,
		ResetWhenFinished: true,
	}
	s.Require().NoError(resettingTask.Insert())
	mergeTask := task.Task{
		Id:               "merge_task",
		Activated:        true,
		CommitQueueMerge: true,
		Version:          v.Id,
		DependsOn: []task.Dependency{
			{
				TaskId:       resettingTask.Id,
				Unattainable: true,
			},
		},
	}
	s.Require().NoError(mergeTask.Insert())
	s.True(mergeTask.Blocked())

	cq := &commitqueue.CommitQueue{
		ProjectID: s.projectRef.Id,
		Queue: []commitqueue.CommitQueueItem{
			{
				Issue:   p.Id.Hex(),
				Source:  commitqueue.SourceDiff,
				Version: v.Id,
			},
		},
	}
	s.Require().NoError(commitqueue.InsertQueue(cq))

	j.TryUnstick(s.ctx, cq, s.projectRef, "")
	s.NoError(j.Error())

	dbCQ, err := commitqueue.FindOneId(cq.ProjectID)
	s.Require().NoError(err)
	s.Require().NotZero(dbCQ)
	s.Len(dbCQ.Queue, 1, "commit queue item should remain enqueued even when merge task is blocked if waiting to reset dependencies")
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

func (s *commitQueueSuite) TestAddMergeTaskAndVariant() {
	s.NoError(db.ClearCollections(distro.Collection, evergreen.ConfigCollection))
	config, err := evergreen.GetConfig(s.ctx)
	s.NoError(err)
	config.CommitQueue.MergeTaskDistro = "d"
	s.NoError(config.CommitQueue.Set(s.ctx))
	s.NoError((&distro.Distro{
		Id: config.CommitQueue.MergeTaskDistro,
	}).Insert(s.ctx))

	project := &model.Project{}
	patchDoc := &patch.Patch{}
	ref := &model.ProjectRef{}

	pp, err := AddMergeTaskAndVariant(s.ctx, patchDoc, project, ref, commitqueue.SourceDiff)
	s.NoError(err)
	s.NotZero(pp)

	s.Require().Len(patchDoc.BuildVariants, 1)
	s.Equal(evergreen.MergeTaskVariant, patchDoc.BuildVariants[0])
	s.Require().Len(patchDoc.Tasks, 1)
	s.Equal(evergreen.MergeTaskName, patchDoc.Tasks[0])

	s.Require().Len(pp.BuildVariants, 1)
	s.Equal(evergreen.MergeTaskVariant, pp.BuildVariants[0].Name)
	s.Require().Len(pp.BuildVariants[0].Tasks, 1)
	s.True(project.BuildVariants[0].Tasks[0].CommitQueueMerge)
	s.Equal(evergreen.MergeTaskGroup, project.BuildVariants[0].Tasks[0].Name)
	s.True(project.BuildVariants[0].Tasks[0].IsGroup)
	s.Equal(evergreen.MergeTaskVariant, project.BuildVariants[0].Tasks[0].Variant)
	s.Require().Len(pp.Tasks, 1)
	s.Equal(evergreen.MergeTaskName, project.Tasks[0].Name)
	s.Require().Len(pp.TaskGroups, 1)
	s.Equal(evergreen.MergeTaskGroup, project.TaskGroups[0].Name)
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
		Project:       "evergreen",
		BuildVariants: []string{"my-variant"},
		Tasks:         []string{"my-task"},
		VariantsTasks: []patch.VariantTasks{
			{Variant: "my-variant", Tasks: []string{"my-task"}},
		},
	}

	projectConfig, pp, err := updatePatch(s.ctx, s.settings, githubToken, projectRef, patchDoc)
	s.NoError(err)
	s.NotEqual("abcdef", patchDoc.Patches[0].Githash)
	s.NotEqual(model.Project{}, projectConfig)

	s.Empty(patchDoc.Tasks)
	s.Empty(patchDoc.VariantsTasks)
	s.Empty(patchDoc.BuildVariants)

	s.Require().NotZero(pp)
	s.NotEmpty(pp.BuildVariants)
	s.NotEmpty(pp.Tasks)
}

func TestAddMergeTaskDependencies(t *testing.T) {
	assert.NoError(t, db.ClearCollections(task.Collection))
	j := commitQueueJob{}
	mergeTask1 := task.Task{
		Id:               "1",
		Requester:        evergreen.MergeTestRequester,
		DisplayName:      evergreen.MergeTaskName,
		BuildVariant:     evergreen.MergeTaskVariant,
		Version:          "v1",
		CommitQueueMerge: true,
	}
	assert.NoError(t, mergeTask1.Insert())
	mergeTask2 := task.Task{
		Id:               "2",
		Requester:        evergreen.MergeTestRequester,
		DisplayName:      evergreen.MergeTaskName,
		BuildVariant:     evergreen.MergeTaskVariant,
		Version:          "v2",
		CommitQueueMerge: true,
	}
	assert.NoError(t, mergeTask2.Insert())
	mergeTask3 := task.Task{
		Id:               "3",
		Requester:        evergreen.MergeTestRequester,
		DisplayName:      evergreen.MergeTaskName,
		BuildVariant:     evergreen.MergeTaskVariant,
		Version:          "v3",
		CommitQueueMerge: true,
	}
	assert.NoError(t, mergeTask3.Insert())
	cq := commitqueue.CommitQueue{
		Queue: []commitqueue.CommitQueueItem{
			{Version: mergeTask1.Version},
			{Version: mergeTask2.Version},
			{Version: mergeTask3.Version},
		},
	}

	assert.NoError(t, j.addMergeTaskDependencies(cq))
	dbTask1, err := task.FindOneId(mergeTask1.Id)
	assert.NoError(t, err)
	assert.Equal(t, 2, dbTask1.NumDependents)
	assert.Empty(t, dbTask1.DependsOn)
	dbTask2, err := task.FindOneId(mergeTask2.Id)
	assert.NoError(t, err)
	assert.Equal(t, 1, dbTask2.NumDependents)
	assert.Len(t, dbTask2.DependsOn, 1)
	assert.Equal(t, dbTask1.Id, dbTask2.DependsOn[0].TaskId)
	dbTask3, err := task.FindOneId(mergeTask3.Id)
	assert.NoError(t, err)
	assert.Equal(t, 0, dbTask3.NumDependents)
	assert.Len(t, dbTask3.DependsOn, 1)
	assert.Equal(t, dbTask2.Id, dbTask3.DependsOn[0].TaskId)
}
