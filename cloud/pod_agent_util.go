package cloud

import (
	"fmt"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/pod"
)

// bootstrapContainerCommand returns the script to bootstrap the pod's primary container
// to run the agent.
func bootstrapContainerCommand(settings evergreen.Settings, opts pod.TaskContainerCreationOptions) []string {
	scriptCmds := downloadPodProvisioningScriptCommand(settings, opts)

	return append(invokeShellScriptCommand(opts), scriptCmds)
}

// invokeShellScriptCommand returns the arguments to invoke an in-line shell
// script in the pod's container.
func invokeShellScriptCommand(opts pod.TaskContainerCreationOptions) []string {
	if opts.OS == pod.OSWindows {
		return []string{"powershell.exe", "-noninteractive", "-noprofile", "-Command"}
	}

	return []string{"bash", "-c"}
}

// downloadPodProvisioningScriptCommand returns the command to download and
// execute the provisioning script for a pod's container options.
func downloadPodProvisioningScriptCommand(settings evergreen.Settings, opts pod.TaskContainerCreationOptions) string {
	const (
		curlDefaultNumRetries = 10
		curlDefaultMaxSecs    = 60
	)
	retryArgs := curlRetryArgs(curlDefaultNumRetries, curlDefaultMaxSecs)

	// Return the command to download the provisioning script from the app
	// server and run it in a shell script.

	if opts.OS == pod.OSLinux {
		// For Linux, run the script using bash syntax to interpolate the
		// environment variables and pipe the script into another bash instance.
		return fmt.Sprintf("curl %s -L -H \"%s: ${%s}\" -H \"%s: ${%s}\" %s/rest/v2/pods/${%s}/provisioning_script | bash -s", retryArgs, evergreen.PodHeader, pod.PodIDEnvVar, evergreen.PodSecretHeader, pod.PodSecretEnvVar, strings.TrimSuffix(settings.ApiUrl, "/"), pod.PodIDEnvVar)
	}

	// For Windows, run the script using PowerShell syntax to interpolate the
	// environment variables and pipe the script into another PowerShell
	// instance.
	return fmt.Sprintf("curl.exe %s -L -H \"%s: $env:%s\" -H \"%s: $env:%s\" %s/rest/v2/pods/$env:%s/provisioning_script | powershell.exe -noprofile -noninteractive -", retryArgs, evergreen.PodHeader, pod.PodIDEnvVar, evergreen.PodSecretHeader, pod.PodSecretEnvVar, strings.TrimSuffix(settings.ApiUrl, "/"), pod.PodIDEnvVar)
}

// curlRetryArgs constructs options to configure the curl retry behavior.
func curlRetryArgs(numRetries, maxSecs int) string {
	return fmt.Sprintf("--retry %d --retry-max-time %d", numRetries, maxSecs)
}
