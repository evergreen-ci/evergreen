package evergreen

// this is a temporary collection of service flags used to toggle
// between different implementations of runner-service while we
// migrate from the legacy implementation to the amboy-backed queues.

const (
	UseNewHostProvisioning = false
	UseNewHostStarting     = false
)
