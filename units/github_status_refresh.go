package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
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
	githubStatusRefreshJobName = "github-status-refresh"
)

func init() {
	registry.AddJobType(githubStatusRefreshJobName, func() amboy.Job { return makeGithubStatusRefreshJob() })
}

// NewGithubStatusRefreshJob is a job that re-sends github statuses to the PR associated with the given patch.
func NewGithubStatusRefreshJob(p *patch.Patch) amboy.Job {
	job := makeGithubStatusRefreshJob()
	job.FetchID = p.Version
	job.patch = p

	job.SetID(fmt.Sprintf("%s:%s-%s", githubStatusRefreshJobName, p.Version, time.Now().String()))
	return job
}

type githubStatusRefreshJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	env      evergreen.Environment
	sender   send.Sender

	urlBase      string
	patch        *patch.Patch
	builds       []build.Build
	childPatches []patch.Patch

	FetchID string `bson:"fetch_id" json:"fetch_id" yaml:"fetch_id"`
}

func makeGithubStatusRefreshJob() *githubStatusRefreshJob {
	j := &githubStatusRefreshJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    githubStatusRefreshJobName,
				Version: 0,
			},
		},
	}
	j.SetPriority(1)
	return j
}

func (j *githubStatusRefreshJob) shouldUpdate(ctx context.Context) (bool, error) {
	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return false, errors.Wrap(err, "error retrieving admin settings")
	}
	if flags.GithubStatusAPIDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"job":     j.Name,
			"message": "GitHub status updates are disabled, not updating status",
		})
		return false, nil

	}
	return true, nil
}

func (j *githubStatusRefreshJob) fetch(ctx context.Context) error {
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}
	uiConfig := evergreen.UIConfig{}
	var err error
	if err := uiConfig.Get(ctx); err != nil {
		return errors.Wrap(err, "retrieving UI config")
	}
	j.urlBase = uiConfig.Url
	if j.urlBase == "" {
		return errors.New("url base doesn't exist")
	}
	j.sender, err = j.env.GetGitHubSender(j.patch.GithubPatchData.BaseOwner, j.patch.GithubPatchData.BaseRepo)
	if err != nil {
		return err
	}
	if j.patch == nil {
		j.patch, err = patch.FindOneId(j.FetchID)
		if err != nil {
			return errors.Wrap(err, "finding patch")
		}
		if j.patch == nil {
			return errors.New("patch not found")
		}
	}

	j.builds, err = build.Find(build.ByVersion(j.FetchID))
	if err != nil {
		return errors.Wrap(err, "finding builds")
	}

	if len(j.patch.Triggers.ChildPatches) > 0 {
		j.childPatches, err = patch.Find(patch.ByStringIds(j.patch.Triggers.ChildPatches))
		if err != nil {
			return errors.Wrap(err, "finding child patches")
		}
	}
	return nil
}

func (j *githubStatusRefreshJob) sendStatus(status *message.GithubStatus) {
	c := message.MakeGithubStatusMessageWithRepo(*status)
	if !c.Loggable() {
		j.AddError(errors.Errorf("status message is invalid: %+v", status))
		return
	}
	j.AddError(c.SetPriority(level.Notice))

	j.sender.Send(c)
	grip.Info(message.Fields{
		"ticket":   thirdparty.GithubInvestigation,
		"message":  "called github status refresh",
		"caller":   githubStatusRefreshJobName,
		"context":  status.Context,
		"patch_id": j.FetchID,
		"job_id":   j.ID(),
	})
}

// sendChildPatchStatuses iterates through child patches if relevant and builds/sends statuses.
func (j *githubStatusRefreshJob) sendChildPatchStatuses() error {
	if len(j.childPatches) == 0 {
		return nil
	}

	status := &message.GithubStatus{
		Owner: j.patch.GithubPatchData.BaseOwner,
		Repo:  j.patch.GithubPatchData.BaseRepo,
		Ref:   j.patch.GithubPatchData.HeadHash,
	}

	for _, childPatch := range j.childPatches {
		projectIdentifier, err := model.GetIdentifierForProject(childPatch.Project)
		if err != nil {
			return errors.Wrap(err, "finding project identifier")
		}
		status.Context, err = patch.GetGithubContextForChildPatch(projectIdentifier, j.patch, &childPatch)
		if err != nil {
			return errors.Wrapf(err, "getting github context for child patch '%s'", childPatch.Id.Hex())
		}

		status.URL = childPatch.GetURL(j.urlBase)
		status.State, status.Description = getGithubStateAndDescriptionForPatch(&childPatch)
		j.sendStatus(status)
	}
	return nil
}

func getGithubStateAndDescriptionForPatch(p *patch.Patch) (message.GithubState, string) {
	var state message.GithubState
	if evergreen.IsSuccessfulVersionStatus(p.Status) {
		state = message.GithubStateSuccess
	} else if p.Status == evergreen.VersionFailed {
		state = message.GithubStateFailure
	} else {
		return message.GithubStatePending, evergreen.PRTasksRunningDescription
	}
	duration := p.FinishTime.Sub(p.StartTime).String()
	name := "version"
	if p.IsChild() {
		name = "child patch"
	}
	return state, fmt.Sprintf("%s finished in %s", name, duration)
}

func (j *githubStatusRefreshJob) sendBuildStatuses() {
	status := &message.GithubStatus{
		Owner: j.patch.GithubPatchData.BaseOwner,
		Repo:  j.patch.GithubPatchData.BaseRepo,
		Ref:   j.patch.GithubPatchData.HeadHash,
	}
	for _, b := range j.builds {
		status.Context = fmt.Sprintf("%s/%s", evergreenContext, b.BuildVariant)
		status.URL = b.GetURL(j.urlBase)

		switch b.Status {
		case evergreen.BuildSucceeded:
			status.State = message.GithubStateSuccess
		case evergreen.BuildFailed:
			status.State = message.GithubStateFailure
		default:
			status.State = message.GithubStatePending
		}

		query := db.Query(task.ByBuildId(b.Id)).WithFields(task.StatusKey, task.IsEssentialToSucceedKey, task.ActivatedKey)
		tasks, err := task.FindAll(query)
		if err != nil {
			j.AddError(errors.Wrapf(err, "finding tasks in build '%s'", b.Id))
			continue
		}
		status.Description = b.GetPRNotificationDescription(tasks)

		j.sendStatus(status)
	}
}

func (j *githubStatusRefreshJob) Run(ctx context.Context) {
	shouldUpdate, err := j.shouldUpdate(ctx)
	if err != nil {
		j.AddError(err)
		return
	}
	if !shouldUpdate {
		return
	}
	if err = j.fetch(ctx); err != nil {
		j.AddError(err)
		return
	}

	status := &message.GithubStatus{
		URL:     j.patch.GetURL(j.urlBase),
		Context: evergreenContext,
		Owner:   j.patch.GithubPatchData.BaseOwner,
		Repo:    j.patch.GithubPatchData.BaseRepo,
		Ref:     j.patch.GithubPatchData.HeadHash,
	}
	status.State, status.Description = getGithubStateAndDescriptionForPatch(j.patch)

	// Send patch status
	j.sendStatus(status)

	// Send child patch statuses.
	if err := j.sendChildPatchStatuses(); err != nil {
		j.AddError(err)
		return
	}

	// For each build, send build status.
	j.sendBuildStatuses()
}
