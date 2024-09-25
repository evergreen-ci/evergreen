package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/githubapp"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

const (
	githubStatusUpdateJobName = "github-status-update"

	githubUpdateTypeNewPatch              = "new-patch"
	githubUpdateTypeSuccessMessage        = "success-message"
	githubUpdateTypeRequestAuth           = "request-auth"
	githubUpdateTypePushToCommitQueue     = "commit-queue-push"
	githubUpdateTypeDeleteFromCommitQueue = "commit-queue-delete"
	githubUpdateTypeProcessingError       = "processing-error"
)

const (
	// GitHub intent processing errors
	ProjectDisabled             = "project was disabled"
	PatchingDisabled            = "patching was disabled"
	commitQueueDisabled         = "merge queue disabled for project"
	ignoredFiles                = "all patched files are ignored"
	PatchTaskSyncDisabled       = "task sync was disabled for patches"
	invalidAlias                = "alias not found"
	NoTasksOrVariants           = "no tasks/variants were configured"
	noChildPatchTasksOrVariants = "no tasks/variants were configured for child patch"
	NoSyncTasksOrVariants       = "no tasks/variants were configured for sync"
	GitHubInternalError         = "GitHub returned an error"
	InvalidConfig               = "config file was invalid: sync with base branch & run `evergreen validate -p <project>`"
	EmptyConfig                 = "config file was empty"
	ProjectFailsValidation      = "project fails validation: sync with base branch & run `evergreen validate -p <project>`"
	OtherErrors                 = "Evergreen error"
	MergeBaseTooOld             = "merge base is too old"
)

func init() {
	registry.AddJobType(githubStatusUpdateJobName, func() amboy.Job { return makeGithubStatusUpdateJob() })
}

type githubStatusUpdateJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	env      evergreen.Environment
	urlBase  string
	sender   send.Sender

	FetchID       string `bson:"fetch_id" json:"fetch_id" yaml:"fetch_id"`
	UpdateType    string `bson:"update_type" json:"update_type" yaml:"update_type"`
	Owner         string `bson:"owner" json:"owner" yaml:"owner"`
	Repo          string `bson:"repo" json:"repo" yaml:"repo"`
	Ref           string `bson:"ref" json:"ref" yaml:"ref"`
	GithubContext string `bson:"github_context" json:"github_context" yaml:"github_context"`
	Description   string `bson:"description" json:"description" yaml:"description"`
}

func makeGithubStatusUpdateJob() *githubStatusUpdateJob {
	j := &githubStatusUpdateJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    githubStatusUpdateJobName,
				Version: 0,
			},
		},
	}
	return j
}

// NewGithubStatusUpdateJobWithSuccessMessage creates a job to send a passing status to Github with a message.
func NewGithubStatusUpdateJobWithSuccessMessage(githubContext, owner, repo, ref, description string) amboy.Job {
	job := makeGithubStatusUpdateJob()
	job.GithubContext = githubContext
	job.UpdateType = githubUpdateTypeSuccessMessage
	job.Owner = owner
	job.Repo = repo
	job.Ref = ref
	job.Description = description
	job.SetID(fmt.Sprintf("%s:%s-%s", githubStatusUpdateJobName, job.UpdateType, time.Now().String()))
	return job
}

// NewGithubStatusUpdateJobForNewPatch creates a job to update github's API
// for a newly created patch, reporting it as pending, with description
// "preparing to run tasks"
func NewGithubStatusUpdateJobForNewPatch(patchID string) amboy.Job {
	job := makeGithubStatusUpdateJob()
	job.FetchID = patchID
	job.UpdateType = githubUpdateTypeNewPatch

	job.SetID(fmt.Sprintf("%s:%s-%s-%s", githubStatusUpdateJobName, job.UpdateType, patchID, time.Now().String()))

	return job
}

// NewGithubStatusUpdateJobForExternalPatch prompts on Github for a user to
// manually authorize this patch
func NewGithubStatusUpdateJobForExternalPatch(patchID string) amboy.Job {
	job := makeGithubStatusUpdateJob()
	job.FetchID = patchID
	job.UpdateType = githubUpdateTypeRequestAuth

	job.SetID(fmt.Sprintf("%s:%s-%s-%s", githubStatusUpdateJobName, job.UpdateType, patchID, time.Now().String()))
	return job
}

func NewGithubStatusUpdateJobForPushToCommitQueue(owner, repo, ref string, prNumber int, patchId string) amboy.Job {
	job := makeGithubStatusUpdateJob()
	job.UpdateType = githubUpdateTypePushToCommitQueue
	job.Owner = owner
	job.Repo = repo
	job.Ref = ref
	job.FetchID = patchId

	job.SetID(fmt.Sprintf("%s:%s-%s-%s-%d-%s", githubStatusUpdateJobName, job.UpdateType, owner, repo, prNumber, time.Now().String()))
	return job
}

func NewGithubStatusUpdateJobForDeleteFromCommitQueue(owner, repo, ref string, prNumber int) amboy.Job {
	job := makeGithubStatusUpdateJob()
	job.UpdateType = githubUpdateTypeDeleteFromCommitQueue
	job.Owner = owner
	job.Repo = repo
	job.Ref = ref

	job.SetID(fmt.Sprintf("%s:%s-%s-%s-%d-%s", githubStatusUpdateJobName, job.UpdateType, owner, repo, prNumber, time.Now().String()))
	return job
}

// NewGithubStatusUpdateJobForProcessingError marks a ref as failed because the
// evergreen encountered an error creating a patch
func NewGithubStatusUpdateJobForProcessingError(githubContext, owner, repo, ref, description string) amboy.Job {
	job := makeGithubStatusUpdateJob()
	job.Owner = owner
	job.Repo = repo
	job.Ref = ref
	job.UpdateType = githubUpdateTypeProcessingError
	job.GithubContext = githubContext
	job.Description = description

	job.SetID(fmt.Sprintf("%s:%s-%s-%s-%s-%s", githubStatusUpdateJobName, job.UpdateType, owner, repo, description, time.Now().String()))

	return job
}

func (j *githubStatusUpdateJob) preamble(ctx context.Context) error {
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}
	uiConfig := evergreen.UIConfig{}
	if err := uiConfig.Get(ctx); err != nil {
		return err
	}
	j.urlBase = uiConfig.Url
	if len(j.urlBase) == 0 {
		return errors.New("UI URL is empty")
	}
	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return errors.Wrap(err, "getting service flags")
	}
	if flags.GithubStatusAPIDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"job":     githubStatusUpdateJobName,
			"message": "GitHub status updates are disabled, not updating status",
		})
		return errors.New("GitHub status updates are disabled, not updating status")
	}

	return nil
}

func (j *githubStatusUpdateJob) fetch() (*message.GithubStatus, error) {
	status := message.GithubStatus{
		Owner: j.Owner,
		Repo:  j.Repo,
		Ref:   j.Ref,
	}

	if j.UpdateType == githubUpdateTypeProcessingError {
		status.Context = j.GithubContext
		status.State = message.GithubStateFailure
		status.Description = j.Description

	} else if j.UpdateType == githubUpdateTypeSuccessMessage {
		status.Context = j.GithubContext
		status.State = message.GithubStateSuccess
		status.Description = j.Description

	} else if j.UpdateType == githubUpdateTypeNewPatch {
		status.URL = fmt.Sprintf("%s/version/%s?redirect_spruce_users=true", j.urlBase, j.FetchID)
		status.Context = thirdparty.GithubStatusDefaultContext
		status.State = message.GithubStatePending
		status.Description = "preparing to run tasks"

	} else if j.UpdateType == githubUpdateTypeRequestAuth {
		status.URL = fmt.Sprintf("%s/patch/%s", j.urlBase, j.FetchID)
		status.Context = thirdparty.GithubStatusDefaultContext
		status.Description = "patch must be manually authorized"
		status.State = message.GithubStateFailure

	} else if j.UpdateType == githubUpdateTypePushToCommitQueue {
		status.Context = commitqueue.GithubContext
		status.Description = "added to queue"
		status.State = message.GithubStatePending
		if j.FetchID != "" {
			status.URL = fmt.Sprintf("%s/patch/%s?redirect_spruce_users=true", j.urlBase, j.FetchID)
		}
	} else if j.UpdateType == githubUpdateTypeDeleteFromCommitQueue {
		status.Context = commitqueue.GithubContext
		status.Description = "removed from queue"
		status.State = message.GithubStateSuccess
	}

	if j.UpdateType == githubUpdateTypeRequestAuth || j.UpdateType == githubUpdateTypeNewPatch {
		patchDoc, err := patch.FindOne(patch.ById(mgobson.ObjectIdHex(j.FetchID)))
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if patchDoc == nil {
			return nil, errors.New("patch not found")
		}

		status.Owner = patchDoc.GithubPatchData.BaseOwner
		status.Repo = patchDoc.GithubPatchData.BaseRepo
		status.Ref = patchDoc.GithubPatchData.HeadHash
	}

	return &status, nil
}

func (j *githubStatusUpdateJob) setSender(owner, repo string) error {
	var err error
	j.sender, err = j.env.GetGitHubSender(owner, repo, githubapp.CreateGitHubAppAuth(j.env.Settings()).CreateGitHubSenderInstallationToken)
	return err
}

func (j *githubStatusUpdateJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	j.AddError(j.preamble(ctx))
	if j.HasErrors() {
		return
	}

	status, err := j.fetch()
	if err != nil {
		j.AddError(err)
		return
	}

	err = j.setSender(status.Owner, status.Repo)
	if err != nil {
		j.AddError(err)
		return
	}

	c := message.MakeGithubStatusMessageWithRepo(*status)
	if !c.Loggable() {
		j.AddError(errors.Errorf("status message is invalid: %+v", status))
		return
	}
	j.AddError(c.SetPriority(level.Notice))

	j.sender.Send(c)
	grip.Info(message.Fields{
		"ticket":  thirdparty.GithubInvestigation,
		"message": "called github status send",
		"caller":  githubStatusUpdateJobName,
	})
}
