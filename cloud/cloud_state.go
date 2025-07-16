package cloud

type CloudInstanceState struct {
	// Status is the current status of the instance.
	Status CloudStatus
	// StateReason is a human-readable explanation of why the instance is in
	// its current state.
	StateReason string
}

type CloudStatus int

const (
	// StatusUnknown is a catch-all for unrecognized status codes.
	StatusUnknown = CloudStatus(iota)

	// StatusPending indicates that it is not yet clear if the instance has been
	// successfully started or not (e.g. pending request).
	StatusPending

	// StatusInitializing means the instance request has been successfully
	// fulfilled, but it's not yet done booting up.
	StatusInitializing

	// StatusFailed indicates that an attempt to start the instance has failed.
	// This could be due to billing, lack of capacity, etc.
	StatusFailed

	// StatusRunning means the machine is done booting, and active.
	StatusRunning

	// StatusStopping indicates that the instance is in the processing of
	// stopping but has not yet stopped completely.
	StatusStopping

	// StatusStopped indicates that the instance is shut down, but can be
	// started again.
	StatusStopped

	// StatusTerminated indicates that the instance is deleted.
	StatusTerminated

	// StatusNonExistent indicates that the instance doesn't exist.
	StatusNonExistent
)

func (stat CloudStatus) String() string {
	switch stat {
	case StatusPending:
		return "pending"
	case StatusFailed:
		return "failed"
	case StatusInitializing:
		return "initializing"
	case StatusRunning:
		return "running"
	case StatusStopping:
		return "stopping"
	case StatusStopped:
		return "stopped"
	case StatusTerminated:
		return "terminated"
	case StatusNonExistent:
		return "nonexistent"
	case StatusUnknown:
		return "unknown"
	default:
		return "invalid"
	}
}
