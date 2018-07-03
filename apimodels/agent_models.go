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

// TaskEndDetail contains data sent from the agent to the
// API server after each task run.
type TaskEndDetail struct {
	Status      string `bson:"status,omitempty" json:"status,omitempty"`
	Type        string `bson:"type,omitempty" json:"type,omitempty"`
	Description string `bson:"desc,omitempty" json:"desc,omitempty"`
	TimedOut    bool   `bson:"timed_out,omitempty" json:"timed_out,omitempty"`
}

type TaskEndDetails struct {
	TimeoutStage string `bson:"timeout_stage,omitempty" json:"timeout_stage,omitempty"`
	TimedOut     bool   `bson:"timed_out,omitempty" json:"timed_out,omitempty"`
}

type GetNextTaskDetails struct {
	TaskGroup string `json:"task_group"`
}

// ExpansionVars is a map of expansion variables for a project.
type ExpansionVars struct {
	Vars        map[string]string `json:"vars"`
	PrivateVars map[string]bool   `json:"private_vars"`
}

// NextTaskResponse represents the response sent back when an agent asks for a next task
type NextTaskResponse struct {
	TaskId     string `json:"task_id,omitempty"`
	TaskSecret string `json:"task_secret,omitempty"`
	TaskGroup  string `json:"task_group,omitempty"`
	Version    string `json:"version,omitempty"`
	Build      string `json:"build,omitempty"`
	// ShouldExit indicates that something has gone wrong, so the agent
	// should exit immediately when it receives this message. ShouldExit can
	// interrupt a task group.
	ShouldExit bool `json:"should_exit,omitempty"`
	// NewAgent indicates a new agent available, so the agent should exit
	// gracefully. Practically speaking, this means that if the agent is
	// currently in a task group, it should only exit when it has finished
	// the task group.
	NewAgent bool `json:"new_agent,omitempty"`
}

// EndTaskResponse is what is returned when the task ends
type EndTaskResponse struct {
	ShouldExit bool `json:"should_exit,omitempty"`
}

type CreateHost struct {
	// EC2-related settings
	AMI            string      `mapstructure:"ami" json:"ami"`
	Distro         string      `mapstructure:"distro" json:"distro"`
	EBSDevices     []EbsDevice `mapstructure:"ebs_block_device" json:"ebs_block_device"`
	InstanceType   string      `mapstructure:"instance_type" json:"instance_type"`
	Region         string      `mapstructure:"region" json:"region"`
	SecurityGroups []string    `mapstructure:"security_group_ids" json:"security_group_ids"`
	Spot           bool        `mapstructure:"spot" json:"spot"`
	Subnet         string      `mapstructure:"subnet_id" json:"subnet_id"`
	UserdataFile   string      `mapstructure:"userdata_file" json:"userdata_file"`
	VPC            string      `mapstructure:"vpc_id" json:"vpc_id"`

	// authentication settings
	AWSKeyID  string `mapstructure:"aws_access_key_id" json:"aws_access_key_id"`
	AWSSecret string `mapstructure:"aws_secret_access_key" json:"aws_secret_access_key"`
	KeyName   string `mapstructure:"key_name" json:"key_name"`

	// agent-controlled settings
	CloudProvider       string `mapstructure:"provider" json:"provider"`
	NumHosts            int    `mapstructure:"num_hosts" json:"num_hosts"`
	Scope               string `mapstructure:"scope" json:"scope"`
	SetupTimeoutSecs    int    `mapstructure:"timeout_setup_secs" json:"timeout_setup_secs"`
	TeardownTimeoutSecs int    `mapstructure:"timeout_teardown_secs" json:"timeout_teardown_secs"`
}

type EbsDevice struct {
	IOPS       int    `mapstructure:"ebs_iops" json:"ebs_iops"`
	SizeGB     int    `mapstructure:"ebs_size" json:"ebs_size"`
	SnapshotID string `mapstructure:"ebs_snapshot_id" json:"ebs_snapshot_id"`
}
