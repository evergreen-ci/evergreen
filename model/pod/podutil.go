package pod

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/pkg/errors"
)

const (
	executable = "./evergreen"
	shell      = "CMD-SHELL"
)

// Constants representing default curl retry arguments.
const (
	curlDefaultNumRetries = 10
	curlDefaultMaxSecs    = 100
)

// CurlCommandWithRetry returns the command to curl the evergreen client and retries the request.
func (p *Pod) CurlCommandWithRetry(settings *evergreen.Settings, numRetries, maxRetrySecs int) (string, error) {
	var retryArgs string
	if numRetries != 0 && maxRetrySecs != 0 {
		retryArgs = " " + curlRetryArgs(numRetries, maxRetrySecs)
	}
	cmds, err := p.curlCommands(settings, retryArgs)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return strings.Join(cmds, " && "), nil
}

// CurlCommandWithDefaultRetry is the same as CurlCommandWithRetry using the
// default retry parameters.
func (p *Pod) CurlCommandWithDefaultRetry(settings *evergreen.Settings) (string, error) {
	return p.CurlCommandWithRetry(settings, curlDefaultNumRetries, curlDefaultMaxSecs)
}

func (p *Pod) curlCommands(settings *evergreen.Settings, curlArgs string) ([]string, error) {
	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		return nil, errors.Wrap(err, "getting service flags")
	}

	var curlCmd string
	if !flags.S3BinaryDownloadsDisabled && settings.PodInit.S3BaseURL != "" {
		// Attempt to download the agent from S3, but fall back to downloading from
		// the app server if it fails.
		curlCmd = fmt.Sprintf("(curl -LO '%s'%s || curl -LO '%s'%s)", curlArgs, p.S3ClientURL(settings), curlArgs, p.ClientURL(settings))
	} else {
		curlCmd += fmt.Sprintf("curl -LO '%s'%s", p.ClientURL(settings), curlArgs)
	}

	agentCmd := strings.Join(p.AgentCommand(settings), " ")

	return []string{
		shell,
		curlCmd,
		agentCmd,
		fmt.Sprintf("chmod +x %s", p.BinaryName()),
	}, nil
}

// AgentCommand returns the arguments to start the agent.
func (p *Pod) AgentCommand(settings *evergreen.Settings) []string {
	return []string{
		executable,
		"agent",
		fmt.Sprintf("--api_server=%s", settings.ApiUrl),
		fmt.Sprintf("--mode=pod"),
		fmt.Sprintf("--pod_id=%s", p.ID),
		fmt.Sprintf("pod_secret=%s", p.Secret),
		fmt.Sprintf("--log_prefix=%s", filepath.Join(p.TaskContainerCreationOpts.WorkingDir, "agent")),
		fmt.Sprintf("--working_directory=%s", p.TaskContainerCreationOpts.WorkingDir),
		"--cleanup",
	}
}

// ClientURL returns the URL used to get the latest Evergreen client version
// directly from the Evergreen server.
func (p *Pod) ClientURL(settings *evergreen.Settings) string {
	return strings.Join([]string{
		strings.TrimSuffix(settings.ApiUrl, "/"),
		strings.TrimSuffix(settings.ClientBinariesDir, "/"),
		p.ExecutableSubPath(),
	}, "/")
}

// S3ClientURL returns the URL in S3 where the Evergreen client version can be
// retrieved for this server's particular Evergreen build version.
func (p *Pod) S3ClientURL(settings *evergreen.Settings) string {
	return strings.Join([]string{
		strings.TrimSuffix(settings.PodInit.S3BaseURL, "/"),
		evergreen.BuildRevision,
		p.ExecutableSubPath(),
	}, "/")
}

// BinaryName returns the file name of the binary.
func (p *Pod) BinaryName() string {
	name := "evergreen"
	if p.TaskContainerCreationOpts.OS == OSWindows {
		return name + ".exe"
	}
	return name
}

// ExecutableSubPath returns the directory containing the compiled agents.
func (p *Pod) ExecutableSubPath() string {
	return filepath.Join(string(p.TaskContainerCreationOpts.Arch), p.BinaryName())
}

func curlRetryArgs(numRetries, maxSecs int) string {
	return fmt.Sprintf("--retry %d --retry-max-time %d", numRetries, maxSecs)
}
