package cloud

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/pod"
)

// bootstrapContainerCommand returns the script to bootstrap the pod's primary container
// to run the agent.
func bootstrapContainerCommand(settings *evergreen.Settings, p *pod.Pod) []string {
	scriptCmds := downloadPodProvisioningScriptCommand(settings, p)

	return append(invokeShellScriptCommand(p), scriptCmds)
}

// invokeShellScriptCommand returns the arguments to invoke an in-line shell
// script in the pod's container.
func invokeShellScriptCommand(p *pod.Pod) []string {
	if p.TaskContainerCreationOpts.OS == pod.OSWindows {
		return []string{"powershell.exe", "-noninteractive", "-noprofile", "-Command"}
	}

	return []string{"bash", "-c"}
}

func downloadPodProvisioningScriptCommand(settings *evergreen.Settings, p *pod.Pod) string {
	const (
		curlDefaultNumRetries = 10
		curlDefaultMaxSecs    = 60
	)
	retryArgs := curlRetryArgs(curlDefaultNumRetries, curlDefaultMaxSecs)

	// Return the command to download the provisioning script from the app
	// server and run it in a shell script.

	if p.TaskContainerCreationOpts.OS == pod.OSLinux {
		// For Linux, run the script using bash syntax to interpolate the
		// environment variables and pipe the script into another bash instance.
		return fmt.Sprintf("curl %s -L -H \"%s: ${%s}\" -H \"%s: ${%s}\" %s/rest/v2/pods/${%s}/provisioning_script | bash -s", retryArgs, evergreen.PodHeader, pod.PodIDEnvVar, evergreen.PodSecretHeader, pod.PodSecretEnvVar, strings.TrimSuffix(settings.ApiUrl, "/"), pod.PodIDEnvVar)
	}

	// For Windows, run the script using PowerShell syntax to interpolate the
	// environment variables and pipe the script into another PowerShell
	// instance.
	return fmt.Sprintf("curl.exe %s -L -H \"%s: $env:%s\" -H \"%s: $env:%s\" %s/rest/v2/pods/$env:%s/provisioning_script | powershell.exe -noprofile -noninteractive -", retryArgs, evergreen.PodHeader, pod.PodIDEnvVar, evergreen.PodSecretHeader, pod.PodSecretEnvVar, strings.TrimSuffix(settings.ApiUrl, "/"), pod.PodIDEnvVar)
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
		"--mode=pod",
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
