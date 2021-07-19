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
// kim: TODO: test
func MakeECSClient(settings *evergreen.Settings) (cocoa.ECSClient, error) {
	return ecs.NewBasicECSClient(podAWSOptions(settings))
}

// MakeSecretsManagerClient creates a cocoa.SecretsManagerClient to interact
// with Secrets Manager.
// kim: TODO: test
func MakeSecretsManagerClient(settings *evergreen.Settings) (cocoa.SecretsManagerClient, error) {
	return secret.NewBasicSecretsManagerClient(podAWSOptions(settings))
}

// MakeSecretsManagerVault creates a cocoa.Vault backed by Secrets Manager.
// kim: TODO: test
func MakeSecretsManagerVault(settings *evergreen.Settings) (cocoa.Vault, error) {
	client, err := MakeSecretsManagerClient(settings)
	if err != nil {
		return nil, errors.Wrap(err, "initializing Secrets Manager client")
	}
	return secret.NewBasicSecretsManager(client), nil
}

// ExportPod exports the pod DB model to its equivalent cocoa.ECSPod backed by
// the given ECS client and secrets vault.
// kim: TODO: test
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
// kim: TODO: test
func ExportPodStatus(s pod.Status) (cocoa.ECSPodStatus, error) {
	switch s {
	case pod.InitializingStatus:
		return "", errors.Errorf("a pod that is initializing does not exist in ECS yet")
	case pod.StartingStatus:
		return cocoa.StartingStatus, nil
	case pod.RunningStatus:
		return cocoa.RunningStatus, nil
	case pod.TerminatedStatus:
		return cocoa.DeletedStatus, nil
	default:
		return "", errors.Errorf("no equivalent ECS status for pod status '%s'", s)
	}
}

// ExportPodResources exports the ECS pod resource information into the
// equivalent cocoa.ECSPodResources.
// kim: TODO: test
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

	if info.Cluster != "" {
		res.SetCluster(info.Cluster)
	}

	if info.DefinitionID != "" {
		taskDef := cocoa.NewECSTaskDefinition().
			SetID(info.DefinitionID).
			SetOwned(true)
		res.SetTaskDefinition(*taskDef)
	}

	return *res
}

// podAWSOptions creates options to initialize an AWS client for pod management.
// kim: TODO: test
func podAWSOptions(settings *evergreen.Settings) awsutil.ClientOptions {
	return *awsutil.NewClientOptions().
		SetRegion(settings.Providers.AWS.Pod.Region).
		SetRole(settings.Providers.AWS.Pod.Role).
		SetRetryOptions(awsClientDefaultRetryOptions())
}
