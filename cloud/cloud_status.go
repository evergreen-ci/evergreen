package cloud

type CloudStatus int

const (
	//Catch-all for unrecognized status codes
	StatusUnknown = CloudStatus(iota)

	//StatusPending indicates that it is not yet clear if the instance
	//has been successfully started or not (e.g., pending spot request)
	StatusPending

	//StatusInitializing means the instance request has been successfully
	//fulfilled, but it's not yet done booting up
	StatusInitializing

	//StatusFailed indicates that an attempt to start the instance has failed;
	//Could be due to billing, lack of capacity, etc.
	StatusFailed

	//StatusRunning means the machine is done booting, and active
	StatusRunning

	StatusStopped
	StatusTerminated
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
	case StatusStopped:
		return "stopped"
	case StatusTerminated:
		return "terminated"
	default:
		return "unknown"
	}
}
