package ecs

import (
	"context"
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
func (m *BasicECSPodCreator) CreatePod(ctx context.Context, opts ...*cocoa.ECSPodCreationOptions) (cocoa.ECSPod, error) {
	mergedPodCreationOpts := cocoa.MergeECSPodCreationOptions(opts...)
	mergedPodExecutionOpts := cocoa.MergeECSPodExecutionOptions(mergedPodCreationOpts.ExecutionOpts)

	if err := mergedPodCreationOpts.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid pod creation options")
	}

	if err := mergedPodExecutionOpts.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid pod execution options")
	}

	secrets := m.getSecrets(mergedPodCreationOpts)

	if err := m.createSecrets(ctx, secrets); err != nil {
		return nil, errors.Wrap(err, "creating secrets")
	}

	taskDefinition := m.exportPodCreationOptions(mergedPodCreationOpts)

	registerOut, err := m.client.RegisterTaskDefinition(ctx, taskDefinition)
	if err != nil {
		return nil, errors.Wrap(err, "registering task definition")
	}

	if registerOut.TaskDefinition == nil || registerOut.TaskDefinition.TaskDefinitionArn == nil {
		return nil, errors.New("expected a task definition from ECS, but none was returned")
	}

	taskDef := cocoa.NewECSTaskDefinition().
		SetID(utility.FromStringPtr(registerOut.TaskDefinition.TaskDefinitionArn)).
		SetOwned(true)

	runTask := m.exportTaskExecutionOptions(mergedPodExecutionOpts, *taskDef)

	runOut, err := m.client.RunTask(ctx, runTask)
	if err != nil {
		return nil, errors.Wrapf(err, "running task for definition '%s' in cluster '%s'", utility.FromStringPtr(runTask.TaskDefinition), utility.FromStringPtr(runTask.Cluster))
	}

	if len(runOut.Failures) > 0 {
		catcher := grip.NewBasicCatcher()
		for _, failure := range runOut.Failures {
			catcher.Errorf("task '%s': %s: %s\n", *failure.Arn, *failure.Detail, *failure.Reason)
		}
		return nil, errors.Wrap(catcher.Resolve(), "running task")
	}

	if len(runOut.Tasks) == 0 || runOut.Tasks[0].TaskArn == nil {
		return nil, errors.New("expected a task to be running in ECS, but none was returned")
	}

	resources := cocoa.NewECSPodResources().
		SetCluster(utility.FromStringPtr(mergedPodExecutionOpts.Cluster)).
		SetSecrets(translatePodSecrets(secrets)).
		SetTaskDefinition(*taskDef).
		SetTaskID(utility.FromStringPtr(runOut.Tasks[0].TaskArn))

	options := NewBasicECSPodOptions().
		SetClient(m.client).
		SetVault(m.vault).
		SetStatus(cocoa.StatusRunning).
		SetResources(*resources)

	p, err := NewBasicECSPod(options)
	if err != nil {
		return nil, errors.Wrap(err, "creating pod")
	}

	return p, nil
}

// CreatePodFromExistingDefinition creates a new pod backed by AWS ECS from an
// existing definition.
func (m *BasicECSPodCreator) CreatePodFromExistingDefinition(ctx context.Context, def cocoa.ECSTaskDefinition, opts ...*cocoa.ECSPodExecutionOptions) (cocoa.ECSPod, error) {
	return nil, errors.New("TODO: implement")
}

// getSecrets retrieves the secrets from the secret environment variables for the pod.
func (m *BasicECSPodCreator) getSecrets(merged cocoa.ECSPodCreationOptions) []cocoa.SecretOptions {
	var secrets []cocoa.SecretOptions

	for _, def := range merged.ContainerDefinitions {
		for _, variable := range def.EnvVars {
			if variable.SecretOpts != nil {
				secrets = append(secrets, *variable.SecretOpts)
			}
		}
	}

	return secrets
}

// createSecrets creates secrets that do not already exist.
func (m *BasicECSPodCreator) createSecrets(ctx context.Context, secrets []cocoa.SecretOptions) error {
	for _, secret := range secrets {
		if !utility.FromBoolPtr(secret.Exists) {
			if m.vault == nil {
				return errors.New("no vault was specified")
			}
			arn, err := m.vault.CreateSecret(ctx, secret.PodSecret.NamedSecret)
			if err != nil {
				return err
			}
			// Pods must use the secret's ARN once the secret is created
			// because that uniquely identifies the resource.
			secret.SetName(arn)
		}
	}

	return nil
}

// exportTags converts a mapping of string-string into ECS tags.
func exportTags(tags map[string]string) []*ecs.Tag {
	var ecsTags []*ecs.Tag
	for k, v := range tags {
		ecsTag := &ecs.Tag{}
		ecsTag.SetKey(k).SetValue(v)
		ecsTags = append(ecsTags, ecsTag)
	}
	return ecsTags
}

// exportStrategy converts the strategy and parameter into an ECS placement
// strategy.
func exportStrategy(strategy *cocoa.ECSPlacementStrategy, param *cocoa.ECSStrategyParameter) []*ecs.PlacementStrategy {
	var placementStrat ecs.PlacementStrategy
	placementStrat.SetType(string(*strategy)).SetField(utility.FromStringPtr(param))
	return []*ecs.PlacementStrategy{&placementStrat}
}

// exportEnvVars converts the non-secret environment variables into ECS
// environment variables.
func exportEnvVars(envVars []cocoa.EnvironmentVariable) []*ecs.KeyValuePair {
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
func exportSecrets(envVars []cocoa.EnvironmentVariable) []*ecs.Secret {
	var secrets []*ecs.Secret
	for _, envVar := range envVars {
		if envVar.SecretOpts != nil {
			var secret ecs.Secret
			secret.SetName(utility.FromStringPtr(envVar.Name))
			secret.SetValueFrom(utility.FromStringPtr(envVar.SecretOpts.Name))
			secrets = append(secrets, &secret)
		}
	}
	return secrets
}

// translatePodSecrets translates secret options into pod secrets.
func translatePodSecrets(secrets []cocoa.SecretOptions) []cocoa.PodSecret {
	var podSecrets []cocoa.PodSecret

	for _, secret := range secrets {
		podSecrets = append(podSecrets, secret.PodSecret)
	}

	return podSecrets
}

// exportPodCreationOptions converts options to create a pod into its equivalent ECS task definition.
func (m *BasicECSPodCreator) exportPodCreationOptions(opts cocoa.ECSPodCreationOptions) *ecs.RegisterTaskDefinitionInput {
	var taskDef ecs.RegisterTaskDefinitionInput

	var containerDefs []*ecs.ContainerDefinition
	for _, def := range opts.ContainerDefinitions {
		containerDefs = append(containerDefs, exportContainerDefinition(def))
	}
	taskDef.SetContainerDefinitions(containerDefs)

	if mem := utility.FromIntPtr(opts.MemoryMB); mem != 0 {
		taskDef.SetMemory(strconv.Itoa(mem))
	}

	if cpu := utility.FromIntPtr(opts.CPU); cpu != 0 {
		taskDef.SetCpu(strconv.Itoa(cpu))
	}

	taskDef.SetFamily(utility.FromStringPtr(opts.Name)).
		SetTaskRoleArn(utility.FromStringPtr(opts.TaskRole)).
		SetExecutionRoleArn(utility.FromStringPtr(opts.ExecutionRole)).
		SetTags(exportTags(opts.Tags))

	return &taskDef
}

// exportContainerDefinition converts a container definition into an ECS
// container definition input.
func exportContainerDefinition(def cocoa.ECSContainerDefinition) *ecs.ContainerDefinition {
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
		SetEnvironment(exportEnvVars(def.EnvVars)).
		SetSecrets(exportSecrets(def.EnvVars))
	return &containerDef
}

// exportTaskExecutionOptions converts execution options and a task definition
// into an ECS task execution input.
func (m *BasicECSPodCreator) exportTaskExecutionOptions(opts cocoa.ECSPodExecutionOptions, taskDef cocoa.ECSTaskDefinition) *ecs.RunTaskInput {
	var runTask ecs.RunTaskInput
	runTask.SetCluster(utility.FromStringPtr(opts.Cluster)).
		SetTaskDefinition(utility.FromStringPtr(taskDef.ID)).
		SetTags(exportTags(opts.Tags)).
		SetEnableExecuteCommand(utility.FromBoolPtr(opts.SupportsDebugMode)).
		SetPlacementStrategy(exportStrategy(opts.PlacementOpts.Strategy, opts.PlacementOpts.StrategyParameter))
	return &runTask
}
