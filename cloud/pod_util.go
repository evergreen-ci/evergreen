package cloud

import (
	"strings"

	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/cocoa/awsutil"
	"github.com/evergreen-ci/cocoa/ecs"
	"github.com/evergreen-ci/cocoa/secret"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/utility"
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

// ExportECSPod exports the pod DB model to its equivalent cocoa.ECSPod backed
// by the given ECS client and secret vault.
func ExportECSPod(p *pod.Pod, c cocoa.ECSClient, v cocoa.Vault) (cocoa.ECSPod, error) {
	stat, err := exportECSPodStatusInfo(p)
	if err != nil {
		return nil, errors.Wrap(err, "exporting pod status info")
	}

	res := exportECSPodResources(p.Resources)
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

// exportECSPodStatusInfo exports the pod's status information to its equivalent
// cocoa.ECSPodStatusInfo.
func exportECSPodStatusInfo(p *pod.Pod) (*cocoa.ECSPodStatusInfo, error) {
	ps, err := exportECSPodStatus(p.Status)
	if err != nil {
		return nil, errors.Wrap(err, "exporting pod status")
	}

	status := cocoa.NewECSPodStatusInfo().SetStatus(ps)

	for _, container := range p.Resources.Containers {
		status.AddContainers(exportECSContainerStatusInfo(container))
	}

	return status, nil
}

// exportECSContainerStatusInfo exports the container's resource and status
// information into its equivalent cocoa.ECSContainerStatusInfo. The status is
// not currently tracked for containers, so it is set to unknown.
func exportECSContainerStatusInfo(info pod.ContainerResourceInfo) cocoa.ECSContainerStatusInfo {
	return *cocoa.NewECSContainerStatusInfo().
		SetName(info.Name).
		SetContainerID(info.ExternalID).
		SetStatus(cocoa.StatusUnknown)
}

// exportECSStatus exports the pod status to the equivalent cocoa.ECSStatus.
func exportECSPodStatus(s pod.Status) (cocoa.ECSStatus, error) {
	switch s {
	case pod.StatusInitializing:
		return cocoa.StatusUnknown, errors.Errorf("a pod that is initializing does not exist in ECS yet")
	case pod.StatusStarting:
		return cocoa.StatusStarting, nil
	case pod.StatusRunning, pod.StatusDecommissioned:
		return cocoa.StatusRunning, nil
	case pod.StatusTerminated:
		return cocoa.StatusDeleted, nil
	default:
		return cocoa.StatusUnknown, errors.Errorf("no equivalent ECS status for pod status '%s'", s)
	}
}

// exportECSPodResources exports the ECS pod resource information into the
// equivalent cocoa.ECSPodResources.
func exportECSPodResources(info pod.ResourceInfo) cocoa.ECSPodResources {
	res := cocoa.NewECSPodResources()

	for _, container := range info.Containers {
		res.AddContainers(exportECSContainerResources(container))
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

// ImportECSPodResources imports the ECS pod resource information into the
// equivalent pod.ResourceInfo.
func ImportECSPodResources(res cocoa.ECSPodResources) pod.ResourceInfo {
	var containerResources []pod.ContainerResourceInfo
	for _, c := range res.Containers {
		var secretIDs []string
		for _, secret := range c.Secrets {
			secretIDs = append(secretIDs, utility.FromStringPtr(secret.ID))
		}
		containerResources = append(containerResources, pod.ContainerResourceInfo{
			ExternalID: utility.FromStringPtr(c.ContainerID),
			Name:       utility.FromStringPtr(c.Name),
			SecretIDs:  secretIDs,
		})
	}

	var defID string
	if res.TaskDefinition != nil {
		defID = utility.FromStringPtr(res.TaskDefinition.ID)
	}

	return pod.ResourceInfo{
		ExternalID:   utility.FromStringPtr(res.TaskID),
		DefinitionID: defID,
		Cluster:      utility.FromStringPtr(res.Cluster),
		Containers:   containerResources,
	}

}

// exportECSContainerResources exports the ECS container resource information
// into the equivalent cocoa.ECSContainerResources.
func exportECSContainerResources(info pod.ContainerResourceInfo) cocoa.ECSContainerResources {
	res := cocoa.NewECSContainerResources().
		SetContainerID(info.ExternalID).
		SetName(info.Name)

	for _, id := range info.SecretIDs {
		s := cocoa.NewContainerSecret().
			SetID(id).
			SetOwned(true)
		res.AddSecrets(*s)
	}

	return *res
}

const (
	// agentContainerName is the standard name for the container that's running
	// the agent in a pod.
	agentContainerName = "evg-agent"
	// agentPort is the standard port that the agent runs on.
	agentPort = 2285
)

// ExportECSPodCreationOptions exports the ECS pod resources into
// cocoa.ECSPodExecutionOptions.
func ExportECSPodCreationOptions(settings *evergreen.Settings, p *pod.Pod) (*cocoa.ECSPodCreationOptions, error) {
	ecsConf := settings.Providers.AWS.Pod.ECS
	execOpts, err := exportECSPodExecutionOptions(ecsConf, p.TaskContainerCreationOpts.OS)
	if err != nil {
		return nil, errors.Wrap(err, "exporting pod execution options")
	}

	containerDef, err := exportECSPodContainerDef(settings, p)
	if err != nil {
		return nil, errors.Wrap(err, "exporting pod container definition")
	}

	opts := cocoa.NewECSPodCreationOptions().
		SetName(strings.Join([]string{strings.TrimRight(ecsConf.TaskDefinitionPrefix, "-"), "agent", p.ID}, "-")).
		SetTaskRole(ecsConf.TaskRole).
		SetExecutionRole(ecsConf.ExecutionRole).
		SetExecutionOptions(*execOpts).
		AddContainerDefinitions(*containerDef)

	if len(ecsConf.AWSVPC.Subnets) != 0 || len(ecsConf.AWSVPC.SecurityGroups) != 0 {
		opts.SetNetworkMode(cocoa.NetworkModeAWSVPC)
	}

	return opts, nil
}

// exportECSPodContainerDef exports the ECS pod container definition into the
// equivalent cocoa.ECSContainerDefintion.
func exportECSPodContainerDef(settings *evergreen.Settings, p *pod.Pod) (*cocoa.ECSContainerDefinition, error) {
	script, err := agentScript(settings, p)
	if err != nil {
		return nil, errors.Wrap(err, "building agent script")
	}
	return cocoa.NewECSContainerDefinition().
		SetName(agentContainerName).
		SetImage(p.TaskContainerCreationOpts.Image).
		SetMemoryMB(p.TaskContainerCreationOpts.MemoryMB).
		SetCPU(p.TaskContainerCreationOpts.CPU).
		SetWorkingDir(p.TaskContainerCreationOpts.WorkingDir).
		SetCommand(script).
		SetEnvironmentVariables(exportPodEnvVars(settings.Providers.AWS.Pod.SecretsManager, p)).
		AddPortMappings(*cocoa.NewPortMapping().SetContainerPort(agentPort)), nil
}

// exportECSPodExecutionOptions exports the ECS configuration into
// cocoa.ECSPodExecutionOptions.
func exportECSPodExecutionOptions(ecsConfig evergreen.ECSConfig, podOS pod.OS) (*cocoa.ECSPodExecutionOptions, error) {
	opts := cocoa.NewECSPodExecutionOptions()

	if len(ecsConfig.AWSVPC.Subnets) != 0 || len(ecsConfig.AWSVPC.SecurityGroups) != 0 {
		opts.SetAWSVPCOptions(*cocoa.NewAWSVPCOptions().
			SetSubnets(ecsConfig.AWSVPC.Subnets).
			SetSecurityGroups(ecsConfig.AWSVPC.SecurityGroups))
	}

	for _, cluster := range ecsConfig.Clusters {
		if string(podOS) == string(cluster.Platform) {
			return opts.SetCluster(cluster.Name), nil
		}
	}

	return nil, errors.Errorf("pod OS ('%s') did not match any ECS cluster platforms", string(podOS))
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

// exportPodEnvVars converts a map of environment variables and a map of secrets
// to a slice of cocoa.EnvironmentVariables.
func exportPodEnvVars(smConf evergreen.SecretsManagerConfig, p *pod.Pod) []cocoa.EnvironmentVariable {
	var allEnvVars []cocoa.EnvironmentVariable

	for k, v := range p.TaskContainerCreationOpts.EnvVars {
		allEnvVars = append(allEnvVars, *cocoa.NewEnvironmentVariable().SetName(k).SetValue(v))
	}

	for k, v := range p.TaskContainerCreationOpts.EnvSecrets {
		secretName := strings.Join([]string{strings.TrimRight(smConf.SecretPrefix, "/"), "agent", p.ID, k}, "/")
		allEnvVars = append(allEnvVars, *cocoa.NewEnvironmentVariable().SetName(k).SetSecretOptions(
			*cocoa.NewSecretOptions().
				SetName(secretName).
				SetNewValue(v).
				SetOwned(true)))
	}

	return allEnvVars
}
