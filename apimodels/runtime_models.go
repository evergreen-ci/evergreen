package apimodels

// Struct for reporting process timeouts
type ProcessTimeoutResponse struct {
	Status        string      `json:"status"`
	LateProcesses interface{} `json:"late_mci_processes,omitempty"`
}
