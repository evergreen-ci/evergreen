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
func (pc *BasicECSPodCreator) CreatePod(ctx context.Context, opts ...*cocoa.ECSPodCreationOptions) (cocoa.ECSPod, error) {
	mergedPodCreationOpts := cocoa.MergeECSPodCreationOptions(opts...)
	mergedPodExecutionOpts := cocoa.MergeECSPodExecutionOptions(mergedPodCreationOpts.ExecutionOpts)

	if err := mergedPodCreationOpts.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid pod creation options")
	}

	if err := mergedPodExecutionOpts.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid pod execution options")
	}

	secrets := pc.getSecrets(mergedPodCreationOpts)

	repoCreds, err := pc.getRepoCreds(mergedPodCreationOpts)
	if err != nil {
		return nil, errors.Wrap(err, "getting secret repository credentials")
	}

	resolvedSecrets, err := pc.createSecrets(ctx, pc.mergeSecrets(secrets, repoCreds))
	if err != nil {
		return nil, errors.Wrap(err, "creating secret repository credentials")
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
		SetContainers(pc.translateContainerResources(runOut.Tasks[0].Containers, resolvedSecrets)).
		SetTaskDefinition(*taskDef).
		SetTaskID(utility.FromStringPtr(runOut.Tasks[0].TaskArn))

	podOpts := NewBasicECSPodOptions().
		SetClient(pc.client).
		SetVault(pc.vault).
		SetStatusInfo(pc.translatePodStatusInfo(runOut.Tasks[0])).
		SetResources(*resources)

	p, err := NewBasicECSPod(podOpts)
	if err != nil {
		return nil, errors.Wrap(err, "creating pod")
	}

	return p, nil
}

// CreatePodFromExistingDefinition creates a new pod backed by AWS ECS from an
// existing definition.
func (pc *BasicECSPodCreator) CreatePodFromExistingDefinition(ctx context.Context, def cocoa.ECSTaskDefinition, opts ...*cocoa.ECSPodExecutionOptions) (cocoa.ECSPod, error) {
	return nil, errors.New("TODO: implement")
}

// getSecrets retrieves the secrets from the secret environment variables for
// each container.
func (pc *BasicECSPodCreator) getSecrets(opts cocoa.ECSPodCreationOptions) map[string][]cocoa.SecretOptions {
	containerSecrets := map[string][]cocoa.SecretOptions{}

	for _, def := range opts.ContainerDefinitions {
		containerName := utility.FromStringPtr(def.Name)
		for _, envVar := range def.EnvVars {
			if envVar.SecretOpts != nil {
				containerSecrets[containerName] = append(containerSecrets[containerName], *envVar.SecretOpts)
			}
		}
	}

	return containerSecrets
}

// createSecrets creates secrets that do not already exist for each container.
func (pc *BasicECSPodCreator) createSecrets(ctx context.Context, containerSecrets map[string][]cocoa.SecretOptions) (map[string][]cocoa.SecretOptions, error) {
	resolvedSecrets := map[string][]cocoa.SecretOptions{}

	for containerName, secrets := range containerSecrets {
		for _, opts := range secrets {
			resolvedOpts, err := pc.createSecret(ctx, opts)
			if err != nil {
				return nil, errors.Wrapf(err, "creating secret '%s' for container '%s'", utility.FromStringPtr(opts.ContainerSecret.Name), containerName)
			}
			resolvedSecrets[containerName] = append(resolvedSecrets[containerName], *resolvedOpts)
		}
	}

	return resolvedSecrets, nil
}

// createSecret creates a single secret for a container if it does not exist
// yet.
func (pc *BasicECSPodCreator) createSecret(ctx context.Context, secret cocoa.SecretOptions) (*cocoa.SecretOptions, error) {
	if utility.FromBoolPtr(secret.Exists) {
		return &secret, nil
	}
	if pc.vault == nil {
		return nil, errors.New("no vault was specified")
	}
	arn, err := pc.vault.CreateSecret(ctx, secret.ContainerSecret.NamedSecret)
	if err != nil {
		return nil, err
	}
	// Pods must use the secret's ARN once the secret is created
	// because that uniquely identifies the resource.
	secret.SetName(arn)

	return &secret, nil
}

// getRepoCreds retrieves the secret repository credentials for each container.
func (pc *BasicECSPodCreator) getRepoCreds(opts cocoa.ECSPodCreationOptions) (map[string]cocoa.SecretOptions, error) {
	containerCreds := map[string]cocoa.SecretOptions{}

	for _, def := range opts.ContainerDefinitions {
		if def.RepoCreds == nil {
			continue
		}
		opts := cocoa.NewSecretOptions().
			SetName(utility.FromStringPtr(def.RepoCreds.SecretName)).
			SetOwned(utility.FromBoolPtr(def.RepoCreds.Owned))
		if def.RepoCreds.NewCreds != nil {
			val, err := json.Marshal(def.RepoCreds.NewCreds)
			if err != nil {
				return nil, errors.Wrap(err, "formatting new credentials to create")
			}
			opts.SetValue(string(val)).SetExists(false)
		} else {
			opts.SetExists(true)
		}
		containerCreds[utility.FromStringPtr(def.Name)] = *opts
	}

	return containerCreds, nil
}

// mergeSecrets merges the secrets for each container with the secret repository
// credentials for each container.
func (pc *BasicECSPodCreator) mergeSecrets(secrets map[string][]cocoa.SecretOptions, repoCreds map[string]cocoa.SecretOptions) map[string][]cocoa.SecretOptions {
	merged := map[string][]cocoa.SecretOptions{}

	for containerName, secrets := range secrets {
		merged[containerName] = append(merged[containerName], secrets...)
	}
	for containerName, secret := range repoCreds {
		merged[containerName] = append(merged[containerName], secret)
	}

	return merged
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
		constraint.SetType("memberOf").SetExpression(filter)
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
		secret.SetValueFrom(utility.FromStringPtr(envVar.SecretOpts.Name))
		secrets = append(secrets, &secret)
	}

	return secrets
}

// translateContainerResources translates the stored secrets into the resources
// associated with each container.
func (pc *BasicECSPodCreator) translateContainerResources(containers []*ecs.Container, containerSecrets map[string][]cocoa.SecretOptions) []cocoa.ECSContainerResources {
	containerResourcesSet := map[string]cocoa.ECSContainerResources{}

	for _, container := range containers {
		if container == nil {
			continue
		}
		name := utility.FromStringPtr(container.Name)
		res := containerResourcesSet[name]
		res.SetContainerID(utility.FromStringPtr(container.ContainerArn)).SetName(name)
		containerResourcesSet[name] = res
	}

	for name, opts := range containerSecrets {
		res := containerResourcesSet[name]
		res.SetSecrets(pc.translateContainerSecrets(opts))
		containerResourcesSet[name] = res
	}

	var containerResources []cocoa.ECSContainerResources

	for name := range containerResourcesSet {
		containerResources = append(containerResources, containerResourcesSet[name])
	}

	return containerResources
}

func (pc *BasicECSPodCreator) translatePodStatusInfo(task *ecs.Task) cocoa.ECSPodStatusInfo {
	return *cocoa.NewECSPodStatusInfo().
		SetStatus(pc.translateECSStatus(task.LastStatus)).
		SetContainers(pc.translateContainerStatusInfo(task.Containers))
}

func (pc *BasicECSPodCreator) translateContainerStatusInfo(containers []*ecs.Container) []cocoa.ECSContainerStatusInfo {
	var statuses []cocoa.ECSContainerStatusInfo

	for _, container := range containers {
		if container == nil {
			continue
		}
		status := cocoa.NewECSContainerStatusInfo().
			SetContainerID(utility.FromStringPtr(container.ContainerArn)).
			SetName(utility.FromStringPtr(container.Name)).
			SetStatus(pc.translateECSStatus(container.LastStatus))
		statuses = append(statuses, *status)
	}

	return statuses
}

// translateECSStatus translate the ECS status into its equivalent cocoa
// status.
func (pc *BasicECSPodCreator) translateECSStatus(status *string) cocoa.ECSStatus {
	if status == nil {
		return cocoa.StatusUnknown
	}
	switch *status {
	case "PROVISIONING", "PENDING", "ACTIVATING":
		return cocoa.StatusStarting
	case "RUNNING":
		return cocoa.StatusRunning
	case "DEACTIVATING", "STOPPING", "DEPROVISIONING":
		return cocoa.StatusStopped
	default:
		return cocoa.StatusUnknown
	}
}

// translateContainerSecrets translates secret options into container secrets.
func (pc *BasicECSPodCreator) translateContainerSecrets(secrets []cocoa.SecretOptions) []cocoa.ContainerSecret {
	var containerSecrets []cocoa.ContainerSecret

	for _, secret := range secrets {
		containerSecrets = append(containerSecrets, secret.ContainerSecret)
	}

	return containerSecrets
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
		SetPortMappings(pc.exportPortMappings(def.PortMappings))
	return &containerDef
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
		SetPlacementConstraints(pc.exportPlacementConstraints(opts.PlacementOpts))
	return &runTask
}

// exportPortMappings converts port mappings into ECS port mappings.
func (pc *BasicECSPodCreator) exportPortMappings(mappings []cocoa.PortMapping) []*ecs.PortMapping {
	var converted []*ecs.PortMapping
	for _, pm := range mappings {
		mapping := &ecs.PortMapping{}
		mapping.SetContainerPort(int64(utility.FromIntPtr(pm.ContainerPort))).
			SetHostPort(int64(utility.FromIntPtr(pm.HostPort)))
		converted = append(converted, mapping)
	}
	return converted
}
