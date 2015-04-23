package apimodels

// Contains the pid sent by the agent at the beginning of each task run
type TaskStartRequest struct {
	Pid string `json:"pid"`
}

// Contains any data sent back to the agent as a response to a hearbeat
type HeartbeatResponse struct {
	Abort bool `json:"abort,omitempty"`
}

// Contains data sent from the agent to the API server after a task has completed
type TaskEndRequest struct {
	Status        string         `bson:"status,omitempty" json:"status,omitempty"`
	StatusDetails TaskEndDetails `bson:"status_details,omitempty" json:"status_details,omitempty"`
}

// some any additional details we want to pass alongside the markEnd call
type TaskEndDetails struct {
	TimeoutStage string `bson:"timeout_stage,omitempty" json:"timeout_stage,omitempty"`
	TimedOut     bool   `bson:"timed_out,omitempty" json:"timed_out,omitempty"`
}

// Contains data sent back by the api server once the agent responds that it has
// completed a task
type TaskEndResponse struct {
	TaskId     string `json:"task_id,omitempty"`
	TaskSecret string `json:"task_secret,omitempty"`
	ConfigDir  string `json:"config_dir,omitempty"`
	Message    string `json:"message,omitempty"`
	WorkDir    string `json:"work_dir,omitempty"`
	RunNext    bool   `json:"run_next,omitempty"`
}

// map of Expansion vars for this project
type ExpansionVars map[string]string
