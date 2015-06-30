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

type TaskEndDetail struct {
	Status      string `bson:"status,omitempty" json:"status,omitempty"`
	Type        string `bson:"type,omitempty" json:"type,omitempty"`
	Description string `bson:"desc,omitempty" json:"desc,omitempty"`
	TimedOut    bool   `bson:"timed_out,omitempty" json:"timed_out,omitempty"`
}

// Legacy agent
type TaskEndRequest struct {
	Status        string         `bson:"status,omitempty" json:"status,omitempty"`
	StatusDetails TaskEndDetails `bson:"status_details,omitempty" json:"status_details,omitempty"`
}
type TaskEndDetails struct {
	TimeoutStage string `bson:"timeout_stage,omitempty" json:"timeout_stage,omitempty"`
	TimedOut     bool   `bson:"timed_out,omitempty" json:"timed_out,omitempty"`
}

// TaskEndResponse contains data sent by the API server to the agent - in
// response to a TaskEndRequest.
type TaskEndResponse struct {
	TaskId     string `json:"task_id,omitempty"`
	TaskSecret string `json:"task_secret,omitempty"`
	Message    string `json:"message,omitempty"`
	RunNext    bool   `json:"run_next,omitempty"`
}

// ExpansionVars is a map of expansion variables for a project.
type ExpansionVars map[string]string
