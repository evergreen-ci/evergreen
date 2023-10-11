package units

import (
	"context"
	"fmt"
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

	s.NoError(db.ClearCollections(patch.Collection, build.Collection, task.Collection, model.ProjectRefCollection, evergreen.ConfigCollection))

	uiConfig := evergreen.UIConfig{}
	uiConfig.Url = "https://example.com"
	s.Require().NoError(uiConfig.Set(s.ctx))

	s.env = &mock.Environment{}
	s.Require().NoError(s.env.Configure(s.ctx))

	pRef := model.ProjectRef{
		Id:         "myChildProject",
		Identifier: "myChildProjectIdentifier",
	}
	s.NoError(pRef.Insert())

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
	s.NoError(s.patchDoc.Insert())

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

func (s *githubStatusRefreshSuite) TestFetch() {
	b := build.Build{
		Id:      "b1",
		Version: s.patchDoc.Version,
		Status:  evergreen.BuildStarted,
	}
	s.NoError(b.Insert())
	childPatch := patch.Patch{
		Id: mgobson.NewObjectId(),
	}
	s.NoError(childPatch.Insert())
	s.patchDoc.Triggers.ChildPatches = []string{childPatch.Id.Hex()}
	s.NoError(s.patchDoc.SetChildPatches())

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
	s.NoError(tsk.Insert())

	b := build.Build{
		Id:           "b1",
		BuildVariant: "myBuild",
		Version:      s.patchDoc.Version,
		Status:       evergreen.BuildStarted,
	}
	s.NoError(b.Insert())

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
	s.NoError(childPatch.Insert())
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
	s.Equal(fmt.Sprintf("https://example.com/version/%s?redirect_spruce_users=true", s.patchDoc.Version), status.URL)
	s.Equal("evergreen", status.Context)
	s.Equal(message.GithubStatePending, status.State)
	s.Equal("tasks are running", status.Description)

	// Child patch status
	status = s.getAndValidateStatus(s.env.InternalSender)
	s.Equal(fmt.Sprintf("https://example.com/version/%s/downstream-projects?redirect_spruce_users=true", childPatch.Id.Hex()), status.URL)
	s.Equal("evergreen/myChildProjectIdentifier", status.Context)
	s.Equal(message.GithubStatePending, status.State)
	s.Equal("tasks are running", status.Description)

	// Build status
	status = s.getAndValidateStatus(s.env.InternalSender)
	s.Equal(fmt.Sprintf("https://example.com/build/%s?redirect_spruce_users=true", b.Id), status.URL)
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
	s.NoError(tsk.Insert())

	b := build.Build{
		Id:           "b1",
		BuildVariant: "myBuild",
		Version:      s.patchDoc.Version,
		Status:       evergreen.BuildStarted,
	}
	s.NoError(b.Insert())

	job, ok := NewGithubStatusRefreshJob(s.patchDoc).(*githubStatusRefreshJob)
	s.Require().NotNil(job)
	s.Require().True(ok)
	s.Require().NotNil(job.patch)
	job.env = s.env
	job.Run(s.ctx)
	s.False(job.HasErrors())

	status := s.getAndValidateStatus(s.env.InternalSender)
	// Patch status
	s.Equal(fmt.Sprintf("https://example.com/version/%s?redirect_spruce_users=true", s.patchDoc.Version), status.URL)
	s.Equal("evergreen", status.Context)
	s.Equal(message.GithubStatePending, status.State)
	s.Equal("tasks are running", status.Description)

	// Build status
	status = s.getAndValidateStatus(s.env.InternalSender)
	s.Equal(fmt.Sprintf("https://example.com/build/%s?redirect_spruce_users=true", b.Id), status.URL)
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
	s.NoError(tsk.Insert())

	b := build.Build{
		Id:           "b1",
		BuildVariant: "myBuild",
		Version:      s.patchDoc.Version,
		Status:       evergreen.BuildStarted,
	}
	s.NoError(b.Insert())

	job, ok := NewGithubStatusRefreshJob(s.patchDoc).(*githubStatusRefreshJob)
	s.Require().NotNil(job)
	s.Require().True(ok)
	s.Require().NotNil(job.patch)
	job.env = s.env
	job.Run(s.ctx)
	s.False(job.HasErrors())

	// Patch status
	status := s.getAndValidateStatus(s.env.InternalSender)
	s.Equal(fmt.Sprintf("https://example.com/version/%s?redirect_spruce_users=true", s.patchDoc.Version), status.URL)
	s.Equal("evergreen", status.Context)
	s.Equal(message.GithubStatePending, status.State)
	s.Equal("tasks are running", status.Description)

	// Build status
	status = s.getAndValidateStatus(s.env.InternalSender)
	s.Equal(fmt.Sprintf("https://example.com/build/%s?redirect_spruce_users=true", b.Id), status.URL)
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
	s.NoError(failedTask.Insert())
	unscheduledEssentialTask := task.Task{
		Id:                   "t2",
		BuildId:              "b1",
		BuildVariant:         "myBuild",
		Version:              s.patchDoc.Version,
		Activated:            false,
		Status:               evergreen.TaskUndispatched,
		IsEssentialToSucceed: true,
	}
	s.NoError(unscheduledEssentialTask.Insert())

	startTime := time.Now()
	b := build.Build{
		Id:           "b1",
		BuildVariant: "myBuild",
		StartTime:    startTime,
		FinishTime:   startTime.Add(time.Minute),
		Version:      s.patchDoc.Version,
		Status:       evergreen.BuildFailed,
	}
	s.NoError(b.Insert())

	job, ok := NewGithubStatusRefreshJob(s.patchDoc).(*githubStatusRefreshJob)
	s.Require().NotNil(job)
	s.Require().True(ok)
	s.Require().NotNil(job.patch)
	job.env = s.env
	job.Run(s.ctx)
	s.False(job.HasErrors())

	// Patch status
	status := s.getAndValidateStatus(s.env.InternalSender)
	s.Equal(fmt.Sprintf("https://example.com/version/%s?redirect_spruce_users=true", s.patchDoc.Version), status.URL)
	s.Equal("evergreen", status.Context)
	s.Equal(message.GithubStatePending, status.State)
	s.Equal("tasks are running", status.Description)

	// Build status
	status = s.getAndValidateStatus(s.env.InternalSender)
	s.Equal(fmt.Sprintf("https://example.com/build/%s?redirect_spruce_users=true", b.Id), status.URL)
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
	s.NoError(b.Insert())
	t1 := task.Task{
		Id:      "t1",
		Version: s.patchDoc.Version,
		BuildId: b.Id,
		Status:  evergreen.TaskSucceeded,
	}
	s.NoError(t1.Insert())

	childPatch := patch.Patch{
		Id:         mgobson.NewObjectId(),
		Status:     evergreen.VersionSucceeded,
		Project:    "myChildProject",
		Activated:  true,
		StartTime:  startTime,
		FinishTime: startTime.Add(12 * time.Minute),
		Triggers: patch.TriggerInfo{
			ParentPatch: s.patchDoc.Id.Hex(),
		},
		DisplayNewUI: true,
	}
	s.NoError(childPatch.Insert())
	s.patchDoc.Triggers.ChildPatches = []string{childPatch.Id.Hex()}
	s.patchDoc.Status = evergreen.VersionSucceeded

	job, ok := NewGithubStatusRefreshJob(s.patchDoc).(*githubStatusRefreshJob)
	s.Require().NotNil(job)
	s.Require().True(ok)
	s.Require().NotNil(job.patch)

	job.env = s.env
	job.Run(s.ctx)
	s.Zero(job.Error())

	// Patch status
	status := s.getAndValidateStatus(s.env.InternalSender)
	s.Equal(fmt.Sprintf("https://example.com/version/%s?redirect_spruce_users=true", s.patchDoc.Version), status.URL)
	s.Equal("evergreen", status.Context)
	s.Equal(message.GithubStateSuccess, status.State)
	s.Equal("version finished in 10m0s", status.Description)

	// Child patch status
	status = s.getAndValidateStatus(s.env.InternalSender)
	s.Equal(fmt.Sprintf("https://example.com/version/%s/downstream-projects?redirect_spruce_users=true", childPatch.Id.Hex()), status.URL)
	s.Equal("evergreen/myChildProjectIdentifier", status.Context)
	s.Equal(message.GithubStateSuccess, status.State)
	s.Equal("child patch finished in 12m0s", status.Description)

	// Build status
	status = s.getAndValidateStatus(s.env.InternalSender)
	s.Equal(fmt.Sprintf("https://example.com/build/%s?redirect_spruce_users=true", b.Id), status.URL)
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
	s.NoError(b.Insert())
	t1 := task.Task{
		Id:      "t1",
		Version: s.patchDoc.Version,
		BuildId: b.Id,
		Status:  evergreen.TaskFailed,
	}
	s.NoError(t1.Insert())

	childPatch := patch.Patch{
		Id:         mgobson.NewObjectId(),
		Status:     evergreen.VersionFailed,
		Project:    "myChildProject",
		Activated:  true,
		StartTime:  startTime,
		FinishTime: startTime.Add(12 * time.Minute),
		Triggers: patch.TriggerInfo{
			ParentPatch: s.patchDoc.Id.Hex(),
		},
		DisplayNewUI: true,
	}
	s.NoError(childPatch.Insert())
	s.patchDoc.Triggers.ChildPatches = []string{childPatch.Id.Hex()}
	s.patchDoc.Status = evergreen.VersionSucceeded

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
	s.Equal(fmt.Sprintf("https://example.com/version/%s?redirect_spruce_users=true", s.patchDoc.Version), status.URL)
	s.Equal("evergreen", status.Context)
	s.Equal(message.GithubStateFailure, status.State)
	s.Equal("version finished in 10m0s", status.Description)

	// Child patch status
	status = s.getAndValidateStatus(s.env.InternalSender)
	s.Equal(fmt.Sprintf("https://example.com/version/%s/downstream-projects?redirect_spruce_users=true", childPatch.Id.Hex()), status.URL)
	s.Equal("evergreen/myChildProjectIdentifier", status.Context)
	s.Equal(message.GithubStateFailure, status.State)
	s.Equal("child patch finished in 12m0s", status.Description)

	// Build status
	status = s.getAndValidateStatus(s.env.InternalSender)
	s.Equal(fmt.Sprintf("https://example.com/build/%s?redirect_spruce_users=true", b.Id), status.URL)
	s.Equal("evergreen/myBuild", status.Context)
	s.Equal("none succeeded, 1 failed in 1m0s", status.Description)
	s.Equal(message.GithubStateFailure, status.State)
}

func (s *githubStatusRefreshSuite) getAndValidateStatus(sender *send.InternalSender) *message.GithubStatus {
	msg, ok := sender.GetMessageSafe()
	s.Require().True(ok)
	raw := msg.Message
	s.Require().NotNil(raw)
	status, ok := raw.Raw().(*message.GithubStatus)
	s.Require().True(ok)

	s.Equal("evergreen-ci", status.Owner)
	s.Equal("evergreen", status.Repo)
	s.Equal("776f608b5b12cd27b8d931c8ee4ca0c13f857299", status.Ref)
	return status
}
