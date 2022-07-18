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
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// GET /rest/v2/agent/cedar_config
type agentCedarConfig struct {
	settings *evergreen.Settings
}

func makeAgentCedarConfig(settings *evergreen.Settings) gimlet.RouteHandler {
	return &agentCedarConfig{
		settings: settings,
	}
}

func (h *agentCedarConfig) Factory() gimlet.RouteHandler {
	return &agentCedarConfig{
		settings: h.settings,
	}
}

func (h *agentCedarConfig) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *agentCedarConfig) Run(ctx context.Context) gimlet.Responder {
	data := apimodels.CedarConfig{
		BaseURL:  h.settings.Cedar.BaseURL,
		RPCPort:  h.settings.Cedar.RPCPort,
		Username: h.settings.Cedar.User,
		APIKey:   h.settings.Cedar.APIKey,
	}
	return gimlet.NewJSONResponse(data)
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
		SplunkServerURL:   h.settings.Splunk.ServerURL,
		SplunkClientToken: h.settings.Splunk.Token,
		SplunkChannel:     h.settings.Splunk.Channel,
		S3Key:             h.settings.Providers.AWS.S3.Key,
		S3Secret:          h.settings.Providers.AWS.S3.Secret,
		S3Bucket:          h.settings.Providers.AWS.S3.Bucket,
		TaskSync:          h.settings.Providers.AWS.TaskSync,
		EC2Keys:           h.settings.Providers.AWS.EC2Keys,
		LogkeeperURL:      h.settings.LoggerConfig.LogkeeperURL,
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
	if h.hostID = r.Header.Get(evergreen.HostHeader); h.hostID == "" {
		return errors.New("missing host ID")
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

	oauthToken, err := h.settings.GetGithubOauthToken()
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting GitHub OAuth token"))
	}

	e, err := model.PopulateExpansions(t, host, oauthToken)
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
	pp, err := model.ParserProjectFindOneById(t.Version)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}
	// handle legacy
	if pp == nil || pp.ConfigUpdateNumber < v.ConfigUpdateNumber {
		pp = &model.ParserProject{}
		if err = util.UnmarshalYAMLWithFallback([]byte(v.Config), pp); err != nil {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusNotFound,
				Message:    "invalid version config",
			})
		}
	}
	if pp.Functions == nil {
		pp.Functions = map[string]*model.YAMLCommandSet{}
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
		WorkDir:             host.Distro.WorkDir,
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
		message := fmt.Sprintf("reading file definitions for task  %v: %v", h.taskID, err)
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
		message := fmt.Sprintf("updating artifact file info for task %v: %v", t.Id, err)
		grip.Error(message)
		return gimlet.MakeJSONInternalErrorResponder(errors.New(message))
	}
	return gimlet.NewJSONResponse(fmt.Sprintf("Artifact files for task %v successfully attached", t.Id))
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
	projParams, err := model.FindParametersForVersion(v)
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
	taskID        string
	taskStartInfo apimodels.TaskStartRequest
}

func makeStartTask() gimlet.RouteHandler {
	return &startTaskHandler{}
}

func (h *startTaskHandler) Factory() gimlet.RouteHandler {
	return &startTaskHandler{}
}

func (h *startTaskHandler) Parse(ctx context.Context, r *http.Request) error {
	if h.taskID = gimlet.GetVars(r)["task_id"]; h.taskID == "" {
		return errors.New("missing task ID")
	}
	if err := utility.ReadJSON(r.Body, &h.taskStartInfo); err != nil {
		return errors.Wrapf(err, "reading task start request for %s", h.taskID)
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

	host, err := host.FindOne(host.ByRunningTaskId(t.Id))
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding host running task %s", t.Id))
	}

	if host == nil {
		message := fmt.Sprintf("no host found running task %v", t.Id)
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

	msg := fmt.Sprintf("Task %v started on host %v", t.Id, host.Id)

	if host.Distro.IsEphemeral() {
		queue := evergreen.GetEnvironment().RemoteQueue()
		job := units.NewCollectHostIdleDataJob(host, t, idleTimeStartAt, t.StartTime)
		if err = queue.Put(ctx, job); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "queuing host idle stats for %s", msg))
		}
	}

	return gimlet.NewJSONResponse(msg)
}

// POST /task/{task_id}/end
type endTaskHandler struct{}

func makeEndTask() gimlet.RouteHandler {
	return &endTaskHandler{}
}

func (h *endTaskHandler) Factory() gimlet.RouteHandler {
	return &endTaskHandler{}
}

func (h *endTaskHandler) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *endTaskHandler) Run(ctx context.Context) gimlet.Responder {
	return nil
}
