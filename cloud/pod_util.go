package cloud

import (
	"context"
	"math"

	"github.com/aws/aws-sdk-go-v2/aws"
	ecsTypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	resourcegroupstaggingapiTypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/cocoa/awsutil"
	"github.com/evergreen-ci/cocoa/ecs"
	cocoaMock "github.com/evergreen-ci/cocoa/mock"
	"github.com/evergreen-ci/cocoa/secret"
	"github.com/evergreen-ci/cocoa/tag"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/model/pod/definition"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// MakeECSClient creates a cocoa.ECSClient to interact with ECS.
func MakeECSClient(ctx context.Context, settings *evergreen.Settings) (cocoa.ECSClient, error) {
	switch settings.Providers.AWS.Pod.SecretsManager.ClientType {
	case evergreen.AWSClientTypeMock:
		// This should only ever be used for testing purposes.
		return &cocoaMock.ECSClient{}, nil
	default:
		return ecs.NewBasicClient(ctx, podAWSOptions(settings))
	}
}

// MakeSecretsManagerClient creates a cocoa.SecretsManagerClient to interact
// with Secrets Manager.
func MakeSecretsManagerClient(ctx context.Context, settings *evergreen.Settings) (cocoa.SecretsManagerClient, error) {
	switch settings.Providers.AWS.Pod.SecretsManager.ClientType {
	case evergreen.AWSClientTypeMock:
		// This should only ever be used for testing purposes.
		return &cocoaMock.SecretsManagerClient{}, nil
	default:
		return secret.NewBasicSecretsManagerClient(ctx, podAWSOptions(settings))
	}
}

// MakeTagClient creates a cocoa.TagClient to interact with the Resource Groups
// Tagging API.
func MakeTagClient(ctx context.Context, settings *evergreen.Settings) (cocoa.TagClient, error) {
	return tag.NewBasicTagClient(ctx, podAWSOptions(settings))
}

const (
	// SecretsManagerResourceFilter is the name of the resource filter to find
	// Secrets Manager secrets.
	SecretsManagerResourceFilter = "secretsmanager:secret"
	// PodDefinitionResourceFilter is the name of the resource filter to find
	// ECS pod definitions.
	PodDefinitionResourceFilter = "ecs:task-definition"
)

// MakeSecretsManagerVault creates a cocoa.Vault backed by Secrets Manager with
// an optional cocoa.SecretCache.
func MakeSecretsManagerVault(c cocoa.SecretsManagerClient) (cocoa.Vault, error) {
	return secret.NewBasicSecretsManager(*secret.NewBasicSecretsManagerOptions().
		SetClient(c).
		SetCache(model.ContainerSecretCache{}))
}

// MakeECSPodDefinitionManager creates a cocoa.ECSPodDefinitionManager that
// creates pod definitions in ECS and secrets backed by an optional cocoa.Vault.
func MakeECSPodDefinitionManager(c cocoa.ECSClient, v cocoa.Vault) (cocoa.ECSPodDefinitionManager, error) {
	return ecs.NewBasicPodDefinitionManager(*ecs.NewBasicPodDefinitionManagerOptions().
		SetClient(c).
		SetVault(v).
		SetCache(definition.PodDefinitionCache{}))
}

// MakeECSPodCreator creates a cocoa.ECSPodCreator to create pods backed by ECS
// and secrets backed by an optional cocoa.Vault.
func MakeECSPodCreator(c cocoa.ECSClient, v cocoa.Vault) (cocoa.ECSPodCreator, error) {
	return ecs.NewBasicPodCreator(*ecs.NewBasicPodCreatorOptions().
		SetClient(c).
		SetVault(v).
		SetCache(definition.PodDefinitionCache{}))
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

	opts := ecs.NewBasicPodOptions().
		SetClient(c).
		SetVault(v).
		SetResources(res).
		SetStatusInfo(*stat)

	return ecs.NewBasicPod(opts)
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
		taskDef := cocoa.NewECSTaskDefinition().SetID(info.DefinitionID)
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
		s := cocoa.NewContainerSecret().SetID(id)
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
	// awsLogsGroup is the log configuration option name for specifying the log group.
	awsLogsGroup = "awslogs-group"
	// awsLogsGroup is the log configuration option name for specifying the AWS region.
	awsLogsRegion = "awslogs-region"
	// awsLogsStreamPrefix is the log configuration option name for specifying the log stream prefix.
	awsLogsStreamPrefix = "awslogs-stream-prefix"
)

// ExportECSPodDefinitionOptions exports the ECS pod creation options into
// cocoa.ECSPodDefinitionOptions to create the pod definition.
func ExportECSPodDefinitionOptions(settings *evergreen.Settings, opts pod.TaskContainerCreationOptions) (*cocoa.ECSPodDefinitionOptions, error) {
	ecsConf := settings.Providers.AWS.Pod.ECS

	containerDef, err := exportECSPodContainerDef(settings, opts)
	if err != nil {
		return nil, errors.Wrap(err, "exporting pod container definition")
	}

	defOpts := cocoa.NewECSPodDefinitionOptions().
		SetName(opts.GetFamily(ecsConf)).
		SetCPU(opts.CPU).
		SetMemoryMB(opts.MemoryMB).
		SetTaskRole(ecsConf.TaskRole).
		SetExecutionRole(ecsConf.ExecutionRole).
		AddContainerDefinitions(*containerDef)
	if len(ecsConf.AWSVPC.Subnets) != 0 || len(ecsConf.AWSVPC.SecurityGroups) != 0 {
		defOpts.SetNetworkMode(cocoa.NetworkModeAWSVPC)
	}

	if err := defOpts.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid pod definition options")
	}

	return defOpts, nil
}

// exportECSPodContainerDef exports the ECS pod container definition into the
// equivalent cocoa.ECSContainerDefintion.
func exportECSPodContainerDef(settings *evergreen.Settings, opts pod.TaskContainerCreationOptions) (*cocoa.ECSContainerDefinition, error) {
	ecsConf := settings.Providers.AWS.Pod.ECS
	def := cocoa.NewECSContainerDefinition().
		SetName(agentContainerName).
		SetImage(opts.Image).
		SetMemoryMB(opts.MemoryMB).
		SetCPU(opts.CPU).
		SetWorkingDir(opts.WorkingDir).
		SetCommand(bootstrapContainerCommand(settings, opts)).
		SetEnvironmentVariables(exportPodEnvSecrets(opts)).
		AddPortMappings(*cocoa.NewPortMapping().SetContainerPort(agentPort))

	if opts.RepoCredsExternalID != "" {
		def.SetRepositoryCredentials(*cocoa.NewRepositoryCredentials().SetID(opts.RepoCredsExternalID))
	}
	if ecsConf.LogRegion != "" && ecsConf.LogGroup != "" && ecsConf.LogStreamPrefix != "" {
		def.SetLogConfiguration(*cocoa.NewLogConfiguration().SetLogDriver(string(ecsTypes.LogDriverAwslogs)).SetOptions(map[string]string{
			awsLogsGroup:        ecsConf.LogGroup,
			awsLogsRegion:       ecsConf.LogRegion,
			awsLogsStreamPrefix: ecsConf.LogStreamPrefix,
		}))
	}

	return def, nil
}

// ExportECSPodExecutionOptions exports the ECS configuration into
// cocoa.ECSPodExecutionOptions.
func ExportECSPodExecutionOptions(ecsConfig evergreen.ECSConfig, containerOpts pod.TaskContainerCreationOptions) (*cocoa.ECSPodExecutionOptions, error) {
	execOpts := cocoa.NewECSPodExecutionOptions().
		SetOverrideOptions(exportECSOverridePodDef(containerOpts)).
		// This enables the ability to connect directly to a running container
		// in ECS (e.g. similar to SSH'ing into a host), which is convenient for
		// debugging issues.
		SetSupportsDebugMode(true)

	if len(ecsConfig.AWSVPC.Subnets) != 0 || len(ecsConfig.AWSVPC.SecurityGroups) != 0 {
		execOpts.SetAWSVPCOptions(*cocoa.NewAWSVPCOptions().
			SetSubnets(ecsConfig.AWSVPC.Subnets).
			SetSecurityGroups(ecsConfig.AWSVPC.SecurityGroups))
	}

	// Pods need to run inside container instances that have a compatible
	// environment, so specifying the capacity provider essentially specifies
	// the host environment it must run inside.
	var foundCapacityProvider bool
	for _, cp := range ecsConfig.CapacityProviders {
		if containerOpts.OS == pod.OSWindows && !containerOpts.WindowsVersion.Matches(cp.WindowsVersion) {
			continue
		}
		if containerOpts.OS.Matches(cp.OS) && containerOpts.Arch.Matches(cp.Arch) {
			execOpts.SetCapacityProvider(cp.Name)
			foundCapacityProvider = true
			break
		}
	}
	if !foundCapacityProvider {
		if containerOpts.OS == pod.OSWindows {
			return nil, errors.Errorf("container OS '%s' with version '%s' and arch '%s' did not match any recognized capacity provider", containerOpts.OS, containerOpts.WindowsVersion, containerOpts.Arch)
		}
		return nil, errors.Errorf("container OS '%s' and arch '%s' did not match any recognized capacity provider", containerOpts.OS, containerOpts.Arch)
	}

	var foundCluster bool
	for _, cluster := range ecsConfig.Clusters {
		if containerOpts.OS.Matches(cluster.OS) {
			execOpts.SetCluster(cluster.Name)
			foundCluster = true
			break
		}
	}
	if !foundCluster {
		return nil, errors.Errorf("container OS '%s' did not match any recognized ECS cluster", containerOpts.OS)
	}

	if err := execOpts.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid options")
	}

	return execOpts, nil
}

// exportECSOverridePodDef exports the pod definition options that should be
// overridden when starting the pod. It explicitly overrides the pod
// definitions's environment variables to inject those that are pod-specific.
func exportECSOverridePodDef(opts pod.TaskContainerCreationOptions) cocoa.ECSOverridePodDefinitionOptions {
	overrideContainerDef := cocoa.NewECSOverrideContainerDefinition().SetName(agentContainerName)

	for name, value := range opts.EnvVars {
		overrideContainerDef.AddEnvironmentVariables(*cocoa.NewKeyValue().
			SetName(name).
			SetValue(value))
	}

	return *cocoa.NewECSOverridePodDefinitionOptions().AddContainerDefinitions(*overrideContainerDef)
}

// ExportECSPodDefinition exports the pod definition into an
// cocoa.ECSTaskDefinition.
func ExportECSPodDefinition(podDef definition.PodDefinition) cocoa.ECSTaskDefinition {
	return *cocoa.NewECSTaskDefinition().SetID(podDef.ExternalID)
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

// exportPodEnvSecrets converts the secret environment variables into to a slice
// of cocoa.EnvironmentVariables.
func exportPodEnvSecrets(opts pod.TaskContainerCreationOptions) []cocoa.EnvironmentVariable {
	var allEnvVars []cocoa.EnvironmentVariable

	// This intentionally does not set the plaintext environment variables
	// because some of them (such as the pod ID) vary between each pod. If they
	// were included in the pod definition, it would reduce the reusability of
	// pod definitions.
	// Instead of setting these per-pod values in the pod definition, these
	// environment variables are injected via overriding options when the pod is
	// started.

	for envVarName, s := range opts.EnvSecrets {
		secretOpts := cocoa.NewSecretOptions().SetID(s.ExternalID)

		allEnvVars = append(allEnvVars, *cocoa.NewEnvironmentVariable().
			SetName(envVarName).
			SetSecretOptions(*secretOpts))
	}

	return allEnvVars
}

// GetFilteredResourceIDs gets resources that match the given resource and tag
// filters. If the limit is positive, it will return at most that many results.
// If the limit is zero, this will return no results. If the limit is negative,
// the results are unlimited
func GetFilteredResourceIDs(ctx context.Context, c cocoa.TagClient, resources []string, tags map[string][]string, limit int) ([]string, error) {
	if limit == 0 {
		return []string{}, nil
	}
	if limit < 0 {
		limit = math.MaxInt64
	}

	var tagFilters []resourcegroupstaggingapiTypes.TagFilter
	for key, vals := range tags {
		tagFilters = append(tagFilters, resourcegroupstaggingapiTypes.TagFilter{
			Key:    aws.String(key),
			Values: vals,
		})
	}

	var allIDs []string
	remaining := limit
	var nextToken *string
	for {
		var (
			ids []string
			err error
		)
		ids, nextToken, err = getResourcesPage(ctx, c, resources, tagFilters, nextToken, limit)
		if err != nil {
			return nil, errors.Wrap(err, "getting resources matching filters")
		}
		allIDs = append(allIDs, ids...)
		remaining = remaining - len(ids)
		if remaining <= 0 {
			break
		}
		if len(ids) == 0 {
			break
		}
		if nextToken == nil {
			break
		}
	}

	return allIDs, nil
}

func getResourcesPage(ctx context.Context, c cocoa.TagClient, resourceFilters []string, tagFilters []resourcegroupstaggingapiTypes.TagFilter, nextToken *string, limit int) ([]string, *string, error) {
	var ids []string

	resp, err := c.GetResources(ctx, &resourcegroupstaggingapi.GetResourcesInput{
		PaginationToken:     nextToken,
		ResourceTypeFilters: resourceFilters,
		TagFilters:          tagFilters,
	})
	if err != nil {
		return nil, nil, errors.Wrap(err, "getting resources")
	}
	if resp == nil {
		return nil, nil, errors.Errorf("unexpected nil response for getting resources")
	}

	for _, tagMapping := range resp.ResourceTagMappingList {
		if len(ids) >= limit {
			break
		}

		if tagMapping.ResourceARN == nil {
			continue
		}

		ids = append(ids, utility.FromStringPtr(tagMapping.ResourceARN))
	}

	return ids, resp.PaginationToken, nil
}

// NoopECSPodDefinitionCache is an implementation of cocoa.ECSPodDefinitionCache
// that no-ops for all operations.
type NoopECSPodDefinitionCache struct{}

// Put is a no-op.
func (c *NoopECSPodDefinitionCache) Put(context.Context, cocoa.ECSPodDefinitionItem) error {
	return nil
}

// Delete is a no-op.
func (c *NoopECSPodDefinitionCache) Delete(context.Context, string) error {
	return nil
}

// NoopSecretCache is an implementation of cocoa.SecretCache that no-ops for all
// operations.
type NoopSecretCache struct {
	Tag string
}

// Put is a no-op.
func (c *NoopSecretCache) Put(context.Context, cocoa.SecretCacheItem) error {
	return nil
}

// Delete is a no-op.
func (c *NoopSecretCache) Delete(context.Context, string) error {
	return nil
}

// GetTag returns the tag field.
func (c *NoopSecretCache) GetTag() string {
	return c.Tag
}
