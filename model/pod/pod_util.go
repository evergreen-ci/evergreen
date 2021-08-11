package pod

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/pkg/errors"
)

// Constants representing default curl retry arguments.
const (
	curlDefaultNumRetries = 10
	curlDefaultMaxSecs    = 100
)

// AgentScript returns the script to provision and run the agent in the pod's container.
func (p *Pod) AgentScript(settings *evergreen.Settings) ([]string, error) {
	retryArgs := " " + curlRetryArgs(curlDefaultNumRetries, curlDefaultMaxSecs)

	curlCmd, err := p.curlCommands(settings, retryArgs)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	agentCmd := strings.Join(p.agentCommand(settings), " ")

	return []string{
		"CMD-SHELL",
		strings.Join([]string{curlCmd, fmt.Sprintf("chmod +x %s", p.binaryName()), agentCmd}, " && "),
	}, nil
}

// agentCommand returns the arguments to start the agent.
func (p *Pod) agentCommand(settings *evergreen.Settings) []string {
	return []string{
		fmt.Sprintf("./%s", p.binaryName()),
		"agent",
		fmt.Sprintf("--api_server=%s", settings.ApiUrl),
		fmt.Sprintf("--mode=pod"),
		fmt.Sprintf("--pod_id=%s", p.ID),
		fmt.Sprintf("--pod_secret=%s", p.Secret),
		fmt.Sprintf("--log_prefix=%s", filepath.Join(p.TaskContainerCreationOpts.WorkingDir, "agent")),
		fmt.Sprintf("--working_directory=%s", p.TaskContainerCreationOpts.WorkingDir),
	}
}

// clientURL returns the URL used to get the latest Evergreen client version
// directly from the Evergreen server.
func (p *Pod) clientURL(settings *evergreen.Settings) string {
	return strings.Join([]string{
		strings.TrimSuffix(settings.ApiUrl, "/"),
		strings.TrimSuffix(settings.ClientBinariesDir, "/"),
		p.clientURLSubpath(),
	}, "/")
}

// s3ClientURL returns the URL in S3 where the Evergreen client version can be
// retrieved for this server's particular Evergreen build version.
func (p *Pod) s3ClientURL(settings *evergreen.Settings) string {
	return strings.Join([]string{
		strings.TrimSuffix(settings.PodInit.S3BaseURL, "/"),
		evergreen.BuildRevision,
		p.clientURLSubpath(),
	}, "/")
}

// binaryName returns the file name of the binary.
func (p *Pod) binaryName() string {
	name := "evergreen"
	if p.TaskContainerCreationOpts.OS == OSWindows {
		return name + ".exe"
	}
	return name
}

// clientURLSubpath returns the URL path to the compiled agent.
func (p *Pod) clientURLSubpath() string {
	return filepath.Join(
		string(p.TaskContainerCreationOpts.OS)+"_"+string(p.TaskContainerCreationOpts.Arch),
		p.binaryName(),
	)
}

func (p *Pod) curlCommands(settings *evergreen.Settings, curlArgs string) (string, error) {
	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		return "", errors.Wrap(err, "getting service flags")
	}
	var curlCmd string
	if !flags.S3BinaryDownloadsDisabled && settings.PodInit.S3BaseURL != "" {
		// Attempt to download the agent from S3, but fall back to downloading from
		// the app server if it fails.
		curlCmd = fmt.Sprintf("(curl -LO '%s'%s || curl -LO '%s'%s)", p.s3ClientURL(settings), curlArgs, p.clientURL(settings), curlArgs)
	} else {
		curlCmd = fmt.Sprintf("curl -LO '%s'%s", p.clientURL(settings), curlArgs)
	}

	return curlCmd, nil
}

func curlRetryArgs(numRetries, maxSecs int) string {
	return fmt.Sprintf("--retry %d --retry-max-time %d", numRetries, maxSecs)
}
