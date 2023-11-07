package apimodels

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	ProviderEC2                     = "ec2"
	ProviderDocker                  = "docker"
	ScopeTask                       = "task"
	ScopeBuild                      = "build"
	DefaultSetupTimeoutSecs         = 600
	DefaultTeardownTimeoutSecs      = 21600
	DefaultContainerWaitTimeoutSecs = 600
	DefaultPollFrequency            = 30
	DefaultRetries                  = 2
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

// CheckMergeRequest holds information sent by the agent to get a PR and check mergeability.
type CheckMergeRequest struct {
	PRNum int    `json:"pr_num"`
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
}

type PullRequestInfo struct {
	Mergeable      *bool  `json:"mergeable"`
	MergeCommitSHA string `json:"merge_commit_sha"`
}

// TaskTestResultsInfo contains metadata related to test results persisted for
// a given task.
type TaskTestResultsInfo struct {
	Service string `json:"service"`
	Failed  bool   `json:"failed"`
}

// TaskEndDetail contains data sent from the agent to the API server after each task run.
// This should be used to store data relating to what happened when the task ran
type TaskEndDetail struct {
	Status          string          `bson:"status,omitempty" json:"status,omitempty"`
	Type            string          `bson:"type,omitempty" json:"type,omitempty"`
	Description     string          `bson:"desc,omitempty" json:"desc,omitempty"`
	TimedOut        bool            `bson:"timed_out,omitempty" json:"timed_out,omitempty"`
	TimeoutType     string          `bson:"timeout_type,omitempty" json:"timeout_type,omitempty"`
	TimeoutDuration time.Duration   `bson:"timeout_duration,omitempty" json:"timeout_duration,omitempty" swaggertype:"primitive,integer"`
	OOMTracker      *OOMTrackerInfo `bson:"oom_killer,omitempty" json:"oom_killer,omitempty"`
	Modules         ModuleCloneInfo `bson:"modules,omitempty" json:"modules,omitempty"`
	TraceID         string          `bson:"trace_id,omitempty" json:"trace_id,omitempty"`
	DiskDevices     []string        `bson:"data_disk,omitempty" json:"data_disk,omitempty"`
}

type OOMTrackerInfo struct {
	Detected bool  `bson:"detected" json:"detected"`
	Pids     []int `bson:"pids" json:"pids"`
}

type LogInfo struct {
	Command string `bson:"command" json:"command"`
	URL     string `bson:"url" json:"url"`
}

type DisableInfo struct {
	Reason string `bson:"reason" json:"reason"`
}

type ModuleCloneInfo struct {
	Prefixes map[string]string `bson:"prefixes,omitempty" json:"prefixes,omitempty"`
}

type TaskEndDetails struct {
	TimeoutStage string `bson:"timeout_stage,omitempty" json:"timeout_stage,omitempty"`
	TimedOut     bool   `bson:"timed_out,omitempty" json:"timed_out,omitempty"`
}

type GetNextTaskDetails struct {
	TaskGroup     string `json:"task_group"`
	AgentRevision string `json:"agent_revision"`
	// EC2InstanceID is the ID of the instance running the agent if the agent is
	// running on an EC2 host. For non-EC2 hosts, this will not be populated.
	EC2InstanceID string `json:"instance_id,omitempty"`
}

type AgentSetupData struct {
	SplunkServerURL        string                  `json:"splunk_server_url"`
	SplunkClientToken      string                  `json:"splunk_client_token"`
	SplunkChannel          string                  `json:"splunk_channel"`
	TaskSync               evergreen.S3Credentials `json:"task_sync"`
	EC2Keys                []evergreen.EC2Key      `json:"ec2_keys"`
	TraceCollectorEndpoint string                  `json:"trace_collector_endpoint"`
}

// NextTaskResponse represents the response sent back when an agent asks for a next task
type NextTaskResponse struct {
	TaskId              string `json:"task_id,omitempty"`
	TaskSecret          string `json:"task_secret,omitempty"`
	TaskGroup           string `json:"task_group,omitempty"`
	Version             string `json:"version,omitempty"`
	Build               string `json:"build,omitempty"`
	ShouldExit          bool   `json:"should_exit,omitempty"`
	ShouldTeardownGroup bool   `json:"should_teardown_group,omitempty"`
}

// EndTaskResponse is what is returned when the task ends
type EndTaskResponse struct {
	ShouldExit bool `json:"should_exit,omitempty"`
}

type CreateHost struct {
	// agent-controlled settings
	CloudProvider       string `mapstructure:"provider" json:"provider" yaml:"provider" plugin:"expand"`
	NumHosts            string `mapstructure:"num_hosts" json:"num_hosts" yaml:"num_hosts" plugin:"expand"`
	Scope               string `mapstructure:"scope" json:"scope" yaml:"scope" plugin:"expand"`
	SetupTimeoutSecs    int    `mapstructure:"timeout_setup_secs" json:"timeout_setup_secs" yaml:"timeout_setup_secs"`
	TeardownTimeoutSecs int    `mapstructure:"timeout_teardown_secs" json:"timeout_teardown_secs" yaml:"timeout_teardown_secs"`
	Retries             int    `mapstructure:"retries" json:"retries" yaml:"retries"`

	// EC2-related settings
	AMI             string      `mapstructure:"ami" json:"ami" yaml:"ami" plugin:"expand"`
	Distro          string      `mapstructure:"distro" json:"distro" yaml:"distro" plugin:"expand"`
	EBSDevices      []EbsDevice `mapstructure:"ebs_block_device" json:"ebs_block_device" yaml:"ebs_block_device" plugin:"expand"`
	InstanceType    string      `mapstructure:"instance_type" json:"instance_type" yaml:"instance_type" plugin:"expand"`
	IPv6            bool        `mapstructure:"ipv6" json:"ipv6" yaml:"ipv6"`
	Region          string      `mapstructure:"region" json:"region" yaml:"region" plugin:"expand"`
	SecurityGroups  []string    `mapstructure:"security_group_ids" json:"security_group_ids" yaml:"security_group_ids" plugin:"expand"`
	Subnet          string      `mapstructure:"subnet_id" json:"subnet_id" yaml:"subnet_id" plugin:"expand"`
	UserdataFile    string      `mapstructure:"userdata_file" json:"userdata_file" yaml:"userdata_file" plugin:"expand"`
	UserdataCommand string      `json:"userdata_command" yaml:"userdata_command" plugin:"expand"`
	AWSKeyID        string      `mapstructure:"aws_access_key_id" json:"aws_access_key_id" yaml:"aws_access_key_id" plugin:"expand"`
	AWSSecret       string      `mapstructure:"aws_secret_access_key" json:"aws_secret_access_key" yaml:"aws_secret_access_key" plugin:"expand"`
	KeyName         string      `mapstructure:"key_name" json:"key_name" yaml:"key_name" plugin:"expand"`

	// docker-related settings
	Image                    string           `mapstructure:"image" json:"image" yaml:"image" plugin:"expand"`
	Command                  string           `mapstructure:"command" json:"command" yaml:"command" plugin:"expand"`
	PublishPorts             bool             `mapstructure:"publish_ports" json:"publish_ports" yaml:"publish_ports"`
	Registry                 RegistrySettings `mapstructure:"registry" json:"registry" yaml:"registry" plugin:"expand"`
	Background               bool             `mapstructure:"background" json:"background" yaml:"background"` // default is true
	ContainerWaitTimeoutSecs int              `mapstructure:"container_wait_timeout_secs" json:"container_wait_timeout_secs" yaml:"container_wait_timeout_secs"`
	PollFrequency            int              `mapstructure:"poll_frequency_secs" json:"poll_frequency_secs" yaml:"poll_frequency_secs"` // poll frequency in seconds
	StdinFile                string           `mapstructure:"stdin_file_name" json:"stdin_file_name" yaml:"stdin_file_name" plugin:"expand"`
	// StdinFileContents is the full file content of the StdinFile on the host,
	// which is then sent to the app server.
	StdinFileContents []byte            `mapstructure:"-" json:"stdin_file_contents" yaml:"-"`
	StdoutFile        string            `mapstructure:"stdout_file_name" json:"stdout_file_name" yaml:"stdout_file_name" plugin:"expand"`
	StderrFile        string            `mapstructure:"stderr_file_name" json:"stderr_file_name" yaml:"stderr_file_name" plugin:"expand"`
	EnvironmentVars   map[string]string `mapstructure:"environment_vars" json:"environment_vars" yaml:"environment_vars" plugin:"expand"`
	ExtraHosts        []string          `mapstructure:"extra_hosts" json:"extra_hosts" yaml:"extra_hosts" plugin:"expand"`
}

type EbsDevice struct {
	DeviceName string `mapstructure:"device_name" json:"device_name" yaml:"device_name"`
	IOPS       int    `mapstructure:"ebs_iops" json:"ebs_iops" yaml:"ebs_iops"`
	Throughput int    `mapstructure:"ebs_throughput" json:"ebs_throughput" yaml:"ebs_throughput"`
	SizeGiB    int    `mapstructure:"ebs_size" json:"ebs_size" yaml:"ebs_size"`
	SnapshotID string `mapstructure:"ebs_snapshot_id" json:"ebs_snapshot_id" yaml:"ebs_snapshot_id"`
}

type RegistrySettings struct {
	Name     string `mapstructure:"registry_name" json:"registry_name" yaml:"registry_name"`
	Username string `mapstructure:"registry_username" json:"registry_username" yaml:"registry_username"`
	Password string `mapstructure:"registry_password" json:"registry_password" yaml:"registry_password"`
}

type InstallationToken struct {
	Token string `json:"token"`
}

func (ted *TaskEndDetail) IsEmpty() bool {
	return ted == nil || ted.Status == ""
}

func (ch *CreateHost) validateDocker(ctx context.Context) error {
	catcher := grip.NewBasicCatcher()

	catcher.Add(ch.setNumHosts())
	catcher.Add(ch.validateAgentOptions())

	if ch.Image == "" {
		catcher.New("Docker image must be set")
	}
	if ch.Distro == "" {
		settings, err := evergreen.GetConfig(ctx)
		if err != nil {
			catcher.New("error getting config to set default distro")
		} else {
			ch.Distro = settings.Providers.Docker.DefaultDistro
		}

	}
	if ch.ContainerWaitTimeoutSecs <= 0 {
		ch.ContainerWaitTimeoutSecs = DefaultContainerWaitTimeoutSecs
	} else if ch.ContainerWaitTimeoutSecs >= 3600 || ch.ContainerWaitTimeoutSecs <= 10 {
		catcher.New("container wait timeout (seconds) must be between 10 and 3600 seconds")
	}

	if ch.PollFrequency <= 0 {
		ch.PollFrequency = DefaultPollFrequency
	} else if ch.PollFrequency > 60 {
		catcher.New("poll frequency must not be greater than 60 seconds")
	}

	if (ch.Registry.Username != "" && ch.Registry.Password == "") ||
		(ch.Registry.Username == "" && ch.Registry.Password != "") {
		catcher.New("username and password must both be set or unset")
	}

	for _, h := range ch.ExtraHosts {
		catcher.ErrorfWhen(len(strings.Split(h, ":")) != 2, "extra host '%s' must be of the form hostname:IP", h)
	}

	return catcher.Resolve()
}

func (ch *CreateHost) validateEC2() error {
	catcher := grip.NewBasicCatcher()

	catcher.Add(ch.setNumHosts())
	catcher.Add(ch.validateAgentOptions())
	if ch.Region == "" {
		ch.Region = evergreen.DefaultEC2Region
	}

	if (ch.AMI != "" && ch.Distro != "") || (ch.AMI == "" && ch.Distro == "") {
		catcher.New("must set exactly one of AMI or distro")
	}
	if ch.AMI != "" {
		if ch.InstanceType == "" {
			catcher.New("instance type must be set if AMI is set")
		}
		if len(ch.SecurityGroups) == 0 {
			catcher.New("must specify security group IDs if AMI is set")
		}
		if ch.Subnet == "" {
			catcher.New("subnet ID must be set if AMI is set")
		}
	}

	if !(ch.AWSKeyID == "" && ch.AWSSecret == "" && ch.KeyName == "") &&
		!(ch.AWSKeyID != "" && ch.AWSSecret != "" && ch.KeyName != "") {
		catcher.New("AWS access key ID, AWS secret access key, and key name must all be set or unset")
	}

	return catcher.Resolve()
}

func (ch *CreateHost) validateAgentOptions() error {
	catcher := grip.NewBasicCatcher()
	if ch.Retries > 10 {
		catcher.New("retries must not be greater than 10")
	}
	if ch.Retries <= 0 {
		ch.Retries = DefaultRetries
	}
	if ch.Scope == "" {
		ch.Scope = ScopeTask
	}
	if ch.Scope != ScopeTask && ch.Scope != ScopeBuild {
		catcher.New("scope must be build or task")
	}
	if ch.SetupTimeoutSecs == 0 {
		ch.SetupTimeoutSecs = DefaultSetupTimeoutSecs
	}
	if ch.SetupTimeoutSecs < 60 || ch.SetupTimeoutSecs > 3600 {
		catcher.New("timeout setup (seconds) must be between 60 and 3600")
	}
	if ch.TeardownTimeoutSecs == 0 {
		ch.TeardownTimeoutSecs = DefaultTeardownTimeoutSecs
	}
	if ch.TeardownTimeoutSecs < 60 || ch.TeardownTimeoutSecs > 604800 {
		catcher.New("timeout teardown (seconds) must be between 60 and 604800")
	}
	return catcher.Resolve()
}

func (ch *CreateHost) setNumHosts() error {
	if ch.NumHosts == "" {
		ch.NumHosts = "1"
	}
	if ch.CloudProvider == ProviderDocker && ch.NumHosts != "1" {
		return errors.Errorf("num hosts cannot be greater than 1 for cloud provider '%s'", ProviderDocker)
	} else {
		numHosts, err := strconv.Atoi(ch.NumHosts)
		if err != nil {
			return errors.Wrapf(err, "parsing num hosts specification '%s' as an int", ch.NumHosts)
		}
		if numHosts > 10 || numHosts < 0 {
			return errors.New("num hosts must be between 1 and 10")
		} else if numHosts == 0 {
			ch.NumHosts = "1"
		}
	}
	return nil
}

func (ch *CreateHost) Validate(ctx context.Context) error {
	if ch.CloudProvider == ProviderEC2 || ch.CloudProvider == "" { //default
		ch.CloudProvider = ProviderEC2
		return ch.validateEC2()
	}

	if ch.CloudProvider == ProviderDocker {
		return ch.validateDocker(ctx)
	}

	return errors.Errorf("cloud provider must be either '%s' or '%s'", ProviderEC2, ProviderDocker)
}

func (ch *CreateHost) Expand(exp *util.Expansions) error {
	return errors.Wrap(util.ExpandValues(ch, exp), "error expanding host.create")
}

type GeneratePollResponse struct {
	Finished bool   `json:"finished"`
	Error    string `json:"error"`
}

// DistroView represents the view of data that the agent uses from the distro
// it is running on.
type DistroView struct {
	CloneMethod         string   `json:"clone_method"`
	DisableShallowClone bool     `json:"disable_shallow_clone"`
	Mountpoints         []string `json:"mountpoints"`
}

// ExpansionsAndVars represents expansions, project variables, and parameters
// used when running a task.
type ExpansionsAndVars struct {
	// Expansions contain the expansions for a task.
	Expansions util.Expansions `json:"expansions"`
	// Parameters contain the parameters for a task.
	Parameters map[string]string `json:"parameters"`
	// Vars contain the project variables and parameters.
	Vars map[string]string `json:"vars"`
	// PrivateVars contain the project private variables.
	PrivateVars map[string]bool `json:"private_vars"`
}
