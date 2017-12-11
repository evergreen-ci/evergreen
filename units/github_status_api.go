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
	"github.com/k0kubun/pp"
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
)

func init() {
	registry.AddJobType(githubStatusUpdateJobName, func() amboy.Job { return makeGithubStatusUpdateJob() })
}

type githubStatusUpdateJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	logger   grip.Journaler
	env      evergreen.Environment

	VersionID string `bson:"build_id" json:"build_id" yaml:"build_id"`
	Owner     string `bson:"owner" json:"owner" yaml:"owner"`
	Repo      string `bson:"repo" json:"repo" yaml:"repo"`
	PRNumber  int    `bson:"pr_number" json:"pr_number" yaml:"pr_number"`
	Ref       string `bson:"ref" json:"ref" yaml:"ref"`

	URLPath     string `bson:"url_path" json:"url_path" yaml:"url_path"`
	Description string `bson:"description" json:"description" yaml:"description"`
	Context     string `bson:"context" json:"context" yaml:"context"`
	GHStatus    string `bson:"gh_status" json:"gh_status" yaml:"gh_status"`
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
func NewGithubStatusUpdateJobForBuild(b *build.Build) amboy.Job {
	job := makeGithubStatusUpdateJob()
	job.VersionID = b.Version
	job.URLPath = fmt.Sprintf("/build/%s", b.Id)
	job.Context = fmt.Sprintf("evergreen-%s", b.BuildVariant)
	job.Description = taskStatusToDesc(b)

	switch b.Status {
	case evergreen.BuildSucceeded:
		job.GHStatus = githubStatusSuccess

	case evergreen.BuildFailed:
		job.GHStatus = githubStatusFailure

	default:
		job.GHStatus = githubStatusPending
	}

	job.SetID(fmt.Sprintf("%s:build-%s-%s-%d", githubStatusUpdateJobName, b.Id, job.Context, time.Now()))
	return job
}

func repoReference(owner, repo string, prNumber int, ref string) string {
	return fmt.Sprintf("%s/%s#%d@%s", owner, repo, prNumber, ref)
}

// NewGithubStatusUpdateForPatch creates a job to update github's API from a
// Patch. Status will be reported as 'evergreen'
func NewGithubStatusUpdateJobForPatch(patchDoc *patch.Patch) amboy.Job {
	job := makeGithubStatusUpdateJob()
	job.Owner = patchDoc.GithubPatchData.BaseOwner
	job.Repo = patchDoc.GithubPatchData.BaseRepo
	job.PRNumber = patchDoc.GithubPatchData.PRNumber
	job.Ref = patchDoc.GithubPatchData.HeadHash

	job.URLPath = fmt.Sprintf("/version/%s", patchDoc.Version)
	job.Context = "evergreen"

	switch patchDoc.Status {
	case evergreen.PatchSucceeded:
		job.GHStatus = githubStatusSuccess
		job.Description = fmt.Sprintf("finished in %s", patchDoc.FinishTime.Sub(patchDoc.StartTime).String())

	case evergreen.PatchFailed:
		job.GHStatus = githubStatusFailure
		job.Description = fmt.Sprintf("finished in %s", patchDoc.FinishTime.Sub(patchDoc.StartTime).String())

	default:
		job.GHStatus = githubStatusPending
	}
	job.SetID(fmt.Sprintf("%s:version-%s-%s-%d", githubStatusUpdateJobName, patchDoc.Id, job.Context, time.Now()))

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
		TargetURL:   github.String(fmt.Sprintf("%s%s", evergreenBaseURL, j.URLPath)),
		Context:     github.String(j.Context),
		Description: github.String(j.Description),
		State:       github.String(j.GHStatus),
	}
	status, resp, err := client.Repositories.CreateStatus(ctx, j.Owner, j.Repo, j.Ref, &newStatus)
	if err != nil {
		return err
	}
	pp.Println(status, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		repo := repoReference(j.Owner, j.Repo, j.PRNumber, j.Ref)
		return errors.Errorf("Github status creation for %s: expected http Status code: 201 Created, got %s", repo, http.StatusText(http.StatusCreated))
	}
	if status == nil {
		return errors.New("nil response from github")
	}

	return nil
}

func (j *githubStatusUpdateJob) fetch() error {
	if j.GHStatus != githubStatusPending && j.GHStatus != githubStatusSuccess &&
		j.GHStatus != githubStatusFailure && j.GHStatus != githubStatusError {
		return errors.New("Invalid status")
	}

	if j.VersionID != "" {
		patchDoc, err := patch.FindOne(patch.ByVersion(j.VersionID))
		if err != nil {
			return err
		}
		if patchDoc == nil {
			return errors.New("can't find patch")
		}
		j.Owner = patchDoc.GithubPatchData.BaseOwner
		j.Repo = patchDoc.GithubPatchData.BaseRepo
		j.PRNumber = patchDoc.GithubPatchData.PRNumber
		j.Ref = patchDoc.GithubPatchData.HeadHash
	}

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
