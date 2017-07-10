package subprocess

import "github.com/mongodb/grip"

// envHasMarkers returns a bool indicating if both marker vars are found in an environment var list
func envHasMarkers(env []string, pidMarker, taskMarker string) bool {
	hasPidMarker := false
	hasTaskMarker := false
	for _, envVar := range env {
		if envVar == pidMarker {
			hasPidMarker = true
		}
		if envVar == taskMarker {
			hasTaskMarker = true
		}
	}
	return hasPidMarker && hasTaskMarker
}

// KillSpawnedProcs cleans up any tasks that were spawned by the given task.
func KillSpawnedProcs(taskId string, logger grip.Journaler) error {
	// Clean up all shell processes spawned during the execution of this task by this agent,
	// by calling the platform-specific "cleanup" function
	return cleanup(taskId, logger)
}
