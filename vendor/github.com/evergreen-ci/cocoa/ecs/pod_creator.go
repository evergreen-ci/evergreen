package ecs

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// BasicECSPodCreator provides an cocoa.ECSPodCreator implementation to create
// AWS ECS pods.
type BasicECSPodCreator struct {
	client cocoa.ECSClient
	vault  cocoa.Vault
}

// NewBasicECSPodCreator creates a helper to create pods backed by AWS ECS.
func NewBasicECSPodCreator(c cocoa.ECSClient, v cocoa.Vault) (*BasicECSPodCreator, error) {
	if c == nil {
		return nil, errors.New("missing client")
	}
	return &BasicECSPodCreator{
		client: c,
		vault:  v,
	}, nil
}

// CreatePod creates a new pod backed by AWS ECS.
func (pc *BasicECSPodCreator) CreatePod(ctx context.Context, opts ...cocoa.ECSPodCreationOptions) (cocoa.ECSPod, error) {
	mergedPodCreationOpts := cocoa.MergeECSPodCreationOptions(opts...)
	var mergedPodExecutionOpts cocoa.ECSPodExecutionOptions
	if mergedPodCreationOpts.ExecutionOpts != nil {
		mergedPodExecutionOpts = cocoa.MergeECSPodExecutionOptions(*mergedPodCreationOpts.ExecutionOpts)
	}

	if err := mergedPodCreationOpts.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid pod creation options")
	}

	if err := mergedPodExecutionOpts.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid pod execution options")
	}

	if err := pc.createSecrets(ctx, &mergedPodCreationOpts); err != nil {
		return nil, errors.Wrap(err, "creating new secrets")
	}
	taskDefinition := pc.exportPodCreationOptions(mergedPodCreationOpts)

	registerOut, err := pc.client.RegisterTaskDefinition(ctx, taskDefinition)
	if err != nil {
		return nil, errors.Wrap(err, "registering task definition")
	}

	if registerOut.TaskDefinition == nil || registerOut.TaskDefinition.TaskDefinitionArn == nil {
		return nil, errors.New("expected a task definition from ECS, but none was returned")
	}

	taskDef := cocoa.NewECSTaskDefinition().
		SetID(utility.FromStringPtr(registerOut.TaskDefinition.TaskDefinitionArn)).
		SetOwned(true)

	runTask := pc.exportTaskExecutionOptions(mergedPodExecutionOpts, *taskDef)

	runOut, err := pc.client.RunTask(ctx, runTask)
	if err != nil {
		return nil, errors.Wrapf(err, "running task for definition '%s' in cluster '%s'", utility.FromStringPtr(runTask.TaskDefinition), utility.FromStringPtr(runTask.Cluster))
	}

	if len(runOut.Failures) > 0 {
		catcher := grip.NewBasicCatcher()
		for _, failure := range runOut.Failures {
			catcher.Errorf("task '%s': %s: %s\n", utility.FromStringPtr(failure.Arn), utility.FromStringPtr(failure.Detail), utility.FromStringPtr(failure.Reason))
		}
		return nil, errors.Wrap(catcher.Resolve(), "running task")
	}

	if len(runOut.Tasks) == 0 || runOut.Tasks[0].TaskArn == nil {
		return nil, errors.New("expected a task to be running in ECS, but none was returned")
	}

	resources := cocoa.NewECSPodResources().
		SetCluster(utility.FromStringPtr(mergedPodExecutionOpts.Cluster)).
		SetContainers(pc.translateContainerResources(runOut.Tasks[0].Containers, mergedPodCreationOpts.ContainerDefinitions)).
		SetTaskDefinition(*taskDef).
		SetTaskID(utility.FromStringPtr(runOut.Tasks[0].TaskArn))

	podOpts := NewBasicECSPodOptions().
		SetClient(pc.client).
		SetVault(pc.vault).
		SetStatusInfo(translatePodStatusInfo(runOut.Tasks[0])).
		SetResources(*resources)

	p, err := NewBasicECSPod(podOpts)
	if err != nil {
		return nil, errors.Wrap(err, "creating pod")
	}

	return p, nil
}

// CreatePodFromExistingDefinition creates a new pod backed by AWS ECS from an
// existing definition.
func (pc *BasicECSPodCreator) CreatePodFromExistingDefinition(ctx context.Context, def cocoa.ECSTaskDefinition, opts ...cocoa.ECSPodExecutionOptions) (cocoa.ECSPod, error) {
	return nil, errors.New("TODO: implement")
}

// createSecrets creates any necessary secrets from the secret environment
// variables for each container. Once the secrets are created, their IDs are
// set.
func (pc *BasicECSPodCreator) createSecrets(ctx context.Context, opts *cocoa.ECSPodCreationOptions) error {
	var defs []cocoa.ECSContainerDefinition
	for i, def := range opts.ContainerDefinitions {
		defs = append(defs, def)
		containerName := utility.FromStringPtr(def.Name)

		var envVars []cocoa.EnvironmentVariable
		for _, envVar := range def.EnvVars {
			if envVar.SecretOpts == nil || envVar.SecretOpts.NewValue == nil {
				envVars = append(envVars, envVar)
				defs[i].EnvVars = append(defs[i].EnvVars, envVar)
				continue
			}

			id, err := pc.createSecret(ctx, *envVar.SecretOpts)
			if err != nil {
				return errors.Wrapf(err, "creating secret environment variable '%s' for container '%s'", utility.FromStringPtr(opts.Name), containerName)
			}

			updated := *envVar.SecretOpts
			updated.SetID(id)
			envVar.SecretOpts = &updated
			envVars = append(envVars, envVar)
		}

		defs[i].EnvVars = envVars

		repoCreds := def.RepoCreds
		if def.RepoCreds != nil && def.RepoCreds.NewCreds != nil {
			val, err := json.Marshal(def.RepoCreds.NewCreds)
			if err != nil {
				return errors.Wrap(err, "formatting new repository credentials to create")
			}
			secretOpts := cocoa.NewSecretOptions().
				SetName(utility.FromStringPtr(def.RepoCreds.Name)).
				SetNewValue(string(val))
			id, err := pc.createSecret(ctx, *secretOpts)
			if err != nil {
				return errors.Wrapf(err, "creating repository credentials for container '%s'", utility.FromStringPtr(def.Name))
			}

			updated := *def.RepoCreds
			updated.SetID(id)
			repoCreds = &updated
		}

		defs[i].RepoCreds = repoCreds
	}

	// Since the options format makes extensive use of pointers and pointers may
	// be shared between the input and the options used during pod creation, we
	// have to avoid mutating the original input. Therefore, replace the
	// entire slice of container definitions to create a separate slice in
	// memory and avoid mutating the original input's container definitions.
	opts.ContainerDefinitions = defs

	return nil
}

// createSecret creates a single secret. It returns the newly-created secret's
// ID.
func (pc *BasicECSPodCreator) createSecret(ctx context.Context, secret cocoa.SecretOptions) (id string, err error) {
	if pc.vault == nil {
		return "", errors.New("no vault was specified")
	}
	return pc.vault.CreateSecret(ctx, *cocoa.NewNamedSecret().
		SetName(utility.FromStringPtr(secret.Name)).
		SetValue(utility.FromStringPtr(secret.NewValue)))
}

// exportTags converts a mapping of tag names to values into ECS tags.
func (pc *BasicECSPodCreator) exportTags(tags map[string]string) []*ecs.Tag {
	var ecsTags []*ecs.Tag

	for k, v := range tags {
		var tag ecs.Tag
		tag.SetKey(k).SetValue(v)
		ecsTags = append(ecsTags, &tag)
	}

	return ecsTags
}

// exportStrategy converts the strategy and parameter into an ECS placement
// strategy.
func (pc *BasicECSPodCreator) exportStrategy(opts *cocoa.ECSPodPlacementOptions) []*ecs.PlacementStrategy {
	var placementStrat ecs.PlacementStrategy
	placementStrat.SetType(string(*opts.Strategy)).SetField(utility.FromStringPtr(opts.StrategyParameter))
	return []*ecs.PlacementStrategy{&placementStrat}
}

// exportPlacementConstraints converts the placement options into ECS placement
// constraints.
func (pc *BasicECSPodCreator) exportPlacementConstraints(opts *cocoa.ECSPodPlacementOptions) []*ecs.PlacementConstraint {
	var constraints []*ecs.PlacementConstraint

	for _, filter := range opts.InstanceFilters {
		var constraint ecs.PlacementConstraint
		if filter == cocoa.ConstraintDistinctInstance {
			constraint.SetType(filter)
		} else {
			constraint.SetType("memberOf").SetExpression(filter)
		}
		constraints = append(constraints, &constraint)
	}

	return constraints
}

// exportEnvVars converts the non-secret environment variables into ECS
// environment variables.
func (pc *BasicECSPodCreator) exportEnvVars(envVars []cocoa.EnvironmentVariable) []*ecs.KeyValuePair {
	var converted []*ecs.KeyValuePair

	for _, envVar := range envVars {
		if envVar.SecretOpts != nil {
			continue
		}
		var pair ecs.KeyValuePair
		pair.SetName(utility.FromStringPtr(envVar.Name)).SetValue(utility.FromStringPtr(envVar.Value))
		converted = append(converted, &pair)
	}

	return converted
}

// exportSecrets converts environment variables backed by secrets into ECS
// Secrets.
func (pc *BasicECSPodCreator) exportSecrets(envVars []cocoa.EnvironmentVariable) []*ecs.Secret {
	var secrets []*ecs.Secret

	for _, envVar := range envVars {
		if envVar.SecretOpts == nil {
			continue
		}

		var secret ecs.Secret
		secret.SetName(utility.FromStringPtr(envVar.Name))
		secret.SetValueFrom(utility.FromStringPtr(envVar.SecretOpts.ID))
		secrets = append(secrets, &secret)
	}

	return secrets
}

// translateContainerResources translates the containers and stored secrets
// into the resources associated with each container.
func (pc *BasicECSPodCreator) translateContainerResources(containers []*ecs.Container, defs []cocoa.ECSContainerDefinition) []cocoa.ECSContainerResources {
	var resources []cocoa.ECSContainerResources

	for _, container := range containers {
		if container == nil {
			continue
		}

		name := utility.FromStringPtr(container.Name)
		res := cocoa.NewECSContainerResources().
			SetContainerID(utility.FromStringPtr(container.ContainerArn)).
			SetName(name).
			SetSecrets(pc.translateContainerSecrets(defs))
		resources = append(resources, *res)
	}

	return resources
}

// translateContainerSecrets translates the given secrets for a container into
// a slice of container secrets.
func (pc *BasicECSPodCreator) translateContainerSecrets(defs []cocoa.ECSContainerDefinition) []cocoa.ContainerSecret {
	var translated []cocoa.ContainerSecret

	for _, def := range defs {
		for _, envVar := range def.EnvVars {
			if envVar.SecretOpts == nil {
				continue
			}

			cs := cocoa.NewContainerSecret().
				SetID(utility.FromStringPtr(envVar.SecretOpts.ID)).
				SetOwned(utility.FromBoolPtr(envVar.SecretOpts.Owned))
			if name := utility.FromStringPtr(envVar.SecretOpts.Name); name != "" {
				cs.SetName(name)
			}
			translated = append(translated, *cs)
		}

		if def.RepoCreds != nil {
			cs := cocoa.NewContainerSecret().
				SetID(utility.FromStringPtr(def.RepoCreds.ID)).
				SetOwned(utility.FromBoolPtr(def.RepoCreds.Owned))
			if name := utility.FromStringPtr(def.RepoCreds.Name); name != "" {
				cs.SetName(name)
			}
			translated = append(translated, *cs)
		}

	}

	return translated
}

// translatePodStatusInfo translates an ECS task to its equivalent cocoa
// status information.
func translatePodStatusInfo(task *ecs.Task) cocoa.ECSPodStatusInfo {
	return *cocoa.NewECSPodStatusInfo().
		SetStatus(translateECSStatus(task.LastStatus)).
		SetContainers(translateContainerStatusInfo(task.Containers))
}

// translateContainerStatusInfo translates an ECS container to its equivalent
// cocoa container status information.
func translateContainerStatusInfo(containers []*ecs.Container) []cocoa.ECSContainerStatusInfo {
	var statuses []cocoa.ECSContainerStatusInfo

	for _, container := range containers {
		if container == nil {
			continue
		}
		status := cocoa.NewECSContainerStatusInfo().
			SetContainerID(utility.FromStringPtr(container.ContainerArn)).
			SetName(utility.FromStringPtr(container.Name)).
			SetStatus(translateECSStatus(container.LastStatus))
		statuses = append(statuses, *status)
	}

	return statuses
}

// translateECSStatus translate the ECS status into its equivalent cocoa
// status.
func translateECSStatus(status *string) cocoa.ECSStatus {
	if status == nil {
		return cocoa.StatusUnknown
	}
	switch *status {
	case TaskStatusProvisioning, TaskStatusPending, TaskStatusActivating:
		return cocoa.StatusStarting
	case TaskStatusRunning:
		return cocoa.StatusRunning
	case TaskStatusDeactivating, TaskStatusStopping, TaskStatusDeprovisioning, TaskStatusStopped:
		return cocoa.StatusStopped
	default:
		return cocoa.StatusUnknown
	}
}

// exportPodCreationOptions converts options to create a pod into its equivalent
// ECS task definition.
func (pc *BasicECSPodCreator) exportPodCreationOptions(opts cocoa.ECSPodCreationOptions) *ecs.RegisterTaskDefinitionInput {
	var taskDef ecs.RegisterTaskDefinitionInput

	var containerDefs []*ecs.ContainerDefinition
	for _, def := range opts.ContainerDefinitions {
		containerDefs = append(containerDefs, pc.exportContainerDefinition(def))
	}
	taskDef.SetContainerDefinitions(containerDefs)

	if mem := utility.FromIntPtr(opts.MemoryMB); mem != 0 {
		taskDef.SetMemory(strconv.Itoa(mem))
	}

	if cpu := utility.FromIntPtr(opts.CPU); cpu != 0 {
		taskDef.SetCpu(strconv.Itoa(cpu))
	}

	if opts.NetworkMode != nil {
		taskDef.SetNetworkMode(string(*opts.NetworkMode))
	}

	taskDef.SetFamily(utility.FromStringPtr(opts.Name)).
		SetTaskRoleArn(utility.FromStringPtr(opts.TaskRole)).
		SetExecutionRoleArn(utility.FromStringPtr(opts.ExecutionRole)).
		SetTags(pc.exportTags(opts.Tags))

	return &taskDef
}

// exportContainerDefinition converts a container definition into an ECS
// container definition input.
func (pc *BasicECSPodCreator) exportContainerDefinition(def cocoa.ECSContainerDefinition) *ecs.ContainerDefinition {
	var containerDef ecs.ContainerDefinition
	if mem := utility.FromIntPtr(def.MemoryMB); mem != 0 {
		containerDef.SetMemory(int64(mem))
	}
	if cpu := utility.FromIntPtr(def.CPU); cpu != 0 {
		containerDef.SetCpu(int64(cpu))
	}
	if dir := utility.FromStringPtr(def.WorkingDir); dir != "" {
		containerDef.SetWorkingDirectory(dir)
	}
	containerDef.SetCommand(utility.ToStringPtrSlice(def.Command)).
		SetImage(utility.FromStringPtr(def.Image)).
		SetName(utility.FromStringPtr(def.Name)).
		SetEnvironment(pc.exportEnvVars(def.EnvVars)).
		SetSecrets(pc.exportSecrets(def.EnvVars)).
		SetRepositoryCredentials(pc.exportRepoCreds(def.RepoCreds)).
		SetPortMappings(pc.exportPortMappings(def.PortMappings))
	return &containerDef
}

// exportRepoCreds exports the repository credentials into ECS repository
// credentials.
func (pc *BasicECSPodCreator) exportRepoCreds(creds *cocoa.RepositoryCredentials) *ecs.RepositoryCredentials {
	if creds == nil {
		return nil
	}
	var converted ecs.RepositoryCredentials
	converted.SetCredentialsParameter(utility.FromStringPtr(creds.ID))
	return &converted
}

// exportTaskExecutionOptions converts execution options and a task definition
// into an ECS task execution input.
func (pc *BasicECSPodCreator) exportTaskExecutionOptions(opts cocoa.ECSPodExecutionOptions, taskDef cocoa.ECSTaskDefinition) *ecs.RunTaskInput {
	var runTask ecs.RunTaskInput
	runTask.SetCluster(utility.FromStringPtr(opts.Cluster)).
		SetTaskDefinition(utility.FromStringPtr(taskDef.ID)).
		SetTags(pc.exportTags(opts.Tags)).
		SetEnableExecuteCommand(utility.FromBoolPtr(opts.SupportsDebugMode)).
		SetPlacementStrategy(pc.exportStrategy(opts.PlacementOpts)).
		SetPlacementConstraints(pc.exportPlacementConstraints(opts.PlacementOpts)).
		SetNetworkConfiguration(pc.exportAWSVPCOptions(opts.AWSVPCOpts))
	if opts.PlacementOpts != nil && opts.PlacementOpts.Group != nil {
		runTask.SetGroup(utility.FromStringPtr(opts.PlacementOpts.Group))
	}
	return &runTask
}

// exportPortMappings converts port mappings into ECS port mappings.
func (pc *BasicECSPodCreator) exportPortMappings(mappings []cocoa.PortMapping) []*ecs.PortMapping {
	var converted []*ecs.PortMapping
	for _, pm := range mappings {
		mapping := &ecs.PortMapping{}
		if pm.ContainerPort != nil {
			mapping.SetContainerPort(int64(utility.FromIntPtr(pm.ContainerPort)))
		}
		if pm.HostPort != nil {
			mapping.SetHostPort(int64(utility.FromIntPtr(pm.HostPort)))
		}
		converted = append(converted, mapping)
	}
	return converted
}

// exportAWSVPCOptions converts AWSVPC options into ECS AWSVPC options.
func (pc *BasicECSPodCreator) exportAWSVPCOptions(opts *cocoa.AWSVPCOptions) *ecs.NetworkConfiguration {
	if opts == nil {
		return nil
	}

	var converted ecs.AwsVpcConfiguration
	if len(opts.Subnets) != 0 {
		converted.SetSubnets(utility.ToStringPtrSlice(opts.Subnets))
	}
	if len(opts.SecurityGroups) != 0 {
		converted.SetSecurityGroups(utility.ToStringPtrSlice(opts.SecurityGroups))
	}

	var networkConf ecs.NetworkConfiguration
	networkConf.SetAwsvpcConfiguration(&converted)

	return &networkConf
}
