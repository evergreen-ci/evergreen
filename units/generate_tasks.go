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
	// hasGeneratedTasksOtelAttribute uses a hyphenated legacy key; renaming it would break
	// existing Honeycomb queries, so new attributes below use the underscore namespace instead.
	hasGeneratedTasksOtelAttribute = "evergreen.generate-tasks.has_generated_tasks"

	generateTasksOutcomeAttribute     = "evergreen.generate_tasks.outcome"
	generateTasksIsSaveErrorAttribute = "evergreen.generate_tasks.is_save_error"
	// generateTasksNumCreatedAttribute and generateTasksNumActivatedAttribute intentionally
	// match the unexported numGenerateTasksAttribute and numActivatedGenerateTasksAttribute
	// keys in model/generate.go, so a single Honeycomb column covers both the root job span
	// and the model's child span.
	generateTasksNumCreatedAttribute             = "evergreen.generate_tasks.num_created"
	generateTasksNumActivatedAttribute           = "evergreen.generate_tasks.num_activated"
	generateTasksNumActivatedForVersionAttribute = "evergreen.generate_tasks.num_activated_for_version"
)

// generateTasksOutcome classifies why a generate.tasks job execution ended the way it did,
// distinguishing outcomes that share the same underlying error (e.g. mongo.ErrNoDocuments is
// returned by three different race-detection sites).
type generateTasksOutcome string

const (
	outcomeGenerated           generateTasksOutcome = "generated"
	outcomeTaskNotRunning      generateTasksOutcome = "task_not_running"
	outcomeAlreadyGenerated    generateTasksOutcome = "already_generated_before_start"
	outcomeRaceOnReload        generateTasksOutcome = "race_on_reload"
	outcomeRaceOnSave          generateTasksOutcome = "race_on_save"
	outcomeRaceOnErrorHandling generateTasksOutcome = "race_on_error_handling"
	outcomeError               generateTasksOutcome = "error"
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
func NewGenerateTasksJob(env evergreen.Environment, versionID, taskID string, ts string) amboy.Job {
	j := makeGenerateTaskJob()
	j.TaskID = taskID
	j.env = env

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

// generateTasksSpanAttributes returns the identity attributes that identify which task,
// version, build, and project a generate.tasks execution belongs to. These are set on both
// the root job span (so the wide event carries identity) and inherited by child spans.
func generateTasksSpanAttributes(t *task.Task) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String(evergreen.TaskIDOtelAttribute, t.Id),
		attribute.Int(evergreen.TaskExecutionOtelAttribute, t.Execution),
		attribute.String(evergreen.VersionIDOtelAttribute, t.Version),
		attribute.String(evergreen.BuildIDOtelAttribute, t.BuildId),
		attribute.String(evergreen.ProjectIDOtelAttribute, t.Project),
		attribute.String(evergreen.VersionRequesterOtelAttribute, t.Requester),
		attribute.Bool(hasGeneratedTasksOtelAttribute, t.GeneratedTasks),
	}
}

func (j *generateTasksJob) generate(ctx context.Context, t *task.Task) (generateTasksOutcome, error) {
	ctx = utility.ContextWithAttributes(ctx, generateTasksSpanAttributes(t))
	ctx, span := tracer.Start(ctx, "task-generation")
	defer span.End()

	if t.GeneratedTasks {
		return outcomeAlreadyGenerated, mongo.ErrNoDocuments
	}
	if t.Status != evergreen.TaskStarted {
		grip.Debug(ctx, message.Fields{
			"message": "task is not running, not generating tasks",
			"task":    t.Id,
			"version": t.Version,
			"job":     j.ID(),
		})
		return outcomeTaskNotRunning, nil
	}

	v, err := model.VersionFindOneId(ctx, t.Version)
	if err != nil {
		return outcomeError, errors.Wrapf(err, "finding version '%s'", t.Version)
	}
	if v == nil {
		return outcomeError, errors.Errorf("version '%s' not found", t.Version)
	}
	// Opt out of read coalescing: this path mutates the parser project in place (via
	// GeneratedProject.NewVersion -> addGeneratedProjectToConfig), so it must not share the pointer
	// with concurrent readers. It's low-volume background work, so opting out is cheap.
	project, parserProject, err := model.FindAndTranslateProjectForVersionWithOpts(ctx, j.env.Settings(), v, false, false)
	if err != nil {
		return outcomeError, errors.Wrapf(err, "loading project for version '%s'", t.Version)
	}

	// Get task again, to exit early if another generator finished while we looked for a
	// version. This makes it less likely that we enter `NewVersion` with a config that has
	// already been updated by another generator and therefore will be invalid because the
	// config has already been modified. We do this again in `handleError` to reduce the chances
	// of this race as close to zero as possible.
	taskId := t.Id
	t, err = task.FindOneIdWithGeneratedJSON(ctx, taskId)
	if err != nil {
		return outcomeError, errors.Wrapf(err, "finding task '%s'", taskId)
	}
	if t == nil {
		return outcomeError, errors.Errorf("task '%s' not found", taskId)
	}
	if t.GeneratedTasks {
		grip.Debug(ctx, message.Fields{
			"message": "attempted to generate tasks after getting config, but generator already ran for this task",
			"task":    t.Id,
			"version": t.Version,
			"job":     j.ID(),
		})
		return outcomeRaceOnReload, mongo.ErrNoDocuments
	}

	files, err := task.GeneratedJSONFind(ctx, j.env.Settings(), t)
	if err != nil {
		return outcomeError, errors.Wrapf(err, "finding generated JSON for task '%s'", t.Id)
	}

	var projects []model.GeneratedProject
	projects, err = parseProjectsAsString(ctx, files)
	if err != nil {
		return outcomeError, errors.Wrap(err, "parsing JSON from `generate.tasks`")
	}

	g, err := model.MergeGeneratedProjects(ctx, projects)
	if err != nil {
		return outcomeError, errors.Wrap(err, "merging generated projects")
	}
	g.Task = t

	p, pp, v, err := g.NewVersion(ctx, project, parserProject, v)
	if err != nil {
		return j.handleError(ctx, errors.WithStack(err))
	}

	pref, err := model.FindMergedProjectRef(ctx, t.Project, t.Version, true)
	if err != nil {
		return j.handleError(ctx, errors.WithStack(err))
	}
	if pref == nil {
		return j.handleError(ctx, errors.Errorf("project '%s' not found", t.Project))
	}

	if err = validator.CheckProjectConfigurationIsValid(ctx, j.env.Settings(), p, pref); err != nil {
		return j.handleError(ctx, errors.WithStack(err))
	}

	if err := g.CheckForCycles(ctx, v, p, pref); err != nil {
		return outcomeError, errors.Wrap(err, "checking new dependency graph for cycles")
	}

	// Ignore the cancel signal from the job's context, because it's better
	// to try finishing than to exit early after a SIGTERM from app server shutdown.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Minute)
	defer cancel()

	err = g.Save(ctx, j.env.Settings(), p, pp, v)

	// If the version or parser project has changed there was a race. Another generator will try again.
	if adb.ResultsNotFound(err) || db.IsDuplicateKey(err) {
		return outcomeRaceOnSave, err
	}
	if err != nil {
		return outcomeError, errors.Wrap(err, evergreen.SaveGenerateTasksError)
	}
	return outcomeGenerated, nil
}

// handleError return mongo.ErrNoDocuments if generate.tasks has already run.
// Otherwise, it returns the given error.
func (j *generateTasksJob) handleError(ctx context.Context, handledError error) (generateTasksOutcome, error) {
	// Get task again, to exit nil if another generator finished, which caused us to error.
	// Checking this again here makes it very unlikely that there is a race, because both
	// `t.GeneratedTasks` checks must have been in between the racing generator's call to
	// save the config and set the task's boolean.
	t, err := task.FindOneId(ctx, j.TaskID)
	if err != nil {
		return outcomeError, errors.Wrapf(err, "finding task '%s'", j.TaskID)
	}
	if t == nil {
		return outcomeError, errors.Errorf("task '%s' not found", j.TaskID)
	}
	if t.GeneratedTasks {
		grip.Debug(ctx, message.Fields{
			"message": "handleError encountered task that is already generating, nooping",
			"task":    t.Id,
			"version": t.Version,
			"job":     j.ID(),
		})
		return outcomeRaceOnErrorHandling, mongo.ErrNoDocuments
	}

	return outcomeError, handledError
}

const maxGenerateTasksErrMsgLength = 1024 * 250 // 250k chars * 4 bytes/char = 1 MB

func (j *generateTasksJob) Run(ctx context.Context) {
	defer j.MarkComplete()
	start := time.Now()

	span := trace.SpanFromContext(ctx)

	t, err := task.FindOneId(ctx, j.TaskID)
	if err != nil {
		span.SetAttributes(attribute.String(generateTasksOutcomeAttribute, string(outcomeError)))
		j.AddError(errors.Wrapf(err, "finding task '%s'", j.TaskID))
		return
	}
	if t == nil {
		span.SetAttributes(attribute.String(generateTasksOutcomeAttribute, string(outcomeError)))
		j.AddError(errors.Errorf("task '%s' not found", j.TaskID))
		return
	}
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}
	span.SetAttributes(generateTasksSpanAttributes(t)...)

	outcome, err := j.generate(ctx, t)
	shouldNoop := adb.ResultsNotFound(err) || db.IsDuplicateKey(err)
	if err != nil && len(err.Error()) > maxGenerateTasksErrMsgLength {
		// If the error is excessively long (e.g. due to lots of validation
		// errors), truncate it to avoid hitting the 16 MB limit when saving
		// the generate.tasks error message back to the DB.
		err = errors.New(err.Error()[:maxGenerateTasksErrMsgLength] + "(truncated due to excessively long errors)")
	}

	span.SetAttributes(
		attribute.String(generateTasksOutcomeAttribute, string(outcome)),
		attribute.Bool(generateTasksIsSaveErrorAttribute, err != nil && strings.Contains(err.Error(), evergreen.SaveGenerateTasksError)),
	)

	grip.InfoWhen(ctx, err == nil, message.Fields{
		"message":       "generate.tasks finished",
		"operation":     "generate.tasks",
		"duration_secs": time.Since(start).Seconds(),
		"task":          t.Id,
		"job":           j.ID(),
		"version":       t.Version,
	})
	grip.DebugWhen(ctx, shouldNoop, message.WrapError(err, message.Fields{
		"message":       "generate.tasks noop",
		"operation":     "generate.tasks",
		"duration_secs": time.Since(start).Seconds(),
		"task":          t.Id,
		"job":           j.ID(),
		"version":       t.Version,
	}))
	if err != nil && !shouldNoop {
		grip.Debug(ctx, message.WrapError(err, message.Fields{
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
		j.AddError(task.MarkGeneratedTasksErr(ctx, j.TaskID, err))
		return
	}
	if !shouldNoop {
		j.AddError(task.MarkGeneratedTasks(ctx, j.TaskID))

		// The accurate counts are only computed and persisted inside
		// model.saveNewBuildsAndTasks, so re-fetch the task to read them back for the span.
		// This telemetry-only query must not fail the job.
		if generatedTask, findErr := task.FindOneId(ctx, j.TaskID); findErr != nil {
			grip.Warning(ctx, message.WrapError(findErr, message.Fields{
				"message": "finding task to record generate.tasks counts on span",
				"task":    j.TaskID,
				"job":     j.ID(),
			}))
		} else if generatedTask != nil {
			span.SetAttributes(
				attribute.Int(generateTasksNumCreatedAttribute, generatedTask.NumGeneratedTasks),
				attribute.Int(generateTasksNumActivatedAttribute, generatedTask.NumActivatedGeneratedTasks),
			)
		}

		if t.IsPatchRequest() {
			activatedTasks, err := task.CountActivatedTasksForVersion(ctx, t.Version)
			if err != nil {
				j.AddError(err)
				return
			}
			span.SetAttributes(attribute.Int(generateTasksNumActivatedForVersionAttribute, activatedTasks))
			if activatedTasks > evergreen.NumTasksForLargePatch {
				grip.Info(ctx, message.Fields{
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
			jobs = append(jobs, NewGenerateTasksJob(env, versionID, t.Id, ts))
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

func parseProjectsAsString(ctx context.Context, files task.GeneratedJSONFiles) ([]model.GeneratedProject, error) {
	_, span := tracer.Start(ctx, "parse-projects")
	defer span.End()
	catcher := grip.NewBasicCatcher()
	var projects []model.GeneratedProject
	for _, f := range files {
		p, err := model.ParseProjectFromJSONString(f)
		if err != nil {
			catcher.Add(err)
		}
		projects = append(projects, p)
	}
	return projects, catcher.Resolve()
}
