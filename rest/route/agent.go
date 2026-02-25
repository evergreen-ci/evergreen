package route

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/githubapp"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/model/s3lifecycle"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testlog"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v70/github"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// GET /rest/v2/agent/perf_monitoring_url
type agentPerfURL struct {
	perfURL string
}

func makeGetPerfURL(perfURL string) *agentPerfURL {
	return &agentPerfURL{
		perfURL: perfURL,
	}
}

func (h *agentPerfURL) Factory() gimlet.RouteHandler {
	return &agentPerfURL{
		perfURL: h.perfURL,
	}
}

func (*agentPerfURL) Parse(_ context.Context, _ *http.Request) error { return nil }

func (h *agentPerfURL) Run(ctx context.Context) gimlet.Responder {
	return gimlet.NewTextResponse(h.perfURL)
}

// GET /rest/v2/agent/setup
type agentSetup struct {
	settings *evergreen.Settings
}

func makeAgentSetup(settings *evergreen.Settings) gimlet.RouteHandler {
	return &agentSetup{
		settings: settings,
	}
}

func (h *agentSetup) Factory() gimlet.RouteHandler {
	return &agentSetup{
		settings: h.settings,
	}
}

func (h *agentSetup) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *agentSetup) Run(ctx context.Context) gimlet.Responder {
	data := apimodels.AgentSetupData{
		SplunkServerURL:    h.settings.Splunk.SplunkConnectionInfo.ServerURL,
		SplunkClientToken:  h.settings.Splunk.SplunkConnectionInfo.Token,
		SplunkChannel:      h.settings.Splunk.SplunkConnectionInfo.Channel,
		TaskOutput:         h.settings.Buckets.Credentials,
		MaxExecTimeoutSecs: h.settings.TaskLimits.MaxExecTimeoutSecs,
		PSLoggingDisabled:  h.settings.ServiceFlags.PSLoggingDisabled,
	}

	if h.settings.Tracer.Enabled {
		data.TraceCollectorEndpoint = h.settings.Tracer.CollectorEndpoint
	}

	return gimlet.NewJSONResponse(data)
}

// POST /task/{task_id}/update_push_status
type updatePushStatusHandler struct {
	taskID  string
	pushLog model.PushLog
}

func makeUpdatePushStatus() gimlet.RouteHandler {
	return &updatePushStatusHandler{}
}

func (h *updatePushStatusHandler) Factory() gimlet.RouteHandler {
	return &updatePushStatusHandler{}
}

func (h *updatePushStatusHandler) Parse(ctx context.Context, r *http.Request) error {
	if h.taskID = gimlet.GetVars(r)["task_id"]; h.taskID == "" {
		return errors.New("missing task ID")
	}
	if err := utility.ReadJSON(r.Body, &h.pushLog); err != nil {
		return errors.Wrap(err, "reading push log from JSON request body")
	}
	return nil
}

// Run updates the status for a file that a task is pushing to s3 for s3 copy
func (h *updatePushStatusHandler) Run(ctx context.Context) gimlet.Responder {
	t, err := task.FindOneId(ctx, h.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", h.taskID))
	}
	if t == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", h.taskID),
		})
	}

	err = errors.Wrapf(h.pushLog.UpdateStatus(ctx, h.pushLog.Status),
		"updating pushlog status failed for task %s", t.Id)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"task":      t.Id,
			"project":   t.Project,
			"version":   t.Version,
			"execution": t.Execution,
		}))
		return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err, "updating pushlog status failed for task %s", t.Id))
	}

	return gimlet.NewJSONResponse(struct{}{})
}

// POST /task/{task_id}/new_push
type newPushHandler struct {
	taskID    string
	s3CopyReq apimodels.S3CopyRequest
}

func makeNewPush() gimlet.RouteHandler {
	return &newPushHandler{}
}

func (h *newPushHandler) Factory() gimlet.RouteHandler {
	return &newPushHandler{}
}

func (h *newPushHandler) Parse(ctx context.Context, r *http.Request) error {
	if h.taskID = gimlet.GetVars(r)["task_id"]; h.taskID == "" {
		return errors.New("missing task ID")
	}
	if err := utility.ReadJSON(r.Body, &h.s3CopyReq); err != nil {
		return errors.Wrap(err, "reading s3 copy request from JSON request body")
	}
	return nil
}

// Run updates when a task is pushing to s3 for s3 copy
func (h *newPushHandler) Run(ctx context.Context) gimlet.Responder {
	t, err := task.FindOneId(ctx, h.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", h.taskID))
	}
	if t == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", h.taskID),
		})
	}

	// Get the version for this task, so we can check if it has
	// any already-done pushes
	v, err := model.VersionFindOne(ctx, model.VersionById(t.Version))
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err, "problem querying task %s with version id %s", t.Id, t.Version))
	}

	// Check for an already-pushed file with this same file path,
	// but from a conflicting or newer commit sequence num
	if v == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("version for task '%s' not found", h.taskID),
		})
	}

	copyToLocation := strings.Join([]string{h.s3CopyReq.S3DestinationBucket, h.s3CopyReq.S3DestinationPath}, "/")

	newestPushLog, err := model.FindPushLogAfter(ctx, copyToLocation, v.RevisionOrderNumber)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err, "querying for push log at '%s' for '%s'", copyToLocation, t.Id))
	}
	if newestPushLog != nil {
		// the error is not being returned in order to avoid a retry
		grip.Warningln("conflict with existing pushed file:", copyToLocation)
		return gimlet.NewJSONResponse(struct{}{})
	}

	// It's now safe to put the file in its permanent location.
	newPushLog := model.NewPushLog(v, t, copyToLocation)
	if err = newPushLog.Insert(ctx); err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err, "creating new push log: %+v", newPushLog))
	}
	return gimlet.NewJSONResponse(newPushLog)
}

// POST /task/{task_id}/restart
type markTaskForRestartHandler struct {
	taskID string
}

func makeMarkTaskForRestart() gimlet.RouteHandler {
	return &markTaskForRestartHandler{}
}

func (h *markTaskForRestartHandler) Factory() gimlet.RouteHandler {
	return &markTaskForRestartHandler{}
}

func (h *markTaskForRestartHandler) Parse(ctx context.Context, r *http.Request) error {
	if h.taskID = gimlet.GetVars(r)["task_id"]; h.taskID == "" {
		return errors.New("missing task ID")
	}
	return nil
}

func (h *markTaskForRestartHandler) Run(ctx context.Context) gimlet.Responder {
	t, err := task.FindOneId(ctx, h.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", h.taskID))
	}
	if t == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", h.taskID),
		})
	}
	taskToRestart := t
	if t.IsPartOfDisplay(ctx) {
		dt, err := t.GetDisplayTask(ctx)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "getting display task for execution task '%s'", h.taskID))
		}
		taskToRestart = dt
	}
	// If the task is a display task that has already been marked for an automatic restart
	// by another execution task, we don't want to mark it for a restart again nor do we
	// want to error out since the display task has not been automatically restarted yet.
	if taskToRestart.IsAutomaticRestart {
		return gimlet.NewJSONResponse(struct{}{})
	}

	if taskToRestart.NumAutomaticRestarts >= evergreen.MaxAutomaticRestarts {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("task has already reached the maximum (%d) number of automatic restarts", evergreen.MaxAutomaticRestarts),
		})
	}
	projectRef, err := model.FindMergedProjectRef(ctx, t.Project, t.Version, false)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding project '%s' for version '%s'", t.Project, t.Version))
	}
	if projectRef == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    errors.Errorf("project '%s' not found", t.Project).Error(),
		})
	}
	settings, err := evergreen.GetConfig(ctx)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting admin settings"))
	}
	maxDailyAutoRestarts := settings.TaskLimits.MaxDailyAutomaticRestarts
	if err = projectRef.CheckAndUpdateAutoRestartLimit(ctx, maxDailyAutoRestarts); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "checking auto restart limit for '%s'", projectRef.Id))
	}
	if err = taskToRestart.SetResetWhenFinishedWithInc(ctx); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "setting reset when finished for task '%s'", h.taskID))
	}
	return gimlet.NewJSONResponse(struct{}{})
}

// GET /task/{task_id}/expansions_and_vars
type getExpansionsAndVarsHandler struct {
	settings *evergreen.Settings
	taskID   string
	hostID   string
}

func makeGetExpansionsAndVars(settings *evergreen.Settings) gimlet.RouteHandler {
	return &getExpansionsAndVarsHandler{
		settings: settings,
	}
}

func (h *getExpansionsAndVarsHandler) Factory() gimlet.RouteHandler {
	return &getExpansionsAndVarsHandler{
		settings: h.settings,
	}
}

func (h *getExpansionsAndVarsHandler) Parse(ctx context.Context, r *http.Request) error {
	if h.taskID = gimlet.GetVars(r)["task_id"]; h.taskID == "" {
		return errors.New("missing task ID")
	}
	h.hostID = r.Header.Get(evergreen.HostHeader)
	podID := r.Header.Get(evergreen.PodHeader)
	userKey := r.Header.Get(evergreen.AuthorizationHeader)
	if h.hostID == "" && podID == "" && userKey == "" {
		return errors.New("missing both host and pod ID")
	}
	return nil
}

// hostServicePasswordPlaceholder is the placeholder name for a service user's
// password on a host. If the service user's password is present in any task
// logs, it will be replaced with this placeholder string.
const hostServicePasswordPlaceholder = "host_service_password"

func (h *getExpansionsAndVarsHandler) Run(ctx context.Context) gimlet.Responder {
	t, err := task.FindOneId(ctx, h.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", h.taskID))
	}
	if t == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", h.taskID),
		})
	}
	var foundHost *host.Host
	if h.hostID != "" {
		foundHost, err = host.FindOneId(ctx, h.hostID)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "getting host '%s'", h.hostID))
		}
		if foundHost == nil {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusNotFound,
				Message:    fmt.Sprintf("host '%s' not found", h.hostID)},
			)
		}
	}

	pRef, err := model.FindBranchProjectRef(ctx, t.Project)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding project ref '%s'", t.Project))
	}
	if pRef == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("project ref '%s' not found", t.Project),
		})
	}
	knownHosts := h.settings.Expansions[evergreen.GithubKnownHosts]
	e, err := model.PopulateExpansions(ctx, t, foundHost, knownHosts)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "populating expansions"))
	}

	res := apimodels.ExpansionsAndVars{
		Expansions:         e,
		Parameters:         map[string]string{},
		Vars:               map[string]string{},
		PrivateVars:        map[string]bool{},
		RedactKeys:         h.settings.LoggerConfig.RedactKeys,
		InternalRedactions: map[string]string{},
	}

	if foundHost != nil && foundHost.ServicePassword != "" {
		res.InternalRedactions[hostServicePasswordPlaceholder] = foundHost.ServicePassword
	}

	projectVars, err := model.FindMergedProjectVars(ctx, t.Project)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting merged project vars"))
	}
	if projectVars != nil {
		res.Vars = projectVars.GetVars(ctx, t)
		if projectVars.PrivateVars != nil {
			res.PrivateVars = projectVars.PrivateVars
		}

		user := gimlet.GetUser(ctx)
		isUserRequest := user != nil
		// If from debug session request, filter out admin-only vars if user is not an admin
		isAdmin := isUserRequest && user.HasPermission(ctx, gimlet.PermissionOpts{
			Resource:      pRef.Id,
			ResourceType:  evergreen.ProjectResourceType,
			Permission:    evergreen.PermissionProjectSettings,
			RequiredLevel: evergreen.ProjectSettingsEdit.Value,
		})
		if isUserRequest && projectVars.AdminOnlyVars != nil && !isAdmin {
			for adminOnlyVar := range projectVars.AdminOnlyVars {
				if projectVars.AdminOnlyVars[adminOnlyVar] {
					delete(res.Vars, adminOnlyVar)
				}
			}
		}
	}

	v, err := model.VersionFindOne(ctx, model.VersionById(t.Version).WithFields(model.VersionParametersKey))
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding version '%s'", t.Version))
	}
	if v == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("version '%s' not found", t.Version),
		})
	}

	for _, param := range v.Parameters {
		res.Parameters[param.Key] = param.Value
	}

	return gimlet.NewJSONResponse(res)
}

// GET /task/{task_id}/project_ref
type getProjectRefHandler struct {
	taskID string
}

func makeGetProjectRef() gimlet.RouteHandler {
	return &getProjectRefHandler{}
}

func (h *getProjectRefHandler) Factory() gimlet.RouteHandler {
	return &getProjectRefHandler{}
}

func (h *getProjectRefHandler) Parse(ctx context.Context, r *http.Request) error {
	if h.taskID = gimlet.GetVars(r)["task_id"]; h.taskID == "" {
		return errors.New("missing task ID")
	}
	return nil
}

func (h *getProjectRefHandler) Run(ctx context.Context) gimlet.Responder {
	t, err := task.FindOneId(ctx, h.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", h.taskID))
	}
	if t == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", h.taskID),
		})
	}

	p, err := model.FindMergedProjectRef(ctx, t.Project, t.Version, true)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}

	if p == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("project ref '%s' not found", t.Project),
		})
	}
	user := gimlet.GetUser(ctx)
	isUserRequest := user != nil
	// If from debug session request, return minimal response
	if isUserRequest {
		redactedProjectRef := map[string]interface{}{
			"repo_name":      p.Repo,
			"branch_name":    p.Branch,
			"owner_name":     p.Owner,
			"id":             p.Id,
			"repo_ref_id":    p.RepoRefId,
			"identifier":     p.Identifier,
			"test_selection": p.TestSelection,
		}
		return gimlet.NewJSONResponse(redactedProjectRef)
	}

	return gimlet.NewJSONResponse(p)
}

// GET /task/{task_id}/parser_project
type getParserProjectHandler struct {
	taskID string
	env    evergreen.Environment
}

func makeGetParserProject(env evergreen.Environment) gimlet.RouteHandler {
	return &getParserProjectHandler{env: env}
}

func (h *getParserProjectHandler) Factory() gimlet.RouteHandler {
	return &getParserProjectHandler{env: h.env}
}

func (h *getParserProjectHandler) Parse(ctx context.Context, r *http.Request) error {
	if h.taskID = gimlet.GetVars(r)["task_id"]; h.taskID == "" {
		return errors.New("missing task ID")
	}
	return nil
}

func (h *getParserProjectHandler) Run(ctx context.Context) gimlet.Responder {
	t, err := task.FindOneId(ctx, h.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", h.taskID))
	}
	if t == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", h.taskID),
		})
	}
	v, err := model.VersionFindOne(ctx, model.VersionById(t.Version).WithFields(model.VersionIdKey, model.VersionProjectStorageMethodKey))
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}

	if v == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("version '%s' not found", t.Version),
		})
	}

	pp, err := model.ParserProjectFindOneByID(ctx, h.env.Settings(), v.ProjectStorageMethod, v.Id)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding parser project '%s'", v.Id))
	}
	if pp == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("parser project '%s' not found", v.Id),
		})
	}
	projBytes, err := pp.MarshalBSON()
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "marshalling project bytes to bson"))
	}
	return gimlet.NewBinaryResponse(projBytes)
}

// GET /task/{task_id}/distro_view
type getDistroViewHandler struct {
	hostID string
}

func makeGetDistroView() gimlet.RouteHandler {
	return &getDistroViewHandler{}
}

func (h *getDistroViewHandler) Factory() gimlet.RouteHandler {
	return &getDistroViewHandler{}
}

func (h *getDistroViewHandler) Parse(ctx context.Context, r *http.Request) error {
	if h.hostID = r.Header.Get(evergreen.HostHeader); h.hostID == "" {
		return errors.New("missing host ID")
	}
	return nil
}

func (h *getDistroViewHandler) Run(ctx context.Context) gimlet.Responder {
	host, err := host.FindOneId(ctx, h.hostID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting host"))
	}
	if host == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("host '%s' not found", h.hostID)},
		)
	}

	dv := apimodels.DistroView{
		DisableShallowClone: host.Distro.DisableShallowClone,
		Mountpoints:         host.Distro.Mountpoints,
		ExecUser:            host.Distro.ExecUser,
	}
	return gimlet.NewJSONResponse(dv)
}

// GET /task/{task_id}/host_view
type getHostViewHandler struct {
	hostID string
}

func makeGetHostView() gimlet.RouteHandler {
	return &getHostViewHandler{}
}

func (rh *getHostViewHandler) Factory() gimlet.RouteHandler {
	return &getHostViewHandler{}
}

func (rh *getHostViewHandler) Parse(ctx context.Context, r *http.Request) error {
	if rh.hostID = r.Header.Get(evergreen.HostHeader); rh.hostID == "" {
		return errors.New("missing host ID")
	}
	return nil
}

func (rh *getHostViewHandler) Run(ctx context.Context) gimlet.Responder {
	h, err := host.FindOneId(ctx, rh.hostID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting host"))
	}
	if h == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("host '%s' not found", rh.hostID)},
		)
	}

	hv := apimodels.HostView{
		Hostname: h.Host,
	}
	return gimlet.NewJSONResponse(hv)
}

// POST /task/{task_id}/files
type attachFilesHandler struct {
	taskID string
	files  []artifact.File
}

func makeAttachFiles() gimlet.RouteHandler {
	return &attachFilesHandler{}
}

func (h *attachFilesHandler) Factory() gimlet.RouteHandler {
	return &attachFilesHandler{}
}

func (h *attachFilesHandler) Parse(ctx context.Context, r *http.Request) error {
	if h.taskID = gimlet.GetVars(r)["task_id"]; h.taskID == "" {
		return errors.New("missing task ID")
	}
	err := utility.ReadJSON(r.Body, &h.files)
	if err != nil {
		message := fmt.Sprintf("reading file definitions for task  %s: %v", h.taskID, err)
		grip.Error(message)
		return errors.Wrap(err, message)
	}
	return nil
}

// Run updates file mappings for a task or build
func (h *attachFilesHandler) Run(ctx context.Context) gimlet.Responder {
	t, err := task.FindOneId(ctx, h.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", h.taskID))
	}
	if t == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", h.taskID),
		})
	}

	discoverAndCacheBucketLifecycleRules(ctx, t, h.files)

	entry := &artifact.Entry{
		TaskId:          t.Id,
		TaskDisplayName: t.DisplayName,
		BuildId:         t.BuildId,
		Execution:       t.Execution,
		CreateTime:      time.Now(),
		Files:           artifact.EscapeFiles(h.files),
	}

	if err = entry.Upsert(ctx); err != nil {
		message := fmt.Sprintf("updating artifact file info for task %s: %v", t.Id, err)
		grip.Error(message)
		return gimlet.MakeJSONInternalErrorResponder(errors.New(message))
	}
	return gimlet.NewJSONResponse(fmt.Sprintf("Artifact files for task %s successfully attached", t.Id))
}

// discoverAndCacheBucketLifecycleRules will look at all the buckets that the files are being uploaded
// to and check if we have lifecycle rules cached for them. If not, it will attempt to discover
// and cache them. This is best-effort and will not fail the file upload if discovery fails.
func discoverAndCacheBucketLifecycleRules(ctx context.Context, t *task.Task, files []artifact.File) {
	bucketsToDiscover := make(map[string]*artifact.File)
	for i := range files {
		file := &files[i]
		if file.Bucket == "" {
			continue
		}

		if _, exists := bucketsToDiscover[file.Bucket]; !exists {
			bucketsToDiscover[file.Bucket] = file
		}
	}

	cachedBuckets := []string{}
	for bucketName, file := range bucketsToDiscover {
		region := evergreen.DefaultS3Region

		var roleARN *string
		if file.AWSRoleARN != "" {
			roleARN = &file.AWSRoleARN
		}

		var externalID *string
		if file.ExternalID != "" {
			externalID = &file.ExternalID
		}

		wasCached := s3lifecycle.DiscoverAndCacheProjectBucket(ctx, bucketName, region, roleARN, externalID, t.Project, cloud.NewS3LifecycleClient())
		if wasCached {
			cachedBuckets = append(cachedBuckets, bucketName)
		}
	}

	if len(cachedBuckets) > 0 {
		grip.Info(message.Fields{
			"message":    "successfully cached bucket lifecycle rules",
			"buckets":    cachedBuckets,
			"task_id":    t.Id,
			"project":    t.Project,
			"num_cached": len(cachedBuckets),
		})
	}
}

// POST /rest/v2/task/{task_id}/set_results_info
type setTaskResultsInfoHandler struct {
	taskID string
	info   apimodels.TaskTestResultsInfo
}

func makeSetTaskResultsInfoHandler() gimlet.RouteHandler {
	return &setTaskResultsInfoHandler{}
}

func (h *setTaskResultsInfoHandler) Factory() gimlet.RouteHandler {
	return &setTaskResultsInfoHandler{}
}

func (h *setTaskResultsInfoHandler) Parse(ctx context.Context, r *http.Request) error {
	h.taskID = gimlet.GetVars(r)["task_id"]

	if err := gimlet.GetJSON(r.Body, &h.info); err != nil {
		return errors.Wrap(err, "reading test results info from JSON request body")
	}

	return nil
}

func (h *setTaskResultsInfoHandler) Run(ctx context.Context) gimlet.Responder {
	t, err := task.FindOneId(ctx, h.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", h.taskID))
	}
	if t == nil {
		return gimlet.MakeJSONInternalErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", h.taskID),
		})
	}

	if err = t.SetResultsInfo(ctx, h.info.Failed); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "setting results info for task '%s'", h.taskID))
	}

	return gimlet.NewTextResponse("Results info set in task")
}

// POST /task/{task_id}/test_logs
type attachTestLogHandler struct {
	settings *evergreen.Settings
	taskID   string
	log      testlog.TestLog
}

func makeAttachTestLog(settings *evergreen.Settings) gimlet.RouteHandler {
	return &attachTestLogHandler{
		settings: settings,
	}
}

func (h *attachTestLogHandler) Factory() gimlet.RouteHandler {
	return &attachTestLogHandler{
		settings: h.settings,
	}
}

func (h *attachTestLogHandler) Parse(ctx context.Context, r *http.Request) error {
	if h.taskID = gimlet.GetVars(r)["task_id"]; h.taskID == "" {
		return errors.New("missing task ID")
	}
	err := utility.ReadJSON(r.Body, &h.log)
	if err != nil {
		return errors.Wrap(err, "reading test log from JSON request body")
	}
	return nil
}

// Run is the API Server hook for getting
// the test logs and storing them in the test_logs collection.
func (h *attachTestLogHandler) Run(ctx context.Context) gimlet.Responder {
	if h.settings.ServiceFlags.TaskLoggingDisabled {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusConflict,
			Message:    "task logging is disabled",
		})
	}
	t, err := task.FindOneId(ctx, h.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", h.taskID))
	}
	if t == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", h.taskID),
		})
	}

	// enforce proper taskID and Execution
	h.log.Task = t.Id
	h.log.TaskExecution = t.Execution

	grip.Debug(message.Fields{
		"message":      "received test log",
		"task":         t.Id,
		"project":      t.Project,
		"requester":    t.Requester,
		"version":      t.Version,
		"display_name": t.DisplayName,
		"execution":    t.Execution,
		"log_length":   len(h.log.Lines),
	})

	if err = h.log.Insert(ctx); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}
	logReply := struct {
		Id string `json:"_id"`
	}{h.log.Id}
	return gimlet.NewJSONResponse(logReply)
}

// POST /task/{task_id}/test_results
type attachTestResultsHandler struct {
	env    evergreen.Environment
	body   apimodels.AttachTestResultsRequest
	taskID string
}

func makeAttachTestResults(env evergreen.Environment) gimlet.RouteHandler {
	return &attachTestResultsHandler{
		env: env,
	}
}

func (h *attachTestResultsHandler) Factory() gimlet.RouteHandler {
	return &attachTestResultsHandler{
		env: h.env,
	}
}

func (h *attachTestResultsHandler) Parse(ctx context.Context, r *http.Request) error {
	if h.taskID = gimlet.GetVars(r)["task_id"]; h.taskID == "" {
		return errors.New("missing task ID")
	}
	err := utility.ReadJSON(r.Body, &h.body)
	if err != nil {
		return errors.Wrap(err, "reading test results from JSON request body")
	}
	return nil
}

func (h *attachTestResultsHandler) Run(ctx context.Context) gimlet.Responder {
	t, err := task.FindOneId(ctx, h.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", h.taskID))
	}
	if t == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", h.taskID),
		})
	}
	var record testresult.DbTaskTestResults
	if !t.HasTestResults {
		record = testresult.DbTaskTestResults{
			ID:        h.body.Info.ID(),
			Info:      h.body.Info,
			CreatedAt: h.body.CreatedAt,
		}
		_, err = h.env.CedarDB().Collection(testresult.Collection).InsertOne(ctx, record)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "inserting test result record"))
		}
	} else {
		err = h.env.CedarDB().Collection(testresult.Collection).FindOne(ctx, task.ByTaskIDAndExecution(h.body.Info.TaskID, h.body.Info.Execution)).Decode(&record)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "finding test result record"))
		}
	}
	err = task.AppendTestResultMetadata(ctx, t, h.env, h.body.FailedSample, h.body.Stats.FailedCount, h.body.Stats.TotalCount, record)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "appending test results to '%s'", h.taskID))
	}
	return gimlet.NewJSONResponse(struct{}{})
}

// POST /task/{task_id}/heartbeat
type heartbeatHandler struct {
	taskID string
}

func makeHeartbeat() gimlet.RouteHandler {
	return &heartbeatHandler{}
}

func (h *heartbeatHandler) Factory() gimlet.RouteHandler {
	return &heartbeatHandler{}
}

func (h *heartbeatHandler) Parse(ctx context.Context, r *http.Request) error {
	if h.taskID = gimlet.GetVars(r)["task_id"]; h.taskID == "" {
		return errors.New("missing task ID")
	}
	return nil
}

// Run handles heartbeat pings from Evergreen agents. If the heartbeating
// task is marked to be aborted, the abort response is sent.
func (h *heartbeatHandler) Run(ctx context.Context) gimlet.Responder {
	t, err := task.FindOneId(ctx, h.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", h.taskID))
	}
	if t == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", h.taskID),
		})
	}

	heartbeatResponse := apimodels.HeartbeatResponse{}
	if t.Aborted {
		grip.Noticef("sending abort signal for task %s", t.Id)
		heartbeatResponse.Abort = true
	}

	if err := t.UpdateHeartbeat(ctx); err != nil {
		grip.Warningf("updating heartbeat for task %s: %+v", t.Id, err)
	}
	return gimlet.NewJSONResponse(heartbeatResponse)
}

// GET /task/{task_id}/
type fetchTaskHandler struct {
	taskID string
}

func makeFetchTask() gimlet.RouteHandler {
	return &fetchTaskHandler{}
}

func (h *fetchTaskHandler) Factory() gimlet.RouteHandler {
	return &fetchTaskHandler{}
}

func (h *fetchTaskHandler) Parse(ctx context.Context, r *http.Request) error {
	if h.taskID = gimlet.GetVars(r)["task_id"]; h.taskID == "" {
		return errors.New("missing task ID")
	}
	return nil
}

// Run loads the task from the database and sends it to the requester.
func (h *fetchTaskHandler) Run(ctx context.Context) gimlet.Responder {
	t, err := task.FindOneId(ctx, h.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", h.taskID))
	}
	if t == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", h.taskID),
		})
	}
	// Remove secret if request is coming from debug session
	user := gimlet.GetUser(ctx)
	isUserRequest := user != nil
	if isUserRequest {
		t.Secret = ""
	}
	return gimlet.NewJSONResponse(t)
}

// POST /task/{task_id}/start
type startTaskHandler struct {
	env           evergreen.Environment
	taskID        string
	hostID        string
	podID         string
	taskStartInfo apimodels.TaskStartRequest
}

func makeStartTask(env evergreen.Environment) gimlet.RouteHandler {
	return &startTaskHandler{
		env: env,
	}
}

func (h *startTaskHandler) Factory() gimlet.RouteHandler {
	return &startTaskHandler{
		env: h.env,
	}
}

func (h *startTaskHandler) Parse(ctx context.Context, r *http.Request) error {
	if h.taskID = gimlet.GetVars(r)["task_id"]; h.taskID == "" {
		return errors.New("missing task ID")
	}
	if err := utility.ReadJSON(r.Body, &h.taskStartInfo); err != nil {
		return errors.Wrapf(err, "reading task start request for %s", h.taskID)
	}
	h.hostID = r.Header.Get(evergreen.HostHeader)
	h.podID = r.Header.Get(evergreen.PodHeader)
	if h.hostID == "" && h.podID == "" {
		return errors.New("missing both host and pod ID")
	}
	return nil
}

// Run retrieves the task from the request and acquires the global lock.
// With the lock, it marks associated tasks, builds, and versions as started.
// It then updates the host document with relevant information, including the pid
// of the agent, and ensures that the host has the running task field set.
func (h *startTaskHandler) Run(ctx context.Context) gimlet.Responder {
	var err error

	t, err := task.FindOneId(ctx, h.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", h.taskID))
	}
	if t == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", h.taskID),
		})
	}
	dependenciesMetTime := t.ScheduledTime
	if t.DependenciesMetTime.After(dependenciesMetTime) {
		dependenciesMetTime = t.DependenciesMetTime
	}
	grip.Debug(message.Fields{
		"message":                        "marking task started",
		"task_id":                        t.Id,
		"details":                        t.Details,
		"seconds_since_dependencies_met": time.Since(dependenciesMetTime).Seconds(),
	})

	updates := model.StatusChanges{}
	if err = model.MarkStart(ctx, t, &updates); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "marking task '%s' started", t.Id))
	}
	model.UpdateOtelMetadata(ctx, t, h.taskStartInfo.DiskDevices, h.taskStartInfo.TraceID)

	if len(updates.PatchNewStatus) != 0 {
		event.LogPatchStateChangeEvent(ctx, t.Version, updates.PatchNewStatus)
	}
	if len(updates.BuildNewStatus) != 0 {
		event.LogBuildStateChangeEvent(ctx, t.BuildId, updates.BuildNewStatus)
	}

	var msg string
	var foundHost *host.Host
	var foundPod *pod.Pod
	if h.hostID != "" {
		foundHost, err = host.FindOneByTaskIdAndExecution(ctx, t.Id, t.Execution)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding host running task %s", t.Id))
		}

		if foundHost == nil {
			message := fmt.Sprintf("no host found running task %s", t.Id)
			if t.HostId != "" {
				message = fmt.Sprintf("no host found running task %s but task is said to be running on %s",
					t.Id, t.HostId)
			}

			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusNotFound,
				Message:    message,
			})
		}

		msg = fmt.Sprintf("task %s started on host %s", t.Id, foundHost.Id)

		if foundHost.Distro.IsEphemeral() {
			if err = foundHost.IncTaskCount(ctx); err != nil {
				return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "incrementing task count for task '%s' on host '%s'", t.Id, foundHost.Id))
			}
			if err = foundHost.IncIdleTime(ctx, foundHost.WastedComputeTime()); err != nil {
				return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "incrementing total idle time on host '%s'", foundHost.Id))
			}
			grip.Info(foundHost.TaskStartMessage())
		}
	} else {
		foundPod, err = pod.FindOneByID(ctx, h.podID)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding pod running task %s", t.Id))
		}
		if foundPod == nil {
			message := fmt.Sprintf("no pod found running task %s", t.Id)
			if t.PodID != "" {
				message = fmt.Sprintf("no pod found running task %s but task is said to be running on %s",
					t.Id, t.PodID)
			}
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusNotFound,
				Message:    message,
			})
		}
		msg = fmt.Sprintf("task '%s' started on pod '%s'", t.Id, h.podID)
	}
	logTaskStartMessage(foundHost, foundPod, t)
	return gimlet.NewJSONResponse(msg)
}

func logTaskStartMessage(h *host.Host, p *pod.Pod, t *task.Task) {
	msg := message.Fields{
		"stat":                   "task-start-stats",
		"task_id":                t.Id,
		"execution":              t.Execution,
		"version":                t.Version,
		"build":                  t.BuildId,
		"scheduled_time":         t.ScheduledTime,
		"activated_latency_secs": t.StartTime.Sub(t.ActivatedTime).Seconds(),
		"scheduled_latency_secs": t.StartTime.Sub(t.ScheduledTime).Seconds(),
		"started_latency_secs":   t.StartTime.Sub(t.DispatchTime).Seconds(),
		"generator":              t.GenerateTask,
		"group":                  t.TaskGroup,
		"group_max_hosts":        t.TaskGroupMaxHosts,
		"project":                t.Project,
		"requester":              t.Requester,
		"priority":               t.Priority,
		"task":                   t.DisplayName,
		"display_task":           t.DisplayOnly,
		"variant":                t.BuildVariant,
	}

	if !t.DependenciesMetTime.IsZero() {
		msg["dependencies_met_time"] = t.DependenciesMetTime
	}

	if t.ActivatedBy != "" {
		msg["activated_by"] = t.ActivatedBy
	}

	if h != nil {
		msg["distro"] = h.Distro.Id
		msg["host_id"] = h.Id
		msg["provider"] = h.Distro.Provider
		msg["provisioning"] = h.Distro.BootstrapSettings.Method
		if strings.HasPrefix(h.Distro.Provider, "ec2") {
			msg["provider"] = "ec2"
		}
		if h.Provider != evergreen.ProviderNameStatic {
			msg["host_task_count"] = h.TaskCount

			if h.TaskCount == 1 {
				msg["host_provision_time"] = h.TotalIdleTime.Seconds()
			}
		}
	} else if p != nil {
		msg["pod_id"] = p.ID
		msg["pod_provision_time"] = time.Since(p.TimeInfo.Starting).Seconds()
	}
	grip.Info(msg)
}

// GET /task/{task_id}/git/patchfile/{patchfile_id}
type gitServePatchFileHandler struct {
	taskID  string
	patchID string
}

func makeGitServePatchFile() gimlet.RouteHandler {
	return &gitServePatchFileHandler{}
}

func (h *gitServePatchFileHandler) Factory() gimlet.RouteHandler {
	return &gitServePatchFileHandler{}
}

func (h *gitServePatchFileHandler) Parse(ctx context.Context, r *http.Request) error {
	if h.taskID = gimlet.GetVars(r)["task_id"]; h.taskID == "" {
		return errors.New("missing task ID")
	}
	if h.patchID = gimlet.GetVars(r)["patchfile_id"]; h.patchID == "" {
		return errors.New("missing patch ID")
	}
	return nil
}

func (h *gitServePatchFileHandler) Run(ctx context.Context) gimlet.Responder {
	patchContents, err := patch.FetchPatchContents(ctx, h.patchID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "reading patch file from db"))
	}
	return gimlet.NewTextResponse(patchContents)
}

// GET /task/{task_id}/patch
type servePatchHandler struct {
	taskID string
}

func makeServePatch() gimlet.RouteHandler {
	return &servePatchHandler{}
}

func (h *servePatchHandler) Factory() gimlet.RouteHandler {
	return &servePatchHandler{}
}

func (h *servePatchHandler) Parse(ctx context.Context, r *http.Request) error {
	if h.taskID = gimlet.GetVars(r)["task_id"]; h.taskID == "" {
		return errors.New("missing task ID")
	}
	return nil
}

func (h *servePatchHandler) Run(ctx context.Context) gimlet.Responder {
	t, err := task.FindOneId(ctx, h.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", h.taskID))
	}
	if t == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", h.taskID),
		})
	}

	p, err := patch.FindOne(ctx, patch.ByVersion(t.Version))
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding patch '%s'", t.Version))
	}
	if p == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("patch with ID '%s' not found", t.Version),
		})
	}

	return gimlet.NewJSONResponse(p)
}

// GET /task/{task_id}/version
type serveVersionHandler struct {
	taskID string
}

func makeServeVersion() gimlet.RouteHandler {
	return &serveVersionHandler{}
}

func (h *serveVersionHandler) Factory() gimlet.RouteHandler {
	return &serveVersionHandler{}
}

func (h *serveVersionHandler) Parse(ctx context.Context, r *http.Request) error {
	if h.taskID = gimlet.GetVars(r)["task_id"]; h.taskID == "" {
		return errors.New("missing task ID")
	}
	return nil
}

func (h *serveVersionHandler) Run(ctx context.Context) gimlet.Responder {
	t, err := task.FindOneId(ctx, h.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", h.taskID))
	}
	if t == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", h.taskID),
		})
	}

	v, err := model.VersionFindOneId(ctx, t.Version)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding version '%s'", t.Version))
	}
	if v == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("version with ID '%s' not found", t.Version),
		})
	}

	return gimlet.NewJSONResponse(v)
}

// POST /task/{task_id}/keyval/inc
type keyvalIncHandler struct {
	key string
}

func makeKeyvalPluginInc() gimlet.RouteHandler {
	return &keyvalIncHandler{}
}

func (h *keyvalIncHandler) Factory() gimlet.RouteHandler {
	return &keyvalIncHandler{}
}

func (h *keyvalIncHandler) Parse(ctx context.Context, r *http.Request) error {
	err := utility.ReadJSON(r.Body, &h.key)
	if err != nil {
		return errors.Wrap(err, "could not get key")
	}
	return nil
}

func (h *keyvalIncHandler) Run(ctx context.Context) gimlet.Responder {
	keyVal := &model.KeyVal{Key: h.key}
	if err := keyVal.Inc(ctx); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "doing findAndModify on key '%s'", h.key))
	}
	return gimlet.NewJSONResponse(keyVal)
}

// GET /task/{task_id}/manifest/load
type manifestLoadHandler struct {
	taskID   string
	settings *evergreen.Settings
}

func makeManifestLoad(settings *evergreen.Settings) gimlet.RouteHandler {
	return &manifestLoadHandler{
		settings: settings,
	}
}

func (h *manifestLoadHandler) Factory() gimlet.RouteHandler {
	return &manifestLoadHandler{
		settings: h.settings,
	}
}

func (h *manifestLoadHandler) Parse(ctx context.Context, r *http.Request) error {
	if h.taskID = gimlet.GetVars(r)["task_id"]; h.taskID == "" {
		return errors.New("missing task ID")
	}
	return nil
}

// Run attempts to get the manifest, if it exists it updates the expansions and returns
// If it does not exist it performs GitHub API calls for each of the project's modules and gets
// the head revision of the branch and inserts it into the manifest collection.
// If there is a duplicate key error, then do a find on the manifest again.
func (h *manifestLoadHandler) Run(ctx context.Context) gimlet.Responder {
	task, err := task.FindOneId(ctx, h.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", h.taskID))
	}
	if task == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", h.taskID),
		})
	}

	projectRef, err := model.FindMergedProjectRef(ctx, task.Project, task.Version, true)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding project '%s'", task.Project))
	}
	if projectRef == nil {
		return gimlet.MakeJSONErrorResponder(errors.Errorf("project ref '%s' doesn't exist", task.Project))
	}

	v, err := model.VersionFindOne(ctx, model.VersionById(task.Version))
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "retrieving version for task"))
	}
	if v == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("version not found: %s", task.Version),
		})
	}
	currentManifest, err := manifest.FindFromVersion(ctx, v.Id, v.Identifier, v.Revision, v.Requester)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "retrieving manifest with version id '%s'", task.Version))
	}

	env := evergreen.GetEnvironment()
	project, _, err := model.FindAndTranslateProjectForVersion(ctx, env.Settings(), v, false)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "loading project from version"))
	}
	if project == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    "unable to find project for version",
		})
	}

	if currentManifest != nil && project.Modules.IsIdentical(*currentManifest) {
		return gimlet.NewJSONResponse(currentManifest)
	}

	// attempt to insert a manifest after making GitHub API calls
	manifest, err := model.CreateManifest(ctx, v, project.Modules, projectRef)
	if err != nil {
		if apiErr, ok := errors.Cause(err).(thirdparty.APIRequestError); ok && apiErr.StatusCode == http.StatusNotFound {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "manifest resource not found"))
		}
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "storing new manifest"))
	}
	return gimlet.NewJSONResponse(manifest)
}

// POST /task/{task_id}/downstreamParams
type setDownstreamParamsHandler struct {
	taskID           string
	downstreamParams []patch.Parameter
}

func makeSetDownstreamParams() gimlet.RouteHandler {
	return &setDownstreamParamsHandler{}
}

func (h *setDownstreamParamsHandler) Factory() gimlet.RouteHandler {
	return &setDownstreamParamsHandler{}
}

func (h *setDownstreamParamsHandler) Parse(ctx context.Context, r *http.Request) error {
	if h.taskID = gimlet.GetVars(r)["task_id"]; h.taskID == "" {
		return errors.New("missing task ID")
	}
	err := utility.ReadJSON(r.Body, &h.downstreamParams)
	if err != nil {
		errorMessage := fmt.Sprintf("reading downstream expansions for task %s", h.taskID)
		grip.Error(message.Fields{
			"message": errorMessage,
			"task_id": h.taskID,
		})
		return errors.Wrap(err, errorMessage)
	}
	return nil
}

// Run updates file mappings for a task or build.
func (h *setDownstreamParamsHandler) Run(ctx context.Context) gimlet.Responder {
	t, err := task.FindOneId(ctx, h.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", h.taskID))
	}
	if t == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", h.taskID),
		})
	}
	grip.Infoln("Setting downstream expansions for task:", t.Id)

	p, err := patch.FindOne(ctx, patch.ByVersion(t.Version))

	if err != nil {
		errorMessage := fmt.Sprintf("loading patch: %s: ", err.Error())
		grip.Error(message.Fields{
			"message": errorMessage,
			"task_id": t.Id,
		})
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "loading patch"))
	}

	if p == nil {
		errorMessage := "patch not found"
		grip.Error(message.Fields{
			"message": errorMessage,
			"task_id": t.Id,
		})
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    errorMessage,
		})
	}

	if err = p.SetDownstreamParameters(ctx, h.downstreamParams); err != nil {
		errorMessage := fmt.Sprintf("setting patch parameters: %s", err.Error())
		grip.Error(message.Fields{
			"message": errorMessage,
			"task_id": t.Id,
		})
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "setting patch parameters"))
	}

	return gimlet.NewJSONResponse(fmt.Sprintf("Downstream patches for %v have successfully been set", p.Id))
}

// GET /rest/v2/task/{task_id}/installation_token/{owner}/{repo}
// This route is used to clone the source and modules when using git.get_project
// and only meant for internal use.
// It returns an installation token that's attached to Evergreen's GitHub app.
// See createGitHubDynamicAccessToken or tokens created for users using their GitHub app.
type createInstallationTokenForClone struct {
	owner string
	repo  string

	env evergreen.Environment
}

func makeCreateInstallationToken(env evergreen.Environment) gimlet.RouteHandler {
	return &createInstallationTokenForClone{
		env: env,
	}
}

func (g *createInstallationTokenForClone) Factory() gimlet.RouteHandler {
	return &createInstallationTokenForClone{
		env: g.env,
	}
}

func (g *createInstallationTokenForClone) Parse(ctx context.Context, r *http.Request) error {
	if g.owner = gimlet.GetVars(r)["owner"]; g.owner == "" {
		return errors.New("missing owner")
	}

	if g.repo = gimlet.GetVars(r)["repo"]; g.repo == "" {
		return errors.New("missing repo")
	}

	return nil
}

func (g *createInstallationTokenForClone) Run(ctx context.Context) gimlet.Responder {
	const lifetime = 50 * time.Minute
	// because this token will be used for cloning, restrict the token to read only
	opts := &github.InstallationTokenOptions{
		Permissions: &github.InstallationPermissions{
			Contents: utility.ToStringPtr(thirdparty.GithubPermissionRead),
		},
	}
	token, err := githubapp.CreateGitHubAppAuth(g.env.Settings()).CreateCachedInstallationToken(ctx, g.owner, g.repo, lifetime, opts)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "creating installation token for '%s/%s'", g.owner, g.repo))
	}
	if token == "" {
		return gimlet.MakeJSONErrorResponder(errors.Errorf("no installation token returned for '%s/%s'", g.owner, g.repo))
	}

	return gimlet.NewJSONResponse(&apimodels.Token{
		Token: token,
	})
}

// POST /task/{task_id}/check_run
type checkRunHandler struct {
	taskID         string
	checkRunOutput *github.CheckRunOutput
	settings       *evergreen.Settings
}

func makeCheckRun(settings *evergreen.Settings) gimlet.RouteHandler {
	return &checkRunHandler{
		settings: settings,
	}
}

func (h *checkRunHandler) Factory() gimlet.RouteHandler {
	return &checkRunHandler{}
}

func (h *checkRunHandler) Parse(ctx context.Context, r *http.Request) error {
	if h.taskID = gimlet.GetVars(r)["task_id"]; h.taskID == "" {
		return errors.New("missing task ID")
	}

	output := github.CheckRunOutput{}
	err := utility.ReadJSON(r.Body, &output)
	if err != nil {
		errorMessage := fmt.Sprintf("reading checkRun for task '%s'", h.taskID)
		grip.Error(message.Fields{
			"message": errorMessage,
			"task_id": h.taskID,
		})
		return errors.Wrap(err, errorMessage)
	}

	// output is empty if it does not specify the three fields Evergreen processes.
	if output.Title == nil && output.Summary == nil && len(output.Annotations) == 0 {
		return nil
	}

	h.checkRunOutput = &output
	err = thirdparty.ValidateCheckRunOutput(h.checkRunOutput)
	if err != nil {
		errorMessage := fmt.Sprintf("validating checkRun for task '%s'", h.taskID)
		grip.Error(message.Fields{
			"message": errorMessage,
			"task_id": h.taskID,
			"error":   err.Error(),
		})
		return errors.Wrap(err, errorMessage)
	}

	return nil
}

func (h *checkRunHandler) Run(ctx context.Context) gimlet.Responder {
	env := evergreen.GetEnvironment()
	t, err := task.FindOneId(ctx, h.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", h.taskID))
	}
	if t == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", h.taskID),
		})
	}

	if !evergreen.IsGitHubPatchRequester(t.Requester) {
		return gimlet.NewJSONResponse(fmt.Sprintf("checkRun not upserted for '%s', task requester is not a github patch", t.Id))

	}

	p, err := patch.FindOneId(ctx, t.Version)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}
	if p == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    "empty patch for task",
		})
	}

	gh := p.GithubPatchData
	if t.CheckRunId != nil {
		_, err := thirdparty.UpdateCheckRun(ctx, gh.BaseOwner, gh.BaseRepo, env.Settings().Api.URL, utility.FromInt64Ptr(t.CheckRunId), t, h.checkRunOutput)
		if err != nil {
			errorMessage := fmt.Sprintf("updating checkRun for task: '%s'", t.Id)
			grip.Error(message.Fields{
				"message":      errorMessage,
				"error":        err.Error(),
				"task_id":      t.Id,
				"check_run_id": t.CheckRunId,
			})
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "updating check run"))
		}
		return gimlet.NewJSONResponse(fmt.Sprintf("Successfully updated check run for  '%v'", t.Id))
	}

	checkRun, err := thirdparty.CreateCheckRun(ctx, gh.BaseOwner, gh.BaseRepo, gh.HeadHash, env.Settings().Api.URL, t, h.checkRunOutput)

	if err != nil {
		errorMessage := fmt.Sprintf("creating checkRun for task: '%s'", t.Id)
		grip.Error(message.Fields{
			"message": errorMessage,
			"error":   err.Error(),
			"task_id": t.Id,
		})
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "creating check run"))
	}

	if checkRun == nil {
		errorMessage := fmt.Sprintf("created checkRun not return for task: '%s'", t.Id)
		grip.Error(message.Fields{
			"message": errorMessage,
			"task_id": t.Id,
		})
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "creating check run"))
	}

	checkRunInt := utility.FromInt64Ptr(checkRun.ID)
	if err = t.SetCheckRunId(ctx, checkRunInt); err != nil {
		err = errors.Wrap(err, "setting check run ID on task")
		grip.Error(message.WrapError(err,
			message.Fields{
				"task_id":      t.Id,
				"check_run_id": checkRunInt,
			}))
		return gimlet.MakeJSONInternalErrorResponder(err)
	}

	return gimlet.NewJSONResponse(fmt.Sprintf("Successfully created check run for  '%v'", t.Id))
}

// GET /rest/v2/task/{task_id}/github_dynamic_access_token/{owner}/{repo}
// This route is used to create user-used GitHub access token for a task.
// It returns an installation token using the task's project's GitHub app and
// gets the intersecting permissions from the requester's permission group and the
// permissions requested.
// See createInstallationToken for tokens created with our own GitHub app,
// for example to use for cloning sources and modules.
type createGitHubDynamicAccessToken struct {
	owner  string
	repo   string
	taskID string

	permissions    github.InstallationPermissions
	allPermissions bool

	env evergreen.Environment
}

func makeCreateGitHubDynamicAccessToken(env evergreen.Environment) gimlet.RouteHandler {
	return &createGitHubDynamicAccessToken{env: env}
}

func (h *createGitHubDynamicAccessToken) Factory() gimlet.RouteHandler {
	return &createGitHubDynamicAccessToken{env: h.env}
}

func (h *createGitHubDynamicAccessToken) Parse(ctx context.Context, r *http.Request) error {
	if h.owner = gimlet.GetVars(r)["owner"]; h.owner == "" {
		return errors.New("missing owner")
	}
	if h.repo = gimlet.GetVars(r)["repo"]; h.repo == "" {
		return errors.New("missing repo")
	}
	if h.taskID = gimlet.GetVars(r)["task_id"]; h.taskID == "" {
		return errors.New("missing task_id")
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return errors.Wrap(err, "reading body")
	}
	// If the body is an empty json object or a null json object, we want to set allPermissions to true.
	if string(body) == "{}" || string(body) == "null" {
		h.allPermissions = true
		return nil
	}

	err = json.Unmarshal(body, &h.permissions)

	errorMessage := fmt.Sprintf("reading permissions body for task '%s'", h.taskID)
	grip.Error(message.WrapError(err, message.Fields{
		"message": errorMessage,
		"task_id": h.taskID,
	}))

	return errors.Wrap(err, errorMessage)
}

func (h *createGitHubDynamicAccessToken) Run(ctx context.Context) gimlet.Responder {
	t, err := task.FindOneId(ctx, h.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", h.taskID))
	}
	if t == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", h.taskID),
		})
	}

	// When creating a token for a task, we want to consider the project's
	// permission groups. These permission groups can restrict the permissions
	// so that each requester only gets the permissions they have been set.
	p, err := model.FindMergedProjectRef(ctx, t.Project, t.Version, true)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}
	if p == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("project ref '%s' not found", t.Project),
		})
	}
	requesterPermissionGroup, _ := p.GetGitHubPermissionGroup(t.Requester)
	// If the requester has no permissions, they should not be able to create a token.
	// GitHub interprets an empty token as having all permissions.
	if requesterPermissionGroup.HasNoPermissions() {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusUnauthorized,
			Message:    "requester does not have permission to create a token",
		})
	}
	intersection, err := requesterPermissionGroup.Intersection(model.GitHubDynamicTokenPermissionGroup{
		Permissions:    h.permissions,
		AllPermissions: h.allPermissions,
	})
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	// If all permissions is true, we want to send an empty permissions object to the GitHub API.
	permissions := &intersection.Permissions
	if intersection.AllPermissions {
		permissions = nil
	} else if intersection.HasNoPermissions() {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusUnauthorized,
			Message:    "the intersection of the project setting's requester permissions and provided permissions does not have any permissions to create a token",
		})
	}

	// The token also should use the project's GitHub app.
	githubAppAuth, err := p.GetGitHubAppAuth(ctx)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}
	if githubAppAuth == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("github app auth not found for project '%s'", t.Project),
		})
	}

	// This cannot use a cached token because if the token was shared, it
	// wouldn't be possible to revoke them without revoking tokens that other
	// tasks could be using.
	token, permissions, err := githubAppAuth.CreateInstallationToken(ctx, h.owner, h.repo, &github.InstallationTokenOptions{
		Permissions: permissions,
	})
	if err != nil {
		// This intentionally returns a 4xx error to prevent the agent from
		// retrying because CreateInstallationToken already retries internally.
		// It's assumed that if the token can't be created after retries, it's
		// not a transient issue.
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "creating installation token for '%s/%s'", h.owner, h.repo))
	}
	if token == "" {
		return gimlet.MakeJSONErrorResponder(errors.Errorf("no installation token returned for '%s/%s'", h.owner, h.repo))
	}

	return gimlet.NewJSONResponse(&apimodels.Token{
		Token:       token,
		Permissions: permissions,
	})
}

// DELETE /rest/v2/task/{task_id}/github_dynamic_access_token
// This route is used to revoke user-used GitHub access token for a task.
type revokeGitHubDynamicAccessToken struct {
	taskID string
	body   apimodels.Token

	env evergreen.Environment
}

func makeRevokeGitHubDynamicAccessToken(env evergreen.Environment) gimlet.RouteHandler {
	return &revokeGitHubDynamicAccessToken{env: env}
}

func (h *revokeGitHubDynamicAccessToken) Factory() gimlet.RouteHandler {
	return &revokeGitHubDynamicAccessToken{env: h.env}
}

func (h *revokeGitHubDynamicAccessToken) Parse(ctx context.Context, r *http.Request) error {
	if h.taskID = gimlet.GetVars(r)["task_id"]; h.taskID == "" {
		return errors.New("missing task_id")
	}

	if err := utility.ReadJSON(r.Body, &h.body); err != nil {
		return errors.Wrapf(err, "reading token JSON request body for task '%s'", h.taskID)
	}

	if h.body.Token == "" {
		return errors.New("missing token")
	}
	return nil
}

func (h *revokeGitHubDynamicAccessToken) Run(ctx context.Context) gimlet.Responder {
	if err := thirdparty.RevokeInstallationToken(ctx, h.body.Token); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "revoking token for task '%s'", h.taskID))
	}

	return gimlet.NewJSONResponse(struct{}{})
}

// POST /rest/v2/task/{task_id}/aws/assume_role
// This route is used to assume an AWS arn role for a task.
type awsAssumeRole struct {
	taskID string
	hostID string
	body   apimodels.AssumeRoleRequest

	stsManager cloud.STSManager
}

func makeAWSAssumeRole(stsManager cloud.STSManager) gimlet.RouteHandler {
	return &awsAssumeRole{stsManager: stsManager}
}

func (h *awsAssumeRole) Factory() gimlet.RouteHandler {
	return &awsAssumeRole{stsManager: h.stsManager}
}

func (h *awsAssumeRole) Parse(ctx context.Context, r *http.Request) error {
	if h.taskID = gimlet.GetVars(r)["task_id"]; h.taskID == "" {
		return errors.New("missing task_id")
	}

	h.hostID = r.Header.Get(evergreen.HostHeader)
	if h.hostID == "" {
		return errors.New("missing host ID")
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return errors.Wrap(err, "reading body")
	}

	if err = json.Unmarshal(body, &h.body); err != nil {
		return errors.Wrapf(err, "reading assume role body for task '%s'", h.taskID)
	}

	return errors.Wrapf(h.body.Validate(), "validating assume role body for task '%s'", h.taskID)
}

func (h *awsAssumeRole) Run(ctx context.Context) gimlet.Responder {
	creds, err := h.stsManager.AssumeRole(ctx, h.taskID, h.hostID, cloud.AssumeRoleOptions{
		RoleARN:         h.body.RoleARN,
		Policy:          h.body.Policy,
		DurationSeconds: h.body.DurationSeconds,
	})
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "assuming role for task '%s'", h.taskID))
	}
	return gimlet.NewJSONResponse(apimodels.AWSCredentials{
		AccessKeyID:     creds.AccessKeyID,
		SecretAccessKey: creds.SecretAccessKey,
		SessionToken:    creds.SessionToken,
		Expiration:      creds.Expiration.Format(time.RFC3339),
		ExternalID:      creds.ExternalID,
	})
}
