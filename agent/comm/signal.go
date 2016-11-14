package comm

// Signal describes the various conditions under which the agent
// will complete execution of a task.
type Signal int64

// Recognized agent signals.
const (
	// HeartbeatMaxFailed indicates that repeated attempts to send heartbeat to
	// the API server fails.
	HeartbeatMaxFailed Signal = iota
	// IncorrectSecret indicates that the secret for the task the agent is running
	// does not match the task secret held by API server.
	IncorrectSecret
	// AbortedByUser indicates a user decided to prematurely end the task.
	AbortedByUser
	// IdleTimeout indicates the task appears to be idle - e.g. no logs produced
	// for the duration indicated by DefaultIdleTimeout.
	IdleTimeout
	// Completed indicates that the task completed without incident. This signal is
	// used internally to shut down the signal handler.
	Completed
	// Directory Failure indicates that the task failed due to a problem for the agent
	// creating or moving into a new directory.
	DirectoryFailure
)
