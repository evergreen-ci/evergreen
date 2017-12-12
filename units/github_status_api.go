package units

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/admin"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/google/go-github/github"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
)

const (
	githubStatusUpdateJobName = "github-status-update"
	githubStatusError         = "error"
	githubStatusFailure       = "failure"
	githubStatusPending       = "pending"
	githubStatusSuccess       = "success"

	githubUpdateTypeBuild = "build"
	githubUpdateTypePatch = "patch"
)

func init() {
	registry.AddJobType(githubStatusUpdateJobName, func() amboy.Job { return makeGithubStatusUpdateJob() })
}

type githubStatus struct {
	owner    string
	repo     string
	prNumber int
	ref      string

	urlPath     string
	description string
	context     string
	ghStatus    string
}

func (status *githubStatus) Valid() bool {
	if status.owner == "" || status.repo == "" || status.prNumber == 0 ||
		status.ref == "" || status.urlPath == "" || status.context == "" {
		return false
	}
	if status.ghStatus != githubStatusError && status.ghStatus != githubStatusFailure &&
		status.ghStatus != githubStatusPending && status.ghStatus != githubStatusSuccess {
		return false
	}

	return true
}

type githubStatusUpdateJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	env      evergreen.Environment

	FetchID    string `bson:"fetch_id" json:"fetch_id" yaml:"fetch_id"`
	UpdateType string `bson:"update_type" json:"update_type" yaml:"update_type"`
}

func makeGithubStatusUpdateJob() *githubStatusUpdateJob {
	return &githubStatusUpdateJob{
		env: evergreen.GetEnvironment(),
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    githubStatusUpdateJobName,
				Version: 0,
				Format:  amboy.BSON,
			},
		},
	}
}

// NewGithubStatusUpdateJobForBuild creates a job to update github's API from a Build.
// Status will be reported as 'evergreen-[build variant name]'
func NewGithubStatusUpdateJobForBuild(buildID string) amboy.Job {
	job := makeGithubStatusUpdateJob()
	job.FetchID = buildID
	job.UpdateType = githubUpdateTypeBuild

	job.SetID(fmt.Sprintf("%s:%s-%s-%d", githubStatusUpdateJobName, job.UpdateType, buildID, time.Now()))
	return job
}

func repoReference(owner, repo string, prNumber int, ref string) string {
	return fmt.Sprintf("%s/%s#%d@%s", owner, repo, prNumber, ref)
}

// NewGithubStatusUpdateForPatch creates a job to update github's API from a
// Patch. Status will be reported as 'evergreen'
func NewGithubStatusUpdateJobForPatch(version string) amboy.Job {
	job := makeGithubStatusUpdateJob()
	job.FetchID = version
	job.UpdateType = githubUpdateTypePatch

	job.SetID(fmt.Sprintf("%s:%s-%s-%d", githubStatusUpdateJobName, job.UpdateType, version, time.Now()))

	return job
}

func (j *githubStatusUpdateJob) sendStatusUpdate(status *githubStatus) {
	if !status.Valid() {
		j.AddError(errors.New("status is invalid"))
	}
	if j.env.Settings() == nil || j.env.Settings().Ui.Url == "" {
		j.AddError(errors.New("ui not configured"))
	}
	evergreenBaseURL := j.env.Settings().Ui.Url

	githubOauthToken, err := j.env.Settings().GetGithubOauthToken()
	if err != nil {
		j.AddError(err)
	}

	token := strings.Split(githubOauthToken, " ")
	if len(token) != 2 || token[0] != "token" {
		j.AddError(errors.New("github token format expected to be 'token [oauthtoken]'"))
	}
	if j.HasErrors() {
		return
	}

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token[1]},
	)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	newStatus := github.RepoStatus{
		TargetURL:   github.String(fmt.Sprintf("%s%s", evergreenBaseURL, status.urlPath)),
		Context:     github.String(status.context),
		Description: github.String(status.description),
		State:       github.String(status.ghStatus),
	}
	respStatus, resp, err := client.Repositories.CreateStatus(ctx, status.owner, status.repo, status.ref, &newStatus)
	if err != nil {
		j.AddError(err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		repo := repoReference(status.owner, status.repo, status.prNumber, status.ref)
		j.AddError(errors.Errorf("Github status creation for %s: expected http Status code: 201 Created, got %s", repo, http.StatusText(http.StatusCreated)))
	}
	if respStatus == nil {
		j.AddError(errors.New("nil response from github"))
	}
}

func (j *githubStatusUpdateJob) fetch(status *githubStatus) {
	patchVersion := j.FetchID
	if j.UpdateType == "build" {
		b, err := build.FindOne(build.ById(j.FetchID))
		if err != nil {
			j.AddError(err)
		}
		if b == nil {
			j.AddError(errors.New("can't find build"))
		}
		if j.HasErrors() {
			return
		}

		patchVersion = b.Version
		status.context = fmt.Sprintf("evergreen-%s", b.BuildVariant)
		status.description = taskStatusToDesc(b)
		status.urlPath = fmt.Sprintf("/build/%s", b.Id)

		switch b.Status {
		case evergreen.BuildSucceeded:
			status.ghStatus = githubStatusSuccess

		case evergreen.BuildFailed:
			status.ghStatus = githubStatusFailure

		default:
			j.AddError(errors.New("build status is pending; refusing to update status"))
			return
		}
	}

	patchDoc, err := patch.FindOne(patch.ByVersion(patchVersion))
	if err != nil {
		j.AddError(err)
	}
	if patchDoc == nil {
		j.AddError(errors.New("can't find patch"))
	}
	if j.HasErrors() {
		return
	}

	if j.UpdateType == "patch" {
		status.urlPath = fmt.Sprintf("/version/%s", patchVersion)
		status.context = "evergreen"

		switch patchDoc.Status {
		case evergreen.PatchSucceeded:
			status.ghStatus = githubStatusSuccess
			status.description = fmt.Sprintf("finished in %s", patchDoc.FinishTime.Sub(patchDoc.StartTime).String())

		case evergreen.PatchFailed:
			status.ghStatus = githubStatusFailure
			status.description = fmt.Sprintf("finished in %s", patchDoc.FinishTime.Sub(patchDoc.StartTime).String())

		case evergreen.PatchCreated:
			status.ghStatus = githubStatusPending
			status.description = "preparing to run tasks"

		case evergreen.PatchStarted:
			status.ghStatus = githubStatusPending
			status.description = "tasks are running"

		default:
			j.AddError(errors.New("unknown patch status"))
			return
		}
	}

	status.owner = patchDoc.GithubPatchData.BaseOwner
	status.repo = patchDoc.GithubPatchData.BaseRepo
	status.prNumber = patchDoc.GithubPatchData.PRNumber
	status.ref = patchDoc.GithubPatchData.HeadHash
}

func (j *githubStatusUpdateJob) Run() {
	defer j.MarkComplete()

	adminSettings, err := admin.GetSettings()
	if err != nil {
		j.AddError(errors.Wrap(err, "error retrieving admin settings"))
	}
	if adminSettings.ServiceFlags.GithubPRTestingDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"job":     githubStatusUpdateJobName,
			"message": "github pr testing is disabled, not updating status",
		})
		j.AddError(errors.New("github pr testing is disabled, not updating status"))
		return
	}

	status := githubStatus{}
	j.fetch(&status)
	if j.HasErrors() {
		return
	}

	j.sendStatusUpdate(&status)
	if j.HasErrors() {
		grip.Alert(message.Fields{
			"message": "github API failure",
			"error":   j.Error(),
		})
		return
	}
}

func taskStatusToDesc(b *build.Build) string {
	success := 0
	failed := 0
	systemError := 0
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
		}
	}

	if success == 0 && failed == 0 && systemError == 0 {
		return "no tasks were run"
	}

	if success != 0 && failed == 0 && systemError == 0 {
		return "all tasks succceeded!"
	}
	if success == 0 && (failed != 0 || systemError != 0) {
		return "all tasks failed!"
	}

	desc := fmt.Sprintf("%s, %s", taskStatusSubformat(success, "succeeded"),
		taskStatusSubformat(failed, "failed"))
	if systemError > 0 {
		desc += fmt.Sprintf(", %d internal errors", systemError)
	}

	return desc
}

func taskStatusSubformat(n int, verb string) string {
	if n == 0 {
		return fmt.Sprintf("none %s", verb)
	}
	return fmt.Sprintf("%d %s, ", n, verb)
}
