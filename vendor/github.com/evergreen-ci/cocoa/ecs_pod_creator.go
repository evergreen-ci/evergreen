package cocoa

import (
	"context"

	"github.com/pkg/errors"

	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
)

// ECSPodCreator provides a means to create a new pod backed by AWS ECS.
type ECSPodCreator interface {
	// CreatePod creates a new pod backed by ECS with the given options. Options
	// are applied in the order they're specified and conflicting options are
	// overwritten.
	CreatePod(ctx context.Context, opts ...*ECSPodCreationOptions) (ECSPod, error)
	// CreatePodFromExistingDefinition creates a new pod backed by ECS from an
	// existing task definition.
	CreatePodFromExistingDefinition(ctx context.Context, def ECSTaskDefinition, opts ...*ECSPodExecutionOptions) (ECSPod, error)
}

// ECSPodCreationOptions provide options to create a pod backed by ECS.
type ECSPodCreationOptions struct {
	// Name is the friendly name of the pod. By default, this is a random
	// string.
	Name *string
	// ContainerDefinitions defines settings that apply to individual containers
	// within the pod. This is required.
	ContainerDefinitions []ECSContainerDefinition
	// MemoryMB is the hard memory limit (in MB) across all containers in the
	// pod. If this is not specified, then each container is required to specify
	// its own memory. This is ignored for pods running Windows containers.
	MemoryMB *int
	// CPU is the hard CPU limit (in CPU units) across all containers in the
	// pod. 1024 CPU units is equivalent to 1 vCPU on a machine. If this is not
	// specified, then each container is required to specify its own CPU.
	// This is ignored for pods running Windows containers.
	CPU *int
	// TaskRole is the role that the pod can use. Depending on the
	// configuration, this may be required if
	// (ECSPodExecutionOptions).SupportsDebugMode is true.
	TaskRole *string
	// Tags are resource tags to apply to the pod definition.
	Tags map[string]string
	// ExecutionOpts specify options to configure how the pod executes.
	ExecutionOpts *ECSPodExecutionOptions
}

// NewECSPodCreationOptions returns new uninitialized options to create a pod.
func NewECSPodCreationOptions() *ECSPodCreationOptions {
	return &ECSPodCreationOptions{}
}

// SetName sets the friendly name of the pod.
func (o *ECSPodCreationOptions) SetName(name string) *ECSPodCreationOptions {
	o.Name = &name
	return o
}

// SetContainerDefinitions sets the container definitions for the pod. This
// overwrites any existing container definitions.
func (o *ECSPodCreationOptions) SetContainerDefinitions(defs []ECSContainerDefinition) *ECSPodCreationOptions {
	o.ContainerDefinitions = defs
	return o
}

// AddContainerDefinitions add new container definitions to the existing ones
// for the pod.
func (o *ECSPodCreationOptions) AddContainerDefinitions(defs ...ECSContainerDefinition) *ECSPodCreationOptions {
	o.ContainerDefinitions = append(o.ContainerDefinitions, defs...)
	return o
}

// SetMemoryMB sets the memory limit (in MB) that applies across the entire
// pod's containers.
func (o *ECSPodCreationOptions) SetMemoryMB(mem int) *ECSPodCreationOptions {
	o.MemoryMB = &mem
	return o
}

// SetCPU sets the CPU limit (in CPU units) that applies across the entire pod's
// containers.
func (o *ECSPodCreationOptions) SetCPU(cpu int) *ECSPodCreationOptions {
	o.CPU = &cpu
	return o
}

// SetTaskRole sets the task role that the pod can use.
func (o *ECSPodCreationOptions) SetTaskRole(role string) *ECSPodCreationOptions {
	o.TaskRole = &role
	return o
}

// SetTags sets the tags for the pod definition. This overwrites any existing
// tags.
func (o *ECSPodCreationOptions) SetTags(tags map[string]string) *ECSPodCreationOptions {
	o.Tags = tags
	return o
}

// AddTags adds new tags to the existing ones for the pod definition.
func (o *ECSPodCreationOptions) AddTags(tags map[string]string) *ECSPodCreationOptions {
	if o.Tags == nil {
		o.Tags = make(map[string]string)
	}

	for k, v := range tags {
		o.Tags[k] = v
	}

	return o
}

// SetExecutionOptions sets the options to configure how the pod executes.
func (o *ECSPodCreationOptions) SetExecutionOptions(opts ECSPodExecutionOptions) *ECSPodCreationOptions {
	o.ExecutionOpts = &opts
	return o
}

// validateContainerDefinitions checks that all the individual container definitions are valid.
func (o *ECSPodCreationOptions) validateContainerDefinitions() error {
	var totalContainerMemMB, totalContainerCPU int
	catcher := grip.NewBasicCatcher()

	catcher.NewWhen(len(o.ContainerDefinitions) == 0, "must specify at least one container definition")

	for i := range o.ContainerDefinitions {
		catcher.Wrapf(o.ContainerDefinitions[i].Validate(), "container definition '%s'", o.ContainerDefinitions[i].Name)

		if o.ContainerDefinitions[i].MemoryMB != nil {
			totalContainerMemMB += *o.ContainerDefinitions[i].MemoryMB
		} else if o.MemoryMB == nil {
			catcher.Errorf("must specify container-level memory to allocate for each container if pod-level memory is not specified")
		}

		if o.ContainerDefinitions[i].CPU != nil {
			totalContainerCPU += *o.ContainerDefinitions[i].CPU
		} else if o.CPU == nil {
			catcher.Errorf("must specify container-level CPU to allocate for each container if pod-level CPU is not specified")
		}

		if len(o.ContainerDefinitions[i].EnvVars) > 0 {
			for _, envVar := range o.ContainerDefinitions[i].EnvVars {
				if envVar.SecretOpts != nil && o.ExecutionOpts.ExecutionRole == nil {
					catcher.Errorf("must specify execution role ARN when specifying container secrets")
				}
			}
		}
	}

	if o.MemoryMB != nil {
		catcher.ErrorfWhen(*o.MemoryMB < totalContainerMemMB, "total memory requested for the individual containers (%d MB) is greater than the memory available for the entire task (%d MB)", totalContainerMemMB, *o.MemoryMB)
	}
	if o.CPU != nil {
		catcher.ErrorfWhen(*o.CPU < totalContainerCPU, "total CPU requested for the individual containers (%d units) is greater than the memory available for the entire task (%d units)", totalContainerCPU, *o.CPU)
	}

	return catcher.Resolve()
}

// Validate checks that all the required parameters are given and the values are
// valid.
func (o *ECSPodCreationOptions) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(o.Name != nil && *o.Name == "", "cannot specify an empty name")
	catcher.NewWhen(o.MemoryMB != nil && *o.MemoryMB <= 0, "must have positive memory value if non-default")
	catcher.NewWhen(o.CPU != nil && *o.CPU <= 0, "must have positive CPU value if non-default")

	catcher.Wrap(o.validateContainerDefinitions(), "invalid container definitions")

	if o.ExecutionOpts != nil {
		catcher.Wrap(o.ExecutionOpts.Validate(), "invalid execution options")
	}

	if catcher.HasErrors() {
		return catcher.Resolve()
	}

	if o.Name == nil {
		o.Name = utility.ToStringPtr(utility.RandomString())
	}

	if o.ExecutionOpts == nil {
		placementOpts := NewECSPodPlacementOptions().SetStrategy(StrategyBinpack).SetStrategyParameter(StrategyParamBinpackMemory)
		o.ExecutionOpts = NewECSPodExecutionOptions().SetPlacementOptions(*placementOpts)
	}

	return nil
}

// MergeECSPodCreationOptions merges all the given options to create an ECS pod.
// Options are applied in the order that they're specified and conflicting
// options are overwritten.
func MergeECSPodCreationOptions(opts ...*ECSPodCreationOptions) ECSPodCreationOptions {
	merged := ECSPodCreationOptions{}

	for _, opt := range opts {
		if opt == nil {
			continue
		}

		if opt.Name != nil {
			merged.Name = opt.Name
		}

		if opt.ContainerDefinitions != nil {
			merged.ContainerDefinitions = opt.ContainerDefinitions
		}

		if opt.MemoryMB != nil {
			merged.MemoryMB = opt.MemoryMB
		}

		if opt.CPU != nil {
			merged.CPU = opt.CPU
		}

		if opt.TaskRole != nil {
			merged.TaskRole = opt.TaskRole
		}

		if opt.Tags != nil {
			merged.Tags = opt.Tags
		}

		if opt.ExecutionOpts != nil {
			merged.ExecutionOpts = opt.ExecutionOpts
		}
	}

	return merged
}

// MergeECSPodExecutionOptions merges all the given options to execute an ECS pod.
// Options are applied in the order that they're specified and conflicting
// options are overwritten.
func MergeECSPodExecutionOptions(opts ...*ECSPodExecutionOptions) ECSPodExecutionOptions {
	merged := ECSPodExecutionOptions{}

	for _, opt := range opts {
		if opt == nil {
			continue
		}

		if opt.Cluster != nil {
			merged.Cluster = opt.Cluster
		}

		if opt.PlacementOpts != nil {
			merged.PlacementOpts = opt.PlacementOpts
		}

		if opt.SupportsDebugMode != nil {
			merged.SupportsDebugMode = opt.SupportsDebugMode
		}

		if opt.Tags != nil {
			merged.Tags = opt.Tags
		}

		if opt.ExecutionRole != nil {
			merged.ExecutionRole = opt.ExecutionRole
		}
	}

	return merged
}

// ECSContainerDefinition defines settings that apply to a single container
// within an ECS pod.
type ECSContainerDefinition struct {
	// Name is the friendly name of the container. By default, this is a random
	// string.
	Name *string
	// Image is the Docker image to use. This is required.
	Image *string
	// Command is the command to run, separated into individual arguments. By
	// default, there is no command.
	Command []string
	// MemoryMB is the amount of memory (in MB) to allocate. This must be
	// set if a pod-level memory limit is not given.
	MemoryMB *int
	// CPU is the number of CPU units to allocate. 1024 CPU units is equivalent
	// to 1 vCPU on a machine. This must be set if a pod-level CPU limit is not
	// given.
	CPU *int
	// EnvVars are environment variables to make available in the container.
	EnvVars []EnvironmentVariable
	// Tags are resource tags to apply.
	Tags []string
}

// NewECSContainerDefinition returns a new uninitialized container definition.
func NewECSContainerDefinition() *ECSContainerDefinition {
	return &ECSContainerDefinition{}
}

// SetName sets the friendly name for the container.
func (d *ECSContainerDefinition) SetName(name string) *ECSContainerDefinition {
	d.Name = &name
	return d
}

// SetImage sets the image for the container.
func (d *ECSContainerDefinition) SetImage(img string) *ECSContainerDefinition {
	d.Image = &img
	return d
}

// SetCommand sets the command for the container to run.
func (d *ECSContainerDefinition) SetCommand(cmd []string) *ECSContainerDefinition {
	d.Command = cmd
	return d
}

// SetMemoryMB sets the amount of memory (in MB) to allocate.
func (d *ECSContainerDefinition) SetMemoryMB(mem int) *ECSContainerDefinition {
	d.MemoryMB = &mem
	return d
}

// SetCPU sets the number of CPU units to allocate.
func (d *ECSContainerDefinition) SetCPU(cpu int) *ECSContainerDefinition {
	d.CPU = &cpu
	return d
}

// SetTags sets the tags for the container. This overwrites any existing tags.
func (d *ECSContainerDefinition) SetTags(tags []string) *ECSContainerDefinition {
	d.Tags = tags
	return d
}

// AddTags adds new tags to the existing ones for the container.
func (d *ECSContainerDefinition) AddTags(tags ...string) *ECSContainerDefinition {
	d.Tags = append(d.Tags, tags...)
	return d
}

// SetEnvironmentVariables sets the environment variables for the container.
// This overwrites any existing environment variables.
func (d *ECSContainerDefinition) SetEnvironmentVariables(envVars []EnvironmentVariable) *ECSContainerDefinition {
	d.EnvVars = envVars
	return d
}

// AddEnvironmentVariables adds new environment variables to the existing ones
// for the container.
func (d *ECSContainerDefinition) AddEnvironmentVariables(envVars ...EnvironmentVariable) *ECSContainerDefinition {
	d.EnvVars = append(d.EnvVars, envVars...)
	return d
}

// Validate checks that the image is provided and that all of its environment
// variables are valid.
func (d *ECSContainerDefinition) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(d.Image == nil, "must specify an image")
	catcher.NewWhen(d.Image != nil && *d.Image == "", "cannot specify an empty image")
	catcher.NewWhen(d.MemoryMB != nil && *d.MemoryMB <= 0, "must have positive memory value if non-default")
	catcher.NewWhen(d.CPU != nil && *d.CPU <= 0, "must have positive CPU value if non-default")
	for _, ev := range d.EnvVars {
		catcher.Wrapf(ev.Validate(), "environment variable '%s'", ev.Name)
	}
	if catcher.HasErrors() {
		return catcher.Resolve()
	}

	if d.Name == nil {
		d.Name = utility.ToStringPtr(utility.RandomString())
	}

	return nil
}

// EnvironmentVariable represents an environment variable, which can be
// optionally backed by a secret.
type EnvironmentVariable struct {
	Name       *string
	Value      *string
	SecretOpts *SecretOptions
}

// NewEnvironmentVariable returns a new uninitialized environment variable.
func NewEnvironmentVariable() *EnvironmentVariable {
	return &EnvironmentVariable{}
}

// SetName sets the environment variable name.
func (e *EnvironmentVariable) SetName(name string) *EnvironmentVariable {
	e.Name = &name
	return e
}

// SetValue sets the environment variable's value. This is mutually exclusive
// with setting the (EnvironmentVariable).SecretOptions.
func (e *EnvironmentVariable) SetValue(val string) *EnvironmentVariable {
	e.Value = &val
	return e
}

// SetSecretOptions sets the environment variable's secret value. This is
// mutually exclusive with setting the non-secret (EnvironmentVariable).Value.
func (e *EnvironmentVariable) SetSecretOptions(opts SecretOptions) *EnvironmentVariable {
	e.SecretOpts = &opts
	return e
}

// Validate checks that the environment variable name is given and that either
// the raw environment variable value or the referenced secret is given.
func (e *EnvironmentVariable) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(e.Name == nil, "must specify a name")
	catcher.NewWhen(e.Name != nil && *e.Name == "", "cannot specify an empty name")
	catcher.NewWhen(e.Value == nil && e.SecretOpts == nil, "must either specify a value or reference a secret")
	catcher.NewWhen(e.Value != nil && e.SecretOpts != nil, "cannot both specify a value and reference a secret")
	if e.SecretOpts != nil {
		catcher.Wrap(e.SecretOpts.Validate(), "invalid secret options")
	}
	return catcher.Resolve()
}

// SecretOptions represents a secret with a name and value that may or may not
// be owned by its pod.
type SecretOptions struct {
	PodSecret
	// Exists determines whether or not the secret already exists or must be
	// created before it can be used.
	Exists *bool
}

// NewSecretOptions returns new uninitialized options for a secret.
func NewSecretOptions() *SecretOptions {
	return &SecretOptions{}
}

// SetName sets the secret name.
func (s *SecretOptions) SetName(name string) *SecretOptions {
	s.Name = &name
	return s
}

// SetValue sets the secret value.
func (s *SecretOptions) SetValue(val string) *SecretOptions {
	s.Value = &val
	return s
}

// SetExists sets whether or not the secret already exists or must be created.
func (s *SecretOptions) SetExists(exists bool) *SecretOptions {
	s.Exists = &exists
	return s
}

// SetOwned returns whether or not the secret is owned by its pod.
func (s *SecretOptions) SetOwned(owned bool) *SecretOptions {
	s.Owned = &owned
	return s
}

// Validate validates that the secret name is given and that either the secret
// already exists or the new secret's value is given.
func (s *SecretOptions) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(s.Name == nil, "must specify a name")
	catcher.NewWhen(s.Name != nil && *s.Name == "", "cannot specify an empty name")
	catcher.NewWhen(!utility.FromBoolPtr(s.Exists) && s.Value == nil, "either a new secret's value must be given or the secret must already exist")
	return catcher.Resolve()
}

// ECSPodExecutionOptions represent options to configure how a pod is started.
type ECSPodExecutionOptions struct {
	// Cluster is the name of the cluster where the pod will run. If none is
	// specified, this will run in the default cluster.
	Cluster *string
	// PlacementOptions specify options that determine how a pod is assigned to
	// a container instance.
	PlacementOpts *ECSPodPlacementOptions
	// SupportsDebugMode indicates that the ECS pod should support debugging, so
	// you can run exec in the pod's containers. In order for this to work, the
	// pod must have the correct permissions to perform this operation when it's
	// defined. By default, this is false.
	SupportsDebugMode *bool
	// Tags are any tags to apply to the running pods.
	Tags map[string]string
	// ExecutionRole is the role that ECS container agent can use. Depending on
	// the configuration, this may be required if the container uses secrets.
	ExecutionRole *string
}

// NewECSPodExecutionOptions returns new uninitialized options to run a pod.
func NewECSPodExecutionOptions() *ECSPodExecutionOptions {
	return &ECSPodExecutionOptions{}
}

// SetCluster sets the name of the cluster where the pod will run.
func (o *ECSPodExecutionOptions) SetCluster(cluster string) *ECSPodExecutionOptions {
	o.Cluster = &cluster
	return o
}

// SetPlacementOptions sets the options that determine how a pod is assigned to
// a container instance.
func (o *ECSPodExecutionOptions) SetPlacementOptions(opts ECSPodPlacementOptions) *ECSPodExecutionOptions {
	o.PlacementOpts = &opts
	return o
}

// SetSupportsDebugMode sets whether or not the pod can run with debug mode
// enabled.
func (o *ECSPodExecutionOptions) SetSupportsDebugMode(supported bool) *ECSPodExecutionOptions {
	o.SupportsDebugMode = &supported
	return o
}

// SetTags sets the tags for the pod itself when it is run. This overwrites any
// existing tags.
func (o *ECSPodExecutionOptions) SetTags(tags map[string]string) *ECSPodExecutionOptions {
	o.Tags = tags
	return o
}

// AddTags adds new tags to the existing ones for the pod itself when it is run.
func (o *ECSPodExecutionOptions) AddTags(tags map[string]string) *ECSPodExecutionOptions {
	if o.Tags == nil {
		o.Tags = make(map[string]string)
	}

	for k, v := range tags {
		o.Tags[k] = v
	}

	return o
}

// SetExecutionRole sets the execution role for the pod itself when it is run. This overwrites any
// existing execution roles.
func (o *ECSPodExecutionOptions) SetExecutionRole(role string) *ECSPodExecutionOptions {
	o.ExecutionRole = &role
	return o
}

// Validate checks that the placement options are valid.
func (o *ECSPodExecutionOptions) Validate() error {
	catcher := grip.NewBasicCatcher()
	if o.PlacementOpts != nil {
		catcher.Wrap(o.PlacementOpts.Validate(), "invalid placement options")
	}
	if catcher.HasErrors() {
		return catcher.Resolve()
	}

	if o.PlacementOpts == nil {
		o.PlacementOpts = NewECSPodPlacementOptions().SetStrategy(StrategyBinpack).SetStrategyParameter(StrategyParamBinpackMemory)
	}

	return nil
}

// ECSPodPlacementOptions represent options to control how an ECS pod is
// assigned to a container instance.
type ECSPodPlacementOptions struct {
	// Strategy is the overall placement strategy. By default, it uses the
	// binpack strategy.
	Strategy *ECSPlacementStrategy

	// StrategyParameter is the parameter that determines how the placement
	// strategy optimizes pod placement. The default value depends on the
	// strategy:
	// If the strategy is spread, it defaults to "host".
	// If the strategy is binpack, it defaults to "memory".
	// If the strategy is random, this does not apply.
	StrategyParameter *ECSStrategyParameter
}

// NewECSPodPlacementOptions creates new options to specify how an ECS pod
// should be assigned to a container instance.
func NewECSPodPlacementOptions() *ECSPodPlacementOptions {
	return &ECSPodPlacementOptions{}
}

// SetStrategy sets the strategy for placing the pod on a container instance.
func (o *ECSPodPlacementOptions) SetStrategy(s ECSPlacementStrategy) *ECSPodPlacementOptions {
	o.Strategy = &s
	return o
}

// SetStrategyParameter sets the parameter to optimize for when placing the pod
// on a container instance.
func (o *ECSPodPlacementOptions) SetStrategyParameter(p ECSStrategyParameter) *ECSPodPlacementOptions {
	o.StrategyParameter = &p
	return o
}

// Validate checks that the the strategy and its parameter to optimize are a
// valid combination.
func (o *ECSPodPlacementOptions) Validate() error {
	catcher := grip.NewBasicCatcher()
	if o.Strategy != nil {
		catcher.Add(o.Strategy.Validate())
	}
	if o.Strategy != nil && o.StrategyParameter != nil {
		catcher.ErrorfWhen(*o.Strategy == StrategyBinpack && *o.StrategyParameter != StrategyParamBinpackMemory && *o.StrategyParameter != StrategyParamBinpackCPU, "strategy parameter cannot be '%s' when the strategy is '%s'", *o.StrategyParameter, *o.Strategy)
		catcher.ErrorfWhen(*o.Strategy != StrategySpread && *o.StrategyParameter == StrategyParamSpreadHost, "strategy parameter cannot be '%s' when the strategy is not '%s'", *o.StrategyParameter, StrategySpread)
	}

	if catcher.HasErrors() {
		return catcher.Resolve()
	}

	if o.Strategy == nil {
		strategy := StrategyBinpack
		o.Strategy = &strategy
	}

	if o.Strategy != nil && o.StrategyParameter == nil {
		if *o.Strategy == StrategyBinpack {
			o.StrategyParameter = utility.ToStringPtr(StrategyParamBinpackMemory)
		}
		if *o.Strategy == StrategySpread {
			o.StrategyParameter = utility.ToStringPtr(StrategyParamSpreadHost)
		}
	}

	return nil
}

// ECSPlacementStrategy represents a placement strategy for ECS pods.
type ECSPlacementStrategy string

const (
	// StrategySpread indicates that the ECS pod will be assigned in such a way
	// to achieve an even spread based on the given ECSStrategyParameter.
	StrategySpread ECSPlacementStrategy = ecs.PlacementStrategyTypeSpread
	// StrategyRandom indicates that the ECS pod should be assigned to a
	// container instance randomly.
	StrategyRandom ECSPlacementStrategy = ecs.PlacementStrategyTypeRandom
	// StrategyBinpack indicates that the the ECS pod will be placed on a
	// container instance with the least amount of memory or CPU that will be
	// sufficient for the pod's requirements if possible.
	StrategyBinpack ECSPlacementStrategy = ecs.PlacementStrategyTypeBinpack
)

// Validate checks that the ECS pod status is one of the recognized placement
// strategies.
func (s ECSPlacementStrategy) Validate() error {
	switch s {
	case StrategySpread, StrategyRandom, StrategyBinpack:
		return nil
	default:
		return errors.Errorf("unrecognized placement strategy '%s'", s)
	}
}

// ECSStrategyParameter represents the parameter that ECS will use with its
// strategy to schedule pods on container instances.
type ECSStrategyParameter = string

const (
	// StrategyParamBinpackMemory indicates ECS should optimize its binpacking strategy based
	// on memory usage.
	StrategyParamBinpackMemory ECSStrategyParameter = "memory"
	// StrategyParamBinpackCPU indicates ECS should optimize its binpacking strategy based
	// on CPU usage.
	StrategyParamBinpackCPU ECSStrategyParameter = "cpu"
	// StrategyParamSpreadHost indicates the ECS should spread pods evenly across all
	// container instances (i.e. hosts).
	StrategyParamSpreadHost ECSStrategyParameter = "host"
)

// ECSTaskDefinition represents options for an existing ECS task definition.
type ECSTaskDefinition struct {
	// ID is the ID of the task definition, which should already exist.
	ID *string
	// Owned determines whether or not the task definition is owned by its pod
	// or not.
	Owned *bool
}

// NewECSTaskDefinition returns a new uninitialized task definition.
func NewECSTaskDefinition() *ECSTaskDefinition {
	return &ECSTaskDefinition{}
}

// SetID sets the task definition ID.
func (d *ECSTaskDefinition) SetID(id string) *ECSTaskDefinition {
	d.ID = &id
	return d
}

// SetOwned sets if the task definition is owned by its pod.
func (d *ECSTaskDefinition) SetOwned(owned bool) *ECSTaskDefinition {
	d.Owned = &owned
	return d
}
