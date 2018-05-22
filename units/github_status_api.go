package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

const (
	githubStatusUpdateJobName = "github-status-update"

	githubStatusError   = "error"
	githubStatusFailure = "failure"
	githubStatusPending = "pending"
	githubStatusSuccess = "success"

	githubUpdateTypeNewPatch    = "new-patch"
	githubUpdateTypeRequestAuth = "request-auth"
	githubUpdateTypeBadConfig   = "bad-config"

	githubStatusAPITimeout = time.Minute
)

func init() {
	registry.AddJobType(githubStatusUpdateJobName, func() amboy.Job { return makeGithubStatusUpdateJob() })
}

type githubStatusUpdateJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	env      evergreen.Environment
	urlBase  string
	sender   send.Sender

	FetchID    string `bson:"fetch_id" json:"fetch_id" yaml:"fetch_id"`
	UpdateType string `bson:"update_type" json:"update_type" yaml:"update_type"`
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
	j.SetDependency(dependency.NewAlways())
	j.SetPriority(1)
	return j
}

// NewGithubStatusUpdateJobForNewPatch creates a job to update github's API
// for a newly created patch, reporting it as pending, with description
// "preparing to run tasks"
func NewGithubStatusUpdateJobForNewPatch(version string) amboy.Job {
	job := makeGithubStatusUpdateJob()
	job.FetchID = version
	job.UpdateType = githubUpdateTypeNewPatch

	job.SetID(fmt.Sprintf("%s:%s-%s-%s", githubStatusUpdateJobName, job.UpdateType, version, time.Now().String()))

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

// NewGithubStatusUpdateJobForBadConfig marks a ref as failed because the
// evergreen configuration is bad
func NewGithubStatusUpdateJobForBadConfig(intentID string) amboy.Job {
	job := makeGithubStatusUpdateJob()
	job.FetchID = intentID
	job.UpdateType = githubUpdateTypeBadConfig

	job.SetID(fmt.Sprintf("%s:%s-%s-%s", githubStatusUpdateJobName, job.UpdateType, intentID, time.Now().String()))

	return job
}

func (j *githubStatusUpdateJob) preamble() error {
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}
	uiConfig := evergreen.UIConfig{}
	if err := uiConfig.Get(); err != nil {
		return err
	}
	j.urlBase = uiConfig.Url

	if j.sender == nil {
		var err error
		j.sender, err = j.env.GetSender(evergreen.SenderGithubStatus)
		if err != nil {
			return err
		}
	}

	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		return errors.Wrap(err, "error retrieving admin settings")
	}
	if flags.GithubStatusAPIDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"job":     githubStatusUpdateJobName,
			"message": "github status updates are disabled, not updating status",
		})
		return errors.New("github status updates are disabled, not updating status")
	}

	return nil
}

func (j *githubStatusUpdateJob) fetch() (*message.GithubStatus, error) {
	var patchDoc *patch.Patch
	var err error
	status := message.GithubStatus{}

	if j.UpdateType == githubUpdateTypeBadConfig {
		var intent patch.Intent
		intent, err = patch.FindIntent(j.FetchID, patch.GithubIntentType)
		if err != nil {
			return nil, errors.Wrap(err, "can't fetch patch intent")
		}
		patchDoc = intent.NewPatch()
		if patchDoc == nil {
			return nil, errors.New("patch is missing")
		}

		var projectRef *model.ProjectRef
		projectRef, err = model.FindOneProjectRefByRepoAndBranchWithPRTesting(patchDoc.GithubPatchData.BaseOwner,
			patchDoc.GithubPatchData.BaseRepo, patchDoc.GithubPatchData.BaseBranch)
		if err != nil {
			return nil, errors.Wrap(err, "can't fetch project ref")
		}
		if projectRef == nil {
			return nil, errors.New("can't find project ref")
		}

		status.URL = fmt.Sprintf("%s/waterfall/%s", j.urlBase, projectRef.Identifier)
		status.Context = "evergreen"
		status.State = githubStatusFailure
		status.Description = "project config was invalid"

	} else if j.UpdateType == githubUpdateTypeNewPatch {
		status.URL = fmt.Sprintf("%s/version/%s", j.urlBase, j.FetchID)
		status.Context = "evergreen"
		status.State = githubStatusPending
		status.Description = "preparing to run tasks"

	} else if j.UpdateType == githubUpdateTypeRequestAuth {
		status.URL = fmt.Sprintf("%s/patch/%s", j.urlBase, j.FetchID)
		status.Context = "evergreen"
		status.Description = "patch must be manually authorized"
		status.State = githubStatusFailure
	}

	if patchDoc == nil {
		patchDoc, err = patch.FindOne(patch.ById(bson.ObjectIdHex(j.FetchID)))
		if err != nil {
			return nil, err
		}
		if patchDoc == nil {
			return nil, errors.New("can't find patch")
		}
	}

	status.Owner = patchDoc.GithubPatchData.BaseOwner
	status.Repo = patchDoc.GithubPatchData.BaseRepo
	status.Ref = patchDoc.GithubPatchData.HeadHash
	return &status, nil
}

func (j *githubStatusUpdateJob) Run(_ context.Context) {
	defer j.MarkComplete()

	j.AddError(j.preamble())
	if j.HasErrors() {
		return
	}

	status, err := j.fetch()
	if err != nil {
		j.AddError(err)
		return
	}

	c := message.MakeGithubStatusMessageWithRepo(*status)
	if !c.Loggable() {
		j.AddError(errors.Errorf("status message is invalid: %+v", status))
		return
	}
	c.SetPriority(level.Notice)

	j.sender.Send(c)
}

func repoReference(owner, repo string, prNumber int, ref string) string {
	return fmt.Sprintf("%s/%s#%d@%s", owner, repo, prNumber, ref)
}
