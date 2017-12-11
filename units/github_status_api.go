package units

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/google/go-github/github"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/message"
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

type githubStatusUpdateJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	logger   grip.Journaler
	env      evergreen.Environment

	FetchID    string `bson:"fetch_id" json:"fetch_id" yaml:"fetch_id"`
	UpdateType string `bson:"update_type" json:"update_type" yaml:"update_type"`

	owner    string
	repo     string
	prNumber int
	ref      string

	urlPath     string
	description string
	context     string
	ghStatus    string
}

func makeGithubStatusUpdateJob() *githubStatusUpdateJob {
	return &githubStatusUpdateJob{
		env:    evergreen.GetEnvironment(),
		logger: logging.MakeGrip(grip.GetSender()),
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

	job.SetID(fmt.Sprintf("%s:%s-%s-%s-%d", githubStatusUpdateJobName, job.UpdateType, buildID, job.context, time.Now()))
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

	job.SetID(fmt.Sprintf("%s:%s-%s-%s-%d", githubStatusUpdateJobName, job.UpdateType, version, job.context, time.Now()))

	return job
}

func (j *githubStatusUpdateJob) sendStatusUpdate() error {
	if j.env.Settings() == nil || j.env.Settings().Ui.Url == "" {
		return errors.New("ui not configured")
	}
	evergreenBaseURL := j.env.Settings().Ui.Url

	githubOauthToken, err := j.env.Settings().GetGithubOauthToken()
	if err != nil {
		return err
	}

	token := strings.Split(githubOauthToken, " ")
	if len(token) != 2 || token[0] != "token" {
		return errors.New("github token format expected to be 'token [oauthtoken]'")
	}

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token[1]},
	)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	newStatus := github.RepoStatus{
		TargetURL:   github.String(fmt.Sprintf("%s%s", evergreenBaseURL, j.urlPath)),
		Context:     github.String(j.context),
		Description: github.String(j.description),
		State:       github.String(j.ghStatus),
	}
	status, resp, err := client.Repositories.CreateStatus(ctx, j.owner, j.repo, j.ref, &newStatus)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		repo := repoReference(j.owner, j.repo, j.prNumber, j.ref)
		return errors.Errorf("Github status creation for %s: expected http Status code: 201 Created, got %s", repo, http.StatusText(http.StatusCreated))
	}
	if status == nil {
		return errors.New("nil response from github")
	}

	return nil
}

func (j *githubStatusUpdateJob) fetch() error {
	patchVersion := j.FetchID
	if j.UpdateType == "build" {
		b, err := build.FindOne(build.ById(j.FetchID))
		if err != nil {
			return err
		}
		if b == nil {
			return errors.New("can't find build")
		}

		patchVersion = b.Version
		j.context = fmt.Sprintf("evergreen-%s", b.BuildVariant)
		j.description = taskStatusToDesc(b)
		j.urlPath = fmt.Sprintf("/build/%s", b.Id)

		switch b.Status {
		case evergreen.BuildSucceeded:
			j.ghStatus = githubStatusSuccess

		case evergreen.BuildFailed:
			j.ghStatus = githubStatusFailure

		default:
			return errors.New("build status is pending; refusing to update status")
		}
	}

	patchDoc, err := patch.FindOne(patch.ByVersion(patchVersion))
	if err != nil {
		return err
	}
	if patchDoc == nil {
		return errors.New("can't find patch")
	}

	if j.UpdateType == "patch" {
		j.urlPath = fmt.Sprintf("/version/%s", patchVersion)
		j.context = "evergreen"

		switch patchDoc.Status {
		case evergreen.PatchSucceeded:
			j.ghStatus = githubStatusSuccess
			j.description = fmt.Sprintf("finished in %s", patchDoc.FinishTime.Sub(patchDoc.StartTime).String())

		case evergreen.PatchFailed:
			j.ghStatus = githubStatusFailure
			j.description = fmt.Sprintf("finished in %s", patchDoc.FinishTime.Sub(patchDoc.StartTime).String())

		case evergreen.PatchCreated:
			j.ghStatus = githubStatusPending
			j.description = "preparing to run tasks"

		case evergreen.PatchStarted:
			j.ghStatus = githubStatusPending
			j.description = "tasks are running"

		default:
			return errors.New("unknown patch status")
		}
	}

	j.owner = patchDoc.GithubPatchData.BaseOwner
	j.repo = patchDoc.GithubPatchData.BaseRepo
	j.prNumber = patchDoc.GithubPatchData.PRNumber
	j.ref = patchDoc.GithubPatchData.HeadHash

	return nil
}

func (j *githubStatusUpdateJob) Run() {
	if err := j.fetch(); err != nil {
		j.AddError(err)
		j.MarkComplete()
		return
	}

	j.AddError(j.sendStatusUpdate())
	if j.HasErrors() {
		j.logger.Error(message.Fields{
			"message": "github API failure",
			"error":   j.Error(),
		})
		return
	}

	j.MarkComplete()
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
