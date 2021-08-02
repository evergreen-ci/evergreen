package pod

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/pkg/errors"
)

// TODO: for binaryName, check for pod OS
const (
	workingDir = "/data/mci"
	binaryName = "evergreen"
	shell      = "CMD-SHELL"
)

// Constants representing default curl retry arguments.
const (
	curlDefaultNumRetries = 10
	curlDefaultMaxSecs    = 100
)

///////////////////////////////////////////////////////////////////////////
// BIG TODO: build shell script that downloads + runs agent
// - download agent + start it inside of container working directory
// TODO: CURL command to download agent
// Assume CURL comes w/ container image (simplicity)

// (curl -LO <some retry options> <evergreen S3 URL> || curl -LO <some retry options> <evergreen app URL>) && ./evergreen agent <agent args>

// TODO: somewhere, put "CMD-SHELL"

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
	if !flags.S3BinaryDownloadsDisabled && settings.HostInit.S3BaseURL != "" {
		// Attempt to download the agent from S3, but fall back to downloading from
		// the app server if it fails.
		// Include -f to return an error code from curl if the HTTP request
		// fails (e.g. it receives 403 Forbidden or 404 Not Found).
		curlCmd = fmt.Sprintf("(curl -fLO '%s'%s || curl -LO '%s'%s)", p.S3ClientURL(settings), curlArgs, p.ClientURL(settings), curlArgs)
	} else {
		curlCmd += fmt.Sprintf("curl -LO '%s'%s", p.ClientURL(settings), curlArgs)
	}
	return []string{
		fmt.Sprintf("cd %s", workingDir),
		shell,
		curlCmd,
		fmt.Sprintf("chmod +x %s", binaryName),
	}, nil
}

// AgentCommand returns the arguments to start the agent. If executablePath is not specified, it
// will be assumed to be in the regular place.
func (p *Pod) AgentCommand(settings *evergreen.Settings, executablePath string) []string {
	if executablePath == "" {
		executablePath = filepath.Join(workingDir, binaryName)
	}

	return []string{
		executablePath,
		"agent",
		fmt.Sprintf("--api_server=%s", settings.ApiUrl),
		fmt.Sprintf("--mode=pod"),
		fmt.Sprintf("--pod_id=%s", p.ID),
		fmt.Sprintf("pod_secret=%s", p.Secret),
		fmt.Sprintf("--log_prefix=%s", filepath.Join(workingDir, "agent")),
		fmt.Sprintf("--working_directory=%s", workingDir),
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
		// TODO (EVG-15165): Add admin setting for S3 base URL for pod
		strings.TrimSuffix(settings.HostInit.S3BaseURL, "/"),
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
	return filepath.Join(string(p.TaskContainerCreationOpts.Arch), binaryName)
}

func curlRetryArgs(numRetries, maxSecs int) string {
	return fmt.Sprintf("--retry %d --retry-max-time %d", numRetries, maxSecs)
}
