package apimodels

import (
	"context"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v70/github"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	ScopeTask                  = "task"
	ScopeBuild                 = "build"
	DefaultSetupTimeoutSecs    = 600
	DefaultTeardownTimeoutSecs = 21600
	DefaultRetries             = 2
)

// TaskStartRequest holds information sent by the agent to the
// API server at the beginning of each task run.
type TaskStartRequest struct {
	TraceID     string   `json:"trace_id,omitempty"`
	DiskDevices []string `json:"disk_devices,omitempty"`
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

// TaskTestResultsInfo contains metadata related to test results persisted for
// a given task.
type TaskTestResultsInfo struct {
	Failed bool `json:"failed"`
}

// AttachTestResultsRequest contains information necessary to send in
// the request by the agent to attach test results.
type AttachTestResultsRequest struct {
	Info         testresult.TestResultsInfo      `json:"info"`
	Stats        testresult.TaskTestResultsStats `json:"stats"`
	FailedSample []string                        `json:"failed_sample"`
	CreatedAt    time.Time                       `json:"created_at"`
}

// TaskEndDetail contains data sent from the agent to the API server after each task run.
// This should be used to store data relating to what happened when the task ran
type TaskEndDetail struct {
	Status         string `bson:"status,omitempty" json:"status,omitempty"`
	Type           string `bson:"type,omitempty" json:"type,omitempty"`
	PostErrored    bool   `bson:"post_errored,omitempty" json:"post_errored,omitempty"`
	Description    string `bson:"desc,omitempty" json:"desc,omitempty"`
	FailingCommand string `bson:"failing_command,omitempty" json:"failing_command,omitempty"`
	// FailureMetadataTags are user metadata tags associated with the
	// command that caused the task to fail.
	FailureMetadataTags []string `bson:"failure_metadata_tags,omitempty" json:"failure_metadata_tags,omitempty"`
	// OtherFailingCommands contains information about commands that failed
	// while the task was running but did not cause the task to fail.
	OtherFailingCommands []FailingCommand    `bson:"other_failing_commands,omitempty" json:"other_failing_commands,omitempty"`
	TimedOut             bool                `bson:"timed_out,omitempty" json:"timed_out,omitempty"`
	TimeoutType          string              `bson:"timeout_type,omitempty" json:"timeout_type,omitempty"`
	TimeoutDuration      time.Duration       `bson:"timeout_duration,omitempty" json:"timeout_duration,omitempty" swaggertype:"primitive,integer"`
	OOMTracker           *OOMTrackerInfo     `bson:"oom_killer,omitempty" json:"oom_killer,omitempty"`
	TimeoutProcessInfo   *TimeoutProcessInfo `bson:"timeout_processes,omitempty" json:"timeout_processes"`
	Modules              ModuleCloneInfo     `bson:"modules,omitempty" json:"modules"`
	TraceID              string              `bson:"trace_id,omitempty" json:"trace_id,omitempty"`
	DiskDevices          []string            `bson:"disk_devices,omitempty" json:"disk_devices,omitempty"`
}

// FailingCommand represents a command that failed in a task.
type FailingCommand struct {
	FullDisplayName     string   `bson:"full_display_name,omitempty" json:"full_display_name,omitempty"`
	FailureMetadataTags []string `bson:"failure_metadata_tags,omitempty" json:"failure_metadata_tags,omitempty"`
}

type OOMTrackerInfo struct {
	Detected bool  `bson:"detected" json:"detected"`
	Pids     []int `bson:"pids" json:"pids"`
}

// TimeoutProcessInfo contains process information collected when a task times out
type TimeoutProcessInfo struct {
	CurrentCommand    string    `bson:"current_command,omitempty" json:"current_command,omitempty"`
	CurrentCommandPID int       `bson:"current_command_pid,omitempty" json:"current_command_pid,omitempty"`
	RunningPIDs       []int     `bson:"running_pids,omitempty" json:"running_pids,omitempty"`
	Timestamp         time.Time `bson:"timestamp,omitempty" json:"timestamp,omitempty"`
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
}

type AgentSetupData struct {
	SplunkServerURL        string                  `json:"splunk_server_url"`
	SplunkClientToken      string                  `json:"splunk_client_token"`
	SplunkChannel          string                  `json:"splunk_channel"`
	TaskOutput             evergreen.S3Credentials `json:"task_output"`
	TraceCollectorEndpoint string                  `json:"trace_collector_endpoint"`
	MaxExecTimeoutSecs     int                     `json:"max_exec_timeout_secs"`
}

// NextTaskResponse represents the response sent back when an agent asks for a next task
type NextTaskResponse struct {
	TaskId              string `json:"task_id,omitempty"`
	TaskExecution       int    `json:"task_execution,omitempty"`
	TaskSecret          string `json:"task_secret,omitempty"`
	TaskGroup           string `json:"task_group,omitempty"`
	Version             string `json:"version,omitempty"`
	Build               string `json:"build,omitempty"`
	ShouldExit          bool   `json:"should_exit,omitempty"`
	ShouldTeardownGroup bool   `json:"should_teardown_group,omitempty"`
	// EstimatedMaxIdleDuration is included in the response when there is no task to run.
	// It helps the host be smart about retries to request the next task.
	EstimatedMaxIdleDuration time.Duration `json:"estimated_max_idle_duration,omitempty"`
}

// EndTaskResponse is what is returned when the task ends
type EndTaskResponse struct {
	ShouldExit bool `json:"should_exit,omitempty"`
}

type CreateHost struct {
	// agent-controlled settings
	NumHosts            string `mapstructure:"num_hosts" json:"num_hosts" yaml:"num_hosts" plugin:"expand"`
	Scope               string `mapstructure:"scope" json:"scope" yaml:"scope" plugin:"expand"`
	SetupTimeoutSecs    int    `mapstructure:"timeout_setup_secs" json:"timeout_setup_secs" yaml:"timeout_setup_secs"`
	TeardownTimeoutSecs int    `mapstructure:"timeout_teardown_secs" json:"timeout_teardown_secs" yaml:"timeout_teardown_secs"`
	Retries             int    `mapstructure:"retries" json:"retries" yaml:"retries"`

	// EC2-related settings
	AMI            string               `mapstructure:"ami" json:"ami" yaml:"ami" plugin:"expand"`
	Distro         string               `mapstructure:"distro" json:"distro" yaml:"distro" plugin:"expand"`
	EBSDevices     []EbsDevice          `mapstructure:"ebs_block_device" json:"ebs_block_device" yaml:"ebs_block_device" plugin:"expand"`
	InstanceType   string               `mapstructure:"instance_type" json:"instance_type" yaml:"instance_type" plugin:"expand"`
	IPv6           bool                 `mapstructure:"ipv6" json:"ipv6" yaml:"ipv6"`
	Region         string               `mapstructure:"region" json:"region" yaml:"region" plugin:"expand"`
	SecurityGroups []string             `mapstructure:"security_group_ids" json:"security_group_ids" yaml:"security_group_ids" plugin:"expand"`
	Subnet         string               `mapstructure:"subnet_id" json:"subnet_id" yaml:"subnet_id" plugin:"expand"`
	Tenancy        evergreen.EC2Tenancy `mapstructure:"tenancy" json:"tenancy" yaml:"tenancy" plugin:"expand"`
	UserdataFile   string               `mapstructure:"userdata_file" json:"userdata_file" yaml:"userdata_file" plugin:"expand"`
	// UserdataCommand is the content of the userdata file. Users can't actually
	// set this directly, instead they pass in a userdata file.
	UserdataCommand string `json:"userdata_command" yaml:"userdata_command" plugin:"expand"`
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

// Token is a struct which wraps a GitHub generated token and associated permissions
type Token struct {
	Token       string                          `json:"token"`
	Permissions *github.InstallationPermissions `json:"permissions"`
}

// AssumeRoleRequest is the details of what role to assume.
type AssumeRoleRequest struct {
	// RoleARN is the Amazon Resource Name (ARN) of the role to assume.
	RoleARN string `json:"role_arn"`
	// Policy is an optional field that can be used to restrict the permissions.
	Policy *string `json:"policy"`
	// DurationSeconds is an optional field of the duration of the role session.
	// It defaults to 15 minutes.
	DurationSeconds *int32 `json:"duration_seconds"`
}

// Validate checks that the request has valid values.
func (ar *AssumeRoleRequest) Validate() error {
	catcher := grip.NewBasicCatcher()

	catcher.NewWhen(ar.RoleARN == "", "must specify role ARN")

	// 0 defaults to 15 minutes.
	catcher.NewWhen(utility.FromInt32Ptr(ar.DurationSeconds) < 0, "cannot specify a negative duration")

	return catcher.Resolve()
}

// AWSCredentials contains AWS credentials.
type AWSCredentials struct {
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	SessionToken    string `json:"session_token"`
	Expiration      string `json:"expiration"`

	// ExternalID is the external ID used to assume the role
	// if available.
	ExternalID string `json:"external_id"`
}

// S3CredentialsRequest contains the s3 bucket to access.
type S3CredentialsRequest struct {
	// Bucket is the name of the S3 bucket to access.
	Bucket string `json:"bucket"`
}

// Validate checks that the request has valid values.
func (ar *S3CredentialsRequest) Validate() error {
	catcher := grip.NewBasicCatcher()

	catcher.NewWhen(ar.Bucket == "", "must specify bucket name")

	return catcher.Resolve()
}

func (ted *TaskEndDetail) IsEmpty() bool {
	return ted == nil || ted.Status == ""
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
	if ch.Tenancy != "" {
		catcher.ErrorfWhen(!evergreen.IsValidEC2Tenancy(ch.Tenancy), "invalid tenancy '%s', allowed values are: %s", ch.Tenancy, evergreen.ValidEC2Tenancies)
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

	numHosts, err := strconv.Atoi(ch.NumHosts)
	if err != nil {
		return errors.Wrapf(err, "parsing num hosts specification '%s' as an int", ch.NumHosts)
	}
	if numHosts > 10 || numHosts < 0 {
		return errors.New("num hosts must be between 1 and 10")
	} else if numHosts == 0 {
		ch.NumHosts = "1"
	}
	return nil
}

func (ch *CreateHost) Validate(ctx context.Context) error {
	return ch.validateEC2()
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
	DisableShallowClone bool     `json:"disable_shallow_clone"`
	Mountpoints         []string `json:"mountpoints"`
	ExecUser            string   `json:"exec_user"`
}

// HostView includes a relevant subset of information the agent's own host.
type HostView struct {
	Hostname string `json:"hostname"`
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
	// RedactKeys contain patterns to match against expansion keys for redaction
	// in logs.
	RedactKeys []string `json:"redact_keys"`
	// InternalRedactions contain Evergreen-internal values that should not be
	// usable by the task but should still be redacted from logs.
	InternalRedactions map[string]string `json:"internal_redactions"`
}

// CheckRunOutput represents the output for a CheckRun.
type CheckRunOutput struct {
	Title            string                `json:"title,omitempty" plugin:"expand"`
	Summary          string                `json:"summary,omitempty" plugin:"expand"`
	Text             string                `json:"text,omitempty" plugin:"expand"`
	AnnotationsCount *int                  `json:"annotations_count,omitempty"`
	AnnotationsURL   string                `json:"annotations_url,omitempty" plugin:"expand"`
	Annotations      []*CheckRunAnnotation `json:"annotations,omitempty" plugin:"expand"`
}

// CheckRunAnnotation represents an annotation object for a CheckRun output.
type CheckRunAnnotation struct {
	Path            string `json:"path,omitempty" plugin:"expand"`
	StartLine       *int   `json:"start_line,omitempty" `
	EndLine         *int   `json:"end_line,omitempty" `
	StartColumn     *int   `json:"start_column,omitempty"`
	EndColumn       *int   `json:"end_column,omitempty" `
	AnnotationLevel string `json:"annotation_level,omitempty" plugin:"expand"`
	Message         string `json:"message,omitempty" plugin:"expand"`
	Title           string `json:"title,omitempty" plugin:"expand"`
	RawDetails      string `json:"raw_details,omitempty" plugin:"expand"`
}
