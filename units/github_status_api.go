package units

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/google/go-github/github"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
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

	githubUpdateTypeBuild            = "build"
	githubUpdateTypePatchWithVersion = "patch-with-version"
	githubUpdateTypeRequestAuth      = "request-auth"
	githubUpdateTypeBadConfig        = "bad-config"
)

func init() {
	registry.AddJobType(githubStatusUpdateJobName, func() amboy.Job { return makeGithubStatusUpdateJob() })
}

type githubStatus struct {
	Owner    string `json:"owner"`
	Repo     string `json:"repo"`
	PRNumber int    `json:"pr_number"`
	Ref      string `json:"ref"`
	URLPath  string `json:"url_path"`

	Description string `json:"description"`
	Context     string `json:"context"`
	State       string `json:"state"`
}

func (status *githubStatus) Valid() bool {
	if status.Owner == "" || status.Repo == "" || status.PRNumber == 0 ||
		status.Ref == "" || status.URLPath == "" || status.Context == "" ||
		!strings.HasPrefix(status.URLPath, "/") {
		return false
	}

	switch status.State {
	case githubStatusError, githubStatusFailure, githubStatusPending, githubStatusSuccess:
		return true
	}

	return false
}

type githubStatusUpdateJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	env      evergreen.Environment

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
	return j
}

// NewGithubStatusUpdateJobForBuild creates a job to update github's API from a Build.
// Status will be reported as 'evergreen-[build variant name]'
func NewGithubStatusUpdateJobForBuild(buildID string) amboy.Job {
	job := makeGithubStatusUpdateJob()
	job.FetchID = buildID
	job.UpdateType = githubUpdateTypeBuild

	job.SetID(fmt.Sprintf("%s:%s-%s-%s", githubStatusUpdateJobName, job.UpdateType, buildID, time.Now().String()))
	return job
}

// NewGithubStatusUpdateJobForPatchWithVersion creates a job to update github's API
// from a Patch with specified version. Status will be reported as 'evergreen'
func NewGithubStatusUpdateJobForPatchWithVersion(version string) amboy.Job {
	job := makeGithubStatusUpdateJob()
	job.FetchID = version
	job.UpdateType = githubUpdateTypePatchWithVersion

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

func NewGithubStatusUpdateJobForBadConfig(intentID string) amboy.Job {
	job := makeGithubStatusUpdateJob()
	job.FetchID = intentID
	job.UpdateType = githubUpdateTypeBadConfig

	job.SetID(fmt.Sprintf("%s:%s-%s-%s", githubStatusUpdateJobName, job.UpdateType, intentID, time.Now().String()))

	return job
}

func (j *githubStatusUpdateJob) sendStatusUpdate(ctx context.Context, status *githubStatus) error {
	catcher := grip.NewBasicCatcher()

	if !status.Valid() {
		catcher.Add(errors.New("status is invalid"))
	}

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	if j.env.Settings() == nil || j.env.Settings().Ui.Url == "" {
		catcher.Add(errors.New("ui not configured"))
	}
	evergreenBaseURL := j.env.Settings().Ui.Url

	githubOauthToken, err := j.env.Settings().GetGithubOauthToken()
	if err != nil {
		catcher.Add(err)
	}

	err = catcher.Resolve()
	if err != nil {
		return err
	}

	httpClient, err := util.GetHttpClientForOauth2(githubOauthToken)
	if err != nil {
		return err
	}
	defer util.PutHttpClientForOauth2(httpClient)
	client := github.NewClient(httpClient)

	newStatus := github.RepoStatus{
		TargetURL:   github.String(fmt.Sprintf("%s%s", evergreenBaseURL, status.URLPath)),
		Context:     github.String(status.Context),
		Description: github.String(status.Description),
		State:       github.String(status.State),
	}

	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	respStatus, resp, err := client.Repositories.CreateStatus(ctx, status.Owner, status.Repo, status.Ref, &newStatus)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		repo := repoReference(status.Owner, status.Repo, status.PRNumber, status.Ref)
		return errors.Errorf("Github status creation for %s: expected http Status code: 201 Created, got %s", repo, http.StatusText(http.StatusCreated))
	}
	if respStatus == nil {
		return errors.New("nil response from github")
	}

	return nil
}

func (j *githubStatusUpdateJob) fetch(status *githubStatus) (err error) {
	var patchDoc *patch.Patch
	patchVersion := j.FetchID

	if j.UpdateType == githubUpdateTypeBadConfig {
		intent, err := patch.FindIntent(j.FetchID, patch.GithubIntentType)
		if err != nil {
			return errors.Wrap(err, "can't fetch patch intent")
		}
		patchDoc = intent.NewPatch()
		if patchDoc == nil {
			return errors.New("patch is missing")
		}

		projectRef, err := model.FindOneProjectRefByRepoAndBranchWithPRTesting(patchDoc.GithubPatchData.BaseOwner,
			patchDoc.GithubPatchData.BaseRepo, patchDoc.GithubPatchData.BaseBranch)
		if err != nil {
			return errors.Wrap(err, "can't fetch project ref")
		}
		if projectRef == nil {
			return errors.New("can't find project ref")
		}

		status.URLPath = fmt.Sprintf("/waterfall/%s", projectRef.Identifier)
		status.Context = "evergreen"
		status.State = githubStatusFailure
		status.Description = "project config was invalid"

	} else if j.UpdateType == githubUpdateTypeBuild {
		b, err := build.FindOne(build.ById(j.FetchID))
		if err != nil {
			return err
		}
		if b == nil {
			return errors.New("can't find build")
		}

		patchVersion = b.Version
		status.Context = fmt.Sprintf("evergreen/%s", b.BuildVariant)
		status.Description = taskStatusToDesc(b)
		status.URLPath = fmt.Sprintf("/build/%s", b.Id)

		switch b.Status {
		case evergreen.BuildSucceeded:
			status.State = githubStatusSuccess

		case evergreen.BuildFailed:
			status.State = githubStatusFailure

		default:
			return errors.New("build status is pending; refusing to update status")
		}
	}

	if patchDoc == nil {
		patchDoc, err = patch.FindOne(patch.ById(bson.ObjectIdHex(patchVersion)))
		if err != nil {
			return err
		}
		if patchDoc == nil {
			return errors.New("can't find patch")
		}
	}

	if j.UpdateType == githubUpdateTypePatchWithVersion {
		status.URLPath = fmt.Sprintf("/version/%s", patchVersion)
		status.Context = "evergreen"

		switch patchDoc.Status {
		case evergreen.PatchSucceeded:
			status.State = githubStatusSuccess
			status.Description = fmt.Sprintf("patch finished in %s", patchDoc.FinishTime.Sub(patchDoc.StartTime).String())

		case evergreen.PatchFailed:
			status.State = githubStatusFailure
			status.Description = fmt.Sprintf("patch finished in %s", patchDoc.FinishTime.Sub(patchDoc.StartTime).String())

		case evergreen.PatchCreated:
			status.State = githubStatusPending
			status.Description = "preparing to run tasks"

		case evergreen.PatchStarted:
			status.State = githubStatusPending
			status.Description = "tasks are running"

		default:
			return errors.New("unknown patch status")
		}

	} else if j.UpdateType == githubUpdateTypeRequestAuth {
		status.URLPath = fmt.Sprintf("/patch/%s", patchVersion)
		status.Context = "evergreen"
		status.Description = "patch must be manually authorized"
		status.State = githubStatusFailure
	}

	status.Owner = patchDoc.GithubPatchData.BaseOwner
	status.Repo = patchDoc.GithubPatchData.BaseRepo
	status.PRNumber = patchDoc.GithubPatchData.PRNumber
	status.Ref = patchDoc.GithubPatchData.HeadHash
	return nil
}

func (j *githubStatusUpdateJob) Run() {
	defer j.MarkComplete()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		j.AddError(errors.Wrap(err, "error retrieving admin settings"))
		return
	}
	if flags.GithubStatusAPIDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"job":     githubStatusUpdateJobName,
			"message": "github status updates are disabled, not updating status",
		})
		j.AddError(errors.New("github status updates are disabled, not updating status"))
		return
	}

	status := githubStatus{}
	if err := j.fetch(&status); err != nil {
		j.AddError(err)
		return
	}

	if err := j.sendStatusUpdate(ctx, &status); err != nil {
		grip.Alert(message.WrapError(err, message.Fields{
			"message":     "github API failure",
			"source":      "status updates",
			"job":         j.ID(),
			"status":      status,
			"fetch_id":    j.FetchID,
			"update_type": j.UpdateType,
		}))
		j.AddError(err)
	}
}

func taskStatusToDesc(b *build.Build) string {
	success := 0
	failed := 0
	systemError := 0
	other := 0
	noReport := 0
	for _, task := range b.Tasks {
		switch task.Status {
		case evergreen.TaskSucceeded:
			success++

		case evergreen.TaskFailed:
			failed++

		case evergreen.TaskSystemFailed, evergreen.TaskTimedOut,
			evergreen.TaskSystemUnresponse, evergreen.TaskSystemTimedOut,
			evergreen.TaskTestTimedOut:
			systemError++

		case evergreen.TaskStarted, evergreen.TaskUnstarted,
			evergreen.TaskUndispatched, evergreen.TaskDispatched,
			evergreen.TaskConflict, evergreen.TaskInactive:
			noReport++

		default:
			other++
		}
	}

	grip.ErrorWhen(other > 0, message.Fields{
		"source":   "status updates",
		"message":  "unknown task status",
		"build_id": b.Id,
	})
	grip.ErrorWhen(noReport > 0, message.Fields{
		"source":   "status updates",
		"message":  "updating status for incomplete build",
		"build_id": b.Id,
	})

	if success == 0 && failed == 0 && systemError == 0 && other == 0 {
		return "no tasks were run"
	}

	desc := fmt.Sprintf("%s, %s", taskStatusSubformat(success, "succeeded"),
		taskStatusSubformat(failed, "failed"))
	if systemError > 0 {
		desc += fmt.Sprintf(", %d internal errors", systemError)
	}
	if other > 0 {
		desc += fmt.Sprintf(", %d other", other)
	}

	return appendTime(b, desc)
}

func taskStatusSubformat(n int, verb string) string {
	if n == 0 {
		return fmt.Sprintf("none %s", verb)
	}
	return fmt.Sprintf("%d %s", n, verb)
}

func repoReference(owner, repo string, prNumber int, ref string) string {
	return fmt.Sprintf("%s/%s#%d@%s", owner, repo, prNumber, ref)
}

func appendTime(b *build.Build, txt string) string {
	return fmt.Sprintf("%s in %s", txt, b.FinishTime.Sub(b.StartTime).String())
}
