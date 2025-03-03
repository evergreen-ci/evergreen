package apimodels

// Struct for reporting process timeouts
type ProcessTimeoutResponse struct {
	Status        string `json:"status"`
	LateProcesses any    `json:"late_mci_processes,omitempty"`
}

type WorkstationSetupCommandOptions struct {
	Directory string
	Quiet     bool
	DryRun    bool
}
