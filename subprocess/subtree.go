package subprocess

import (
	"os"
	"strings"

	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip"
)

const (
	MarkerTaskID   = "EVR_TASK_ID"
	MarkerAgentPID = "EVR_AGENT_PID"
)

func envHasMarkers(key string, env []string) bool {
	// If this agent was started by an integration test, only kill a proc if it was started by this agent
	if os.Getenv(testutil.EnvAll) != "" {
		for _, envVar := range env {
			if strings.HasPrefix(envVar, MarkerTaskID) {
				split := strings.Split(envVar, "=")
				if len(split) != 2 {
					continue
				}
				if split[1] == key {
					return true
				}
			}
		}
		return false
	}

	// Otherwise, kill any proc started by any agent
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
