package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	largeParserProjectTaskStatsJobName        = "large-parser-project-task-stats"
	largeParserProjectTaskStatsJobMaxAttempts = 10
)

func init() {
	registry.AddJobType(largeParserProjectTaskStatsJobName, func() amboy.Job {
		return makeLargeParserProjectTaskStatsJob()
	})
}

type largeParserProjectTaskStatsJob struct {
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`
	env      evergreen.Environment
}

func makeLargeParserProjectTaskStatsJob() *largeParserProjectTaskStatsJob {
	j := &largeParserProjectTaskStatsJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    largeParserProjectTaskStatsJobName,
				Version: 0,
			},
		},
	}
	return j
}

// NewLargeParserProjectTaskStatsJob returns a job to log the current count of
// running tasks with S3-stored parser projects alongside the configured limit.
func NewLargeParserProjectTaskStatsJob(ts string) amboy.Job {
	j := makeLargeParserProjectTaskStatsJob()
	j.SetID(fmt.Sprintf("%s.%s", largeParserProjectTaskStatsJobName, ts))
	j.SetScopes([]string{largeParserProjectTaskStatsJobName})
	j.SetEnqueueAllScopes(true)
	j.UpdateRetryInfo(amboy.JobRetryOptions{
		Retryable:   utility.TruePtr(),
		MaxAttempts: utility.ToIntPtr(largeParserProjectTaskStatsJobMaxAttempts),
	})
	return j
}

func (j *largeParserProjectTaskStatsJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	statsByProject, err := task.GetLargeParserProjectTaskStats(ctx)
	if err != nil {
		j.AddRetryableError(errors.Wrap(err, "getting large parser project task stats"))
		return
	}

	var totalTasks int
	tasksByProject := make(map[string]int, len(statsByProject))
	for _, s := range statsByProject {
		totalTasks += s.RunningTasks
		tasksByProject[s.Project] = s.RunningTasks
	}

	settings := j.env.Settings()
	grip.Info(ctx, message.Fields{
		"job_id":                                 j.ID(),
		"message":                                "large parser project task stats",
		"num_running_large_parser_project_tasks": totalTasks,
		"num_by_project":                         tasksByProject,
		"max_concurrent_large_parser_project_tasks": settings.TaskLimits.MaxConcurrentLargeParserProjectTasks,
	})
}
