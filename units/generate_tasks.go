package units

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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
	env      evergreen.Environment
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
	return j
}

// NewGenerateTasksJob returns a job that dynamically updates the project
// configuration based on the given task's generate.tasks configuration.
func NewGenerateTasksJob(versionID, taskID string, ts string) amboy.Job {
	j := makeGenerateTaskJob()
	j.TaskID = taskID

	j.SetID(fmt.Sprintf("%s-%s-%s", generateTasksJobName, taskID, ts))
	versionScope := fmt.Sprintf("%s.%s", generateTasksJobName, versionID)
	taskScope := fmt.Sprintf("%s.%s", generateTasksJobName, taskID)
	// Setting the version as one of the scopes ensures that, for a given
	// version, only one generate.tasks job can run at a time. Setting the task
	// scope as an enqueued scope ensures that, for a given task in a version,
	// there's only one job that runs generate.tasks on behalf of that task.
	j.SetScopes([]string{versionScope, taskScope})
	j.SetEnqueueScopes(taskScope)
	return j
}

func (j *generateTasksJob) generate(ctx context.Context, t *task.Task) error {
	ctx = utility.ContextWithAttributes(ctx, []attribute.KeyValue{
		attribute.String(evergreen.TaskIDOtelAttribute, t.Id),
		attribute.Int(evergreen.TaskExecutionOtelAttribute, t.Execution),
		attribute.String(evergreen.VersionIDOtelAttribute, t.Version),
		attribute.String(evergreen.BuildIDOtelAttribute, t.BuildId),
		attribute.String(evergreen.ProjectIDOtelAttribute, t.Project),
		attribute.String(evergreen.VersionRequesterOtelAttribute, t.Requester)})
	ctx, span := tracer.Start(ctx, "task-generation")
	defer span.End()
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
		return errors.Wrapf(err, "finding version '%s'", t.Version)
	}
	if v == nil {
		return errors.Errorf("version '%s' not found", t.Version)
	}
	project, parserProject, err := model.FindAndTranslateProjectForVersion(ctx, j.env.Settings(), v)
	if err != nil {
		return errors.Wrapf(err, "loading project for version '%s'", t.Version)
	}

	// Get task again, to exit early if another generator finished while we looked for a
	// version. This makes it less likely that we enter `NewVersion` with a config that has
	// already been updated by another generator and therefore will be invalid because the
	// config has already been modified. We do this again in `handleError` to reduce the chances
	// of this race as close to zero as possible.
	t, err = task.FindOneId(t.Id)
	if err != nil {
		return errors.Wrapf(err, "finding task '%s'", t.Id)
	}
	if t == nil {
		return errors.Errorf("task '%s' not found", t.Id)
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
	projects, err = parseProjectsAsString(ctx, t.GeneratedJSONAsString)
	if err != nil {
		return errors.Wrap(err, "parsing JSON from `generate.tasks`")
	}

	g, err := model.MergeGeneratedProjects(ctx, projects)
	if err != nil {
		return errors.Wrap(err, "merging generated projects")
	}
	g.Task = t

	p, pp, v, err := g.NewVersion(ctx, project, parserProject, v)
	if err != nil {
		return j.handleError(errors.WithStack(err))
	}

	pref, err := model.FindMergedProjectRef(t.Project, t.Version, true)
	if err != nil {
		return j.handleError(errors.WithStack(err))
	}
	if pref == nil {
		return j.handleError(errors.Errorf("project '%s' not found", t.Project))
	}

	if err = validator.CheckProjectConfigurationIsValid(ctx, j.env.Settings(), p, pref); err != nil {
		return j.handleError(errors.WithStack(err))
	}

	if err := g.CheckForCycles(ctx, v, p, pref); err != nil {
		return errors.Wrap(err, "checking new dependency graph for cycles")
	}

	// Don't use the job's context, because it's better to try finishing than to
	// exit early after a SIGTERM from app server shutdown.
	saveCtx, saveCancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer saveCancel()
	// Inject the span context into the saveCtx.
	saveCtx = trace.ContextWithSpanContext(saveCtx, span.SpanContext())
	err = g.Save(saveCtx, j.env.Settings(), p, pp, v)

	// If the version or parser project has changed there was a race. Another generator will try again.
	if adb.ResultsNotFound(err) || db.IsDuplicateKey(err) {
		return err
	}
	if err != nil {
		return errors.Wrap(err, evergreen.SaveGenerateTasksError)
	}
	return nil
}

// handleError return mongo.ErrNoDocuments if generate.tasks has already run.
// Otherwise, it returns the given error.
func (j *generateTasksJob) handleError(handledError error) error {
	// Get task again, to exit nil if another generator finished, which caused us to error.
	// Checking this again here makes it very unlikely that there is a race, because both
	// `t.GeneratedTasks` checks must have been in between the racing generator's call to
	// save the config and set the task's boolean.
	t, err := task.FindOneId(j.TaskID)
	if err != nil {
		return errors.Wrapf(err, "finding task '%s'", j.TaskID)
	}
	if t == nil {
		return errors.Errorf("task '%s' not found", j.TaskID)
	}
	if t.GeneratedTasks {
		grip.Debug(message.Fields{
			"message": "handleError encountered task that is already generating, nooping",
			"task":    t.Id,
			"version": t.Version,
		})
		return mongo.ErrNoDocuments
	}

	return handledError
}

func (j *generateTasksJob) Run(ctx context.Context) {
	defer j.MarkComplete()
	start := time.Now()

	t, err := task.FindOneId(j.TaskID)
	if err != nil {
		j.AddError(errors.Wrapf(err, "finding task '%s'", j.TaskID))
		return
	}
	if t == nil {
		j.AddError(errors.Errorf("task '%s' not found", j.TaskID))
		return
	}
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String(evergreen.ProjectIDOtelAttribute, t.Project))

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
		grip.Debug(message.WrapError(err, message.Fields{
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
		if t.IsPatchRequest() {
			activatedTasks, err := task.CountActivatedTasksForVersion(t.Version)
			if err != nil {
				j.AddError(err)
				return
			}
			if activatedTasks > evergreen.NumTasksForLargePatch {
				grip.Info(message.Fields{
					"message":             "patch has large number of activated tasks",
					"op":                  "generate.tasks",
					"num_tasks_activated": activatedTasks,
					"version":             t.Version,
				})
			}
		}

	}
}

// CreateAndEnqueueGenerateTasks enqueues a generate.tasks job for each task.
// Jobs are segregated by task version into separate queues.
func CreateAndEnqueueGenerateTasks(ctx context.Context, env evergreen.Environment, tasks []task.Task, ts string) error {
	versionTasksMap := make(map[string][]task.Task)
	for _, t := range tasks {
		versionTasksMap[t.Version] = append(versionTasksMap[t.Version], t)
	}
	appCtx, _ := env.Context()
	for versionID, tasks := range versionTasksMap {
		var jobs []amboy.Job
		for _, t := range tasks {
			jobs = append(jobs, NewGenerateTasksJob(versionID, t.Id, ts))
		}
		queueName := fmt.Sprintf("service.generate.tasks.version.%s", versionID)
		queue, err := env.RemoteQueueGroup().Get(appCtx, queueName)
		if err != nil {
			return errors.Wrapf(err, "getting generate tasks queue '%s' for version '%s'", queueName, versionID)
		}
		if err = amboy.EnqueueManyUniqueJobs(ctx, queue, jobs); err != nil {
			return errors.Wrapf(err, "enqueueing generate tasks jobs for version '%s'", versionID)
		}
	}

	return nil
}

func parseProjectsAsString(ctx context.Context, jsonStrings []string) ([]model.GeneratedProject, error) {
	_, span := tracer.Start(ctx, "parse-projects")
	defer span.End()
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
