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
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/githubapp"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testlog"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v52/github"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// GET /rest/v2/agent/cedar_config
type agentCedarConfig struct {
	config evergreen.CedarConfig
}

func makeAgentCedarConfig(config evergreen.CedarConfig) *agentCedarConfig {
	return &agentCedarConfig{
		config: config,
	}
}

func (h *agentCedarConfig) Factory() gimlet.RouteHandler {
	return &agentCedarConfig{
		config: h.config,
	}
}

func (*agentCedarConfig) Parse(_ context.Context, _ *http.Request) error { return nil }

func (h *agentCedarConfig) Run(ctx context.Context) gimlet.Responder {
	return gimlet.NewJSONResponse(apimodels.CedarConfig{
		BaseURL:     h.config.BaseURL,
		GRPCBaseURL: h.config.GRPCBaseURL,
		RPCPort:     h.config.RPCPort,
		Username:    h.config.User,
		APIKey:      h.config.APIKey,
		Insecure:    h.config.Insecure,
	})
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
		TaskOutput:         h.settings.Providers.AWS.TaskOutput,
		TaskSync:           h.settings.Providers.AWS.TaskSync,
		EC2Keys:            h.settings.Providers.AWS.EC2Keys,
		MaxExecTimeoutSecs: h.settings.TaskLimits.MaxExecTimeoutSecs,
	}
	if h.settings.Tracer.Enabled {
		data.TraceCollectorEndpoint = h.settings.Tracer.CollectorEndpoint
	}

	return gimlet.NewJSONResponse(data)
}

// GET /task/{task_id}/pull_request
type agentCheckGetPullRequestHandler struct {
	taskID   string
	settings *evergreen.Settings

	req apimodels.CheckMergeRequest
}

func makeAgentGetPullRequest(settings *evergreen.Settings) gimlet.RouteHandler {
	return &agentCheckGetPullRequestHandler{
		settings: settings,
	}
}

func (h *agentCheckGetPullRequestHandler) Factory() gimlet.RouteHandler {
	return &agentCheckGetPullRequestHandler{
		settings: h.settings,
	}
}

func (h *agentCheckGetPullRequestHandler) Parse(ctx context.Context, r *http.Request) error {
	if h.taskID = gimlet.GetVars(r)["task_id"]; h.taskID == "" {
		return errors.New("missing task ID")
	}
	if err := utility.ReadJSON(r.Body, &h.req); err != nil {
		return errors.Wrap(err, "reading from JSON request body")
	}
	return nil
}

func (h *agentCheckGetPullRequestHandler) Run(ctx context.Context) gimlet.Responder {
	token, err := h.settings.GetGithubOauthToken()
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err, "getting token"))
	}
	pr, err := thirdparty.GetGithubPullRequest(ctx, token, h.req.Owner, h.req.Repo, h.req.PRNum)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(err)
	}
	resp := apimodels.PullRequestInfo{
		Mergeable:      pr.Mergeable,
		MergeCommitSHA: pr.GetMergeCommitSHA(),
	}
	t, err := task.FindOneId(h.taskID)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err, "getting task '%s'", h.taskID))
	}
	if t == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", h.taskID),
		})
	}
	p, err := patch.FindOne(patch.ByVersion(t.Version))
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err, "getting patch for task '%s'", h.taskID))
	}
	if p == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("patch for task '%s' not found", h.taskID),
		})
	}
	if err = p.UpdateMergeCommitSHA(pr.GetMergeCommitSHA()); err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err, "updating merge commit SHA for patch '%s'", p.Id.Hex()))
	}

	return gimlet.NewJSONResponse(resp)
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
	t, err := task.FindOneId(h.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", h.taskID))
	}
	if t == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", h.taskID),
		})
	}

	err = errors.Wrapf(h.pushLog.UpdateStatus(h.pushLog.Status),
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
	t, err := task.FindOneId(h.taskID)
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
	v, err := model.VersionFindOne(model.VersionById(t.Version))
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

	newestPushLog, err := model.FindPushLogAfter(copyToLocation, v.RevisionOrderNumber)
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
	if err = newPushLog.Insert(); err != nil {
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
	t, err := task.FindOneId(h.taskID)
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
	if t.IsPartOfDisplay() {
		dt, err := t.GetDisplayTask()
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
	if err = taskToRestart.SetResetWhenFinishedWithInc(); err != nil {
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
	if h.hostID == "" && podID == "" {
		return errors.New("missing both host and pod ID")
	}
	return nil
}

func (h *getExpansionsAndVarsHandler) Run(ctx context.Context) gimlet.Responder {
	t, err := task.FindOneId(h.taskID)
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

	oauthToken, err := h.settings.GetGithubOauthToken()
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting GitHub OAuth token"))
	}

	pRef, err := model.FindBranchProjectRef(t.Project)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding project ref '%s'", t.Project))
	}
	if pRef == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("project ref '%s' not found", t.Project),
		})
	}

	const ghTokenLifetime = 50 * time.Minute
	appToken, err := githubapp.CreateGitHubAppAuth(h.settings).CreateCachedInstallationToken(ctx, pRef.Owner, pRef.Repo, ghTokenLifetime, nil)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "creating GitHub app token"))
	}

	knownHosts := h.settings.Expansions[evergreen.GithubKnownHosts]
	e, err := model.PopulateExpansions(t, foundHost, oauthToken, appToken, knownHosts)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "populating expansions"))
	}

	res := apimodels.ExpansionsAndVars{
		Expansions:  e,
		Parameters:  map[string]string{},
		Vars:        map[string]string{},
		PrivateVars: map[string]bool{},
		RedactKeys:  h.settings.LoggerConfig.RedactKeys,
	}

	projectVars, err := model.FindMergedProjectVars(t.Project)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting merged project vars"))
	}
	if projectVars != nil {
		res.Vars = projectVars.GetVars(t)
		if projectVars.PrivateVars != nil {
			res.PrivateVars = projectVars.PrivateVars
		}
	}

	v, err := model.VersionFindOneId(t.Version)
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
	t, err := task.FindOneId(h.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", h.taskID))
	}
	if t == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", h.taskID),
		})
	}

	p, err := model.FindMergedProjectRef(t.Project, t.Version, true)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}

	if p == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("project ref '%s' not found", t.Project),
		})
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
	t, err := task.FindOneId(h.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", h.taskID))
	}
	if t == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", h.taskID),
		})
	}
	v, err := model.VersionFindOne(model.VersionById(t.Version))
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
	projBytes, err := bson.Marshal(pp)
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
	}
	return gimlet.NewJSONResponse(dv)
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
	t, err := task.FindOneId(h.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", h.taskID))
	}
	if t == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", h.taskID),
		})
	}

	entry := &artifact.Entry{
		TaskId:          t.Id,
		TaskDisplayName: t.DisplayName,
		BuildId:         t.BuildId,
		Execution:       t.Execution,
		CreateTime:      time.Now(),
		Files:           artifact.EscapeFiles(h.files),
	}

	if err = entry.Upsert(); err != nil {
		message := fmt.Sprintf("updating artifact file info for task %s: %v", t.Id, err)
		grip.Error(message)
		return gimlet.MakeJSONInternalErrorResponder(errors.New(message))
	}
	return gimlet.NewJSONResponse(fmt.Sprintf("Artifact files for task %s successfully attached", t.Id))
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
	t, err := task.FindOneId(h.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", h.taskID))
	}
	if t == nil {
		return gimlet.MakeJSONInternalErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", h.taskID),
		})
	}

	if err = t.SetResultsInfo(h.info.Service, h.info.Failed); err != nil {
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
	t, err := task.FindOneId(h.taskID)
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

	if err = h.log.Insert(); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}
	logReply := struct {
		Id string `json:"_id"`
	}{h.log.Id}
	return gimlet.NewJSONResponse(logReply)
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
	t, err := task.FindOneId(h.taskID)
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

	if err := t.UpdateHeartbeat(); err != nil {
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
	t, err := task.FindOneId(h.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", h.taskID))
	}
	if t == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", h.taskID),
		})
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

	t, err := task.FindOneId(h.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", h.taskID))
	}
	if t == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", h.taskID),
		})
	}
	grip.Debug(message.Fields{
		"message": "marking task started",
		"task_id": t.Id,
		"details": t.Details,
	})

	updates := model.StatusChanges{}
	if err = model.MarkStart(t, &updates); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "marking task '%s' started", t.Id))
	}

	if len(updates.PatchNewStatus) != 0 {
		event.LogPatchStateChangeEvent(t.Version, updates.PatchNewStatus)
	}
	if len(updates.BuildNewStatus) != 0 {
		event.LogBuildStateChangeEvent(t.BuildId, updates.BuildNewStatus)
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
			if err = foundHost.IncTaskCount(); err != nil {
				return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "incrementing task count for task '%s' on host '%s'", t.Id, foundHost.Id))
			}
			if err = foundHost.IncIdleTime(foundHost.WastedComputeTime()); err != nil {
				return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "incrementing total idle time on host '%s'", foundHost.Id))
			}
			grip.Info(foundHost.TaskStartMessage())
		}
	} else {
		foundPod, err = pod.FindOneByID(h.podID)
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
	patchContents, err := patch.FetchPatchContents(h.patchID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "reading patch file from db"))
	}
	return gimlet.NewTextResponse(patchContents)
}

// GET /task/{task_id}/git/patch
type gitServePatchHandler struct {
	taskID  string
	patchID string
}

func makeGitServePatch() gimlet.RouteHandler {
	return &gitServePatchHandler{}
}

func (h *gitServePatchHandler) Factory() gimlet.RouteHandler {
	return &gitServePatchHandler{}
}

func (h *gitServePatchHandler) Parse(ctx context.Context, r *http.Request) error {
	if h.taskID = gimlet.GetVars(r)["task_id"]; h.taskID == "" {
		return errors.New("missing task ID")
	}
	if patchParam, exists := r.URL.Query()["patch"]; exists {
		h.patchID = patchParam[0]
	}
	return nil
}

func (h *gitServePatchHandler) Run(ctx context.Context) gimlet.Responder {
	if h.patchID == "" {
		t, err := task.FindOneId(h.taskID)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", h.taskID))
		}
		if t == nil {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusNotFound,
				Message:    fmt.Sprintf("task '%s' not found", h.taskID),
			})
		}
		h.patchID = t.Version
	}

	p, err := patch.FindOne(patch.ByVersion(h.patchID))
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding patch '%s'", h.patchID))
	}
	if p == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("patch with ID '%s' not found", h.patchID),
		})
	}

	// add on the merge status for the patch, if applicable
	if p.GetRequester() == evergreen.MergeTestRequester {
		builds, err := build.Find(build.ByVersion(p.Version))
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "retrieving builds for task"))
		}
		tasks, err := task.FindWithFields(task.ByVersion(p.Version), task.BuildIdKey, task.StatusKey, task.ActivatedKey, task.DependsOnKey)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "finding tasks for version"))
		}

		p.MergeStatus = evergreen.VersionSucceeded
		for _, b := range builds {
			if b.BuildVariant == evergreen.MergeTaskVariant {
				continue
			}
			complete, buildStatus, err := b.AllUnblockedTasksFinished(tasks)
			if err != nil {
				return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "checking build tasks"))
			}
			if !complete {
				p.MergeStatus = evergreen.VersionStarted
				break
			}
			if buildStatus == evergreen.BuildFailed {
				p.MergeStatus = evergreen.VersionFailed
				break
			}
		}
	}

	return gimlet.NewJSONResponse(p)
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
	if err := keyVal.Inc(); err != nil {
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
	task, err := task.FindOneId(h.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", h.taskID))
	}
	if task == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", h.taskID),
		})
	}

	projectRef, err := model.FindMergedProjectRef(task.Project, task.Version, true)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding project '%s'", task.Project))
	}
	if projectRef == nil {
		return gimlet.MakeJSONErrorResponder(errors.Errorf("project ref '%s' doesn't exist", task.Project))
	}

	v, err := model.VersionFindOne(model.VersionById(task.Version))
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "retrieving version for task"))
	}
	if v == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("version not found: %s", task.Version),
		})
	}
	currentManifest, err := manifest.FindFromVersion(v.Id, v.Identifier, v.Revision, v.Requester)
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
	manifest, err := model.CreateManifest(v, project.Modules, projectRef, h.settings)
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
		return errors.Wrapf(err, errorMessage)
	}
	return nil
}

// Run updates file mappings for a task or build.
func (h *setDownstreamParamsHandler) Run(ctx context.Context) gimlet.Responder {
	t, err := task.FindOneId(h.taskID)
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

	p, err := patch.FindOne(patch.ByVersion(t.Version))

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

	if err = p.SetDownstreamParameters(h.downstreamParams); err != nil {
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
type createInstallationToken struct {
	owner string
	repo  string

	env evergreen.Environment
}

func makeCreateInstallationToken(env evergreen.Environment) gimlet.RouteHandler {
	return &createInstallationToken{
		env: env,
	}
}

func (g *createInstallationToken) Factory() gimlet.RouteHandler {
	return &createInstallationToken{
		env: g.env,
	}
}

func (g *createInstallationToken) Parse(ctx context.Context, r *http.Request) error {
	if g.owner = gimlet.GetVars(r)["owner"]; g.owner == "" {
		return errors.New("missing owner")
	}

	if g.repo = gimlet.GetVars(r)["repo"]; g.repo == "" {
		return errors.New("missing repo")
	}

	return nil
}

func (g *createInstallationToken) Run(ctx context.Context) gimlet.Responder {
	const lifetime = 50 * time.Minute
	token, err := githubapp.CreateGitHubAppAuth(g.env.Settings()).CreateCachedInstallationToken(ctx, g.owner, g.repo, lifetime, nil)
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
		return errors.Wrapf(err, errorMessage)
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
		return errors.Wrapf(err, errorMessage)
	}

	return nil
}

func (h *checkRunHandler) Run(ctx context.Context) gimlet.Responder {
	env := evergreen.GetEnvironment()
	t, err := task.FindOneId(h.taskID)
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

	p, err := patch.FindOneId(t.Version)
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
		_, err := thirdparty.UpdateCheckRun(ctx, gh.BaseOwner, gh.BaseRepo, env.Settings().ApiUrl, utility.FromInt64Ptr(t.CheckRunId), t, h.checkRunOutput)
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

	checkRun, err := thirdparty.CreateCheckRun(ctx, gh.BaseOwner, gh.BaseRepo, gh.HeadHash, env.Settings().ApiUrl, t, h.checkRunOutput)

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
	if err = t.SetCheckRunId(checkRunInt); err != nil {
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

	return errors.Wrapf(err, errorMessage)
}

func (h *createGitHubDynamicAccessToken) Run(ctx context.Context) gimlet.Responder {
	t, err := task.FindOneId(h.taskID)
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
	p, err := model.FindMergedProjectRef(t.Project, t.Version, true)
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
		return gimlet.MakeJSONInternalErrorResponder(err)
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
	githubAppAuth, err := githubapp.FindOneGithubAppAuth(t.Project)
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
	token, err := githubAppAuth.CreateInstallationToken(ctx, h.owner, h.repo, &github.InstallationTokenOptions{
		Permissions: permissions,
	})
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "creating installation token for '%s/%s'", h.owner, h.repo))
	}
	if token == "" {
		return gimlet.MakeJSONInternalErrorResponder(errors.Errorf("no installation token returned for '%s/%s'", h.owner, h.repo))
	}

	return gimlet.NewJSONResponse(&apimodels.Token{
		Token: token,
	})
}

// DELETE /rest/v2/task/{task_id}/github_dynamic_access_tokens
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
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "revoking token for task '%s'", h.taskID))
	}

	return gimlet.NewJSONResponse(struct{}{})
}
