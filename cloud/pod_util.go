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

// MakeECSPodCreator creates a cocoa.ECSPodCreator to create pods with ECS.
func MakeECSPodCreator(c cocoa.ECSClient, v cocoa.Vault) (cocoa.ECSPodCreator, error) {
	return ecs.NewBasicECSPodCreator(c, v)
}

// ExportPod exports the pod DB model to its equivalent cocoa.ECSPod backed by
// the given ECS client and secret vault.
func ExportPod(p *pod.Pod, c cocoa.ECSClient, v cocoa.Vault) (cocoa.ECSPod, error) {
	status, err := ExportPodStatus(p.Status)
	if err != nil {
		return nil, errors.Wrap(err, "exporting pod status")
	}

	res := ExportPodResources(p.Resources)
	if err != nil {
		return nil, errors.Wrap(err, "exporting pod resources")
	}

	opts := ecs.NewBasicECSPodOptions().
		SetClient(c).
		SetVault(v).
		SetResources(res).
		SetStatus(status)

	return ecs.NewBasicECSPod(opts)
}

// ExportStatus exports the pod status to the equivalent cocoa.ECSPodStatus.
func ExportPodStatus(s pod.Status) (cocoa.ECSPodStatus, error) {
	switch s {
	case pod.StatusInitializing:
		return "", errors.Errorf("a pod that is initializing does not exist in ECS yet")
	case pod.StatusStarting:
		return cocoa.StartingStatus, nil
	case pod.StatusRunning:
		return cocoa.RunningStatus, nil
	case pod.StatusTerminated:
		return cocoa.DeletedStatus, nil
	default:
		return "", errors.Errorf("no equivalent ECS status for pod status '%s'", s)
	}
}

// ExportPodResources exports the ECS pod resource information into the
// equivalent cocoa.ECSPodResources.
func ExportPodResources(info pod.ResourceInfo) cocoa.ECSPodResources {
	var secrets []cocoa.PodSecret
	for _, id := range info.SecretIDs {
		s := cocoa.NewPodSecret().
			SetName(id).
			SetOwned(true)
		secrets = append(secrets, *s)
	}

	res := cocoa.NewECSPodResources().SetSecrets(secrets)

	if info.ID != "" {
		res.SetTaskID(info.ID)
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

// ExportPodContainerDef exports the ECS pod container definition into the equivalent cocoa.ECSContainerDefintion.
func ExportPodContainerDef(opts pod.TaskContainerCreationOptions, secretIDs []string) cocoa.ECSContainerDefinition {
	return *cocoa.NewECSContainerDefinition().
		SetImage(opts.Image).
		SetMemoryMB(opts.MemoryMB).
		SetCPU(opts.CPU).
		SetEnvironmentVariables(toEnvVars(opts.EnvVars, opts.EnvSecrets, secretIDs))
}

// ExportPodTaskDef exports the ECS pod resources into cocoa.ECSTaskDefinition.
func ExportPodTaskDef(resources pod.ResourceInfo) cocoa.ECSTaskDefinition {
	return *cocoa.NewECSTaskDefinition().
		SetID(resources.DefinitionID).
		SetOwned(false)
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

// toEnvVars converts a map of environment variables and a map of secrets to a slice of cocoa.EnvironmentVariables.
func toEnvVars(envVars map[string]string, secrets map[string]string, secretIDs []string) []cocoa.EnvironmentVariable {
	allEnvVars := []cocoa.EnvironmentVariable{}

	for k, v := range envVars {
		allEnvVars = append(allEnvVars, *cocoa.NewEnvironmentVariable().SetName(k).SetValue(v))
	}

	for k, v := range secrets {
		owned := false

		for _, id := range secretIDs {
			if id == k {
				owned = true
				break
			}
		}

		allEnvVars = append(allEnvVars, *cocoa.NewEnvironmentVariable().SetSecretOptions(
			*cocoa.NewSecretOptions().
				SetName(k).
				SetValue(v).
				SetExists(false).
				SetOwned(owned)))
	}

	return allEnvVars
}
