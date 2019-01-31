package apimodels

import (
	"strconv"

	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	ProviderEC2                = "ec2"
	ScopeTask                  = "task"
	ScopeBuild                 = "build"
	DefaultSetupTimeoutSecs    = 600
	DefaultTeardownTimeoutSecs = 21600
)

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
	AMI             string      `mapstructure:"ami" json:"ami" plugin:"expand"`
	Distro          string      `mapstructure:"distro" json:"distro" plugin:"expand"`
	EBSDevices      []EbsDevice `mapstructure:"ebs_block_device" json:"ebs_block_device" plugin:"expand"`
	InstanceType    string      `mapstructure:"instance_type" json:"instance_type" plugin:"expand"`
	Region          string      `mapstructure:"region" json:"region" plugin:"expand"`
	SecurityGroups  []string    `mapstructure:"security_group_ids" json:"security_group_ids" plugin:"expand"`
	Spot            bool        `mapstructure:"spot" json:"spot"`
	Subnet          string      `mapstructure:"subnet_id" json:"subnet_id" plugin:"expand"`
	UserdataFile    string      `mapstructure:"userdata_file" json:"-" plugin:"expand"`
	UserdataCommand string      `json:"userdata_command" plugin:"expand"`

	// authentication settings
	AWSKeyID  string `mapstructure:"aws_access_key_id" json:"aws_access_key_id" plugin:"expand"`
	AWSSecret string `mapstructure:"aws_secret_access_key" json:"aws_secret_access_key" plugin:"expand"`
	KeyName   string `mapstructure:"key_name" json:"key_name" plugin:"expand"`

	// agent-controlled settings
	CloudProvider       string `mapstructure:"provider" json:"provider" plugin:"expand"`
	NumHosts            string `mapstructure:"num_hosts" json:"num_hosts" plugin:"expand"`
	Scope               string `mapstructure:"scope" json:"scope" plugin:"expand"`
	SetupTimeoutSecs    int    `mapstructure:"timeout_setup_secs" json:"timeout_setup_secs"`
	TeardownTimeoutSecs int    `mapstructure:"timeout_teardown_secs" json:"timeout_teardown_secs"`
	Retries             int    `mapstructure:"retries" json:"retries"`
}

type EbsDevice struct {
	DeviceName string `mapstructure:"device_name" json:"device_name"`
	IOPS       int    `mapstructure:"ebs_iops" json:"ebs_iops"`
	SizeGiB    int    `mapstructure:"ebs_size" json:"ebs_size"`
	SnapshotID string `mapstructure:"ebs_snapshot_id" json:"ebs_snapshot_id"`
}

func (ch *CreateHost) Validate() error {
	catcher := grip.NewBasicCatcher()
	if (ch.AMI != "" && ch.Distro != "") || (ch.AMI == "" && ch.Distro == "") {
		catcher.Add(errors.New("must set exactly one of ami or distro"))
	}
	if ch.AMI != "" {
		if ch.InstanceType == "" {
			catcher.Add(errors.New("instance_type must be set if ami is set"))
		}
		if len(ch.SecurityGroups) == 0 {
			catcher.Add(errors.New("must specify security_group_ids if ami is set"))
		}
		if ch.Subnet == "" {
			catcher.Add(errors.New("subnet_id must be set if ami is set"))
		}
	}

	if !(ch.AWSKeyID == "" && ch.AWSSecret == "" && ch.KeyName == "") &&
		!(ch.AWSKeyID != "" && ch.AWSSecret != "" && ch.KeyName != "") {
		catcher.Add(errors.New("aws_access_key_id, aws_secret_access_key, key_name must all be set or unset"))
	}

	if ch.NumHosts == "" {
		ch.NumHosts = "1"
	} else {
		numHosts, err := strconv.Atoi(ch.NumHosts)
		if err != nil {
			catcher.Add(errors.Errorf("problem parsing '%s' as an int", ch.NumHosts))
		}
		if numHosts > 10 || numHosts < 0 {
			catcher.Add(errors.New("num_hosts must be between 1 and 10"))
		} else if numHosts == 0 {
			ch.NumHosts = "1"
		}
	}
	if ch.CloudProvider == "" {
		ch.CloudProvider = ProviderEC2
	}
	if ch.CloudProvider != ProviderEC2 {
		catcher.Add(errors.New("only 'ec2' is supported for providers"))
	}
	if ch.Retries > 10 {
		catcher.Add(errors.New("retries must not be greater than 10"))
	}
	if ch.Retries < 1 {
		ch.Retries = 1
	}
	if ch.Scope == "" {
		ch.Scope = ScopeTask
	}
	if ch.Scope != ScopeTask && ch.Scope != ScopeBuild {
		catcher.Add(errors.New("scope must be build or task"))
	}
	if ch.SetupTimeoutSecs == 0 {
		ch.SetupTimeoutSecs = DefaultSetupTimeoutSecs
	}
	if ch.SetupTimeoutSecs < 60 || ch.SetupTimeoutSecs > 3600 {
		catcher.Add(errors.New("timeout_setup_secs must be between 60 and 3600"))
	}
	if ch.TeardownTimeoutSecs == 0 {
		ch.TeardownTimeoutSecs = DefaultTeardownTimeoutSecs
	}
	if ch.TeardownTimeoutSecs < 60 || ch.TeardownTimeoutSecs > 604800 {
		catcher.Add(errors.New("timeout_teardown_secs must be between 60 and 604800"))
	}
	return catcher.Resolve()
}

func (ch *CreateHost) Expand(exp *util.Expansions) error {
	return errors.Wrap(util.ExpandValues(ch, exp), "error expanding host.create")
}

type GeneratePollResponse struct {
	Finished bool `json:"finished"`
}
