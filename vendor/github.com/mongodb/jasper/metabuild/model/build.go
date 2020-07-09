package model

// Target represents a set of commands that can be executed.
type Target struct {
	// Name is the name of the target to execute.
	Name string `yaml:"name"`
	// Tags are labels that allow you to logically group related targets.
	Tags []string `yaml:"tags,omitempty"`
	// ExcludeTags allows you to specify tags that should not be applied to the
	// task. This can be useful for excluding a target from the default tags.
	ExcludeTags []string `yaml:"exclude_tags,omitempty"`
	// RequiredEnvironment specifies prerequisite environment variables names
	// that must be defined (in the local process environment? from another
	// file?) before the target can run.
	RequiredEnvironment []string         `yaml:"required_environment,omitempty"`
	DependencyInfo      []DependencyInfo `yaml:"dependency_info,omitempty"`
	// Commands are the commands that this target will run.
	Commands []Command `yaml:"commands"`
	// Options specify target-specific options.
	Options TargetOptions `yaml:"options,omitempty"`
	// Output specifies target-specific output.
	Output Output `yaml:"output,omitempty"`
}

// DependencyInfo defines necessary prerequisite dependencies and outputs to
// determine if a target's dependencies are satisfied and if the target is
// up-to-date.
type DependencyInfo struct {
	Dependencies []Dependency `yaml:"dependencies,omitempty"`
	// ProducesFiles provides a hint to determine if the target is up-to-date.
	// If all the files exist and are up-to-date with the cached versions, the
	// target is considered up-to-date. If this is not provided, the target is
	// assumed to be out of date.
	ProducesFiles []string `yaml:"produces_files,omitempty"`
	// OnlyAsDependency determines whether this target can be executed directly or
	// is only usable as a dependency of other targets. By default, targets are
	// executable.
	OnlyAsDependency bool `yaml:"only_as_dependency,omitempty"`
	// IgnoreDependencies determines whether dependencies are checked or not
	// before running a target. By default, dependencies are checked before
	// running a target.
	IgnoreDependencies bool `yaml:"ignore_dependencies,omitempty"`
}

// Dependency describes prerequisites for running a target.
type Dependency struct {
	// Targets are names of other targets that this target depends on.
	Targets []string `yaml:"targets,omitempty"`
	// Files are files that this target depends on.
	// kim: TODO: if a target depends on another file which does not exist, how
	// will the build system know how to generate said file (same problem as
	// Make)?
	Files []string `yaml:"files,omitempty"`
}

// TargetOptions specify options that modify the behavior of target execution.
type TargetOptions struct {
	// ContinueOnError determines whether or not to continue running later
	// targets when a target fails during execution.
	ContinueOnError bool
}

// Command describes a command to run.
type Command struct {
	// Command specifies the command in shell syntax. The command is parsed
	// according to shell syntax rules (although it will not be executed within
	// a shell).
	Command string `yaml:"command,omitempty"`
	// Args are the arguments to the command. The first argument should be the
	// binary name or path to it.
	Args    []string       `yaml:"args,omitempty"`
	Options CommandOptions `yaml:"options,omitempty"`
}

// CommandOptions allow you to modify a command's execution behavior.
type CommandOptions struct {
	// Background indicates that the command should run in the background.
	Background bool `yaml:"background,omitempty"`
	// Silent indicates that logs from standard output and standard error should
	// be suppressed.
	Silent bool `yaml:"silent,omitempty"`
}

// Output describes how to process the output of a target.
type Output struct {
	// Path is the path to the files created by the target.
	Path string `yaml:"path"`
	// ArchiveFormat is the kind of archive that should be created.
	ArchiveFormat string `yaml:"format"`
	// ArchivePath specifies where the archive file should be written.
	ArchivePath   string        `yaml:"output_file"`
	UploadOptions UploadOptions `yaml:"upload_options,omitempty"`
}

// UploadOptions describe how the output should be uploaded to a remote store.
type UploadOptions struct {
	// Format describes the format of the output to upload.
	Format ReportFormat `yaml:"format,omitempty"`
	// Report indicates that the output should be reported as test results.
	Report bool `yaml:"report,omitempty"`
	// UploadArchive indicates that the output should be uploaded as an archive.
	UploadArchive bool `yaml:"upload_archive,omitempty"`
	// UploadPath is the remote path where the output will be uploaded.
	UploadPath string `yaml:"upload_path,omitempty"`
}
