package units

import (
	"context"
	"fmt"
	"slices"
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
// statuses of a patch that failed to send. It only resends the statuses that
// failed and are finished, so it can't overwrite a newer status with a stale
// pending one. Only one resync job can be queued per patch at a time.
func NewGithubStatusResyncRefreshJob(patchID string, ts time.Time) amboy.Job {
	job := makeGithubStatusRefreshJob()
	job.FetchID = patchID
	job.IsResync = true

	job.SetID(fmt.Sprintf("%s:resync-%s-%s", githubStatusRefreshJobName, patchID, ts.Format(TSFormat)))
	job.SetScopes([]string{fmt.Sprintf("%s:resync-%s", githubStatusRefreshJobName, patchID)})
	job.SetEnqueueAllScopes(true)
	return job
}

type githubStatusRefreshJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	env      evergreen.Environment
	sender   send.Sender

	urlBase        string
	patch          *patch.Patch
	builds         []build.Build
	childPatches   []patch.Patch
	rateLimited    bool
	resyncContexts []string

	FetchID  string `bson:"fetch_id" json:"fetch_id" yaml:"fetch_id"`
	IsResync bool   `bson:"is_resync" json:"is_resync" yaml:"is_resync"`
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
	if j.IsResync && !flags.GitHubStatusResyncEnabled {
		return false, nil
	}
	return true, nil
}

var (
	errPatchNotFound  = errors.New("patch not found")
	errNoGitHubTarget = errors.New("patch has no GitHub owner, repo, and ref to send statuses to")
)

// prepareResync returns the patch's resync mark, or nil if there is nothing
// to resend. It clears the mark if the patch's statuses can't be resent or
// are no longer worth resending.
func (j *githubStatusRefreshJob) prepareResync(ctx context.Context) (*patch.GitHubStatusResync, error) {
	resync, err := patch.FindGitHubStatusResync(ctx, j.FetchID)
	if err != nil || resync == nil {
		return nil, err
	}

	err = j.loadPatch(ctx)
	if errors.Is(err, errPatchNotFound) || errors.Is(err, errNoGitHubTarget) {
		return nil, j.clearResync(ctx, resync, err.Error())
	}
	if err != nil {
		return nil, err
	}

	// Only the PR's latest commit shows up on the PR, so statuses for an older
	// commit aren't worth resending.
	if j.patch.IsGithubPRPatch() && !j.patch.IsMergeQueuePatch() {
		latest, err := patch.FindLatestGithubPRPatch(ctx, j.patch.GithubPatchData.BaseOwner, j.patch.GithubPatchData.BaseRepo, j.patch.GithubPatchData.PRNumber)
		if err != nil {
			return nil, errors.Wrap(err, "finding latest patch for PR")
		}
		if latest != nil && latest.Id != j.patch.Id {
			return nil, j.clearResync(ctx, resync, "PR has a newer patch")
		}
	}

	j.resyncContexts = resync.Contexts
	return resync, nil
}

func (j *githubStatusRefreshJob) clearResync(ctx context.Context, resync *patch.GitHubStatusResync, reason string) error {
	grip.Info(ctx, message.Fields{
		"message":  "clearing GitHub status resync without resending",
		"reason":   reason,
		"patch_id": j.FetchID,
		"job_id":   j.ID(),
	})
	return patch.ClearGitHubStatusResync(ctx, j.FetchID, resync.LastFailedAt)
}

// sendsContext returns whether the job sends the status with the given
// context. Resyncs only resend statuses that failed to send.
func (j *githubStatusRefreshJob) sendsContext(statusContext string) bool {
	return !j.IsResync || slices.Contains(j.resyncContexts, statusContext)
}

// shouldSend returns whether the job sends the status. Resyncs also leave
// pending statuses to the regular notifications, since a pending status read
// here may already be out of date by the time it's sent.
func (j *githubStatusRefreshJob) shouldSend(status *message.GithubStatus) bool {
	if !j.sendsContext(status.Context) {
		return false
	}
	return !j.IsResync || status.State != message.GithubStatePending
}

// loadPatch loads the patch, if it isn't already loaded, and checks that it
// has a GitHub target to send statuses to.
func (j *githubStatusRefreshJob) loadPatch(ctx context.Context) error {
	if j.patch == nil {
		p, err := patch.FindOneId(ctx, j.FetchID)
		if err != nil {
			return errors.Wrap(err, "finding patch")
		}
		if p == nil {
			return errors.Wrapf(errPatchNotFound, "patch '%s'", j.FetchID)
		}
		j.patch = p
	}

	target := j.newGitHubStatus()
	if target.Owner == "" || target.Repo == "" || target.Ref == "" {
		return errors.Wrapf(errNoGitHubTarget, "patch '%s'", j.patch.Id.Hex())
	}
	return nil
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

	if err = j.loadPatch(ctx); err != nil {
		return err
	}

	target := j.newGitHubStatus()
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
		// Resending a status GitHub rejected won't help, so it doesn't keep
		// the patch marked for another resync.
		if j.IsResync && isRejectedGitHubSend(err) {
			grip.Warning(ctx, message.WrapError(err, message.Fields{
				"message":  "GitHub rejected resent status",
				"context":  status.Context,
				"patch_id": j.FetchID,
				"job_id":   j.ID(),
			}))
			return
		}
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
func (j *githubStatusRefreshJob) sendChildPatchStatuses(ctx context.Context) {
	status := j.newGitHubStatus()
	for _, childPatch := range j.childPatches {
		if j.rateLimited {
			return
		}
		projectIdentifier, err := model.GetIdentifierForProject(ctx, childPatch.Project)
		if err != nil {
			j.AddError(errors.Wrapf(err, "finding project identifier for child patch '%s'", childPatch.Id.Hex()))
			continue
		}
		status.Context, err = patch.GetGithubContextForChildPatch(projectIdentifier, j.patch, &childPatch)
		if err != nil {
			j.AddError(errors.Wrapf(err, "getting github context for child patch '%s'", childPatch.Id.Hex()))
			continue
		}

		status.URL = childPatch.GetURL(j.urlBase)
		status.State, status.Description = patch.GetGitHubStateAndDescription(childPatch.Status, childPatch.StartTime, childPatch.FinishTime)
		if j.shouldSend(status) {
			j.sendStatus(ctx, status)
		}
	}
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

func (j *githubStatusRefreshJob) sendPatchStatus(ctx context.Context) {
	if !j.sendsContext(thirdparty.GithubStatusDefaultContext) {
		return
	}
	status := j.newGitHubStatus()
	status.URL = j.patch.GetURL(j.urlBase)
	status.Context = thirdparty.GithubStatusDefaultContext
	var err error
	status.State, status.Description, err = j.getGitHubStateAndDescriptionForPatch(ctx)
	if err != nil {
		j.AddError(errors.Wrapf(err, "getting GitHub state for patch '%s'", j.patch.Id.Hex()))
		return
	}
	if j.shouldSend(status) {
		j.sendStatus(ctx, status)
	}
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
		if !j.shouldSend(status) {
			continue
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

	// The patch's mark is cleared after its statuses are sent unless a send
	// failed again in the meantime.
	lastFailedAt := time.Now()
	if j.IsResync {
		resync, err := j.prepareResync(ctx)
		if err != nil {
			j.AddError(err)
			return
		}
		if resync == nil {
			return
		}
		lastFailedAt = resync.LastFailedAt
	}

	j.sendStatuses(ctx)

	if !j.HasErrors() {
		j.AddError(patch.ClearGitHubStatusResync(ctx, j.FetchID, lastFailedAt))
	} else if j.IsResync {
		j.AddError(patch.MarkGitHubStatusResyncAttempted(ctx, j.FetchID))
	}
}

func (j *githubStatusRefreshJob) sendStatuses(ctx context.Context) {
	if err := j.fetch(ctx); err != nil {
		j.AddError(err)
		return
	}
	j.sendPatchStatus(ctx)
	j.sendChildPatchStatuses(ctx)
	j.sendBuildStatuses(ctx)
}
