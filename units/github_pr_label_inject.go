package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const githubPRLabelInjectJobName = "github-pr-label-inject"

func init() {
	registry.AddJobType(githubPRLabelInjectJobName, func() amboy.Job { return makeGithubPRLabelInjectJob() })
}

type githubPRLabelInjectJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	Owner    string   `bson:"owner" json:"owner" yaml:"owner"`
	Repo     string   `bson:"repo" json:"repo" yaml:"repo"`
	PRNumber int      `bson:"pr_number" json:"pr_number" yaml:"pr_number"`
	HeadSHA  string   `bson:"head_sha" json:"head_sha" yaml:"head_sha"`
	Labels   []string `bson:"labels" json:"labels" yaml:"labels"`

	env evergreen.Environment
}

func makeGithubPRLabelInjectJob() *githubPRLabelInjectJob {
	j := &githubPRLabelInjectJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    githubPRLabelInjectJobName,
				Version: 0,
			},
		},
	}
	return j
}

// NewGithubPRLabelInjectJob returns a job that injects the tasks implied by a
// GitHub PR's current label set into the existing PR version for the given head
// SHA. If no such version exists (or the latest PR version is for a different
// head SHA), the job is a no-op; it never creates a version.
func NewGithubPRLabelInjectJob(env evergreen.Environment, owner, repo string, prNumber int, headSHA string, labels []string, ts string) amboy.Job {
	j := makeGithubPRLabelInjectJob()
	j.Owner = owner
	j.Repo = repo
	j.PRNumber = prNumber
	j.HeadSHA = headSHA
	j.Labels = labels
	j.env = env

	j.SetID(fmt.Sprintf("%s.%s.%s.%d.%s.%s", githubPRLabelInjectJobName, owner, repo, prNumber, headSHA, ts))
	// The version's ID is not known at construction time, so we cannot acquire
	// the generate.tasks version scope here to serialize against a concurrent
	// generate.tasks job. Concurrency safety instead relies on the job-ID dedup
	// above (identical deliveries collapse into one job) and on
	// InjectTasksIntoVersion computing an idempotent delta of tasks not already
	// in the version.
	return j
}

func (j *githubPRLabelInjectJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	p, err := patch.FindLatestGithubPRPatch(ctx, j.Owner, j.Repo, j.PRNumber)
	if err != nil {
		j.AddError(errors.Wrapf(err, "finding latest PR patch for '%s/%s' PR #%d", j.Owner, j.Repo, j.PRNumber))
		return
	}
	if p == nil || p.Version == "" || p.GithubPatchData.HeadHash != j.HeadSHA {
		grip.Info(ctx, message.Fields{
			"message":   "no finalized PR version for head SHA, skipping label task injection",
			"job":       j.ID(),
			"owner":     j.Owner,
			"repo":      j.Repo,
			"pr_number": j.PRNumber,
			"head_sha":  j.HeadSHA,
		})
		return
	}

	v, err := model.VersionFindOneId(ctx, p.Version)
	if err != nil {
		j.AddError(errors.Wrapf(err, "finding version '%s'", p.Version))
		return
	}
	if v == nil {
		return
	}

	project, _, err := model.FindAndTranslateProjectForVersion(ctx, j.env.Settings(), v, false)
	if err != nil {
		j.AddError(errors.Wrapf(err, "loading project for version '%s'", v.Id))
		return
	}

	aliases, err := model.FindAliasInProjectRepoOrProjectConfig(ctx, v.Identifier, evergreen.GithubPRAlias, nil)
	if err != nil {
		j.AddError(errors.Wrapf(err, "finding PR aliases for project '%s'", v.Identifier))
		return
	}

	satisfied := model.ProjectAliases(aliases).FilterByLabels(j.Labels)
	pairs, err := project.BuildProjectTVPairsWithAlias(satisfied, evergreen.GithubPRRequester)
	if err != nil {
		j.AddError(errors.Wrapf(err, "resolving label-gated aliases into task/variant pairs for version '%s'", v.Id))
		return
	}

	added, err := model.InjectTasksIntoVersion(ctx, j.env.Settings(), v, project, pairs)
	if err != nil {
		j.AddError(errors.Wrapf(err, "injecting label-implied tasks into version '%s'", v.Id))
		return
	}

	grip.Info(ctx, message.Fields{
		"message":   "injected label-implied tasks into PR version",
		"job":       j.ID(),
		"version":   v.Id,
		"num_added": len(added),
		"labels":    j.Labels,
		"owner":     j.Owner,
		"repo":      j.Repo,
		"pr_number": j.PRNumber,
		"head_sha":  j.HeadSHA,
	})
}
