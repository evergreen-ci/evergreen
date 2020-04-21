package apimodels

import "github.com/mongodb/grip/send"

// Struct for reporting process timeouts
type ProcessTimeoutResponse struct {
	Status        string      `json:"status"`
	LateProcesses interface{} `json:"late_mci_processes,omitempty"`
}

type WorkstationSetupCommandOptions struct {
	Directory string
	Quiet     bool
	DryRun    bool
	Output    send.Sender
}
