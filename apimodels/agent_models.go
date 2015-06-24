package apimodels

// TaskStartRequest holds information sent by the agent to the
// API server at the beginning of each task run.
type TaskStartRequest struct {
	Pid string `json:"pid"`
}

// HeartbeatResponse is sent by the API server in response to
// the agent's heartbeat message.
type HeartbeatResponse struct {
	Abort bool `json:"abort,omitempty"`
}

// TaskEndRequest contains data sent from the agent to the
// API server after each task run.
type TaskEndRequest struct {
	Status      string         `bson:"status,omitempty" json:"status,omitempty"`
	Type        string         `bson:"type,omitempty" json:"type,omitempty"`
	Description string         `bson:"desc,omitempty" json:"desc,omitempty"`
	TimedOut    bool           `bson:"timed_out,omitempty" json:"timed_out,omitempty"`
	Details     TaskEndDetails `bson:"status_details,omitempty" json:"status_details,omitempty"`
}

// EVG-22 Phase 2 (deprecate Status field in TaskEndRequest in agent/backend)
// TaskEndDetails is essentially TaskEndRequest.
type TaskEndDetails struct {
	Status      string `bson:"status,omitempty" json:"status,omitempty"`
	Type        string `bson:"type,omitempty" json:"type,omitempty"`
	Description string `bson:"desc,omitempty" json:"desc,omitempty"`
	TimedOut    bool   `bson:"timed_out,omitempty" json:"timed_out,omitempty"`
}

// TaskEndRequest contains data sent by the API server to the agent - in
// response to a TaskEndRequest.
type TaskEndResponse struct {
	TaskId     string `json:"task_id,omitempty"`
	TaskSecret string `json:"task_secret,omitempty"`
	Message    string `json:"message,omitempty"`
	RunNext    bool   `json:"run_next,omitempty"`
}

// ExpansionVars is a map of expansion variables for a project.
type ExpansionVars map[string]string
