package alerts

import (
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/alert"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/render"
	"gopkg.in/mgo.v2/bson"
	"path/filepath"
)

// QueueProcessor handles looping over any unprocessed alerts in the queue and delivers them
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
	Task         *model.Task
	Build        *build.Build
	Version      *version.Version
	Patch        *patch.Patch
	Host         *host.Host
	FailedTests  []model.TestResult
	Settings     *evergreen.Settings
}

func (qp *QueueProcessor) Name() string {
	return "alerter"
}

// loadAlertContext fetches details from the database for all documents that are associated with the
// AlertRequest. For example, it populates the task/build/version/project using the
// task/build/version/project ids in the alert request document..
func (qp *QueueProcessor) loadAlertContext(a *alert.AlertRequest) (*AlertContext, error) {
	aCtx := &AlertContext{AlertRequest: a}
	aCtx.Settings = qp.config
	taskId, projectId, buildId, versionId := a.TaskId, a.ProjectId, a.BuildId, a.VersionId
	patchId := a.PatchId
	var err error
	if len(a.HostId) > 0 {
		aCtx.Host, err = host.FindOne(host.ById(a.HostId))
		if err != nil {
			return nil, err
		}
	}
	// Fetch task if there's a task ID present; if we find one, populate build/version IDs from it
	if len(taskId) > 0 {
		aCtx.Task, err = model.FindTask(taskId)
		if err != nil {
			return nil, err
		}
		if aCtx.Task != nil && aCtx.Task.Execution != a.Execution {
			oldTaskId := fmt.Sprintf("%s_%v", taskId, a.Execution)
			aCtx.Task, err = model.FindOneOldTask(bson.M{"_id": oldTaskId}, db.NoProjection, db.NoSort)
			if err != nil {
				return nil, err
			}
		}

		if aCtx.Task != nil {
			// override build and version ID with the ones this task belongs to
			buildId = aCtx.Task.BuildId
			versionId = aCtx.Task.Version
			projectId = aCtx.Task.Project
			aCtx.FailedTests = []model.TestResult{}
			for _, test := range aCtx.Task.TestResults {
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
			return nil, err
		}
		if aCtx.Build != nil {
			versionId = aCtx.Build.Version
			projectId = aCtx.Build.Project
		}
	}
	if len(versionId) > 0 {
		aCtx.Version, err = version.FindOne(version.ById(versionId))
		if err != nil {
			return nil, err
		}
		if aCtx.Version != nil {
			projectId = aCtx.Version.Project
		}
	}

	if len(patchId) > 0 {
		if !patch.IsValidId(patchId) {
			return nil, fmt.Errorf("patch id '%v' is not an object id", patchId)
		}
		aCtx.Patch, err = patch.FindOne(patch.ById(patch.NewId(patchId)).Project(patch.ExcludePatchDiff))
	} else if aCtx.Version != nil {
		// patch isn't in URL but the version in context has one, get it
		aCtx.Patch, err = patch.FindOne(patch.ByVersion(aCtx.Version.Id).Project(patch.ExcludePatchDiff))
	}

	// If there's a finalized patch loaded into context but not a version, load the version
	// associated with the patch as the context's version.
	if aCtx.Version == nil && aCtx.Patch != nil && aCtx.Patch.Version != "" {
		aCtx.Version, err = version.FindOne(version.ById(aCtx.Patch.Version).WithoutFields(version.ConfigKey))
		if err != nil {
			return nil, err
		}
	}

	if len(projectId) > 0 {
		aCtx.ProjectRef, err = qp.findProject(projectId)
		if err != nil {
			return nil, err
		}
	}
	return aCtx, nil
}

// findProject is a wrapper around FindProjectRef that caches results by their ID to prevent
// redundantly querying for the same projectref over and over again.
func (qp *QueueProcessor) findProject(projectId string) (*model.ProjectRef, error) {
	if qp.projectsCache == nil { // lazily initialize the cache
		qp.projectsCache = map[string]*model.ProjectRef{}
	}
	if project, ok := qp.projectsCache[projectId]; ok {
		return project, nil
	}
	project, err := model.FindOneProjectRef(projectId)
	if err != nil {
		return nil, err
	}
	if project == nil {
		return nil, nil
	}
	qp.projectsCache[projectId] = project
	return project, nil
}

// getDeliverer returns the correct implementation of Deliverer according to the provider
// specified in a project's alerts configuration.
func (qp *QueueProcessor) getDeliverer(alertConf model.AlertConfig) (Deliverer, error) {
	// TODO email is the only delivery mechanism supported currently.
	if alertConf.Provider == "email" {
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
	}
	return nil, fmt.Errorf("Unknown provider: %v", alertConf.Provider)
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
			return fmt.Errorf("Failed to get email deliverer: %v", err)
		}
		err = deliverer.Deliver(*ctx, alertConfig)
		if err != nil {
			return fmt.Errorf("Failed to send alert: %v", err)
		}
	}
	return nil
}

// Run loops while there are any unprocessed alerts and attempts to deliver them.
func (qp *QueueProcessor) Run(config *evergreen.Settings) error {
	evergreen.Logger.Logf(slogger.INFO, "Starting alert queue processor run")
	home := evergreen.FindEvergreenHome()
	qp.config = config
	qp.render = render.New(render.Options{
		Directory:    filepath.Join(home, "alerts", "templates"),
		DisableCache: !config.Ui.CacheTemplates,
		Funcs:        nil,
	})

	if len(qp.config.SuperUsers) == 0 {
		evergreen.Logger.Logf(slogger.WARN, "WARNING: No superusers configured, some alerts may have no recipient")
	}
	superUsers, err := user.Find(user.ByIds(qp.config.SuperUsers...))
	if err != nil {
		evergreen.Logger.Logf(slogger.ERROR, "Error getting superuser list: %v", err)
		return err
	}
	qp.superUsersConfigs = []model.AlertConfig{}
	for _, u := range superUsers {
		qp.superUsersConfigs = append(qp.superUsersConfigs, model.AlertConfig{"email", bson.M{"rcpt": u.Email()}})
	}

	evergreen.Logger.Logf(slogger.INFO, "Running alert queue processing")
	for {
		nextAlert, err := alert.DequeueAlertRequest()

		if err != nil {
			evergreen.Logger.Logf(slogger.ERROR, "Failed to dequeue alert request: %v", err)
			return err
		}
		if nextAlert == nil {
			evergreen.Logger.Logf(slogger.INFO, "Reached end of queue items - stopping.")
			break
		}

		evergreen.Logger.Logf(slogger.DEBUG, "Processing queue item %v", nextAlert.Id.Hex())

		alertContext, err := qp.loadAlertContext(nextAlert)
		if err != nil {
			evergreen.Logger.Logf(slogger.ERROR, "Failed to load alert context: %v", err)
			return err
		}

		evergreen.Logger.Logf(slogger.DEBUG, "Delivering queue item %v", nextAlert.Id.Hex())
		err = qp.Deliver(nextAlert, alertContext)
		if err != nil {
			evergreen.Logger.Logf(slogger.ERROR, "Got error delivering message: %v", err)
		}

	}
	evergreen.Logger.Logf(slogger.INFO, "Finished alert queue processor run.")
	return nil
}
