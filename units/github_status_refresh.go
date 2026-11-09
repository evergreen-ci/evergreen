package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/githubapp"
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

// NewGithubStatusResyncRefreshJob returns a job that resends the GitHub
// statuses of a patch whose statuses failed to send. The job ID is unique per
// patch and run time.
func NewGithubStatusResyncRefreshJob(patchID string, ts time.Time) amboy.Job {
	job := makeGithubStatusRefreshJob()
	job.FetchID = patchID

	job.SetID(fmt.Sprintf("%s:resync-%s-%s", githubStatusRefreshJobName, patchID, ts.Format(TSFormat)))
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
	rateLimited  bool

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
	return j
}

func (j *githubStatusRefreshJob) shouldUpdate(ctx context.Context) (bool, error) {
	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return false, errors.Wrap(err, "retrieving admin settings")
	}
	if flags.GithubStatusAPIDisabled {
		grip.InfoWhen(ctx, sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
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

	if j.patch == nil {
		j.patch, err = patch.FindOneId(ctx, j.FetchID)
		if err != nil {
			return errors.Wrap(err, "finding patch")
		}
		if j.patch == nil {
			return errors.New("patch not found")
		}
	}

	target := j.newGitHubStatus()
	if target.Owner == "" || target.Repo == "" || target.Ref == "" {
		return errors.Errorf("patch '%s' has no GitHub owner, repo, and ref to send statuses to", j.patch.Id.Hex())
	}

	j.sender, err = j.env.GetGitHubSender(target.Owner, target.Repo, githubapp.CreateGitHubAppAuth(j.env.Settings()).CreateGitHubSenderInstallationToken)
	if err != nil {
		return err
	}

	j.builds, err = build.Find(ctx, build.ByVersion(j.FetchID))
	if err != nil {
		return errors.Wrap(err, "finding builds")
	}

	if len(j.patch.Triggers.ChildPatches) > 0 {
		j.childPatches, err = patch.Find(ctx, patch.ByStringIds(ctx, j.patch.Triggers.ChildPatches))
		if err != nil {
			return errors.Wrap(err, "finding child patches")
		}
	}
	return nil
}

// newGitHubStatus returns a status targeting the GitHub owner, repo, and
// commit that the patch's statuses are reported to.
func (j *githubStatusRefreshJob) newGitHubStatus() *message.GithubStatus {
	if j.patch.IsMergeQueuePatch() {
		return &message.GithubStatus{
			Owner: j.patch.GithubMergeData.Org,
			Repo:  j.patch.GithubMergeData.Repo,
			Ref:   j.patch.GithubMergeData.HeadSHA,
		}
	}
	return &message.GithubStatus{
		Owner: j.patch.GithubPatchData.BaseOwner,
		Repo:  j.patch.GithubPatchData.BaseRepo,
		Ref:   j.patch.GithubPatchData.HeadHash,
	}
}

func (j *githubStatusRefreshJob) sendStatus(ctx context.Context, status *message.GithubStatus) {
	c := message.MakeGithubStatusMessageWithRepo(*status)
	if !c.Loggable() {
		j.AddError(errors.Errorf("status message is invalid: %+v", status))
		return
	}
	j.AddError(c.SetPriority(level.Notice))

	if err := send.SendWithError(ctx, j.sender, c); err != nil {
		j.AddError(err)
		j.rateLimited = isGitHubRateLimit(err)
		return
	}
	grip.Info(ctx, message.Fields{
		"ticket":   thirdparty.GithubInvestigation,
		"message":  "called github status refresh",
		"caller":   githubStatusRefreshJobName,
		"context":  status.Context,
		"patch_id": j.FetchID,
		"job_id":   j.ID(),
	})
}

// sendChildPatchStatuses iterates through child patches if relevant and builds/sends statuses.
func (j *githubStatusRefreshJob) sendChildPatchStatuses(ctx context.Context) error {
	if len(j.childPatches) == 0 {
		return nil
	}

	status := j.newGitHubStatus()

	for _, childPatch := range j.childPatches {
		if j.rateLimited {
			return nil
		}
		projectIdentifier, err := model.GetIdentifierForProject(ctx, childPatch.Project)
		if err != nil {
			return errors.Wrap(err, "finding project identifier")
		}
		status.Context, err = patch.GetGithubContextForChildPatch(projectIdentifier, j.patch, &childPatch)
		if err != nil {
			return errors.Wrapf(err, "getting github context for child patch '%s'", childPatch.Id.Hex())
		}

		status.URL = childPatch.GetURL(j.urlBase)
		status.State, status.Description = patch.GetGitHubStateAndDescription(childPatch.Status, childPatch.StartTime, childPatch.FinishTime)
		j.sendStatus(ctx, status)
	}
	return nil
}

// getGitHubStateAndDescriptionForPatch returns the GitHub state and
// description for the patch's own status context. Parent patches report the
// collective status of the parent and its child patches.
func (j *githubStatusRefreshJob) getGitHubStateAndDescriptionForPatch(ctx context.Context) (message.GithubState, string, error) {
	if !j.patch.IsParent() {
		state, description := patch.GetGitHubStateAndDescription(j.patch.Status, j.patch.StartTime, j.patch.FinishTime)
		return state, description, nil
	}

	collective, err := patch.GetCollectiveGitHubStatus(ctx, j.patch, j.childPatches)
	if err != nil {
		return "", "", err
	}
	return collective.State, collective.Description, nil
}

func (j *githubStatusRefreshJob) sendBuildStatuses(ctx context.Context) {
	status := j.newGitHubStatus()
	for _, b := range j.builds {
		if j.rateLimited {
			return
		}
		// Dormant builds were never scheduled, so nothing will ever run for
		// them and no status could ever resolve; report nothing for them.
		if b.IsDormant() {
			grip.Info(ctx, message.Fields{
				"job":     j.Name,
				"message": "skipping dormant build when sending GitHub statuses",
				"build":   b.Id,
				"variant": b.BuildVariant,
				"patch":   j.FetchID,
			})
			continue
		}
		status.Context = fmt.Sprintf("%s/%s", thirdparty.GithubStatusDefaultContext, b.BuildVariant)
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
		tasks, err := task.FindAll(ctx, query)
		if err != nil {
			j.AddError(errors.Wrapf(err, "finding tasks in build '%s'", b.Id))
			continue
		}
		status.Description = b.GetPRNotificationDescription(ctx, tasks)

		j.sendStatus(ctx, status)
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
	start := time.Now()
	if err = j.fetch(ctx); err != nil {
		j.AddError(err)
		return
	}

	status := j.newGitHubStatus()
	status.URL = j.patch.GetURL(j.urlBase)
	status.Context = thirdparty.GithubStatusDefaultContext
	status.State, status.Description, err = j.getGitHubStateAndDescriptionForPatch(ctx)
	if err != nil {
		j.AddError(errors.Wrapf(err, "getting GitHub state for patch '%s'", j.patch.Id.Hex()))
	} else {
		j.sendStatus(ctx, status)
	}

	// Send child patch statuses.
	if err := j.sendChildPatchStatuses(ctx); err != nil {
		j.AddError(err)
		return
	}

	// For each build, send build status.
	j.sendBuildStatuses(ctx)

	if !j.HasErrors() {
		j.AddError(patch.ClearGitHubStatusResync(ctx, j.FetchID, start))
	}
}
