package cloud

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/pod"
)

// agentScript returns the script to provision and run the agent in the pod's
// container.
func agentScript(settings *evergreen.Settings, p *pod.Pod) []string {
	scriptCmds := []string{downloadAgentCommands(settings, p)}
	if p.TaskContainerCreationOpts.OS == pod.OSLinux {
		scriptCmds = append(scriptCmds, fmt.Sprintf("chmod +x %s", clientName(p)))
	}
	agentCmd := strings.Join(agentCommand(settings, p), " ")
	scriptCmds = append(scriptCmds, agentCmd)

	return append(invokeShellScriptCommand(p), strings.Join(scriptCmds, " && "))
}

// invokeShellScriptCommand returns the arguments to invoke an in-line shell
// script in the pod's container.
func invokeShellScriptCommand(p *pod.Pod) []string {
	if p.TaskContainerCreationOpts.OS == pod.OSWindows {
		return []string{"cmd.exe", "/c"}
	}

	return []string{"bash", "-c"}
}

// agentCommand returns the arguments to start the agent in the pod's container.
func agentCommand(settings *evergreen.Settings, p *pod.Pod) []string {
	var pathSep string
	if p.TaskContainerCreationOpts.OS == pod.OSWindows {
		pathSep = "\\"
	} else {
		pathSep = "/"
	}

	return []string{
		fmt.Sprintf(".%s%s", pathSep, clientName(p)),
		"agent",
		fmt.Sprintf("--api_server=%s", settings.ApiUrl),
		fmt.Sprintf("--mode=pod"),
		fmt.Sprintf("--log_prefix=%s", filepath.Join(p.TaskContainerCreationOpts.WorkingDir, "agent")),
		fmt.Sprintf("--working_directory=%s", p.TaskContainerCreationOpts.WorkingDir),
	}
}

// downloadAgentCommands returns the commands to download the agent in the pod's
// container.
func downloadAgentCommands(settings *evergreen.Settings, p *pod.Pod) string {
	const (
		curlDefaultNumRetries = 10
		curlDefaultMaxSecs    = 100
	)
	retryArgs := curlRetryArgs(curlDefaultNumRetries, curlDefaultMaxSecs)

	var curlCmd string
	if !settings.ServiceFlags.S3BinaryDownloadsDisabled && settings.PodInit.S3BaseURL != "" {
		// Attempt to download the agent from S3, but fall back to downloading
		// from the app server if it fails.
		// Include -f to return an error code from curl if the HTTP request
		// fails (e.g. it receives 403 Forbidden or 404 Not Found).
		curlCmd = fmt.Sprintf("(curl -fLO %s %s || curl -fLO %s %s)", s3ClientURL(settings, p), retryArgs, clientURL(settings, p), retryArgs)
	} else {
		curlCmd = fmt.Sprintf("curl -fLO %s %s", clientURL(settings, p), retryArgs)
	}

	return curlCmd
}

// clientURL returns the URL used to get the latest Evergreen client version
// directly from the Evergreen server.
func clientURL(settings *evergreen.Settings, p *pod.Pod) string {
	return strings.Join([]string{
		strings.TrimSuffix(settings.ApiUrl, "/"),
		strings.TrimSuffix(settings.ClientBinariesDir, "/"),
		clientURLSubpath(p),
	}, "/")
}

// s3ClientURL returns the URL in S3 where the Evergreen client version can be
// retrieved for this server's particular Evergreen build version.
func s3ClientURL(settings *evergreen.Settings, p *pod.Pod) string {
	return strings.Join([]string{
		strings.TrimSuffix(settings.PodInit.S3BaseURL, "/"),
		evergreen.BuildRevision,
		clientURLSubpath(p),
	}, "/")
}

// clientURLSubpath returns the URL path to the compiled agent.
func clientURLSubpath(p *pod.Pod) string {
	return filepath.Join(
		fmt.Sprintf("%s_%s", p.TaskContainerCreationOpts.OS, p.TaskContainerCreationOpts.Arch),
		clientName(p),
	)
}

// clientName returns the file name of the agent binary.
func clientName(p *pod.Pod) string {
	name := "evergreen"
	if p.TaskContainerCreationOpts.OS == pod.OSWindows {
		return name + ".exe"
	}
	return name
}

// curlRetryArgs constructs options to configure the curl retry behavior.
func curlRetryArgs(numRetries, maxSecs int) string {
	return fmt.Sprintf("--retry %d --retry-max-time %d", numRetries, maxSecs)
}
