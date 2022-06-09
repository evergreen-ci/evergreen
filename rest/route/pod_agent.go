package route

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/model/pod/dispatcher"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

/////////////////////////////////////////////////
//
// GET /rest/v2/pods/{pod_id}/provisioning_script

type podProvisioningScript struct {
	settings *evergreen.Settings
	podID    string
}

func makePodProvisioningScript(settings *evergreen.Settings) gimlet.RouteHandler {
	return &podProvisioningScript{settings: settings}
}

func (h *podProvisioningScript) Factory() gimlet.RouteHandler {
	return &podProvisioningScript{settings: h.settings}
}

func (h *podProvisioningScript) Parse(ctx context.Context, r *http.Request) error {
	h.podID = gimlet.GetVars(r)["pod_id"]
	return nil
}

// Run returns the script to provision this pod. Unlike the typical REST routes
// that return JSON responses, this returns text responses for simplicity and
// because the pod's containers unlikely to be equipped with the tooling to
// parse JSON output.
func (h *podProvisioningScript) Run(ctx context.Context) gimlet.Responder {
	p, err := pod.FindOneByID(h.podID)
	if err != nil {
		return gimlet.NewTextInternalErrorResponse(errors.Wrap(err, "finding pod"))
	}
	if p == nil {
		return gimlet.NewTextErrorResponse(errors.Errorf("pod '%s' not found", h.podID))
	}

	grip.Info(message.Fields{
		"message":                   "pod requested its provisioning script",
		"pod":                       p.ID,
		"external_id":               p.Resources.ExternalID,
		"secs_since_pod_allocation": time.Since(p.TimeInfo.Initializing).Seconds(),
		"secs_since_pod_creation":   time.Since(p.TimeInfo.Starting).Seconds(),
	})

	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		gimlet.NewTextInternalErrorResponse(errors.Wrap(err, "getting service flags"))
	}

	script := h.agentScript(p, !flags.S3BinaryDownloadsDisabled)

	return gimlet.NewTextResponse(script)
}

// agentScript returns the script to provision and run the agent in the pod's
// container. On Linux, this is a shell script. On Windows, this is a cmd.exe
// batch script.
func (h *podProvisioningScript) agentScript(p *pod.Pod, downloadFromS3 bool) string {
	scriptCmds := []string{h.downloadAgentCommands(p, downloadFromS3)}
	if p.TaskContainerCreationOpts.OS == pod.OSLinux {
		scriptCmds = append(scriptCmds, fmt.Sprintf("chmod +x %s", h.clientName(p)))
	}
	agentCmd := strings.Join(h.agentCommand(p), " ")
	scriptCmds = append(scriptCmds, agentCmd)

	return strings.Join(scriptCmds, " && ")
}

// agentCommand returns the arguments to start the agent in the pod's container.
func (h *podProvisioningScript) agentCommand(p *pod.Pod) []string {
	var pathSep string
	if p.TaskContainerCreationOpts.OS == pod.OSWindows {
		pathSep = "\\"
	} else {
		pathSep = "/"
	}

	return []string{
		fmt.Sprintf(".%s%s", pathSep, h.clientName(p)),
		"agent",
		fmt.Sprintf("--api_server=%s", h.settings.ApiUrl),
		"--mode=pod",
		fmt.Sprintf("--log_prefix=%s", filepath.Join(p.TaskContainerCreationOpts.WorkingDir, "agent")),
		fmt.Sprintf("--working_directory=%s", p.TaskContainerCreationOpts.WorkingDir),
	}
}

// downloadAgentCommands returns the commands to download the agent in the pod's
// container.
func (h *podProvisioningScript) downloadAgentCommands(p *pod.Pod, downloadFromS3 bool) string {
	const (
		curlDefaultNumRetries = 10
		curlDefaultMaxSecs    = 100
	)
	retryArgs := h.curlRetryArgs(curlDefaultNumRetries, curlDefaultMaxSecs)

	var curlCmd string
	if downloadFromS3 && h.settings.PodInit.S3BaseURL != "" {
		// Attempt to download the agent from S3, but fall back to downloading
		// from the app server if it fails.
		// Include -f to return an error code from curl if the HTTP request
		// fails (e.g. it receives 403 Forbidden or 404 Not Found).
		curlCmd = fmt.Sprintf("(curl -fLO %s %s || curl -fLO %s %s)", h.s3ClientURL(p), retryArgs, h.evergreenClientURL(p), retryArgs)
	} else {
		curlCmd = fmt.Sprintf("curl -fLO %s %s", h.evergreenClientURL(p), retryArgs)
	}

	return curlCmd
}

// evergreenClientURL returns the URL used to get the latest Evergreen client
// version directly from the Evergreen server.
func (h *podProvisioningScript) evergreenClientURL(p *pod.Pod) string {
	return strings.Join([]string{
		strings.TrimSuffix(h.settings.ApiUrl, "/"),
		strings.TrimSuffix(h.settings.ClientBinariesDir, "/"),
		h.clientURLSubpath(p),
	}, "/")
}

// s3ClientURL returns the URL in S3 where the Evergreen client version can be
// retrieved for this server's particular Evergreen build version.
func (h *podProvisioningScript) s3ClientURL(p *pod.Pod) string {
	return strings.Join([]string{
		strings.TrimSuffix(h.settings.PodInit.S3BaseURL, "/"),
		evergreen.BuildRevision,
		h.clientURLSubpath(p),
	}, "/")
}

// clientURLSubpath returns the URL path to the compiled agent.
func (h *podProvisioningScript) clientURLSubpath(p *pod.Pod) string {
	return strings.Join([]string{
		fmt.Sprintf("%s_%s", p.TaskContainerCreationOpts.OS, p.TaskContainerCreationOpts.Arch),
		h.clientName(p),
	}, "/")
}

// clientName returns the file name of the agent binary.
func (h *podProvisioningScript) clientName(p *pod.Pod) string {
	name := "evergreen"
	if p.TaskContainerCreationOpts.OS == pod.OSWindows {
		return name + ".exe"
	}
	return name
}

// curlRetryArgs constructs options to configure the curl retry behavior.
func (h *podProvisioningScript) curlRetryArgs(numRetries, maxSecs int) string {
	return fmt.Sprintf("--retry %d --retry-max-time %d", numRetries, maxSecs)
}

/////////////////////////////////////////
//
// GET /rest/v2/pods/{pod_id}/agent/setup

type podAgentSetup struct {
	settings *evergreen.Settings
}

func makePodAgentSetup(settings *evergreen.Settings) gimlet.RouteHandler {
	return &podAgentSetup{
		settings: settings,
	}
}

func (h *podAgentSetup) Factory() gimlet.RouteHandler {
	return &podAgentSetup{
		settings: h.settings,
	}
}

func (h *podAgentSetup) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *podAgentSetup) Run(ctx context.Context) gimlet.Responder {
	data := apimodels.AgentSetupData{
		SplunkServerURL:   h.settings.Splunk.ServerURL,
		SplunkClientToken: h.settings.Splunk.Token,
		SplunkChannel:     h.settings.Splunk.Channel,
		S3Bucket:          h.settings.Providers.AWS.S3.Bucket,
		S3Key:             h.settings.Providers.AWS.S3.Key,
		S3Secret:          h.settings.Providers.AWS.S3.Secret,
		TaskSync:          h.settings.Providers.AWS.TaskSync,
		LogkeeperURL:      h.settings.LoggerConfig.LogkeeperURL,
	}
	return gimlet.NewJSONResponse(data)
}

////////////////////////////////////////////////
//
// GET /rest/v2/pods/{pod_id}/agent/cedar_config

type podAgentCedarConfig struct {
	settings *evergreen.Settings
}

func makePodAgentCedarConfig(settings *evergreen.Settings) gimlet.RouteHandler {
	return &podAgentCedarConfig{
		settings: settings,
	}
}

func (h *podAgentCedarConfig) Factory() gimlet.RouteHandler {
	return &podAgentCedarConfig{
		settings: h.settings,
	}
}

func (h *podAgentCedarConfig) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *podAgentCedarConfig) Run(ctx context.Context) gimlet.Responder {
	data := apimodels.CedarConfig{
		BaseURL:  h.settings.Cedar.BaseURL,
		RPCPort:  h.settings.Cedar.RPCPort,
		Username: h.settings.Cedar.User,
		APIKey:   h.settings.Cedar.APIKey,
	}
	return gimlet.NewJSONResponse(data)
}

////////////////////////////////////////////////
//
// GET /rest/v2/pods/{pod_id}/agent/next_task

type podAgentNextTask struct {
	env   evergreen.Environment
	podID string
}

func makePodAgentNextTask(env evergreen.Environment) gimlet.RouteHandler {
	return &podAgentNextTask{
		env: env,
	}
}

func (h *podAgentNextTask) Factory() gimlet.RouteHandler {
	return &podAgentNextTask{
		env: h.env,
	}
}

func (h *podAgentNextTask) Parse(ctx context.Context, r *http.Request) error {
	h.podID = gimlet.GetVars(r)["pod_id"]
	if h.podID == "" {
		return errors.New("missing pod ID")
	}
	return nil
}

func (h *podAgentNextTask) Run(ctx context.Context) gimlet.Responder {
	p, err := h.findPod()
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	h.setAgentFirstContactTime(p)

	pd, err := h.findDispatcher()
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	nextTask, err := pd.AssignNextTask(ctx, h.env, p)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "dispatching next task").Error(),
		})
	}
	if nextTask == nil {
		j := units.NewPodTerminationJob(h.podID, "reached end of pod lifecycle", utility.RoundPartOfMinute(0))
		if err := amboy.EnqueueUniqueJob(ctx, h.env.RemoteQueue(), j); err != nil {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    errors.Wrap(err, "enqueueing job to terminate pod").Error(),
			})
		}
	}

	return gimlet.NewJSONResponse(apimodels.NextTaskResponse{})
}

func (h *podAgentNextTask) findPod() (*pod.Pod, error) {
	p, err := pod.FindOneByID(h.podID)
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "finding pod").Error(),
		}
	}
	if p == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("pod '%s' not found", h.podID),
		}
	}

	return p, nil
}

func (h *podAgentNextTask) setAgentFirstContactTime(p *pod.Pod) {
	if !p.TimeInfo.Initializing.IsZero() {
		return
	}

	if err := p.SetAgentStartTime(); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "could not update pod's agent first contact time",
			"pod":     p.ID,
		}))
		return
	}

	grip.Info(message.Fields{
		"message":                   "pod initiated first contact with application server",
		"pod":                       p.ID,
		"secs_since_pod_allocation": time.Since(p.TimeInfo.Initializing).Seconds(),
		"secs_since_pod_creation":   time.Since(p.TimeInfo.Starting).Seconds(),
	})
}

func (h *podAgentNextTask) findDispatcher() (*dispatcher.PodDispatcher, error) {
	pd, err := dispatcher.FindOneByPodID(h.podID)
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "finding dispatcher").Error(),
		}
	}
	if pd == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("dispatcher for pod '%s' not found", h.podID),
		}
	}

	return pd, nil
}
