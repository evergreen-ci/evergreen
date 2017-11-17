package alerts

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/admin"
	"github.com/evergreen-ci/evergreen/model/alert"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/render"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

const (
	EmailProvider = "email"
	JiraProvider  = "jira"
	SlackProvider = "slack"
	RunnerName    = "alerter"
)

// QueueProcessor handles looping over any unprocessed alerts in the queue and delivers them.
//
// This runner is used for build and enqueue failure notifications
type QueueProcessor struct {
	config            *evergreen.Settings
	superUsersConfigs []model.AlertConfig
	projectsCache     map[string]*model.ProjectRef
	render            *render.Render
}

// Deliverer is an interface which handles the actual delivery of an alert.
// (e.g. sending an e-mail, posting to flowdock, etc)
type Deliverer interface {
	Deliver(AlertContext, model.AlertConfig) error
}

// AlertContext is the set of full documents in the DB that are associated with the
// values found in a given AlertRequest
type AlertContext struct {
	AlertRequest *alert.AlertRequest
	ProjectRef   *model.ProjectRef
	Task         *task.Task
	Build        *build.Build
	Version      *version.Version
	Patch        *patch.Patch
	Host         *host.Host
	FailedTests  []task.TestResult
	Settings     *evergreen.Settings
}

func (qp *QueueProcessor) Name() string { return RunnerName }

// loadAlertContext fetches details from the database for all documents that are associated with the
// AlertRequest. For example, it populates the task/build/version/project using the
// task/build/version/project ids in the alert requeset document.
func (qp *QueueProcessor) loadAlertContext(a *alert.AlertRequest) (*AlertContext, error) {
	aCtx := &AlertContext{AlertRequest: a}
	aCtx.Settings = qp.config
	taskId, projectId, buildId, versionId := a.TaskId, a.ProjectId, a.BuildId, a.VersionId
	patchId := a.PatchId
	var err error
	if len(a.HostId) > 0 {
		aCtx.Host, err = host.FindOne(host.ById(a.HostId))
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}
	// Fetch task if there's a task ID present; if we find one, populate build/version IDs from it
	if len(taskId) > 0 {
		aCtx.Task, err = task.FindOne(task.ById(taskId))
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if aCtx.Task != nil && aCtx.Task.Execution != a.Execution {
			oldTaskId := fmt.Sprintf("%s_%v", taskId, a.Execution)
			aCtx.Task, err = task.FindOneOld(task.ById(oldTaskId))
			if err != nil {
				return nil, errors.WithStack(err)
			}
		}

		if aCtx.Task != nil {
			// override build and version ID with the ones this task belongs to
			buildId = aCtx.Task.BuildId
			versionId = aCtx.Task.Version
			projectId = aCtx.Task.Project
			aCtx.FailedTests = []task.TestResult{}
			for _, test := range aCtx.Task.LocalTestResults {
				if test.Status == "fail" {
					aCtx.FailedTests = append(aCtx.FailedTests, test)
				}
			}
		}
	}

	// Fetch build if there's a build ID present; if we find one, populate version ID from it
	if len(buildId) > 0 {
		aCtx.Build, err = build.FindOne(build.ById(buildId))
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if aCtx.Build != nil {
			versionId = aCtx.Build.Version
			projectId = aCtx.Build.Project
		}
	}
	if len(versionId) > 0 {
		aCtx.Version, err = version.FindOne(version.ById(versionId))
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if aCtx.Version != nil {
			projectId = aCtx.Version.Identifier
		}
	}

	if len(patchId) > 0 {
		if !patch.IsValidId(patchId) {
			return nil, errors.Errorf("patch id '%s' is not an object id", patchId)
		}
		aCtx.Patch, err = patch.FindOne(patch.ById(patch.NewId(patchId)).Project(patch.ExcludePatchDiff))
		if err != nil {
			return nil, errors.WithStack(err)
		}
	} else if aCtx.Version != nil {
		// patch isn't in URL but the version in context has one, get it
		aCtx.Patch, err = patch.FindOne(patch.ByVersion(aCtx.Version.Id).Project(patch.ExcludePatchDiff))
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}

	// If there's a finalized patch loaded into context but not a version, load the version
	// associated with the patch as the context's version.
	if aCtx.Version == nil && aCtx.Patch != nil && aCtx.Patch.Version != "" {
		aCtx.Version, err = version.FindOne(version.ById(aCtx.Patch.Version).WithoutFields(version.ConfigKey))
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}

	if len(projectId) > 0 {
		aCtx.ProjectRef, err = qp.findProject(projectId)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}
	return aCtx, nil
}

// findProject is a wrapper around FindProjectRef that caches results by their ID to prevent
// redundantly querying for the same projectref over and over
// again. In the Run() method, we wipe the cache at the beginning of each
// run to avoid stale configurations.
func (qp *QueueProcessor) findProject(projectId string) (*model.ProjectRef, error) {
	if qp.projectsCache == nil { // lazily initialize the cache
		qp.projectsCache = map[string]*model.ProjectRef{}
	}
	if project, ok := qp.projectsCache[projectId]; ok {
		return project, nil
	}
	project, err := model.FindOneProjectRef(projectId)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if project == nil {
		return nil, nil
	}
	qp.projectsCache[projectId] = project
	return project, nil
}

func (qp *QueueProcessor) newJIRAProvider(alertConf model.AlertConfig) (Deliverer, error) {
	// load and validate "project" JSON value
	projectField, ok := alertConf.Settings["project"]
	if !ok {
		return nil, errors.New("missing JIRA project field")
	}
	project, ok := projectField.(string)
	if !ok {
		return nil, errors.New("JIRA project name must be string")
	}
	issueField, ok := alertConf.Settings["issue"]
	if !ok {
		return nil, errors.New("missing JIRA issue field")
	}
	issue, ok := issueField.(string)
	if !ok {
		return nil, errors.New("JIRA issue type must be string")
	}
	// validate Evergreen settings
	if (qp.config.Jira.Host == "") || qp.config.Jira.Username == "" || qp.config.Jira.Password == "" {
		return nil, errors.New(
			"invalid JIRA settings (ensure a 'jira' field exists in Evergreen settings)")
	}
	if qp.config.Ui.Url == "" {
		return nil, errors.New("'ui.url' must be set in Evergreen settings")
	}
	handler := thirdparty.NewJiraHandler(
		qp.config.Jira.Host,
		qp.config.Jira.Username,
		qp.config.Jira.Password,
	)
	return &jiraDeliverer{
		project:   project,
		issueType: issue,
		handler:   &handler,
		uiRoot:    qp.config.Ui.Url,
	}, nil
}

func (qp *QueueProcessor) newSlackProvider(alertConfg model.AlertConfig) (Deliverer, error) {
	if qp.config.Slack.Token == "" {
		return nil, errors.New("slack credentials are not stored")
	}

	if qp.config.Ui.Url == "" {
		return nil, errors.New("'ui.url' must be set in Evergreen settings")
	}

	slackChan, ok := alertConfg.Settings["channel"]
	if !ok {
		return nil, errors.New("must specify a slack channel")
	}
	channel, ok := slackChan.(string)
	if !ok {
		return nil, errors.Errorf("slack channel [%+v] must be string [%T]", slackChan, slackChan)
	}

	opts := send.SlackOptions{
		Channel:       channel,
		Fields:        true,
		AllFields:     true,
		BasicMetadata: false,
		Name:          "evergreen-alerts",
	}

	if err := opts.Validate(); err != nil {
		return nil, errors.Wrap(err, "problem constructing slack options")
	}

	sender, err := send.NewSlackLogger(&opts, qp.config.Slack.Token, qp.config.LoggerConfig.Info())
	if err != nil {
		return nil, errors.Wrap(err, "problem constructing slack logger")
	}

	return &slackDeliverer{
		logger: logging.MakeGrip(sender),
		uiRoot: qp.config.Ui.Url,
	}, nil

}

// getDeliverer returns the correct implementation of Deliverer according to the provider
// specified in a project's alerts configuration.
func (qp *QueueProcessor) getDeliverer(alertConf model.AlertConfig) (Deliverer, error) {
	switch alertConf.Provider {
	case SlackProvider:
		return qp.newSlackProvider(alertConf)
	case JiraProvider:
		return qp.newJIRAProvider(alertConf)
	case EmailProvider:
		return &EmailDeliverer{
			SMTPSettings{
				Server:   qp.config.Alerts.SMTP.Server,
				Port:     qp.config.Alerts.SMTP.Port,
				UseSSL:   qp.config.Alerts.SMTP.UseSSL,
				Username: qp.config.Alerts.SMTP.Username,
				Password: qp.config.Alerts.SMTP.Password,
				From:     qp.config.Alerts.SMTP.From,
			},
			qp.render,
		}, nil
	default:
		return nil, errors.Errorf("unknown provider: %v", alertConf.Provider)
	}
}

func (qp *QueueProcessor) Deliver(req *alert.AlertRequest, ctx *AlertContext) error {
	var alertConfigs []model.AlertConfig
	if ctx.ProjectRef != nil {
		// Project-specific alert - use alert configs defined on the project
		// TODO(EVG-223) patch alerts should go to patch owner
		alertConfigs = ctx.ProjectRef.Alerts[req.Trigger]
	} else if ctx.Host != nil {
		// Host-specific alert - use superuser alert configs for now
		// TODO(EVG-224) spawnhost alerts should go to spawnhost owner
		alertConfigs = qp.superUsersConfigs
	}

	for _, alertConfig := range alertConfigs {
		deliverer, err := qp.getDeliverer(alertConfig)
		if err != nil {
			return errors.Wrap(err, "Failed to get email deliverer")
		}
		err = deliverer.Deliver(*ctx, alertConfig)
		if err != nil {
			return errors.Wrap(err, "Failed to send alert")
		}
	}
	return nil
}

// Run loops while there are any unprocessed alerts and attempts to deliver them.
func (qp *QueueProcessor) Run(ctx context.Context, config *evergreen.Settings) error {
	startTime := time.Now()
	adminSettings, err := admin.GetSettings()
	if err != nil {
		return errors.Wrap(err, "error retrieving admin settings")
	}
	if adminSettings.ServiceFlags.AlertsDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"runner":  qp.Name(),
			"message": "alerts are disabled, exiting",
		})
		return nil
	}
	grip.Info(message.Fields{
		"runner":  qp.Name(),
		"status":  "starting",
		"time":    startTime,
		"message": "starting runner process",
	})

	home := evergreen.FindEvergreenHome()
	qp.config = config
	qp.projectsCache = map[string]*model.ProjectRef{} // wipe the project cache between each run to prevent stale configs.
	qp.render = render.New(render.Options{
		Directory:    filepath.Join(home, "alerts", "templates"),
		DisableCache: !config.Ui.CacheTemplates,
		TextFuncs:    nil,
		HtmlFuncs:    nil,
	})

	if len(qp.config.SuperUsers) == 0 {
		grip.Warning("no superusers configured, some alerts may have no recipient")
	}
	superUsers, err := user.Find(user.ByIds(qp.config.SuperUsers...))
	if err != nil {
		err = errors.Wrap(err, "problem getting superuser list")
		grip.Error(message.Fields{
			"runner":  qp.Name(),
			"error":   err.Error(),
			"status":  "failed",
			"runtime": time.Since(startTime),
			"span":    time.Since(startTime).String(),
		})

		return err
	}
	qp.superUsersConfigs = []model.AlertConfig{}
	for _, u := range superUsers {
		qp.superUsersConfigs = append(qp.superUsersConfigs, model.AlertConfig{"email", bson.M{"rcpt": u.Email()}})
	}

	grip.Info("Running alert queue processing")
	for {
		nextAlert, err := alert.DequeueAlertRequest()

		if err != nil {
			err = errors.Wrap(err, "Failed to dequeue alert request")
			grip.Error(message.Fields{
				"runner":  qp.Name(),
				"error":   err.Error(),
				"status":  "failed",
				"runtime": time.Since(startTime),
				"span":    time.Since(startTime).String(),
			})

			return err
		}
		if nextAlert == nil {
			grip.Info("Reached end of queue items - stopping")
			break
		}

		grip.Debugf("Processing queue item %s", nextAlert.Id.Hex())

		alertContext, err := qp.loadAlertContext(nextAlert)
		if err != nil {
			err = errors.Wrap(err, "Failed to load alert context")
			grip.Error(message.Fields{
				"runner":  qp.Name(),
				"error":   err.Error(),
				"status":  "failed",
				"runtime": time.Since(startTime),
				"span":    time.Since(startTime).String(),
			})

			return err
		}

		grip.Debugln("Delivering queue item", nextAlert.Id.Hex())

		grip.Warning(errors.Wrap(qp.Deliver(nextAlert, alertContext),
			"Got error delivering message"))

	}

	grip.Info(message.Fields{
		"runner":  qp.Name(),
		"runtime": time.Since(startTime),
		"span":    time.Since(startTime).String(),
		"status":  "success",
	})

	return nil
}
