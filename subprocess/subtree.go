package subprocess

import (
	"strings"

	"github.com/mongodb/grip"
)

const (
	MarkerTaskID   = "EVR_TASK_ID"
	MarkerAgentPID = "EVR_AGENT_PID"
)

// envHasMarkers returns a bool indicating if either marker vars is found in an environment var list
func envHasMarkers(env []string) bool {
	for _, envVar := range env {
		if strings.HasPrefix(envVar, MarkerTaskID) || strings.HasPrefix(envVar, MarkerAgentPID) {
			return true
		}
	}

	return false
}

// KillSpawnedProcs cleans up any tasks that were spawned by the given task.
func KillSpawnedProcs(key string, logger grip.Journaler) error {
	// Clean up all shell processes spawned during the execution of this task by this agent,
	// by calling the platform-specific "cleanup" function
	return cleanup(key, logger)
}
