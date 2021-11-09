package shrub

import (
	"encoding/json"
	"errors"
	"fmt"
)

////////////////////////////////////////////////////////////////////////
//
// Specific Command Implementations

func exportCmd(cmd Command) map[string]interface{} {
	if err := cmd.Validate(); err != nil {
		panic(err)
	}

	jsonStruct, err := json.Marshal(cmd)
	if err == nil {
		out := map[string]interface{}{}
		if err = json.Unmarshal(jsonStruct, &out); err == nil {
			return out
		}
	}

	panic(err)
}

type CmdExec struct {
	Binary                        string            `json:"binary,omitempty" yaml:"binary,omitempty"`
	Args                          []string          `json:"args,omitempty" yaml:"args,omitempty"`
	KeepEmptyArgs                 bool              `json:"keep_empty_args,omitempty" yaml:"keep_empty_args,omitempty"`
	Command                       string            `json:"command,omitempty" yaml:"command,omitempty"`
	ContinueOnError               bool              `json:"continue_on_err,omitempty" yaml:"continue_on_err,omitempty"`
	Background                    bool              `json:"background,omitempty" yaml:"background,omitempty"`
	Silent                        bool              `json:"silent,omitempty" yaml:"silent,omitempty"`
	RedirectStandardErrorToOutput bool              `json:"redirect_standard_error_to_output,omitempty" yaml:"redirect_standard_error_to_output,omitempty"`
	IgnoreStandardError           bool              `json:"ignore_standard_error,omitempty" yaml:"ignore_standard_error,omitempty"`
	IgnoreStandardOutput          bool              `json:"ignore_standard_out,omitempty" yaml:"ignore_standard_out,omitempty"`
	Path                          []string          `json:"add_to_path,omitempty" yaml:"add_to_path,omitempty"`
	Env                           map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	AddExpansionsToEnv            bool              `json:"add_expansions_to_env,omitempty" yaml:"add_expansions_to_env,omitempty"`
	IncludeExpansionsInEnv        []string          `json:"include_expansions_in_env,omitempty" yaml:"include_expansions_in_env,omitempty"`
	SystemLog                     bool              `json:"system_log,omitempty" yaml:"system_log,omitempty"`
	WorkingDirectory              string            `json:"working_dir,omitempty" yaml:"working_dir,omitempty"`
}

func (c CmdExec) Name() string    { return "subprocess.exec" }
func (c CmdExec) Validate() error { return nil }
func (c CmdExec) Resolve() *CommandDefinition {
	return &CommandDefinition{
		CommandName: c.Name(),
		Params:      exportCmd(c),
	}
}
func subprocessExecFactory() Command { return CmdExec{} }

type CmdExecShell struct {
	Script                        string            `json:"script" yaml:"script"`
	Shell                         string            `json:"shell,omitempty" yaml:"shell,omitempty"`
	Env                           map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	AddExpansionsToEnv            map[string]string `json:"add_expansions_to_env,omitempty" yaml:"add_expansions_to_env,omitempty"`
	IncludeExpansionsInEnv        map[string]string `json:"include_expansions_in_env,omitempty" yaml:"include_expansions_inenv,omitempty"`
	ContinueOnError               bool              `json:"continue_on_err,omitempty" yaml:"continue_on_err,omitempty"`
	Background                    bool              `json:"background,omitempty" yaml:"background,omitempty"`
	Silent                        bool              `json:"silent,omitempty" yaml:"silent,omitempty"`
	RedirectStandardErrorToOutput bool              `json:"redirect_standard_error_to_output,omitempty" yaml:"redirect_standard_error_to_output,omitempty"`
	IgnoreStandardError           bool              `json:"ignore_standard_error" yaml:"ignore_standard_error"`
	IgnoreStandardOutput          bool              `json:"ignore_standard_out" yaml:"ignore_standard_out"`
	SystemLog                     bool              `json:"system_log,omitempty" yaml:"system_log,omitempty"`
	WorkingDirectory              string            `json:"working_dir,omitempty" yaml:"working_dir,omitempty"`
}

func (c CmdExecShell) Name() string    { return "shell.exec" }
func (c CmdExecShell) Validate() error { return nil }
func (c CmdExecShell) Resolve() *CommandDefinition {
	return &CommandDefinition{
		CommandName: c.Name(),
		Params:      exportCmd(c),
	}
}
func shellExecFactory() Command { return CmdExecShell{} }

type ScriptingTestOptions struct {
	Name        string   `json:"name,omitempty" yaml:"name,omitempty"`
	Args        []string `json:"args,omitempty" yaml:"args,omitempty"`
	Pattern     string   `json:"pattern,omitempty" yaml:"pattern,omitempty"`
	TimeoutSecs int      `json:"timeout_secs,omitempty" yaml:"timeout_secs,omitempty"`
	Count       int      `json:"count,omitempty" yaml:"count,omitempty"`
}

type CmdSubprocessScripting struct {
	Harness                       string                `json:"harness" yaml:"harness"`
	Command                       string                `json:"command,omitempty" yaml:"command,omitempty"`
	Args                          []string              `json:"args,omitempty" yaml:"args,omitempty"`
	TestDir                       string                `json:"test_dir,omitempty" yaml:"test_dir,omitempty"`
	TestOptions                   *ScriptingTestOptions `json:"test_options,omitempty" yaml:"test_options,omitempty"`
	Report                        bool                  `json:"report,omitempty" yaml:"report,omitempty"`
	Script                        string                `json:"script,omitempty" yaml:"script,omitempty"`
	ContinueOnError               bool                  `json:"continue_on_err,omitempty" yaml:"continue_on_err,omitempty"`
	Silent                        bool                  `json:"silent,omitempty" yaml:"silent,omitempty"`
	Path                          []string              `json:"add_to_path,omitempty" yaml:"path,omitempty"`
	Env                           map[string]string     `json:"env,omitempty" yaml:"env,omitempty"`
	AddExpansionsToEnv            bool                  `json:"add_expansions_to_env,omitempty" yaml:"add_expansions_to_env,omitempty"`
	IncludeExpansionsInEnv        []string              `json:"include_expansions_in_env,omitempty" yaml:"include_expansions_in_env,omitempty"`
	RedirectStandardErrorToOutput bool                  `json:"redirect_standard_error_to_output,omitempty" yaml:"redirect_standard_error_to_output,omitempty"`
	IgnoreStandardOutput          bool                  `json:"ignore_standard_out,omitempty" yaml:"ignore_standard_out,omitempty"`
	IgnoreStandardError           bool                  `json:"ignore_standard_error,omitempty" yaml:"ignore_standard_error,omitempty"`
	SystemLog                     bool                  `json:"system_log,omitempty" yaml:"system_log,omitempty"`
	WorkingDir                    string                `json:"working_dir,omitempty" yaml:"working_dir,omitempty"`
	CacheDurationSeconds          int                   `json:"cache_duration_secs,omitempty" yaml:"cache_duration_secs,omitempty"`
	CleanupHarness                bool                  `json:"cleanup_harness,omitempty" yaml:"cleanup_harness,omitempty"`
	LockFile                      string                `json:"lock_file,omitempty" yaml:"lock_file,omitempty"`
	Packages                      []string              `json:"packages,omitempty" yaml:"packages,omitempty"`
	HarnessPath                   string                `json:"harness_path,omitempty" yaml:"harness_path,omitempty"`
	HostPath                      string                `json:"host_path,omitempty" yaml:"host_path,omitempty"`
}

func (c CmdSubprocessScripting) Name() string { return "subprocess.scripting" }

func (c CmdSubprocessScripting) Validate() error {
	return nil
}

func (c CmdSubprocessScripting) Resolve() *CommandDefinition {
	return &CommandDefinition{
		CommandName: c.Name(),
		Params:      exportCmd(c),
	}
}

func subprocessScriptingFactory() Command { return CmdSubprocessScripting{} }

type CmdS3Put struct {
	AWSKey                        string   `json:"aws_key" yaml:"aws_key"`
	AWSSecret                     string   `json:"aws_secret" yaml:"aws_secret"`
	Bucket                        string   `json:"bucket" yaml:"bucket"`
	Region                        string   `json:"region,omitempty" yaml:"region,omitempty"`
	ContentType                   string   `json:"content_type" yaml:"content_type"`
	Permissions                   string   `json:"permissions,omitempty" yaml:"permissions,omitempty"`
	Visibility                    string   `json:"visibility,omitempty" yaml:"visibility,omitempty"`
	LocalFile                     string   `json:"local_file,omitempty" yaml:"local_file,omitempty"`
	LocalFilesIncludeFilter       []string `json:"local_files_include_filter,omitempty" yaml:"local_files_include_filter,omitempty"`
	LocalFilesIncludeFilterPrefix string   `json:"local_files_include_filter_prefix,omitempty" yaml:"local_files_include_filter_prefix,omitempty"`
	RemoteFile                    string   `json:"remote_file" yaml:"remote_file"`
	ResourceDisplayName           string   `json:"display_name,omitempty" yaml:"display_name,omitempty"`
	BuildVariants                 []string `json:"build_variants,omitempty" yaml:"build_variants,omitempty"`
	Optional                      bool     `json:"optional,omitempty" yaml:"optional,omitempty"`
}

func (c CmdS3Put) Name() string { return "s3.put" }
func (c CmdS3Put) Validate() error {
	switch {
	case c.AWSKey == "", c.AWSSecret == "":
		return errors.New("must specify aws credentials")
	case c.LocalFile == "" && len(c.LocalFilesIncludeFilter) == 0:
		return errors.New("must specify a local file to upload")
	default:
		return nil
	}
}
func (c CmdS3Put) Resolve() *CommandDefinition {
	return &CommandDefinition{
		CommandName: c.Name(),
		Params:      exportCmd(c),
	}
}
func s3PutFactory() Command { return CmdS3Put{} }

type CmdS3Get struct {
	AWSKey        string   `json:"aws_key" yaml:"aws_key"`
	AWSSecret     string   `json:"aws_secret" yaml:"aws_secret"`
	Region        string   `json:"region,omitempty" yaml:"region,omitempty"`
	RemoteFile    string   `json:"remote_file" yaml:"remote_file"`
	Bucket        string   `json:"bucket" yaml:"bucket"`
	LocalFile     string   `json:"local_file,omitempty" yaml:"local_file,omitempty"`
	ExtractTo     string   `json:"extract_to,omitempty" yaml:"extract_to,omitempty"`
	BuildVariants []string `json:"build_variants,omitempty" yaml:"build_variants,omitempty"`
}

func (c CmdS3Get) Name() string    { return "s3.get" }
func (c CmdS3Get) Validate() error { return nil }
func (c CmdS3Get) Resolve() *CommandDefinition {
	return &CommandDefinition{
		CommandName: c.Name(),
		Params:      exportCmd(c),
	}
}
func s3GetFactory() Command { return CmdS3Get{} }

type CmdS3Copy struct {
	AWSKey    string `json:"aws_key" yaml:"aws_key"`
	AWSSecret string `json:"aws_secret" yaml:"aws_secret"`
	Files     []struct {
		Source struct {
			Bucket string `json:"bucket" yaml:"bucket"`
			Path   string `json:"path" yaml:"path"`
			Region string `json:"region,omitempty" yaml:"region,omitempty"`
		} `json:"source" yaml:"source"`
		Destination struct {
			Bucket string `json:"bucket" yaml:"bucket"`
			Path   string `json:"path" yaml:"path"`
			Region string `json:"region,omitempty" yaml:"region,omitempty"`
		} `json:"destination" yaml:"destination"`
		DisplayName   string   `json:"display_name,omitempty" yaml:"display_name,omitempty"`
		Permissions   string   `json:"permissions,omitempty" yaml:"permissions,omitempty"`
		BuildVariants []string `json:"build_variants,omitempty" yaml:"build_variants,omitempty"`
		Optional      bool     `json:"optional,omitempty" yaml:"optional,omitempty"`
	} `json:"s3_copy_files" yaml:"s3_copy_files"`
}

func (c CmdS3Copy) Name() string    { return "s3Copy.copy" }
func (c CmdS3Copy) Validate() error { return nil }
func (c CmdS3Copy) Resolve() *CommandDefinition {
	return &CommandDefinition{
		CommandName: c.Name(),
		Params:      exportCmd(c),
	}
}
func s3CopyFactory() Command { return CmdS3Copy{} }

type CmdS3Push struct {
	ExcludeFilter string `json:"exclude,omitempty" yaml:"exclude,omitempty"`
	MaxRetries    int    `json:"max_retries,omitempty" yaml:"max_retries,omitempty"`
}

func (c CmdS3Push) Name() string    { return "s3.push" }
func (c CmdS3Push) Validate() error { return nil }
func (c CmdS3Push) Resolve() *CommandDefinition {
	return &CommandDefinition{
		CommandName: c.Name(),
		Params:      exportCmd(c),
	}
}
func s3PushFactory() Command { return CmdS3Push{} }

type CmdS3Pull struct {
	Task             string `json:"task" yaml:"task"`
	ExcludeFilter    string `json:"exclude,omitempty" yaml:"exclude,omitempty"`
	MaxRetries       int    `json:"max_retries,omitempty" yaml:"max_retries,omitempty"`
	WorkingDir       string `json:"working_dir,omitempty" yaml:"working_dir,omitempty"`
	DeleteOnSync     bool   `json:"delete_on_sync,omitempty" yaml:"delete_on_sync,omitempty"`
	FromBuildVariant string `json:"from_build_variant,omitempty" yaml:"from_build_variant,omitempty"`
}

func (c CmdS3Pull) Name() string    { return "s3.pull" }
func (c CmdS3Pull) Validate() error { return nil }
func (c CmdS3Pull) Resolve() *CommandDefinition {
	return &CommandDefinition{
		CommandName: c.Name(),
		Params:      exportCmd(c),
	}
}
func s3PullFactory() Command { return CmdS3Pull{} }

type CmdGetProject struct {
	Directory         string            `json:"directory" yaml:"directory"`
	Token             string            `json:"token,omitempty" yaml:"token,omitempty"`
	Revisions         map[string]string `json:"revisions,omitempty" yaml:"revisions,omitempty"`
	ShallowClone      bool              `json:"shallow_clone,omitempty" yaml:"shallow_clone,omitempty"`
	RecurseSubmodules bool              `json:"recurse_submodules,omitempty" yaml:"recurse_submodules,omitempty"`
	CommitterName     string            `json:"committer_name,omitempty" yaml:"committer_name,omitempty"`
	CommitterEmail    string            `json:"committer_email,omitempty" yaml:"committer_email,omitempty"`
}

func (c CmdGetProject) Name() string    { return "git.get_project" }
func (c CmdGetProject) Validate() error { return nil }
func (c CmdGetProject) Resolve() *CommandDefinition {
	return &CommandDefinition{
		CommandName: c.Name(),
		Params:      exportCmd(c),
	}
}
func getProjectFactory() Command { return CmdGetProject{} }

type CmdResultsJSON struct {
	File string `json:"file_location" yaml:"file_location"`
}

func (c CmdResultsJSON) Name() string    { return "attach.results" }
func (c CmdResultsJSON) Validate() error { return nil }
func (c CmdResultsJSON) Resolve() *CommandDefinition {
	return &CommandDefinition{
		CommandName: c.Name(),
		Params:      exportCmd(c),
	}
}
func jsonResultsFactory() Command { return CmdResultsJSON{} }

type CmdResultsXunit struct {
	File  string   `json:"file,omitempty" yaml:"file,omitempty"`
	Files []string `json:"files,omitempty" yaml:"files,omitempty"`
}

func (c CmdResultsXunit) Name() string    { return "attach.xunit_results" }
func (c CmdResultsXunit) Validate() error { return nil }
func (c CmdResultsXunit) Resolve() *CommandDefinition {
	return &CommandDefinition{
		CommandName: c.Name(),
		Params:      exportCmd(c),
	}
}
func xunitResultsFactory() Command { return CmdResultsXunit{} }

type CmdResultsGoTest struct {
	JSONFormat   bool     `json:"-" yaml:"-"`
	LegacyFormat bool     `json:"-" yaml:"-"`
	Files        []string `json:"files" yaml:"files"`
}

func (c CmdResultsGoTest) Name() string {
	if c.LegacyFormat {
		return "gotest.parse_files"
	}
	return "gotest.parse_json"
}
func (c CmdResultsGoTest) Validate() error {
	if c.JSONFormat == c.LegacyFormat {
		return errors.New("invalid format for gotest operation")
	}

	return nil
}
func (c CmdResultsGoTest) Resolve() *CommandDefinition {
	if c.JSONFormat {
		return &CommandDefinition{
			CommandName: c.Name(),
			Params:      exportCmd(c),
		}
	}

	return &CommandDefinition{
		CommandName: "gotest.parse_files",
		Params:      exportCmd(c),
	}
}
func goTestResultsFactory() Command { return CmdResultsGoTest{} }

type ArchiveFormat string

const (
	ZIP     ArchiveFormat = "zip"
	TARBALL ArchiveFormat = "tarball"
)

func (f ArchiveFormat) Validate() error {
	switch f {
	case ZIP, TARBALL:
		return nil
	default:
		return fmt.Errorf("'%s' is not a valid archive format", f)
	}
}

func (f ArchiveFormat) createCmdName() string {
	switch f {
	case ZIP:
		return "archive.zip_pack"
	case TARBALL:
		return "archive.targz_pack"
	default:
		panic(f.Validate())
	}
}

func (f ArchiveFormat) extractCmdName() string {
	switch f {
	case ZIP:
		return "archive.zip_extract"
	case TARBALL:
		return "archive.targz_extract"
	case "auto":
		return "archive.auto_extract"
	default:
		panic(f.Validate())
	}

}

type CmdArchiveCreate struct {
	Format       ArchiveFormat `json:"-" yaml:"-"`
	Target       string        `json:"target" yaml:"target"`
	SourceDir    string        `json:"source_dir" yaml:"source_dir"`
	Include      []string      `json:"include" yaml:"include"`
	ExcludeFiles []string      `json:"exclude_files" yaml:"exclude_files"`
}

func (c CmdArchiveCreate) Name() string    { return c.Format.createCmdName() }
func (c CmdArchiveCreate) Validate() error { return c.Format.Validate() }
func (c CmdArchiveCreate) Resolve() *CommandDefinition {
	return &CommandDefinition{
		CommandName: c.Name(),
		Params:      exportCmd(c),
	}
}

type CmdArchiveExtract struct {
	Format          ArchiveFormat `json:"-" yaml:"-"`
	ArchivePath     string        `json:"path" yaml:"path"`
	TargetDirectory string        `json:"destination,omitempty" yaml:"destination,omitempty"`
	Exclude         []string      `json:"exclude_files,omitempty" yaml:"exclude_files,omitempty"`
}

func (c CmdArchiveExtract) Name() string { return c.Format.extractCmdName() }
func (c CmdArchiveExtract) Validate() error {
	err := c.Format.Validate()
	if err != nil && c.Format != "auto" {
		return err
	}

	return nil

}
func (c CmdArchiveExtract) Resolve() *CommandDefinition {
	return &CommandDefinition{
		CommandName: c.Name(),
		Params:      exportCmd(c),
	}
}

func archiveCreateZipFactory() Command      { return CmdArchiveCreate{Format: ZIP} }
func archiveCreateTarballFactory() Command  { return CmdArchiveCreate{Format: TARBALL} }
func archiveExtractZipFactory() Command     { return CmdArchiveExtract{Format: ZIP} }
func archiveExtractTarballFactory() Command { return CmdArchiveExtract{Format: TARBALL} }
func archiveExtractAutoFactory() Command    { return CmdArchiveExtract{Format: "auto"} }

type CmdAttachArtifacts struct {
	Files    []string `json:"files" yaml:"files"`
	Prefix   string   `json:"prefix,omitempty" yaml:"prefix,omitempty"`
	Optional bool     `json:"optional,omitempty" yaml:"optional,omitempty"`
}

func (c CmdAttachArtifacts) Name() string    { return "attach.artifacts" }
func (c CmdAttachArtifacts) Validate() error { return nil }
func (c CmdAttachArtifacts) Resolve() *CommandDefinition {
	return &CommandDefinition{
		CommandName: c.Name(),
		Params:      exportCmd(c),
	}
}
func attachArtifactsFactory() Command { return CmdAttachArtifacts{} }

type CmdHostCreate struct {
	File string `json:"file,omitempty" yaml:"file,omitempty"`

	// agent-controlled settings
	CloudProvider       string `json:"provider,omitempty" yaml:"provider,omitempty"`
	NumHosts            string `json:"num_hosts,omitempty" yaml:"num_hosts,omitempty"`
	Scope               string `json:"scope,omitempty" yaml:"scope,omitempty"`
	SetupTimeoutSecs    int    `json:"timeout_setup_secs,omitempty" yaml:"timeout_setup_secs,omitempty"`
	TeardownTimeoutSecs int    `json:"timeout_teardown_secs,omitempty" yaml:"timeout_teardown_secs,omitempty"`
	Retries             int    `json:"retries,omitempty" yaml:"retries,omitempty"`

	// EC2-related settings
	AMI             string                `json:"ami,omitempty" yaml:"ami,omitempty"`
	Distro          string                `json:"distro,omitempty" yaml:"distro,omitempty"`
	EBSDevices      []HostCreateEBSDevice `json:"ebs_block_device,omitempty" yaml:"ebs_block_device,omitempty"`
	InstanceType    string                `json:"instance_type,omitempty" yaml:"instance_type,omitempty"`
	IPv6            bool                  `json:"ipv6,omitempty" yaml:"ipv6,omitempty"`
	Region          string                `json:"region,omitempty" yaml:"region,omitempty"`
	SecurityGroups  []string              `json:"security_group_ids,omitempty" yaml:"security_group_ids,omitempty"`
	Spot            bool                  `json:"spot,omitempty" yaml:"spot,omitempty"`
	Subnet          string                `json:"subnet_id,omitempty" yaml:"subnet_id,omitempty"`
	UserdataFile    string                `json:"userdata_file,omitempty" yaml:"userdata_file,omitempty"`
	UserdataCommand string                `json:"userdata_command,omitempty" yaml:"userdata_command,omitempty"`
	AWSKeyID        string                `json:"aws_access_key_id,omitempty" yaml:"aws_access_key_id,omitempty"`
	AWSSecret       string                `json:"aws_secret_access_key,omitempty" yaml:"aws_secret_access_key,omitempty"`
	KeyName         string                `json:"key_name,omitempty" yaml:"key_name,omitempty"`

	// Docker-related settings
	Image                    string                           `json:"image,omitempty" yaml:"image,omitempty"`
	Command                  string                           `json:"command,omitempty" yaml:"command,omitempty"`
	PublishPorts             bool                             `json:"publish_ports,omitempty" yaml:"publish_ports,omitempty"`
	Registry                 HostCreateDockerRegistrySettings `json:"registry,omitempty" yaml:"registry,omitempty"`
	Background               bool                             `json:"background,omitempty" yaml:"background,omitempty"`
	ContainerWaitTimeoutSecs int                              `json:"container_wait_timeout_secs,omitempty" yaml:"container_wait_timeout_secs,omitempty"`
	PollFrequency            int                              `json:"poll_frequency_secs,omitempty" yaml:"poll_frequency_secs,omitempty"`
	StdoutFile               string                           `json:"stdout_file_name,omitempty" yaml:"stdout_file_name,omitempty"`
	StderrFile               string                           `json:"stderr_file_name,omitempty" yaml:"stderr_file_name,omitempty"`
	EnvironmentVars          map[string]string                `json:"environment_vars,omitempty" yaml:"environment_vars,omitempty"`
}

type HostCreateEBSDevice struct {
	DeviceName string `json:"device_name,omitempty" yaml:"device_name,omitempty"`
	IOPS       int    `json:"ebs_iops,omitempty" yaml:"ebs_iops,omitempty"`
	SizeGiB    int    `json:"ebs_size,omitempty" yaml:"ebs_size,omitempty"`
	SnapshotID string `json:"ebs_snapshot_id,omitempty" yaml:"ebs_snapshot_id,omitempty"`
}

type HostCreateDockerRegistrySettings struct {
	Name     string `json:"registry_name,omitempty" yaml:"registry_name,omitempty"`
	Username string `json:"registry_username,omitempty" yaml:"registry_username,omitempty"`
	Password string `json:"registry_password,omitempty" yaml:"registry_password,omitempty"`
}

func (c CmdHostCreate) Name() string    { return "host.create" }
func (c CmdHostCreate) Validate() error { return nil }
func (c CmdHostCreate) Resolve() *CommandDefinition {
	return &CommandDefinition{
		CommandName: c.Name(),
		Params:      exportCmd(c),
	}
}

func hostCreateFactory() Command { return CmdHostCreate{} }

type CmdHostList struct {
	Path        string `json:"path,omitempty" yaml:"path,omitempty"`
	Wait        bool   `json:"wait,omitempty" yaml:"wait,omitempty"`
	Silent      bool   `json:"silent,omitempty" yaml:"silent,omitempty"`
	TimeoutSecs int    `json:"timeout_seconds,omitempty" yaml:"timeout_seconds,omitempty"`
	NumHosts    string `json:"num_hosts,omitempty" yaml:"num_hosts,omitempty"`
}

func (c CmdHostList) Name() string    { return "host.list" }
func (c CmdHostList) Validate() error { return nil }
func (c CmdHostList) Resolve() *CommandDefinition {
	return &CommandDefinition{
		CommandName: c.Name(),
		Params:      exportCmd(c),
	}
}

func hostListFactory() Command { return CmdHostList{} }

type CmdExpansionsUpdate struct {
	File              string `json:"file,omitempty" yaml:"file,omitempty"`
	IgnoreMissingFile bool   `json:"ignore_missing_file,omitempty" yaml:"ignore_missing_file,omitempty"`
}

type ExpansionUpdateParams struct {
	Key    string `json:"key,omitempty" yaml:"key,omitempty"`
	Value  string `json:"value,omitempty" yaml:"value,omitempty"`
	Concat string `json:"concat,omitempty" yaml:"concat,omitempty"`
}

func (c CmdExpansionsUpdate) Name() string    { return "expansions.update" }
func (c CmdExpansionsUpdate) Validate() error { return nil }
func (c CmdExpansionsUpdate) Resolve() *CommandDefinition {
	return &CommandDefinition{
		CommandName: c.Name(),
		Params:      exportCmd(c),
	}
}

func expansionsUpdateFactory() Command { return CmdExpansionsUpdate{} }

type CmdExpansionsWrite struct {
	File     string `json:"file,omitempty" yaml:"file,omitempty"`
	Redacted bool   `json:"redacted,omitempty" yaml:"redacted,omitempty"`
}

func (c CmdExpansionsWrite) Name() string    { return "expansions.write" }
func (c CmdExpansionsWrite) Validate() error { return nil }
func (c CmdExpansionsWrite) Resolve() *CommandDefinition {
	return &CommandDefinition{
		CommandName: c.Name(),
		Params:      exportCmd(c),
	}
}

func expansionsWriteFactory() Command { return CmdExpansionsWrite{} }

type CmdJSONSend struct {
	File     string `json:"file" yaml:"file"`
	DataName string `json:"name" yaml:"name"`
}

func (c CmdJSONSend) Name() string    { return "json.send" }
func (c CmdJSONSend) Validate() error { return nil }
func (c CmdJSONSend) Resolve() *CommandDefinition {
	return &CommandDefinition{
		CommandName: c.Name(),
		Params:      exportCmd(c),
	}
}

func jsonSendFactory() Command { return CmdJSONSend{} }

type CmdPerfSend struct {
	File      string `json:"file" yaml:"file"`
	AWSKey    string `json:"aws_key,omitempty" yaml:"aws_key,omitempty"`
	AWSSecret string `json:"aws_secret,omitempty" yaml:"aws_secret,omitempty"`
	Region    string `json:"region,omitempty" yaml:"region,omitempty"`
	Bucket    string `json:"bucket,omitempty" yaml:"bucket,omitempty"`
	Prefix    string `json:"prefix,omitempty" yaml:"prefix,omitempty"`
}

func (c CmdPerfSend) Name() string    { return "perf.send" }
func (c CmdPerfSend) Validate() error { return nil }
func (c CmdPerfSend) Resolve() *CommandDefinition {
	return &CommandDefinition{
		CommandName: c.Name(),
		Params:      exportCmd(c),
	}
}

func perfSendFactory() Command { return CmdPerfSend{} }

type CmdTimeoutUpdate struct {
	TimeoutSecs     int `json:"timeout_secs,omitempty" yaml:"timeout_secs,omitempty"`
	ExecTimeoutSecs int `json:"exec_timeout_secs,omitempty" yaml:"exec_timeout_secs,omitempty"`
}

func (c CmdTimeoutUpdate) Name() string    { return "timeout.update" }
func (c CmdTimeoutUpdate) Validate() error { return nil }
func (c CmdTimeoutUpdate) Resolve() *CommandDefinition {
	return &CommandDefinition{
		CommandName: c.Name(),
		Params:      exportCmd(c),
	}
}
func timeoutUpdateFactory() Command { return CmdTimeoutUpdate{} }
