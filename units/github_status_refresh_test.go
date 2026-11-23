package units

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
)

type githubStatusRefreshSuite struct {
	env      *mock.Environment
	patchDoc *patch.Patch

	suiteCtx context.Context
	cancel   context.CancelFunc
	ctx      context.Context

	suite.Suite
}

func TestGithubStatusRefresh(t *testing.T) {
	s := &githubStatusRefreshSuite{}
	s.suiteCtx, s.cancel = context.WithCancel(context.Background())
	s.suiteCtx = testutil.TestSpan(s.suiteCtx, t)

	suite.Run(t, s)
}

func (s *githubStatusRefreshSuite) TearDownSuite() {
	s.cancel()
}

func (s *githubStatusRefreshSuite) SetupTest() {
	s.ctx = testutil.TestSpan(s.suiteCtx, s.T())

	s.NoError(db.ClearCollections(patch.Collection, build.Collection, task.Collection, model.ProjectRefCollection, evergreen.ConfigCollection, patch.GitHubStatusResyncCollection))

	uiConfig := evergreen.UIConfig{}
	uiConfig.Url = "https://example.com"
	s.Require().NoError(uiConfig.Set(s.ctx))

	s.env = &mock.Environment{}
	s.Require().NoError(s.env.Configure(s.ctx))

	pRef := model.ProjectRef{
		Id:         "myChildProject",
		Identifier: "myChildProjectIdentifier",
	}
	s.NoError(pRef.Insert(s.ctx))

	startTime := time.Now().Truncate(time.Millisecond)
	id := mgobson.NewObjectId()
	s.patchDoc = &patch.Patch{
		Id:           id,
		Version:      id.Hex(),
		Activated:    true,
		DisplayNewUI: true,
		Status:       evergreen.VersionStarted,
		StartTime:    startTime,
		FinishTime:   startTime.Add(10 * time.Minute),
		GithubPatchData: thirdparty.GithubPatch{
			BaseOwner: "evergreen-ci",
			BaseRepo:  "evergreen",
			HeadOwner: "tychoish",
			HeadRepo:  "evergreen",
			PRNumber:  448,
			HeadHash:  "776f608b5b12cd27b8d931c8ee4ca0c13f857299",
		},
	}
	s.NoError(s.patchDoc.Insert(s.ctx))

}

func (s *githubStatusRefreshSuite) TestRunInDegradedMode() {
	flags := evergreen.ServiceFlags{
		GithubStatusAPIDisabled: true,
	}
	s.Require().NoError(evergreen.SetServiceFlags(s.ctx, flags))

	job, ok := NewGithubStatusRefreshJob(s.patchDoc).(*githubStatusRefreshJob)
	s.Require().NotNil(job)
	s.Require().True(ok)
	job.env = s.env
	job.Run(s.ctx)

	s.False(job.HasErrors())
}

func (s *githubStatusRefreshSuite) TestSendErrorIsReported() {
	job := NewGithubStatusRefreshJob(s.patchDoc).(*githubStatusRefreshJob)
	job.sender = &eventSendErrorSender{
		Sender: s.env.InternalSender,
		err:    errors.New("GitHub unavailable"),
	}
	job.sendStatus(s.ctx, &message.GithubStatus{
		Owner:   "evergreen-ci",
		Repo:    "evergreen",
		Ref:     "abc123",
		Context: "evergreen",
		State:   message.GithubStateSuccess,
	})

	s.EqualError(job.Error(), "GitHub unavailable")
}

func (s *githubStatusRefreshSuite) markGitHubStatusResync(contexts ...string) {
	s.markGitHubStatusResyncForPatch(s.patchDoc.Id.Hex(), contexts...)
}

func (s *githubStatusRefreshSuite) markGitHubStatusResyncForPatch(patchID string, contexts ...string) {
	failedAt := time.Now().Add(-time.Minute)
	s.Require().NoError(db.Insert(s.ctx, patch.GitHubStatusResyncCollection, patch.GitHubStatusResync{
		PatchID:      patchID,
		DirtySince:   failedAt,
		LastFailedAt: failedAt,
		Contexts:     contexts,
	}))
}

func (s *githubStatusRefreshSuite) runWithGitHubSender(p *patch.Patch, sender send.Sender) *githubStatusRefreshJob {
	job := NewGithubStatusRefreshJob(p).(*githubStatusRefreshJob)
	job.env = &eventSendEnvironment{Environment: s.env, githubSender: sender}
	job.Run(s.ctx)
	return job
}

func (s *githubStatusRefreshSuite) TestRunClearsGitHubStatusResyncOnSuccess() {
	s.markGitHubStatusResync()

	job := s.runWithGitHubSender(s.patchDoc, s.env.InternalSender)
	s.Require().NoError(job.Error())

	resync, err := patch.FindGitHubStatusResync(s.ctx, s.patchDoc.Id.Hex())
	s.Require().NoError(err)
	s.Nil(resync)
}

func (s *githubStatusRefreshSuite) TestRunFailureDoesNotClearGitHubStatusResync() {
	s.markGitHubStatusResync()
	errSender := &eventSendErrorSender{Sender: s.env.InternalSender, err: errors.New("GitHub unavailable")}

	job := s.runWithGitHubSender(s.patchDoc, errSender)
	s.Error(job.Error())

	resync, err := patch.FindGitHubStatusResync(s.ctx, s.patchDoc.Id.Hex())
	s.Require().NoError(err)
	s.NotNil(resync)
}

func (s *githubStatusRefreshSuite) runResync(sender send.Sender) *githubStatusRefreshJob {
	return s.runResyncForPatch(s.patchDoc.Id.Hex(), sender)
}

func (s *githubStatusRefreshSuite) runResyncForPatch(patchID string, sender send.Sender) *githubStatusRefreshJob {
	job, ok := NewGithubStatusResyncRefreshJob(patchID, time.Now()).(*githubStatusRefreshJob)
	s.Require().True(ok)
	job.env = &eventSendEnvironment{Environment: s.env, githubSender: sender}
	job.Run(s.ctx)
	return job
}

func (s *githubStatusRefreshSuite) enableResync() {
	s.Require().NoError(evergreen.SetServiceFlags(s.ctx, evergreen.ServiceFlags{GitHubStatusResyncEnabled: true}))
}

func (s *githubStatusRefreshSuite) setPatchStatus(status string) {
	s.Require().NoError(db.UpdateId(s.ctx, patch.Collection, s.patchDoc.Id, bson.M{"$set": bson.M{patch.StatusKey: status}}))
}

func (s *githubStatusRefreshSuite) insertFinishedBuild(variant string) {
	startTime := time.Now()
	b := build.Build{
		Id:           "b-" + variant,
		BuildVariant: variant,
		Version:      s.patchDoc.Version,
		Status:       evergreen.BuildSucceeded,
		Activated:    true,
		StartTime:    startTime,
		FinishTime:   startTime.Add(time.Minute),
	}
	s.Require().NoError(b.Insert(s.ctx))
}

func (s *githubStatusRefreshSuite) TestResyncSendsOnlyFailedFinishedStatusesAndClearsMark() {
	s.enableResync()
	s.setPatchStatus(evergreen.VersionSucceeded)
	s.insertFinishedBuild("done")
	s.insertFinishedBuild("sent")
	running := build.Build{Id: "b-running", BuildVariant: "running", Version: s.patchDoc.Version, Status: evergreen.BuildStarted, Activated: true}
	s.Require().NoError(running.Insert(s.ctx))
	otherProject := model.ProjectRef{Id: "otherChildProject", Identifier: "otherChildProjectIdentifier"}
	s.Require().NoError(otherProject.Insert(s.ctx))
	s.insertChildPatch(evergreen.VersionSucceeded)
	otherChild := s.insertChildPatch(evergreen.VersionSucceeded)
	s.Require().NoError(db.UpdateId(s.ctx, patch.Collection, otherChild.Id, bson.M{"$set": bson.M{patch.ProjectKey: otherProject.Id}}))
	s.Require().NoError(s.patchDoc.SetChildPatches(s.ctx))
	s.markGitHubStatusResync("evergreen/myChildProjectIdentifier", "evergreen/running", "evergreen/done")

	job := s.runResync(s.env.InternalSender)
	s.Require().NoError(job.Error())

	status := s.getAndValidateStatus(s.env.InternalSender)
	s.Equal("evergreen/myChildProjectIdentifier", status.Context)
	s.Equal(message.GithubStateSuccess, status.State)
	status = s.getAndValidateStatus(s.env.InternalSender)
	s.Equal("evergreen/done", status.Context)
	s.Equal(message.GithubStateSuccess, status.State)
	_, ok := s.env.InternalSender.GetMessageSafe()
	s.False(ok, "pending statuses and statuses that didn't fail should not be resent")

	resync, err := patch.FindGitHubStatusResync(s.ctx, s.patchDoc.Id.Hex())
	s.Require().NoError(err)
	s.Nil(resync)
}

func (s *githubStatusRefreshSuite) TestResyncSendsFailedPatchStatus() {
	s.enableResync()
	s.setPatchStatus(evergreen.VersionFailed)
	s.markGitHubStatusResync("evergreen")

	job := s.runResync(s.env.InternalSender)
	s.Require().NoError(job.Error())

	status := s.getAndValidateStatus(s.env.InternalSender)
	s.Equal("evergreen", status.Context)
	s.Equal(message.GithubStateFailure, status.State)
	_, ok := s.env.InternalSender.GetMessageSafe()
	s.False(ok)
}

func (s *githubStatusRefreshSuite) TestResyncChildPatchErrorStillSendsBuildStatusesAndKeepsMark() {
	s.enableResync()
	s.insertFinishedBuild("done")
	child := s.insertChildPatch(evergreen.VersionSucceeded)
	s.Require().NoError(db.UpdateId(s.ctx, patch.Collection, child.Id, bson.M{"$set": bson.M{patch.ProjectKey: "deleted-project"}}))
	s.Require().NoError(s.patchDoc.SetChildPatches(s.ctx))
	s.markGitHubStatusResync("evergreen/done")

	job := s.runResync(s.env.InternalSender)
	s.ErrorContains(job.Error(), "project 'deleted-project' does not exist")

	status := s.getAndValidateStatus(s.env.InternalSender)
	s.Equal("evergreen/done", status.Context)
	resync, err := patch.FindGitHubStatusResync(s.ctx, s.patchDoc.Id.Hex())
	s.Require().NoError(err)
	s.Require().NotNil(resync)
	s.False(resync.LastAttemptedAt.IsZero(), "a failed resync should move the patch behind others that haven't failed")
}

func (s *githubStatusRefreshSuite) TestResyncClearsMarkWhenGitHubRejectsStatus() {
	s.enableResync()
	s.insertFinishedBuild("done")
	s.markGitHubStatusResync("evergreen/done")
	errSender := &eventSendErrorSender{
		Sender: s.env.InternalSender,
		err:    &send.GitHubSendError{StatusCode: http.StatusUnprocessableEntity, Attempts: 1, Err: errors.New("invalid status")},
	}

	job := s.runResync(errSender)
	s.Require().NoError(job.Error())

	s.Equal(1, errSender.sends)
	resync, err := patch.FindGitHubStatusResync(s.ctx, s.patchDoc.Id.Hex())
	s.Require().NoError(err)
	s.Nil(resync)
}

func (s *githubStatusRefreshSuite) TestResyncClearsMarkWithoutSendingWhenStatusesCannotOrNeedNotBeResent() {
	s.enableResync()
	s.insertFinishedBuild("done")

	for tName, setup := range map[string]func() string{
		"PRHasNewerPatch": func() string {
			newer := patch.Patch{
				Id:              mgobson.NewObjectId(),
				CreateTime:      time.Now(),
				GithubPatchData: s.patchDoc.GithubPatchData,
			}
			newer.GithubPatchData.HeadHash = "0d4f6f2c1b8e9a7d6c5b4a3f2e1d0c9b8a7f6e5d"
			s.Require().NoError(newer.Insert(s.ctx))
			return s.patchDoc.Id.Hex()
		},
		"PatchNotFound": func() string {
			return mgobson.NewObjectId().Hex()
		},
		"PatchHasNoGitHubTarget": func() string {
			p := patch.Patch{Id: mgobson.NewObjectId()}
			s.Require().NoError(p.Insert(s.ctx))
			return p.Id.Hex()
		},
	} {
		s.Run(tName, func() {
			s.Require().NoError(db.ClearCollections(patch.GitHubStatusResyncCollection))
			s.Require().NoError(db.RemoveAll(s.ctx, patch.Collection, bson.M{"_id": bson.M{"$ne": s.patchDoc.Id}}))
			patchID := setup()
			s.markGitHubStatusResyncForPatch(patchID, "evergreen/done")

			job := s.runResyncForPatch(patchID, s.env.InternalSender)
			s.Require().NoError(job.Error())

			_, ok := s.env.InternalSender.GetMessageSafe()
			s.False(ok)
			resync, err := patch.FindGitHubStatusResync(s.ctx, patchID)
			s.Require().NoError(err)
			s.Nil(resync)
		})
	}
}

func (s *githubStatusRefreshSuite) TestResyncWithoutMarkSendsNothing() {
	s.enableResync()
	s.insertFinishedBuild("done")

	job := s.runResync(s.env.InternalSender)
	s.Require().NoError(job.Error())

	_, ok := s.env.InternalSender.GetMessageSafe()
	s.False(ok)
}

func (s *githubStatusRefreshSuite) TestResyncDoesNothingWhenResyncIsDisabled() {
	s.insertFinishedBuild("done")
	s.markGitHubStatusResync("evergreen/done")

	job := s.runResync(s.env.InternalSender)
	s.Require().NoError(job.Error())

	_, ok := s.env.InternalSender.GetMessageSafe()
	s.False(ok)
	resync, err := patch.FindGitHubStatusResync(s.ctx, s.patchDoc.Id.Hex())
	s.Require().NoError(err)
	s.NotNil(resync)
}

func (s *githubStatusRefreshSuite) TestResyncJobsForTheSamePatchShareAnEnqueueScope() {
	ts := time.Now()
	first := NewGithubStatusResyncRefreshJob("patch", ts)
	sameRun := NewGithubStatusResyncRefreshJob("patch", ts)
	later := NewGithubStatusResyncRefreshJob("patch", ts.Add(5*time.Minute))
	other := NewGithubStatusResyncRefreshJob("other-patch", ts)

	s.Equal(first.ID(), sameRun.ID())
	s.NotEqual(first.ID(), later.ID())
	s.True(first.EnqueueAllScopes())
	s.Equal(first.Scopes(), later.Scopes())
	s.NotEqual(first.Scopes(), other.Scopes())
}

func (s *githubStatusRefreshSuite) TestRunStopsSendingAtGitHubRateLimit() {
	for _, id := range []string{"b1", "b2"} {
		b := build.Build{
			Id:           id,
			BuildVariant: id,
			Version:      s.patchDoc.Version,
			Status:       evergreen.BuildStarted,
			Activated:    true,
		}
		s.Require().NoError(b.Insert(s.ctx))
	}
	s.markGitHubStatusResync()
	errSender := &eventSendErrorSender{
		Sender: s.env.InternalSender,
		err:    &send.GitHubSendError{StatusCode: http.StatusTooManyRequests, Attempts: 1, Err: errors.New("rate limit exceeded")},
	}

	job := s.runWithGitHubSender(s.patchDoc, errSender)
	s.Error(job.Error())

	s.Equal(1, errSender.sends)
	resync, err := patch.FindGitHubStatusResync(s.ctx, s.patchDoc.Id.Hex())
	s.Require().NoError(err)
	s.NotNil(resync)
}

func (s *githubStatusRefreshSuite) TestFetch() {
	b := build.Build{
		Id:      "b1",
		Version: s.patchDoc.Version,
		Status:  evergreen.BuildStarted,
	}
	s.NoError(b.Insert(s.ctx))
	childPatch := patch.Patch{
		Id: mgobson.NewObjectId(),
	}
	s.NoError(childPatch.Insert(s.ctx))
	s.patchDoc.Triggers.ChildPatches = []string{childPatch.Id.Hex()}
	s.NoError(s.patchDoc.SetChildPatches(s.ctx))

	job, ok := NewGithubStatusRefreshJob(s.patchDoc).(*githubStatusRefreshJob)
	s.Require().NotNil(job)
	s.Require().True(ok)
	s.Require().NotNil(job.patch)
	job.env = s.env

	s.NoError(job.fetch(s.ctx))
	s.NotEmpty(job.urlBase)
	s.Len(job.builds, 1)
	s.Len(job.childPatches, 1)
}

func (s *githubStatusRefreshSuite) TestStatusPending() {
	tsk := task.Task{
		Id:           "t1",
		BuildId:      "b1",
		BuildVariant: "myBuild",
		Version:      s.patchDoc.Version,
		Activated:    true,
		Status:       evergreen.TaskStarted,
	}
	s.NoError(tsk.Insert(s.ctx))

	b := build.Build{
		Id:           "b1",
		BuildVariant: "myBuild",
		Version:      s.patchDoc.Version,
		Status:       evergreen.BuildStarted,
	}
	s.NoError(b.Insert(s.ctx))

	childPatch := patch.Patch{
		Id:        mgobson.NewObjectId(),
		Status:    evergreen.VersionStarted,
		Project:   "myChildProject",
		Activated: true,
		Triggers: patch.TriggerInfo{
			ParentPatch: s.patchDoc.Id.Hex(),
		},
		DisplayNewUI: true,
	}
	s.NoError(childPatch.Insert(s.ctx))
	s.patchDoc.Triggers.ChildPatches = []string{childPatch.Id.Hex()}

	job, ok := NewGithubStatusRefreshJob(s.patchDoc).(*githubStatusRefreshJob)
	s.Require().NotNil(job)
	s.Require().True(ok)
	s.Require().NotNil(job.patch)
	job.env = s.env
	job.Run(s.ctx)
	s.False(job.HasErrors())

	status := s.getAndValidateStatus(s.env.InternalSender)
	// Patch status
	s.Equal(fmt.Sprintf("https://example.com/version/%s", s.patchDoc.Version), status.URL)
	s.Equal("evergreen", status.Context)
	s.Equal(message.GithubStatePending, status.State)
	s.Equal("tasks are running", status.Description)

	// Child patch status
	status = s.getAndValidateStatus(s.env.InternalSender)
	s.Equal(fmt.Sprintf("https://example.com/version/%s/downstream-projects", childPatch.Id.Hex()), status.URL)
	s.Equal("evergreen/myChildProjectIdentifier", status.Context)
	s.Equal(message.GithubStatePending, status.State)
	s.Equal("tasks are running", status.Description)

	// Build status
	status = s.getAndValidateStatus(s.env.InternalSender)
	s.Equal(fmt.Sprintf("https://example.com/build/%s", b.Id), status.URL)
	s.Equal("evergreen/myBuild", status.Context)
	s.Equal(message.GithubStatePending, status.State)
	s.Equal("tasks are running", status.Description)
}

func (s *githubStatusRefreshSuite) TestStatusPendingDueToEssentialTaskThatWillRun() {
	tsk := task.Task{
		Id:                   "t1",
		BuildId:              "b1",
		BuildVariant:         "myBuild",
		Version:              s.patchDoc.Version,
		Activated:            true,
		Status:               evergreen.TaskStarted,
		IsEssentialToSucceed: true,
	}
	s.NoError(tsk.Insert(s.ctx))

	b := build.Build{
		Id:           "b1",
		BuildVariant: "myBuild",
		Version:      s.patchDoc.Version,
		Status:       evergreen.BuildStarted,
	}
	s.NoError(b.Insert(s.ctx))

	job, ok := NewGithubStatusRefreshJob(s.patchDoc).(*githubStatusRefreshJob)
	s.Require().NotNil(job)
	s.Require().True(ok)
	s.Require().NotNil(job.patch)
	job.env = s.env
	job.Run(s.ctx)
	s.False(job.HasErrors())

	status := s.getAndValidateStatus(s.env.InternalSender)
	// Patch status
	s.Equal(fmt.Sprintf("https://example.com/version/%s", s.patchDoc.Version), status.URL)
	s.Equal("evergreen", status.Context)
	s.Equal(message.GithubStatePending, status.State)
	s.Equal("tasks are running", status.Description)

	// Build status
	status = s.getAndValidateStatus(s.env.InternalSender)
	s.Equal(fmt.Sprintf("https://example.com/build/%s", b.Id), status.URL)
	s.Equal("evergreen/myBuild", status.Context)
	s.Equal(message.GithubStatePending, status.State)
	s.Equal("tasks are running", status.Description)
}

func (s *githubStatusRefreshSuite) TestStatusPendingDueToAllUnscheduledEssentialTasks() {
	tsk := task.Task{
		Id:                   "t1",
		BuildId:              "b1",
		BuildVariant:         "myBuild",
		Version:              s.patchDoc.Version,
		Activated:            false,
		Status:               evergreen.TaskUndispatched,
		IsEssentialToSucceed: true,
	}
	s.NoError(tsk.Insert(s.ctx))

	b := build.Build{
		Id:           "b1",
		BuildVariant: "myBuild",
		Version:      s.patchDoc.Version,
		Status:       evergreen.BuildStarted,
	}
	s.NoError(b.Insert(s.ctx))

	job, ok := NewGithubStatusRefreshJob(s.patchDoc).(*githubStatusRefreshJob)
	s.Require().NotNil(job)
	s.Require().True(ok)
	s.Require().NotNil(job.patch)
	job.env = s.env
	job.Run(s.ctx)
	s.False(job.HasErrors())

	// Patch status
	status := s.getAndValidateStatus(s.env.InternalSender)
	s.Equal(fmt.Sprintf("https://example.com/version/%s", s.patchDoc.Version), status.URL)
	s.Equal("evergreen", status.Context)
	s.Equal(message.GithubStatePending, status.State)
	s.Equal("tasks are running", status.Description)

	// Build status
	status = s.getAndValidateStatus(s.env.InternalSender)
	s.Equal(fmt.Sprintf("https://example.com/build/%s", b.Id), status.URL)
	s.Equal("evergreen/myBuild", status.Context)
	s.Equal(message.GithubStatePending, status.State)
	s.Equal("1 essential task(s) not scheduled", status.Description)
}

func (s *githubStatusRefreshSuite) TestStatusFailedDueToMixOfFailedAndUnscheduledEssentialTasks() {
	failedTask := task.Task{
		Id:                   "t1",
		BuildId:              "b1",
		BuildVariant:         "myBuild",
		Version:              s.patchDoc.Version,
		Activated:            true,
		Status:               evergreen.TaskFailed,
		IsEssentialToSucceed: true,
	}
	s.NoError(failedTask.Insert(s.ctx))
	unscheduledEssentialTask := task.Task{
		Id:                   "t2",
		BuildId:              "b1",
		BuildVariant:         "myBuild",
		Version:              s.patchDoc.Version,
		Activated:            false,
		Status:               evergreen.TaskUndispatched,
		IsEssentialToSucceed: true,
	}
	s.NoError(unscheduledEssentialTask.Insert(s.ctx))

	startTime := time.Now()
	b := build.Build{
		Id:           "b1",
		BuildVariant: "myBuild",
		StartTime:    startTime,
		FinishTime:   startTime.Add(time.Minute),
		Version:      s.patchDoc.Version,
		Status:       evergreen.BuildFailed,
	}
	s.NoError(b.Insert(s.ctx))

	job, ok := NewGithubStatusRefreshJob(s.patchDoc).(*githubStatusRefreshJob)
	s.Require().NotNil(job)
	s.Require().True(ok)
	s.Require().NotNil(job.patch)
	job.env = s.env
	job.Run(s.ctx)
	s.False(job.HasErrors())

	// Patch status
	status := s.getAndValidateStatus(s.env.InternalSender)
	s.Equal(fmt.Sprintf("https://example.com/version/%s", s.patchDoc.Version), status.URL)
	s.Equal("evergreen", status.Context)
	s.Equal(message.GithubStatePending, status.State)
	s.Equal("tasks are running", status.Description)

	// Build status
	status = s.getAndValidateStatus(s.env.InternalSender)
	s.Equal(fmt.Sprintf("https://example.com/build/%s", b.Id), status.URL)
	s.Equal("evergreen/myBuild", status.Context)
	s.Equal(message.GithubStateFailure, status.State)
	s.Equal("none succeeded, 1 failed, 1 essential task(s) not scheduled in 1m0s", status.Description)
}

func (s *githubStatusRefreshSuite) TestStatusSucceeded() {
	startTime := time.Now()
	b := build.Build{
		Id:           "b1",
		BuildVariant: "myBuild",
		Version:      s.patchDoc.Version,
		Status:       evergreen.BuildSucceeded,
		StartTime:    startTime,
		FinishTime:   startTime.Add(time.Minute),
	}
	s.NoError(b.Insert(s.ctx))
	t1 := task.Task{
		Id:      "t1",
		Version: s.patchDoc.Version,
		BuildId: b.Id,
		Status:  evergreen.TaskSucceeded,
	}
	s.NoError(t1.Insert(s.ctx))

	childPatch := s.insertChildPatch(evergreen.VersionSucceeded)
	s.patchDoc.Status = evergreen.VersionSucceeded

	job, ok := NewGithubStatusRefreshJob(s.patchDoc).(*githubStatusRefreshJob)
	s.Require().NotNil(job)
	s.Require().True(ok)
	s.Require().NotNil(job.patch)

	job.env = s.env
	job.Run(s.ctx)
	s.NoError(job.Error())

	// Patch status
	status := s.getAndValidateStatus(s.env.InternalSender)
	s.Equal(fmt.Sprintf("https://example.com/version/%s", s.patchDoc.Version), status.URL)
	s.Equal("evergreen", status.Context)
	s.Equal(message.GithubStateSuccess, status.State)
	s.Equal("patch finished in 12m0s", status.Description)

	// Child patch status
	status = s.getAndValidateStatus(s.env.InternalSender)
	s.Equal(fmt.Sprintf("https://example.com/version/%s/downstream-projects", childPatch.Id.Hex()), status.URL)
	s.Equal("evergreen/myChildProjectIdentifier", status.Context)
	s.Equal(message.GithubStateSuccess, status.State)
	s.Equal("patch finished in 11m0s", status.Description)

	// Build status
	status = s.getAndValidateStatus(s.env.InternalSender)
	s.Equal(fmt.Sprintf("https://example.com/build/%s", b.Id), status.URL)
	s.Equal("evergreen/myBuild", status.Context)
	s.Equal("1 succeeded, none failed in 1m0s", status.Description)
	s.Equal(message.GithubStateSuccess, status.State)
}

func (s *githubStatusRefreshSuite) TestStatusFailed() {
	startTime := time.Now()
	b := build.Build{
		Id:           "b1",
		BuildVariant: "myBuild",
		Version:      s.patchDoc.Version,
		Status:       evergreen.BuildFailed,
		StartTime:    startTime,
		FinishTime:   startTime.Add(time.Minute),
	}
	s.NoError(b.Insert(s.ctx))
	t1 := task.Task{
		Id:      "t1",
		Version: s.patchDoc.Version,
		BuildId: b.Id,
		Status:  evergreen.TaskFailed,
	}
	s.NoError(t1.Insert(s.ctx))

	childPatch := s.insertChildPatch(evergreen.VersionFailed)
	s.patchDoc.Status = evergreen.VersionFailed

	job, ok := NewGithubStatusRefreshJob(s.patchDoc).(*githubStatusRefreshJob)
	s.Require().NotNil(job)
	s.Require().True(ok)
	s.Require().NotNil(job.patch)

	job.env = s.env
	job.Run(s.ctx)
	s.False(job.HasErrors())

	// Patch status
	status := s.getAndValidateStatus(s.env.InternalSender)
	s.Equal(fmt.Sprintf("https://example.com/version/%s", s.patchDoc.Version), status.URL)
	s.Equal("evergreen", status.Context)
	s.Equal(message.GithubStateFailure, status.State)
	s.Equal("patch finished in 12m0s", status.Description)

	// Child patch status
	status = s.getAndValidateStatus(s.env.InternalSender)
	s.Equal(fmt.Sprintf("https://example.com/version/%s/downstream-projects", childPatch.Id.Hex()), status.URL)
	s.Equal("evergreen/myChildProjectIdentifier", status.Context)
	s.Equal(message.GithubStateFailure, status.State)
	s.Equal("patch finished in 11m0s", status.Description)

	// Build status
	status = s.getAndValidateStatus(s.env.InternalSender)
	s.Equal(fmt.Sprintf("https://example.com/build/%s", b.Id), status.URL)
	s.Equal("evergreen/myBuild", status.Context)
	s.Equal("none succeeded, 1 failed in 1m0s", status.Description)
	s.Equal(message.GithubStateFailure, status.State)
}

func (s *githubStatusRefreshSuite) TestSkipsDormantBuilds() {
	startTime := time.Now()
	dormantBuild := build.Build{
		Id:           "b-dormant",
		BuildVariant: "myDormantBuild",
		Version:      s.patchDoc.Version,
		Status:       evergreen.BuildCreated,
		Activated:    false,
	}
	s.NoError(dormantBuild.Insert(s.ctx))
	activeBuild := build.Build{
		Id:           "b-active",
		BuildVariant: "myBuild",
		Version:      s.patchDoc.Version,
		Status:       evergreen.BuildSucceeded,
		StartTime:    startTime,
		FinishTime:   startTime.Add(time.Minute),
	}
	s.NoError(activeBuild.Insert(s.ctx))
	tsk := task.Task{
		Id:      "t1",
		Version: s.patchDoc.Version,
		BuildId: activeBuild.Id,
		Status:  evergreen.TaskSucceeded,
	}
	s.NoError(tsk.Insert(s.ctx))
	s.patchDoc.Status = evergreen.VersionSucceeded

	job, ok := NewGithubStatusRefreshJob(s.patchDoc).(*githubStatusRefreshJob)
	s.Require().NotNil(job)
	s.Require().True(ok)
	s.Require().NotNil(job.patch)
	job.env = s.env
	job.Run(s.ctx)
	s.NoError(job.Error())

	// Patch status
	status := s.getAndValidateStatus(s.env.InternalSender)
	s.Equal("evergreen", status.Context)
	s.Equal(message.GithubStateSuccess, status.State)

	// Build status for the active build; the dormant build sends nothing.
	status = s.getAndValidateStatus(s.env.InternalSender)
	s.Equal(fmt.Sprintf("https://example.com/build/%s", activeBuild.Id), status.URL)
	s.Equal("evergreen/myBuild", status.Context)
	s.Equal(message.GithubStateSuccess, status.State)

	_, ok = s.env.InternalSender.GetMessageSafe()
	s.False(ok, "no status should be sent for a dormant build")
}

func (s *githubStatusRefreshSuite) TestStatusPendingForDeactivatedAfterRunningBuild() {
	// A build that was activated may have already reported a status, so it gets
	// an update rather than being skipped, even after being deactivated.
	b := build.Build{
		Id:            "b1",
		BuildVariant:  "myBuild",
		Version:       s.patchDoc.Version,
		Status:        evergreen.BuildCreated,
		ActivatedTime: time.Now(),
	}
	s.NoError(b.Insert(s.ctx))

	job, ok := NewGithubStatusRefreshJob(s.patchDoc).(*githubStatusRefreshJob)
	s.Require().True(ok)
	job.env = s.env
	job.Run(s.ctx)
	s.False(job.HasErrors())

	s.getAndValidateStatus(s.env.InternalSender) // patch status
	status := s.getAndValidateStatus(s.env.InternalSender)
	s.Equal("evergreen/myBuild", status.Context)
	s.Equal(message.GithubStatePending, status.State)
}

func (s *githubStatusRefreshSuite) TestStatusPendingForCreatedButActivatedBuild() {
	b := build.Build{
		Id:           "b1",
		BuildVariant: "myBuild",
		Version:      s.patchDoc.Version,
		Status:       evergreen.BuildCreated,
		Activated:    true,
	}
	s.NoError(b.Insert(s.ctx))

	job, ok := NewGithubStatusRefreshJob(s.patchDoc).(*githubStatusRefreshJob)
	s.Require().NotNil(job)
	s.Require().True(ok)
	s.Require().NotNil(job.patch)
	job.env = s.env
	job.Run(s.ctx)
	s.False(job.HasErrors())

	// Patch status
	status := s.getAndValidateStatus(s.env.InternalSender)
	s.Equal("evergreen", status.Context)
	s.Equal(message.GithubStatePending, status.State)

	// Build status: scheduled but not yet started is still pending.
	status = s.getAndValidateStatus(s.env.InternalSender)
	s.Equal("evergreen/myBuild", status.Context)
	s.Equal(message.GithubStatePending, status.State)
}

func (s *githubStatusRefreshSuite) insertChildPatch(status string) patch.Patch {
	childPatch := patch.Patch{
		Id:         mgobson.NewObjectId(),
		Status:     status,
		Project:    "myChildProject",
		Activated:  true,
		StartTime:  s.patchDoc.StartTime.Add(time.Minute),
		FinishTime: s.patchDoc.StartTime.Add(12 * time.Minute),
		Triggers: patch.TriggerInfo{
			ParentPatch: s.patchDoc.Id.Hex(),
		},
		DisplayNewUI: true,
	}
	s.Require().NoError(childPatch.Insert(s.ctx))
	s.patchDoc.Triggers.ChildPatches = append(s.patchDoc.Triggers.ChildPatches, childPatch.Id.Hex())
	return childPatch
}

func (s *githubStatusRefreshSuite) runRefreshJob() {
	job, ok := NewGithubStatusRefreshJob(s.patchDoc).(*githubStatusRefreshJob)
	s.Require().True(ok)
	job.env = s.env
	job.Run(s.ctx)
	s.Require().NoError(job.Error())
}

func (s *githubStatusRefreshSuite) TestParentStatusFailsWhenChildPatchFails() {
	s.patchDoc.Status = evergreen.VersionSucceeded
	s.insertChildPatch(evergreen.VersionFailed)

	s.runRefreshJob()

	status := s.getAndValidateStatus(s.env.InternalSender)
	s.Equal("evergreen", status.Context)
	s.Equal(message.GithubStateFailure, status.State)
	s.Equal("patch finished in 12m0s", status.Description)

	status = s.getAndValidateStatus(s.env.InternalSender)
	s.Equal("evergreen/myChildProjectIdentifier", status.Context)
	s.Equal(message.GithubStateFailure, status.State)
	s.Equal("patch finished in 11m0s", status.Description)
}

func (s *githubStatusRefreshSuite) TestParentStatusPendingWhenChildPatchIsRunning() {
	s.patchDoc.Status = evergreen.VersionSucceeded
	s.insertChildPatch(evergreen.VersionStarted)

	s.runRefreshJob()

	status := s.getAndValidateStatus(s.env.InternalSender)
	s.Equal("evergreen", status.Context)
	s.Equal(message.GithubStatePending, status.State)
	s.Equal("tasks are running", status.Description)

	status = s.getAndValidateStatus(s.env.InternalSender)
	s.Equal("evergreen/myChildProjectIdentifier", status.Context)
	s.Equal(message.GithubStatePending, status.State)
	s.Equal("tasks are running", status.Description)
}

func (s *githubStatusRefreshSuite) TestPatchWithoutChildrenUsesOwnStatus() {
	s.patchDoc.Status = evergreen.VersionFailed

	s.runRefreshJob()

	status := s.getAndValidateStatus(s.env.InternalSender)
	s.Equal("evergreen", status.Context)
	s.Equal(message.GithubStateFailure, status.State)
	s.Equal("patch finished in 10m0s", status.Description)

	_, ok := s.env.InternalSender.GetMessageSafe()
	s.False(ok)
}

func (s *githubStatusRefreshSuite) TestParentStatusPendingWhenChildPatchHasNotStarted() {
	s.patchDoc.Status = evergreen.VersionSucceeded
	s.insertChildPatch(evergreen.VersionCreated)

	s.runRefreshJob()

	status := s.getAndValidateStatus(s.env.InternalSender)
	s.Equal("evergreen", status.Context)
	s.Equal(message.GithubStatePending, status.State)
	s.Equal("tasks are running", status.Description)
}

func (s *githubStatusRefreshSuite) TestMissingChildPatchSkipsParentStatusAndSendsOtherStatuses() {
	b := build.Build{
		Id:           "b1",
		BuildVariant: "myBuild",
		Version:      s.patchDoc.Version,
		Status:       evergreen.BuildStarted,
	}
	s.NoError(b.Insert(s.ctx))
	s.patchDoc.Status = evergreen.VersionSucceeded
	s.insertChildPatch(evergreen.VersionSucceeded)
	missingChildID := mgobson.NewObjectId().Hex()
	s.patchDoc.Triggers.ChildPatches = append(s.patchDoc.Triggers.ChildPatches, missingChildID)

	job, ok := NewGithubStatusRefreshJob(s.patchDoc).(*githubStatusRefreshJob)
	s.Require().True(ok)
	job.env = s.env
	job.Run(s.ctx)
	s.ErrorContains(job.Error(), fmt.Sprintf("child patch '%s' not found", missingChildID))

	status := s.getAndValidateStatus(s.env.InternalSender)
	s.Equal("evergreen/myChildProjectIdentifier", status.Context)

	status = s.getAndValidateStatus(s.env.InternalSender)
	s.Equal("evergreen/myBuild", status.Context)

	_, ok = s.env.InternalSender.GetMessageSafe()
	s.False(ok)
}

func (s *githubStatusRefreshSuite) TestMergeQueuePatchSendsStatusesToMergeGroup() {
	s.patchDoc.GithubPatchData = thirdparty.GithubPatch{}
	s.patchDoc.GithubMergeData = thirdparty.GithubMergeGroup{
		Org:     "mongodb",
		Repo:    "mongo",
		HeadSHA: "a7e4d1c9b2f3e8a6d5c4b3a2f1e0d9c8b7a6f5e4",
	}

	startTime := time.Now()
	b := build.Build{
		Id:           "b1",
		BuildVariant: "myBuild",
		Version:      s.patchDoc.Version,
		Status:       evergreen.BuildSucceeded,
		StartTime:    startTime,
		FinishTime:   startTime.Add(time.Minute),
	}
	s.NoError(b.Insert(s.ctx))
	t1 := task.Task{
		Id:      "t1",
		Version: s.patchDoc.Version,
		BuildId: b.Id,
		Status:  evergreen.TaskSucceeded,
	}
	s.NoError(t1.Insert(s.ctx))

	s.insertChildPatch(evergreen.VersionSucceeded)
	s.patchDoc.Status = evergreen.VersionSucceeded

	s.runRefreshJob()

	// Patch status
	status := s.getStatus(s.env.InternalSender)
	s.validateMergeGroupTarget(status)
	s.Equal("evergreen", status.Context)

	// Child patch status
	status = s.getStatus(s.env.InternalSender)
	s.validateMergeGroupTarget(status)
	s.Equal("evergreen/myChildProjectIdentifier", status.Context)

	// Build status
	status = s.getStatus(s.env.InternalSender)
	s.validateMergeGroupTarget(status)
	s.Equal("evergreen/myBuild", status.Context)
}

func (s *githubStatusRefreshSuite) TestPatchWithoutGitHubTargetErrors() {
	s.patchDoc.GithubPatchData = thirdparty.GithubPatch{}

	job, ok := NewGithubStatusRefreshJob(s.patchDoc).(*githubStatusRefreshJob)
	s.Require().True(ok)
	job.env = s.env
	job.Run(s.ctx)
	s.ErrorContains(job.Error(), "has no GitHub owner, repo, and ref to send statuses to")

	_, ok = s.env.InternalSender.GetMessageSafe()
	s.False(ok, "no status should be sent without a GitHub target")
}

func (s *githubStatusRefreshSuite) TestMergeQueuePatchWithIncompleteMergeDataErrors() {
	for name, mergeData := range map[string]thirdparty.GithubMergeGroup{
		"EmptyMergeData":    {},
		"HeadSHAWithoutOrg": {Repo: "mongo", HeadSHA: "a7e4d1c9b2f3e8a6d5c4b3a2f1e0d9c8b7a6f5e4"},
	} {
		s.Run(name, func() {
			s.patchDoc.Alias = evergreen.CommitQueueAlias
			s.patchDoc.GithubMergeData = mergeData

			job, ok := NewGithubStatusRefreshJob(s.patchDoc).(*githubStatusRefreshJob)
			s.Require().True(ok)
			job.env = s.env
			job.Run(s.ctx)
			s.ErrorContains(job.Error(), "has no GitHub owner, repo, and ref to send statuses to")

			_, ok = s.env.InternalSender.GetMessageSafe()
			s.False(ok, "no status should be sent without a merge group target")
		})
	}
}

func (s *githubStatusRefreshSuite) validateMergeGroupTarget(status *message.GithubStatus) {
	s.Equal("mongodb", status.Owner)
	s.Equal("mongo", status.Repo)
	s.Equal("a7e4d1c9b2f3e8a6d5c4b3a2f1e0d9c8b7a6f5e4", status.Ref)
}

func (s *githubStatusRefreshSuite) getStatus(sender *send.InternalSender) *message.GithubStatus {
	msg, ok := sender.GetMessageSafe()
	s.Require().True(ok)
	raw := msg.Message
	s.Require().NotNil(raw)
	status, ok := raw.Raw().(*message.GithubStatus)
	s.Require().True(ok)
	return status
}

func (s *githubStatusRefreshSuite) getAndValidateStatus(sender *send.InternalSender) *message.GithubStatus {
	status := s.getStatus(sender)
	s.Equal("evergreen-ci", status.Owner)
	s.Equal("evergreen", status.Repo)
	s.Equal("776f608b5b12cd27b8d931c8ee4ca0c13f857299", status.Ref)
	return status
}
