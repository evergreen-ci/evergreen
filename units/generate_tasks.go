package units

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	generateTasksJobName    = "generate-tasks"
	generateTaskRequeueWait = 10 * time.Second
	generateTaskMaxAttempts = 6 * 60 // 1 hour, if generateTaskRequeueWait is 10 seconds
)

func init() {
	registry.AddJobType(generateTasksJobName, func() amboy.Job { return makeGenerateTaskJob() })
}

type generateTasksJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	TaskID   string `bson:"task_id" json:"task_id" yaml:"task_id"`
	Attempt  int    `bson:"attempt" json:"attempt" yaml:"attempt"`

	requeue bool
}

func makeGenerateTaskJob() *generateTasksJob {
	j := &generateTasksJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    generateTasksJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())

	return j
}

func NewGenerateTasksJob(id string, attempt int) amboy.Job {
	j := makeGenerateTaskJob()
	j.TaskID = id

	j.SetID(fmt.Sprintf("%s-%s-%d", generateTasksJobName, id, attempt))
	return j
}

func (j *generateTasksJob) tryRequeue(taskID string) {
	ctx, cancel := context.WithTimeout(context.Background(), generateTaskRequeueWait)
	defer cancel()
	if !j.requeue {
		if err := task.MarkGeneratedTasks(taskID); err != nil {
			j.AddError(errors.Wrapf(err, "problem marking task '%s' as having generated tasks", taskID))
			return
		}
		return
	}
	if j.Attempt == generateTaskMaxAttempts {
		j.AddError(errors.Errorf("reached max max attempts %d, aborting generate.tasks job", generateTaskMaxAttempts))
		return
	}
	newJob := NewGenerateTasksJob(j.TaskID, j.Attempt+1)
	newJob.UpdateTimeInfo(amboy.JobTimeInfo{
		WaitUntil: time.Now().Add(generateTaskRequeueWait),
	})
	err := evergreen.GetEnvironment().RemoteQueue().Put(ctx, newJob)
	grip.Error(message.WrapError(err, message.Fields{
		"message":  "failed to requeue generate task job",
		"task":     j.TaskID,
		"job":      j.ID(),
		"attempts": j.Attempt,
	}))
	j.AddError(err)
}

func (j *generateTasksJob) Run(ctx context.Context) {
	defer j.MarkComplete()
	defer j.tryRequeue(j.TaskID)
	start := time.Now()

	if ctx.Err() != nil {
		j.requeue = true
		return
	}

	t, err := task.FindOneId(j.TaskID)
	if err != nil {
		j.AddError(err)
		return
	}
	if t == nil {
		j.AddError(errors.Errorf("task %s does not exist", j.TaskID))
		return
	}
	if t.GeneratedTasks {
		grip.Error(message.Fields{
			"message": "attempted to generate tasks, but generator already ran for this task",
			"task":    t.Id,
			"version": t.Version,
		})
		return
	}

	projects, err := parseProjects(t.GeneratedJSON)
	if err != nil {
		j.AddError(errors.Wrap(err, "error parsing JSON from `generate.tasks`"))
		return
	}
	g := model.MergeGeneratedProjects(projects)
	g.TaskID = j.TaskID

	p, v, t, pm, err := g.NewVersion()
	if err != nil {
		j.AddError(err)
		return
	}
	if err = validator.CheckProjectConfigurationIsValid(p); err != nil {
		j.AddError(err)
		return
	}

	if ctx.Err() != nil {
		j.requeue = true
		return
	}
	err = g.Save(ctx, p, v, t, pm)
	if adb.ResultsNotFound(err) {
		j.requeue = true
		return
	}
	if err != nil {
		if ctx.Err() == nil {
			j.AddError(errors.Wrap(err, "error updating config in `generate.tasks`"))
		} else {
			j.requeue = true
		}
		return
	}
	grip.Info(message.Fields{
		"message":       "generate.tasks succeeded",
		"attempt":       j.Attempt,
		"duration_secs": time.Since(start).Seconds(),
		"task":          t.Id,
		"version":       t.Version,
	})
}

func parseProjects(jsonBytes []json.RawMessage) ([]model.GeneratedProject, error) {
	catcher := grip.NewBasicCatcher()
	var projects []model.GeneratedProject
	for _, f := range jsonBytes {
		p, err := model.ParseProjectFromJSON(f)
		if err != nil {
			catcher.Add(err)
		}
		projects = append(projects, p)
	}
	if catcher.HasErrors() {
		return nil, catcher.Resolve()
	}
	return projects, nil
}
