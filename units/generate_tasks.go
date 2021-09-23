package units

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
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
	"go.mongodb.org/mongo-driver/mongo"
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
	start := time.Now()
	if t.GeneratedTasks {
		return mongo.ErrNoDocuments
	}
	if t.Status != evergreen.TaskStarted {
		grip.Debug(message.Fields{
			"message": "task is not running, not generating tasks",
			"task":    t.Id,
			"version": t.Version,
		})
		return nil
	}

	v, err := model.VersionFindOneId(t.Version)
	if err != nil {
		return errors.Wrapf(err, "error finding version %s", t.Version)
	}
	if v == nil {
		return errors.Errorf("unable to find version %s", t.Version)
	}
	p, pp, err := model.LoadProjectForVersion(v, t.Project, false)
	if err != nil {
		return errors.Wrapf(err, "error getting project for version %s", t.Version)
	}

	// Get task again, to exit early if another generator finished while we looked for a
	// version. This makes it less likely that we enter `NewVersion` with a config that has
	// already been updated by another generator and therefore will be invalid because the
	// config has already been modified. We do this again in `handleError` to reduce the chances
	// of this race as close to zero as possible.
	t, err = task.FindOneId(t.Id)
	if err != nil {
		return errors.Wrapf(err, "error finding task %s", t.Id)
	}
	if t == nil {
		return errors.Errorf("unable to find task %s", t.Id)
	}
	if t.GeneratedTasks {
		grip.Debug(message.Fields{
			"message": "attempted to generate tasks after getting config, but generator already ran for this task",
			"task":    t.Id,
			"version": t.Version,
		})
		return mongo.ErrNoDocuments
	}

	var projects []model.GeneratedProject
	if len(t.GeneratedJSONAsString) > 0 {
		projects, err = parseProjectsAsString(t.GeneratedJSONAsString)
	} else {
		projects, err = parseProjects(t.GeneratedJSON)
	}
	if err != nil {
		return errors.Wrap(err, "error parsing JSON from `generate.tasks`")
	}
	grip.Debug(message.Fields{
		"message":       "generate.tasks timing",
		"function":      "generate",
		"operation":     "parseProjects",
		"duration_secs": time.Since(start).Seconds(),
		"task":          t.Id,
		"job":           j.ID(),
		"version":       t.Version,
	})
	start = time.Now()

	g, err := model.MergeGeneratedProjects(projects)
	grip.Debug(message.Fields{
		"message":       "generate.tasks timing",
		"function":      "generate",
		"operation":     "MergeGeneratedProjects",
		"duration_secs": time.Since(start).Seconds(),
		"task":          t.Id,
		"job":           j.ID(),
		"version":       t.Version,
	})
	if err != nil {
		return errors.Wrap(err, "error merging generated projects")
	}
	start = time.Now()
	g.TaskID = j.TaskID

	p, pp, v, err = g.NewVersion(p, pp, v)
	if err != nil {
		return j.handleError(pp, v, errors.WithStack(err))
	}
	grip.Debug(message.Fields{
		"message":       "generate.tasks timing",
		"function":      "generate",
		"operation":     "NewVersion",
		"duration_secs": time.Since(start).Seconds(),
		"task":          t.Id,
		"job":           j.ID(),
		"version":       t.Version,
	})
	pref, err := model.FindMergedProjectRef(t.Project)
	if err != nil {
		return j.handleError(pp, v, errors.WithStack(err))
	}
	if pref == nil {
		return j.handleError(pp, v, errors.Errorf("project '%s' not found", t.Project))
	}
	start = time.Now()
	if err = validator.CheckProjectConfigurationIsValid(p, pref); err != nil {
		return j.handleError(pp, v, errors.WithStack(err))
	}
	grip.Debug(message.Fields{
		"message":       "generate.tasks timing",
		"function":      "generate",
		"operation":     "CheckProjectConfigurationIsValid",
		"duration_secs": time.Since(start).Seconds(),
		"task":          t.Id,
		"job":           j.ID(),
		"version":       t.Version,
	})
	start = time.Now()

	// Don't use the job's context, because it's better to finish than to exit early after a
	// SIGTERM from a deploy. This should maybe be a context with timeout.
	err = g.Save(context.Background(), p, pp, v, t)

	// If the version or parser project has changed there was a race. Another generator will try again.
	if adb.ResultsNotFound(err) || db.IsDuplicateKey(err) {
		return err
	}
	if err != nil {
		return errors.Wrap(err, evergreen.SaveGenerateTasksError)
	}
	grip.Debug(message.Fields{
		"message":       "generate.tasks timing",
		"function":      "generate",
		"operation":     "Save",
		"duration_secs": time.Since(start).Seconds(),
		"task":          t.Id,
		"job":           j.ID(),
		"version":       t.Version,
	})
	return nil
}

// handleError return mongo.ErrNoDocuments if another job has raced, the passed in error otherwise.
func (j *generateTasksJob) handleError(pp *model.ParserProject, v *model.Version, handledError error) error {
	// Get task again, to exit nil if another generator finished, which caused us to error.
	// Checking this again here makes it very unlikely that there is a race, because both
	// `t.GeneratedTasks` checks must have been in between the racing generator's call to
	// save the config and set the task's boolean.
	t, err := task.FindOneId(j.TaskID)
	if err != nil {
		return errors.Wrapf(err, "error finding task %s", j.TaskID)
	}
	if t == nil {
		return errors.Errorf("unable to find task %s", j.TaskID)
	}
	if t.GeneratedTasks {
		grip.Debug(message.Fields{
			"message": "handledError encountered task that already generating, nooping",
			"task":    t.Id,
			"version": t.Version,
		})
		return mongo.ErrNoDocuments
	}

	if v == nil {
		return handledError
	}
	versionFromDB, err := model.VersionFindOne(model.VersionById(v.Id).WithFields(model.VersionConfigNumberKey))
	if err != nil {
		return errors.Wrapf(err, "problem finding version %s", v.Id)
	}
	if versionFromDB == nil {
		return errors.Errorf("could not find version %s", v.Id)
	}
	ppFromDB, err := model.ParserProjectFindOne(model.ParserProjectById(v.Id).WithFields(model.ParserProjectConfigNumberKey))
	if err != nil {
		return errors.Wrapf(err, "problem finding parser project %s", v.Id)
	}
	// If the config update number has been updated, then another task has raced with us.
	// The error is therefore not an actual configuration problem but instead a symptom
	// of the race.
	if v.ConfigUpdateNumber != versionFromDB.ConfigUpdateNumber ||
		pp != nil && ppFromDB != nil && pp.ConfigUpdateNumber != ppFromDB.ConfigUpdateNumber {
		return mongo.ErrNoDocuments
	}
	return handledError
}

func (j *generateTasksJob) Run(ctx context.Context) {
	defer j.MarkComplete()
	start := time.Now()

	t, err := task.FindOneId(j.TaskID)
	if err != nil {
		j.AddError(errors.Wrapf(err, "problem finding task %s", j.TaskID))
		return
	}
	if t == nil {
		j.AddError(errors.Errorf("task %s does not exist", j.TaskID))
		return
	}

	err = j.generate(ctx, t)
	shouldNoop := adb.ResultsNotFound(err) || db.IsDuplicateKey(err)

	grip.InfoWhen(err == nil, message.Fields{
		"message":       "generate.tasks finished",
		"operation":     "generate.tasks",
		"duration_secs": time.Since(start).Seconds(),
		"task":          t.Id,
		"job":           j.ID(),
		"version":       t.Version,
	})
	grip.DebugWhen(shouldNoop, message.WrapError(err, message.Fields{
		"message":       "generate.tasks noop",
		"operation":     "generate.tasks",
		"duration_secs": time.Since(start).Seconds(),
		"task":          t.Id,
		"job":           j.ID(),
		"version":       t.Version,
	}))
	if err != nil && !shouldNoop {
		grip.Error(message.WrapError(err, message.Fields{
			"message":       "generate.tasks finished with errors",
			"operation":     "generate.tasks",
			"duration_secs": time.Since(start).Seconds(),
			"task":          t.Id,
			"job":           j.ID(),
			"version":       t.Version,
			"is_save_error": strings.Contains(err.Error(), evergreen.SaveGenerateTasksError),
		}))
	}

	if err != nil && !shouldNoop {
		j.AddError(err)
		j.AddError(task.MarkGeneratedTasksErr(j.TaskID, err))
		return
	}
	if !shouldNoop {
		j.AddError(task.MarkGeneratedTasks(j.TaskID))
	}
}

func parseProjectsAsString(jsonStrings []string) ([]model.GeneratedProject, error) {
	catcher := grip.NewBasicCatcher()
	var projects []model.GeneratedProject
	for _, f := range jsonStrings {
		p, err := model.ParseProjectFromJSONString(f)
		if err != nil {
			catcher.Add(err)
		}
		projects = append(projects, p)
	}
	return projects, catcher.Resolve()
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
	return projects, catcher.Resolve()
}
