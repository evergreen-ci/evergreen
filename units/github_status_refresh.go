package units

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/githubapp"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/utility"
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

	githubStatusReconcileMaxAttempts = 6
	githubStatusReconcileRetryWait   = 20 * time.Second
	githubStatusReconcileDelay       = 15 * time.Second
)

func init() {
	registry.AddJobType(githubStatusRefreshJobName, func() amboy.Job { return makeGithubStatusRefreshJob() })
}

type githubStatusRefreshConfig struct {
	reconcile bool
	delay     time.Duration
}

// NewGithubStatusRefreshJob re-sends GitHub statuses for the given patch.
func NewGithubStatusRefreshJob(p *patch.Patch) amboy.Job {
	return newGithubStatusRefreshJob(p, githubStatusRefreshConfig{})
}

// NewGithubStatusReconcileJob re-sends GitHub statuses for a finished patch,
// posts a terminal success for required evergreen contexts that have no
// matching build, and retries while any evergreen context is still pending on
// GitHub. delay defers the first attempt so in-flight status notifications can
// land first; a non-zero delay also uses a stable job ID so only one reconcile
// is queued per version.
func NewGithubStatusReconcileJob(p *patch.Patch, delay time.Duration) amboy.Job {
	return newGithubStatusRefreshJob(p, githubStatusRefreshConfig{reconcile: true, delay: delay})
}

func newGithubStatusRefreshJob(p *patch.Patch, cfg githubStatusRefreshConfig) amboy.Job {
	job := makeGithubStatusRefreshJob()
	job.FetchID = p.Version
	job.patch = p
	job.ReconcileGitHub = cfg.reconcile

	if cfg.reconcile && cfg.delay > 0 {
		job.SetID(fmt.Sprintf("%s:reconcile:%s", githubStatusRefreshJobName, p.Version))
		job.SetScopes([]string{fmt.Sprintf("%s.%s", githubStatusRefreshJobName, p.Version)})
		job.SetEnqueueAllScopes(true)
		job.SetTimeInfo(amboy.JobTimeInfo{WaitUntil: time.Now().Add(cfg.delay)})
	} else {
		job.SetID(fmt.Sprintf("%s:%s-%s", githubStatusRefreshJobName, p.Version, time.Now().String()))
	}

	if cfg.reconcile {
		job.UpdateRetryInfo(amboy.JobRetryOptions{
			Retryable:   utility.TruePtr(),
			MaxAttempts: utility.ToIntPtr(githubStatusReconcileMaxAttempts),
			WaitUntil:   utility.ToTimeDurationPtr(githubStatusReconcileRetryWait),
		})
	}
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

	// Optional overrides for tests. Nil means use the production GitHub helpers.
	requiredEvergreenContexts    func(ctx context.Context, owner, repo, branch string) []string
	listPendingEvergreenStatuses func(ctx context.Context, owner, repo, ref string) ([]string, error)

	FetchID         string `bson:"fetch_id" json:"fetch_id" yaml:"fetch_id"`
	ReconcileGitHub bool   `bson:"reconcile_github" json:"reconcile_github" yaml:"reconcile_github"`
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

	owner, repo, _ := j.patch.GitHubStatusTarget()
	j.sender, err = j.env.GetGitHubSender(owner, repo, githubapp.CreateGitHubAppAuth(j.env.Settings()).CreateGitHubSenderInstallationToken)
	if err != nil {
		return err
	}

	j.builds, err = build.Find(ctx, build.ByVersion(j.FetchID))
	if err != nil {
		return errors.Wrap(err, "finding builds")
	}

	if len(j.patch.Triggers.ChildPatches) > 0 {
		j.childPatches, err = patch.Find(ctx, patch.ByStringIds(j.patch.Triggers.ChildPatches))
		if err != nil {
			return errors.Wrap(err, "finding child patches")
		}
	}
	return nil
}

func (j *githubStatusRefreshJob) baseStatus() *message.GithubStatus {
	owner, repo, ref := j.patch.GitHubStatusTarget()
	return &message.GithubStatus{
		Owner: owner,
		Repo:  repo,
		Ref:   ref,
	}
}

func (j *githubStatusRefreshJob) sendStatus(ctx context.Context, status *message.GithubStatus) {
	c := message.MakeGithubStatusMessageWithRepo(*status)
	if !c.Loggable() {
		j.AddError(errors.Errorf("status message is invalid: %+v", status))
		return
	}
	j.AddError(c.SetPriority(level.Notice))

	j.sender.Send(ctx, c)
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

	status := j.baseStatus()

	for _, childPatch := range j.childPatches {
		projectIdentifier, err := model.GetIdentifierForProject(ctx, childPatch.Project)
		if err != nil {
			return errors.Wrap(err, "finding project identifier")
		}
		status.Context, err = patch.GetGithubContextForChildPatch(projectIdentifier, j.patch, &childPatch)
		if err != nil {
			return errors.Wrapf(err, "getting github context for child patch '%s'", childPatch.Id.Hex())
		}

		status.URL = childPatch.GetURL(j.urlBase)
		status.State, status.Description = getGithubStateAndDescriptionForPatch(&childPatch)
		j.sendStatus(ctx, status)
	}
	return nil
}

func getGithubStateAndDescriptionForPatch(p *patch.Patch) (message.GithubState, string) {
	var state message.GithubState
	if p.Status == evergreen.VersionSucceeded {
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

func (j *githubStatusRefreshJob) sendBuildStatuses(ctx context.Context) {
	status := j.baseStatus()
	for _, b := range j.builds {
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
		status.Description = b.GetPRNotificationDescription(tasks)

		j.sendStatus(ctx, status)
	}
}

func (j *githubStatusRefreshJob) scheduledGitHubContexts(ctx context.Context) map[string]struct{} {
	scheduled := map[string]struct{}{
		thirdparty.GithubStatusDefaultContext: {},
	}
	for _, b := range j.builds {
		scheduled[fmt.Sprintf("%s/%s", thirdparty.GithubStatusDefaultContext, b.BuildVariant)] = struct{}{}
	}
	for i := range j.childPatches {
		projectIdentifier, err := model.GetIdentifierForProject(ctx, j.childPatches[i].Project)
		if err != nil {
			j.AddError(errors.Wrap(err, "finding project identifier for child patch"))
			continue
		}
		context, err := patch.GetGithubContextForChildPatch(projectIdentifier, j.patch, &j.childPatches[i])
		if err != nil {
			j.AddError(errors.Wrapf(err, "getting github context for child patch '%s'", j.childPatches[i].Id.Hex()))
			continue
		}
		scheduled[context] = struct{}{}
	}
	return scheduled
}

func (j *githubStatusRefreshJob) sendUnscheduledRequiredStatuses(ctx context.Context, contexts []string, scheduled map[string]struct{}) {
	status := j.baseStatus()
	status.URL = j.patch.GetURL(j.urlBase)
	status.State = message.GithubStateSuccess
	status.Description = unscheduledGitHubVariant

	for _, context := range contexts {
		if context == "" {
			continue
		}
		if _, ok := scheduled[context]; ok {
			continue
		}
		status.Context = context
		j.sendStatus(ctx, status)
		scheduled[context] = struct{}{}
	}
}

func (j *githubStatusRefreshJob) requiredContexts(ctx context.Context, owner, repo, branch string) []string {
	if j.requiredEvergreenContexts != nil {
		return j.requiredEvergreenContexts(ctx, owner, repo, branch)
	}
	return thirdparty.GetEvergreenRequiredStatusContexts(ctx, owner, repo, branch)
}

func (j *githubStatusRefreshJob) pendingContexts(ctx context.Context, owner, repo, ref string) ([]string, error) {
	if j.listPendingEvergreenStatuses != nil {
		return j.listPendingEvergreenStatuses(ctx, owner, repo, ref)
	}
	return thirdparty.GetPendingEvergreenCommitStatusContexts(ctx, owner, repo, ref)
}

func (j *githubStatusRefreshJob) reconcileGitHubStatuses(ctx context.Context) {
	if !j.ReconcileGitHub || j.patch == nil || !evergreen.IsFinishedVersionStatus(j.patch.Status) {
		return
	}

	owner, repo, ref := j.patch.GitHubStatusTarget()
	scheduled := j.scheduledGitHubContexts(ctx)

	required := j.requiredContexts(ctx, owner, repo, j.patch.GitHubStatusBranch())
	j.sendUnscheduledRequiredStatuses(ctx, required, scheduled)

	pending, err := j.pendingContexts(ctx, owner, repo, ref)
	if err != nil {
		grip.Error(ctx, message.WrapError(err, message.Fields{
			"job":      j.ID(),
			"job_type": j.Type().Name,
			"message":  "failed to list pending GitHub commit statuses",
			"owner":    owner,
			"repo":     repo,
			"ref":      ref,
			"patch_id": j.FetchID,
		}))
		j.AddRetryableError(errors.Wrap(err, "listing pending GitHub commit statuses"))
		return
	}

	j.sendUnscheduledRequiredStatuses(ctx, pending, scheduled)

	if len(pending) == 0 {
		return
	}

	// Anything still pending after we posted terminal statuses for unscheduled
	// contexts is either a missed send for a real build or GitHub lag. Retry
	// so a later attempt can confirm the statuses landed.
	j.AddRetryableError(errors.Errorf("GitHub still pending for evergreen contexts: %s", strings.Join(pending, ", ")))
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

	status := j.baseStatus()
	status.URL = j.patch.GetURL(j.urlBase)
	status.Context = thirdparty.GithubStatusDefaultContext
	status.State, status.Description = getGithubStateAndDescriptionForPatch(j.patch)

	// Send patch status
	j.sendStatus(ctx, status)

	// Send child patch statuses.
	if err := j.sendChildPatchStatuses(ctx); err != nil {
		j.AddError(err)
		return
	}

	// For each build, send build status.
	j.sendBuildStatuses(ctx)

	j.reconcileGitHubStatuses(ctx)
}
