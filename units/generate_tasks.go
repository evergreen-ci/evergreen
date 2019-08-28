package units

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/evergreen-ci/gimlet"
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
	TaskID   string            `bson:"task_id" json:"task_id" yaml:"task_id"`
	JSON     []json.RawMessage `bson:"json" json:"json" yaml:"json"`
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

func NewGenerateTasksJob(id string, json []json.RawMessage) amboy.Job {
	j := makeGenerateTaskJob()
	j.TaskID = id
	j.JSON = json

	j.SetID(fmt.Sprintf("%s-%s", generateTasksJobName, id))
	return j
}

func (j *generateTasksJob) Run(ctx context.Context) {
	defer j.MarkComplete()
	start := time.Now()

	projects, err := parseProjects(j.JSON)
	if err != nil {
		j.AddError(errors.Wrap(err, "error parsing JSON from `generate.tasks`"))
		return
	}
	g := model.MergeGeneratedProjects(projects)
	g.TaskID = j.TaskID

	var attempt int
	err = util.Retry(
		ctx,
		func() (bool, error) {
			attemptStart := time.Now()
			attempt++

			t, err := task.FindOneId(g.TaskID)
			if err != nil {
				return false, err
			}
			if t == nil {
				return false, errors.Errorf("unable to find task %s", g.TaskID)
			}
			if t.GeneratedTasks {
				return false, nil // already generated tasks, noop
			}
			p, v, t, pm, err := g.NewVersion() // nolint
			if err != nil {
				return false, err
			}
			if err = validator.CheckProjectConfigurationIsValid(p); err != nil {
				return false, err
			}
			err = g.Save(ctx, p, v, t, pm)
			if err != nil && adb.ResultsNotFound(err) {
				return true, gimlet.ErrorResponse{
					StatusCode: http.StatusInternalServerError,
					Message:    errors.Wrap(err, "error updating config in `generate.tasks`").Error(),
				}
			}
			if err = t.MarkGeneratedTasks(); err != nil {
				return true, gimlet.ErrorResponse{
					StatusCode: http.StatusInternalServerError,
					Message:    errors.Wrapf(err, "problem marking task '%s' as having generated tasks", t.Id).Error(),
				}
			}
			grip.Info(message.Fields{
				"message":               "generate.tasks succeeded",
				"attempt":               attempt,
				"attempt_duration_secs": time.Since(attemptStart).Seconds(),
				"total_duration_secs":   time.Since(start).Seconds(),
				"task":                  t.Id,
				"version":               t.Version,
			})
			return false, nil
		}, 100, time.Second, 15*time.Second)
	j.AddError(err)
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
