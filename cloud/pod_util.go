package cloud

import (
	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/cocoa/awsutil"
	"github.com/evergreen-ci/cocoa/ecs"
	"github.com/evergreen-ci/cocoa/secret"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/pkg/errors"
)

// MakeECSClient creates a cocoa.ECSClient to interact with ECS.
func MakeECSClient(settings *evergreen.Settings) (cocoa.ECSClient, error) {
	return ecs.NewBasicECSClient(podAWSOptions(settings))
}

// MakeSecretsManagerClient creates a cocoa.SecretsManagerClient to interact
// with Secrets Manager.
func MakeSecretsManagerClient(settings *evergreen.Settings) (cocoa.SecretsManagerClient, error) {
	return secret.NewBasicSecretsManagerClient(podAWSOptions(settings))
}

// MakeSecretsManagerVault creates a cocoa.Vault backed by Secrets Manager.
func MakeSecretsManagerVault(c cocoa.SecretsManagerClient) cocoa.Vault {
	return secret.NewBasicSecretsManager(c)
}

// MakeECSPodCreator creates a cocoa.ECSPodCreator to create pods backed by ECS and secrets backed by a secret Vault.
func MakeECSPodCreator(c cocoa.ECSClient, v cocoa.Vault) (cocoa.ECSPodCreator, error) {
	return ecs.NewBasicECSPodCreator(c, v)
}

// ExportPod exports the pod DB model to its equivalent cocoa.ECSPod backed by
// the given ECS client and secret vault.
func ExportPod(p *pod.Pod, c cocoa.ECSClient, v cocoa.Vault) (cocoa.ECSPod, error) {
	stat, err := ExportECSPodStatusInfo(p)
	if err != nil {
		return nil, errors.Wrap(err, "exporting pod status info")
	}

	res := ExportPodResources(p.Resources)
	if err != nil {
		return nil, errors.Wrap(err, "exporting pod resources")
	}

	opts := ecs.NewBasicECSPodOptions().
		SetClient(c).
		SetVault(v).
		SetResources(res).
		SetStatusInfo(*stat)

	return ecs.NewBasicECSPod(opts)
}

// ExportECSPodStatusInfo exports the pod's status information to its equivalent
// cocoa.ECSPodStatusInfo.
func ExportECSPodStatusInfo(p *pod.Pod) (*cocoa.ECSPodStatusInfo, error) {
	podStatus, err := ExportECSPodStatus(p.Status)
	if err != nil {
		return nil, errors.Wrap(err, "exporting pod status")
	}

	stat := cocoa.NewECSPodStatusInfo().SetStatus(podStatus)

	for _, container := range p.Resources.Containers {
		containerStatus, err := ExportECSContainerStatusInfo(container)
		if err != nil {
			return nil, errors.Wrapf(err, "exporting container status info for container '%s'", container.Name)
		}
		stat.AddContainers(*containerStatus)
	}

	return stat, nil
}

// ExportECSContainerStatusInfo exports the container's resource and status
// information into its equivalent cocoa.ECSContainerStatusInfo.
func ExportECSContainerStatusInfo(info pod.ContainerResourceInfo) (*cocoa.ECSContainerStatusInfo, error) {
	status, err := ExportECSContainerStatus(info.Status)
	if err != nil {
		return nil, errors.Wrap(err, "exporting container status")
	}
	return cocoa.NewECSContainerStatusInfo().
		SetName(info.Name).
		SetContainerID(info.ExternalID).
		SetStatus(status), nil
}

// ExportECSStatus exports the pod status to the equivalent cocoa.ECSStatus.
func ExportECSPodStatus(s pod.Status) (cocoa.ECSStatus, error) {
	switch s {
	case pod.StatusInitializing:
		return cocoa.StatusUnknown, errors.Errorf("a pod that is initializing does not exist in ECS yet")
	case pod.StatusStarting:
		return cocoa.StatusStarting, nil
	case pod.StatusRunning:
		return cocoa.StatusRunning, nil
	case pod.StatusTerminated:
		return cocoa.StatusDeleted, nil
	default:
		return cocoa.StatusUnknown, errors.Errorf("no equivalent ECS status for pod status '%s'", s)
	}
}

// ExportECSContainerStatus exports the container status to the equivalent
// cocoa.ECSStatus.
func ExportECSContainerStatus(s pod.ContainerStatus) (cocoa.ECSStatus, error) {
	switch s {
	case pod.ContainerStatusStarting:
		return cocoa.StatusStarting, nil
	case pod.ContainerStatusRunning:
		return cocoa.StatusRunning, nil
	case pod.ContainerStatusStopped:
		return cocoa.StatusStopped, nil
	default:
		return cocoa.StatusUnknown, errors.Errorf("no equivalent ECS status for container status '%s'", s)
	}
}

// ImportECSContainerStatus converts the cooca.ECSStatus into its equivalent
// container status.
func ImportECSContainerStatus(s cocoa.ECSStatus) (pod.ContainerStatus, error) {
	switch s {
	case cocoa.StatusStarting:
		return pod.ContainerStatusStarting, nil
	case cocoa.StatusRunning:
		return pod.ContainerStatusRunning, nil
	case cocoa.StatusStopped, cocoa.StatusDeleted:
		return pod.ContainerStatusStopped, nil
	default:
		return "", errors.Errorf("no equivalent container status for ECS status '%s'", s)
	}
}

// ExportPodResources exports the ECS pod resource information into the
// equivalent cocoa.ECSPodResources.
func ExportPodResources(info pod.ResourceInfo) cocoa.ECSPodResources {
	res := cocoa.NewECSPodResources()

	for _, container := range info.Containers {
		res.AddContainers(ExportECSContainerResources(container))
	}

	if info.ExternalID != "" {
		res.SetTaskID(info.ExternalID)
	}

	if info.DefinitionID != "" {
		taskDef := cocoa.NewECSTaskDefinition().
			SetID(info.DefinitionID).
			SetOwned(true)
		res.SetTaskDefinition(*taskDef)
	}

	if info.Cluster != "" {
		res.SetCluster(info.Cluster)
	}

	return *res
}

// ExportECSContainerResources exports the ECS container resource information
// into the equivalent cocoa.ECSContainerResources.
func ExportECSContainerResources(info pod.ContainerResourceInfo) cocoa.ECSContainerResources {
	res := cocoa.NewECSContainerResources().
		SetContainerID(info.ExternalID).
		SetName(info.Name)

	for _, id := range info.SecretIDs {
		s := cocoa.NewContainerSecret().
			SetName(id).
			SetOwned(true)
		res.AddSecrets(*s)
	}

	return *res
}

// ExportPodContainerDef exports the ECS pod container definition into the equivalent cocoa.ECSContainerDefintion.
func ExportPodContainerDef(opts pod.TaskContainerCreationOptions) (*cocoa.ECSContainerDefinition, error) {
	return cocoa.NewECSContainerDefinition().
		SetImage(opts.Image).
		SetMemoryMB(opts.MemoryMB).
		SetCPU(opts.CPU).
		SetEnvironmentVariables(exportEnvVars(opts.EnvVars, opts.EnvSecrets)), nil
}

// ExportPodExecutionOptions exports the ECS configuration into cocoa.ECSPodExecutionOptions.
func ExportPodExecutionOptions(ecsConfig evergreen.ECSConfig, podOS pod.OS) (*cocoa.ECSPodExecutionOptions, error) {
	if len(ecsConfig.Clusters) == 0 {
		return nil, errors.New("must specify at least one cluster to use")
	}

	for _, cluster := range ecsConfig.Clusters {
		if string(podOS) == string(cluster.Platform) {
			return cocoa.NewECSPodExecutionOptions().SetCluster(cluster.Name), nil
		}
	}

	return nil, errors.Errorf("pod OS ('%s') did not match any ECS cluster platforms", string(podOS))
}

// ExportPodCreationOptions exports the ECS pod resources into cocoa.ECSPodExecutionOptions.
func ExportPodCreationOptions(ecsConfig evergreen.ECSConfig, taskContainerCreationOpts pod.TaskContainerCreationOptions) (*cocoa.ECSPodCreationOptions, error) {
	execOpts, err := ExportPodExecutionOptions(ecsConfig, taskContainerCreationOpts.OS)
	if err != nil {
		return nil, errors.Wrap(err, "exporting pod execution options")
	}

	containerDef, err := ExportPodContainerDef(taskContainerCreationOpts)
	if err != nil {
		return nil, errors.Wrap(err, "exporting pod container definition")
	}

	return cocoa.NewECSPodCreationOptions().
		SetTaskRole(ecsConfig.TaskRole).
		SetExecutionRole(ecsConfig.ExecutionRole).
		SetExecutionOptions(*execOpts).
		AddContainerDefinitions(*containerDef), nil
}

// podAWSOptions creates options to initialize an AWS client for pod management.
func podAWSOptions(settings *evergreen.Settings) awsutil.ClientOptions {
	opts := awsutil.NewClientOptions().SetRetryOptions(awsClientDefaultRetryOptions())
	if region := settings.Providers.AWS.Pod.Region; region != "" {
		opts.SetRegion(region)
	}
	if role := settings.Providers.AWS.Pod.Role; role != "" {
		opts.SetRole(role)
	}

	return *opts
}

// exportEnvVars converts a map of environment variables and a map of secrets to a slice of cocoa.EnvironmentVariables.
func exportEnvVars(envVars map[string]string, secrets map[string]string) []cocoa.EnvironmentVariable {
	var allEnvVars []cocoa.EnvironmentVariable

	for k, v := range envVars {
		allEnvVars = append(allEnvVars, *cocoa.NewEnvironmentVariable().SetName(k).SetValue(v))
	}

	for k, v := range secrets {
		allEnvVars = append(allEnvVars, *cocoa.NewEnvironmentVariable().SetName(k).SetSecretOptions(
			*cocoa.NewSecretOptions().
				SetName(k).
				SetValue(v).
				SetExists(false).
				SetOwned(true)))
	}

	return allEnvVars
}
