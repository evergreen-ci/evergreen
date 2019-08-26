package units

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

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
	generateTasksJobName = "generate-tasks"
)

func init() {
	registry.AddJobType(generateTasksJobName, func() amboy.Job { return makeGenerateTaskJob() })
}

type generateTasksJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	TaskID   string `bson:"task_id" json:"task_id" yaml:"task_id"`
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

func NewGenerateTasksJob(id string, ts string) amboy.Job {
	j := makeGenerateTaskJob()
	j.TaskID = id

	j.SetID(fmt.Sprintf("%s-%s-%s", generateTasksJobName, id, ts))
	return j
}

func (j *generateTasksJob) generate(ctx context.Context, t *task.Task) error {
	t, err := task.FindOneId(j.TaskID)
	if err != nil {
		return errors.Wrapf(err, "problem finding task %s", j.TaskID)
	}
	if t == nil {
		return errors.Errorf("task %s does not exist", j.TaskID)
	}
	if t.GeneratedTasks {
		grip.Debug(message.Fields{
			"message": "attempted to generate tasks, but generator already ran for this task",
			"task":    t.Id,
			"version": t.Version,
		})
		return nil
	}

	projects, err := parseProjects(t.GeneratedJSON)
	if err != nil {
		return errors.Wrap(err, "error parsing JSON from `generate.tasks`")
	}
	g := model.MergeGeneratedProjects(projects)
	g.TaskID = j.TaskID

	p, v, t, pm, err := g.NewVersion()
	if err != nil {
		return errors.Wrap(err, "problem creating new version")
	}
	if err = validator.CheckProjectConfigurationIsValid(p); err != nil {
		return errors.Wrap(err, "project configuration was invalid")
	}

	// Don't use the job's context, because it's better to finish than to exit early after a
	// SIGTERM from a deploy. This should maybe be a context with timeout.
	err = g.Save(context.TODO(), p, v, t, pm)

	// If the version has changed return nil because there was a race. Another generator will try again.
	if err != nil && adb.ResultsNotFound(err) {
		return nil
	}
	if err != nil {
		return errors.Wrap(err, "error updating config in `generate.tasks`")
	}
	return nil
}

func (j *generateTasksJob) Run(ctx context.Context) {
	defer j.MarkComplete()
	start := time.Now()

	t, err := task.FindOneId(j.TaskID)
	if err != nil {
		j.AddError(errors.Wrapf(err, "problem finding task %s", j.TaskID))
	}
	if t == nil {
		j.AddError(errors.Errorf("task %s does not exist", j.TaskID))
	}

	err = j.generate(ctx, t)
	j.AddError(err)
	j.AddError(t.SetGenerateTasksError(err))
	j.AddError(task.MarkGeneratedTasks(j.TaskID))

	grip.InfoWhen(err == nil, message.Fields{
		"message":       "generate.tasks finished",
		"duration_secs": time.Since(start).Seconds(),
		"task":          t.Id,
		"version":       t.Version,
	})
	grip.ErrorWhen(err != nil, message.WrapError(err, message.Fields{
		"message":       "generate.tasks finished",
		"duration_secs": time.Since(start).Seconds(),
		"task":          t.Id,
		"version":       t.Version,
	}))
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
