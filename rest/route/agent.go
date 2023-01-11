package route

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
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
		BaseURL:  h.config.BaseURL,
		RPCPort:  h.config.RPCPort,
		Username: h.config.User,
		APIKey:   h.config.APIKey,
		Insecure: h.config.Insecure,
	})
}

// GET /rest/v2/agent/data_pipes_config

type agentDataPipesConfig struct {
	config evergreen.DataPipesConfig
}

func makeAgentDataPipesConfig(config evergreen.DataPipesConfig) *agentDataPipesConfig {
	return &agentDataPipesConfig{
		config: config,
	}
}

func (h *agentDataPipesConfig) Factory() gimlet.RouteHandler {
	return &agentDataPipesConfig{
		config: h.config,
	}
}

func (*agentDataPipesConfig) Parse(_ context.Context, _ *http.Request) error { return nil }

func (h *agentDataPipesConfig) Run(ctx context.Context) gimlet.Responder {
	return gimlet.NewJSONResponse(apimodels.DataPipesConfig{
		Host:         h.config.Host,
		Region:       h.config.Region,
		AWSAccessKey: h.config.AWSAccessKey,
		AWSSecretKey: h.config.AWSSecretKey,
		AWSToken:     h.config.AWSToken,
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
		SplunkServerURL:   h.settings.Splunk.SplunkConnectionInfo.ServerURL,
		SplunkClientToken: h.settings.Splunk.SplunkConnectionInfo.Token,
		SplunkChannel:     h.settings.Splunk.SplunkConnectionInfo.Channel,
		S3Key:             h.settings.Providers.AWS.S3.Key,
		S3Secret:          h.settings.Providers.AWS.S3.Secret,
		S3Bucket:          h.settings.Providers.AWS.S3.Bucket,
		TaskSync:          h.settings.Providers.AWS.TaskSync,
		EC2Keys:           h.settings.Providers.AWS.EC2Keys,
		LogkeeperURL:      h.settings.LoggerConfig.LogkeeperURL,
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

// GET /task/{task_id}/expansions
type getExpansionsHandler struct {
	settings *evergreen.Settings
	taskID   string
	hostID   string
}

func makeGetExpansions(settings *evergreen.Settings) gimlet.RouteHandler {
	return &getExpansionsHandler{
		settings: settings,
	}
}

func (h *getExpansionsHandler) Factory() gimlet.RouteHandler {
	return &getExpansionsHandler{
		settings: h.settings,
	}
}

func (h *getExpansionsHandler) Parse(ctx context.Context, r *http.Request) error {
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

func (h *getExpansionsHandler) Run(ctx context.Context) gimlet.Responder {
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
		foundHost, err = host.FindOneId(h.hostID)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting host"))
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

	e, err := model.PopulateExpansions(ctx, t, foundHost, oauthToken)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(err)
	}

	return gimlet.NewJSONResponse(e)
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

	if p.DefaultLogger == "" {
		// If the default logger is not set at the project level, use
		// the global default logger.
		p.DefaultLogger = evergreen.GetEnvironment().Settings().LoggerConfig.DefaultLogger
	}

	return gimlet.NewJSONResponse(p)
}

// GET /task/{task_id}/parser_project
type getParserProjectHandler struct {
	taskID string
}

func makeGetParserProject() gimlet.RouteHandler {
	return &getParserProjectHandler{}
}

func (h *getParserProjectHandler) Factory() gimlet.RouteHandler {
	return &getParserProjectHandler{}
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
	pp, err := model.GetParserProjectStorage(v.ProjectStorageMethod).FindOneByID(ctx, v.Id)
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
	host, err := host.FindOneId(h.hostID)
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
		CloneMethod:         host.Distro.CloneMethod,
		DisableShallowClone: host.Distro.DisableShallowClone,
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
	grip.Infoln("Attaching files to task:", t.Id)

	entry := &artifact.Entry{
		TaskId:          t.Id,
		TaskDisplayName: t.DisplayName,
		BuildId:         t.BuildId,
		Execution:       t.Execution,
		CreateTime:      time.Now(),
		Files:           h.files,
	}

	if err = entry.Upsert(); err != nil {
		message := fmt.Sprintf("updating artifact file info for task %s: %v", t.Id, err)
		grip.Error(message)
		return gimlet.MakeJSONInternalErrorResponder(errors.New(message))
	}
	return gimlet.NewJSONResponse(fmt.Sprintf("Artifact files for task %s successfully attached", t.Id))
}

// POST /task/{task_id}/test_logs
type attachTestLogHandler struct {
	settings *evergreen.Settings
	taskID   string
	log      model.TestLog
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

// POST /task/{task_id}/results
type attachResultsHandler struct {
	taskID  string
	results task.LocalTestResults
}

func makeAttachResults() gimlet.RouteHandler {
	return &attachResultsHandler{}
}

func (h *attachResultsHandler) Factory() gimlet.RouteHandler {
	return &attachResultsHandler{}
}

func (h *attachResultsHandler) Parse(ctx context.Context, r *http.Request) error {
	if h.taskID = gimlet.GetVars(r)["task_id"]; h.taskID == "" {
		return errors.New("missing task ID")
	}
	err := utility.ReadJSON(r.Body, &h.results)
	if err != nil {
		return errors.Wrap(err, "reading test results from JSON request body")
	}
	return nil
}

// Run attaches the received results to the task in the database.
func (h *attachResultsHandler) Run(ctx context.Context) gimlet.Responder {
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

	// set test result of task
	if err = t.SetResults(h.results.Results); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}
	return gimlet.NewJSONResponse("test results successfully attached")
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

// GET /task/{task_id}/fetch_vars
type fetchExpansionsForTaskHandler struct {
	taskID string
}

func makeFetchExpansionsForTask() gimlet.RouteHandler {
	return &fetchExpansionsForTaskHandler{}
}

func (h *fetchExpansionsForTaskHandler) Factory() gimlet.RouteHandler {
	return &fetchExpansionsForTaskHandler{}
}

func (h *fetchExpansionsForTaskHandler) Parse(ctx context.Context, r *http.Request) error {
	if h.taskID = gimlet.GetVars(r)["task_id"]; h.taskID == "" {
		return errors.New("missing task ID")
	}
	return nil
}

func (h *fetchExpansionsForTaskHandler) Run(ctx context.Context) gimlet.Responder {
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
	projectVars, err := model.FindMergedProjectVars(t.Project)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}
	res := apimodels.ExpansionVars{
		Vars:        map[string]string{},
		PrivateVars: map[string]bool{},
	}
	if projectVars == nil {
		return gimlet.NewJSONResponse(res)
	}
	res.Vars = projectVars.GetVars(t)
	if projectVars.PrivateVars != nil {
		res.PrivateVars = projectVars.PrivateVars
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
	projParams, err := model.FindParametersForVersion(ctx, v)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}
	for _, param := range projParams {
		// If the key doesn't exist the value will default to "" anyway; this prevents
		// an un-specified parameter from overwriting lower-priority expansions.
		if param.Value != "" {
			res.Vars[param.Key] = param.Value
		}
	}
	for _, param := range v.Parameters {
		// We will overwrite empty values here since these were explicitly user-specified.
		res.Vars[param.Key] = param.Value
	}

	return gimlet.NewJSONResponse(res)
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

// POST /task/{task_id}/log
type appendTaskLogHandler struct {
	settings *evergreen.Settings
	taskID   string
	taskLog  model.TaskLog
}

func makeAppendTaskLog(settings *evergreen.Settings) gimlet.RouteHandler {
	return &appendTaskLogHandler{
		settings: settings,
	}
}

func (h *appendTaskLogHandler) Factory() gimlet.RouteHandler {
	return &appendTaskLogHandler{
		settings: h.settings,
	}
}

func (h *appendTaskLogHandler) Parse(ctx context.Context, r *http.Request) error {
	if h.taskID = gimlet.GetVars(r)["task_id"]; h.taskID == "" {
		return errors.New("missing task ID")
	}
	if err := utility.ReadJSON(r.Body, &h.taskLog); err != nil {
		return errors.Wrap(err, "reading task log from JSON request body")
	}
	return nil
}

// Run appends the received logs to the task's internal logs.
func (h *appendTaskLogHandler) Run(ctx context.Context) gimlet.Responder {
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

	h.taskLog.TaskId = t.Id
	h.taskLog.Execution = t.Execution

	if err = h.taskLog.Insert(); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}

	return gimlet.NewJSONResponse("Logs added")
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
	if h.hostID != "" {
		host, err := host.FindOne(host.ByRunningTaskId(t.Id))
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding host running task %s", t.Id))
		}

		if host == nil {
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

		idleTimeStartAt := host.LastTaskCompletedTime
		if idleTimeStartAt.IsZero() || idleTimeStartAt == utility.ZeroTime {
			idleTimeStartAt = host.StartTime
		}

		msg = fmt.Sprintf("task %s started on host %s", t.Id, host.Id)

		if host.Distro.IsEphemeral() {
			queue := h.env.RemoteQueue()
			job := units.NewCollectHostIdleDataJob(host, t, idleTimeStartAt, t.StartTime)
			if err = queue.Put(ctx, job); err != nil {
				return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "queuing host idle stats for %s", msg))
			}
		}
	} else {
		// TODO: EVG-17647 Create job to collect data on idle pods
		msg = fmt.Sprintf("task '%s' started on pod '%s'", t.Id, h.podID)
	}

	return gimlet.NewJSONResponse(msg)
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

		status := evergreen.PatchSucceeded
		for _, b := range builds {
			if b.BuildVariant == evergreen.MergeTaskVariant {
				continue
			}
			complete, buildStatus, err := b.AllUnblockedTasksFinished(tasks)
			if err != nil {
				return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "checking build tasks"))
			}
			if !complete {
				status = evergreen.PatchStarted
				break
			}
			if buildStatus == evergreen.BuildFailed {
				status = evergreen.PatchFailed
				break
			}
		}
		p.MergeStatus = status
	}
	p.MergeStatus = evergreen.PatchSucceeded

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

	project, _, err := model.FindAndTranslateProjectForVersion(v.Id, v.Identifier, v.ProjectStorageMethod)
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
	manifest, err := model.CreateManifest(v, project, projectRef, h.settings)
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
