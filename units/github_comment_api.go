package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
)

const (
	githubCommentUpdateJobName = "github-comment"
)

func init() {
	registry.AddJobType(githubCommentUpdateJobName, func() amboy.Job { return makeGitHubCommentJob() })
}

type githubCommentJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	env      evergreen.Environment
	sender   send.Sender

	Owner   string `bson:"owner" json:"owner" yaml:"owner"`
	Repo    string `bson:"repo" json:"repo" yaml:"repo"`
	PRId    int    `bson:"pr_id" json:"pr_id" yaml:"pr_id"`
	Message string `bson:"description" json:"description" yaml:"description"`
}

func makeGitHubCommentJob() *githubCommentJob {
	j := &githubCommentJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    githubCommentUpdateJobName,
				Version: 0,
			},
		},
	}
	return j
}

// NewGitHubCommentJob creates a job to send a comment to Github PR with a message.
func NewGitHubCommentJob(owner, repo string, prId int, message string) amboy.Job {
	job := makeGitHubCommentJob()
	job.Owner = owner
	job.Repo = repo
	job.Message = message

	job.SetID(fmt.Sprintf("%s:%s", githubCommentUpdateJobName, time.Now().String()))

	return job
}

func (j *githubCommentJob) setSender(owner, repo string, prId int) error {
	var err error
	j.sender, err = j.env.GetGitHubCommentSender(owner, repo, prId)
	return err
}

func (j *githubCommentJob) Run(ctx context.Context) {
	defer j.MarkComplete()
	j.env = evergreen.GetEnvironment()

	if err := j.setSender(j.Owner, j.Repo, j.PRId); err != nil {
		j.AddError(err)
		return
	}

	j.sender.Send(message.NewBytes([]byte(j.Message)))
	grip.Info(message.Fields{
		"ticket":  thirdparty.GithubInvestigation,
		"message": "called github comment send",
		"caller":  githubCommentUpdateJobName,
	})
}
