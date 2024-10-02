package data

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/githubapp"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// ListHostsForTask lists running hosts scoped to the task or the task's build.
func ListHostsForTask(ctx context.Context, taskID string) ([]host.Host, error) {
	env := evergreen.GetEnvironment()
	t, err := task.FindOneId(taskID)
	if err != nil {
		return nil, gimlet.ErrorResponse{StatusCode: http.StatusInternalServerError, Message: errors.Wrapf(err, "finding task '%s'", taskID).Error()}
	}
	if t == nil {
		return nil, gimlet.ErrorResponse{StatusCode: http.StatusInternalServerError, Message: fmt.Sprintf("task '%s' not found", taskID)}
	}

	catcher := grip.NewBasicCatcher()
	hostsSpawnedByTask, err := host.FindHostsSpawnedByTask(ctx, t.Id, t.Execution, []string{evergreen.HostRunning})
	catcher.Add(err)
	hostsSpawnedByBuild, err := host.FindHostsSpawnedByBuild(ctx, t.BuildId)
	catcher.Add(err)
	if catcher.HasErrors() {
		return nil, gimlet.ErrorResponse{StatusCode: http.StatusInternalServerError, Message: catcher.String()}
	}
	hosts := []host.Host{}
	hosts = append(hosts, hostsSpawnedByBuild...)
	hosts = append(hosts, hostsSpawnedByTask...)
	for idx, h := range hosts {
		if h.IsContainer() {
			p, err := h.GetParent(ctx)
			if err != nil {
				return nil, gimlet.ErrorResponse{
					StatusCode: http.StatusInternalServerError,
					Message:    fmt.Sprintf("getting parent for container '%s'", h.Id)}
			}
			if p != nil {
				hosts[idx].Host = p.Host
				hosts[idx].IP = p.IP
			}
			// update port binding if instance status not yet called
			if h.NeedsPortBindings() {
				mgrOpts, err := cloud.GetManagerOptions(h.Distro)
				if err != nil {
					return nil, errors.Wrapf(err, "getting cloud manager options for distro '%s'", h.Distro.Id)
				}
				mgr, err := cloud.GetManager(ctx, env, mgrOpts)
				if err != nil {
					return nil, errors.Wrap(err, "getting cloud manager")
				}
				if err = mgr.SetPortMappings(ctx, &hosts[idx], p); err != nil {
					return nil, errors.Wrapf(err, "getting status for container '%s'", h.Id)
				}
			}
		}
	}
	return hosts, nil
}

// CreateHostsFromTask creates intent hosts for those requested by the
// host.create command in a task.
func CreateHostsFromTask(ctx context.Context, env evergreen.Environment, t *task.Task, user user.DBUser, keyNameOrVal string) error {
	if t == nil {
		return errors.New("no task to create hosts from")
	}
	keyVal, err := user.GetPublicKey(keyNameOrVal)
	if err != nil {
		keyVal = keyNameOrVal
	}

	proj, expansions, err := makeProjectAndExpansionsFromTask(ctx, env.Settings(), t)
	if err != nil {
		return errors.WithStack(err)
	}

	projectTask := proj.FindProjectTask(t.DisplayName)
	if projectTask == nil {
		return errors.Errorf("project config for task '%s' not found", t.Id)
	}

	createHostCmds := []apimodels.CreateHost{}
	catcher := grip.NewBasicCatcher()
	for _, commandConf := range projectTask.Commands {
		var cmds []model.PluginCommandConf
		if commandConf.Function != "" {
			cmds = proj.Functions[commandConf.Function].List()
		} else {
			cmds = []model.PluginCommandConf{commandConf}
		}
		for _, cmd := range cmds {
			createHost, err := createHostFromCommand(cmd)
			if err != nil {
				return err
			}
			if createHost == nil {
				continue
			}
			cmdExpansions := util.NewExpansions(commandConf.Vars)
			cmdExpansions.Update(expansions.Map())
			err = createHost.Expand(cmdExpansions)
			if err != nil {
				catcher.Wrap(err, "handling expansions")
				continue
			}
			createHostCmds = append(createHostCmds, *createHost)
		}
	}
	if catcher.HasErrors() {
		return catcher.Resolve()
	}

	for _, createHost := range createHostCmds {
		err = createHost.Validate(ctx)
		if err != nil {
			catcher.Add(err)
			continue
		}
		numHosts, err := strconv.Atoi(createHost.NumHosts)
		if err != nil {
			catcher.Wrapf(err, "parsing host.create number of hosts '%s' as int", createHost.NumHosts)
			continue
		}
		d, err := distro.GetHostCreateDistro(ctx, createHost)
		if err != nil {
			catcher.Add(err)
			continue
		}
		for i := 0; i < numHosts; i++ {
			_, err := MakeHost(ctx, env, t.Id, user.Username(), keyVal, createHost, *d)
			if err != nil {
				return errors.Wrap(err, "creating intent host")
			}
		}
	}

	return catcher.Resolve()
}

func makeProjectAndExpansionsFromTask(ctx context.Context, settings *evergreen.Settings, t *task.Task) (*model.Project, *util.Expansions, error) {
	v, err := model.VersionFindOne(model.VersionById(t.Version))
	if err != nil {
		return nil, nil, errors.Wrapf(err, "finding version '%s'", t.Version)
	}
	if v == nil {
		return nil, nil, errors.Errorf("version '%s' not found", t.Version)
	}
	project, _, err := model.FindAndTranslateProjectForVersion(ctx, settings, v, false)
	if err != nil {
		return nil, nil, errors.Wrap(err, "loading project")
	}
	h, err := host.FindOne(ctx, host.ById(t.HostId))
	if err != nil {
		return nil, nil, errors.Wrap(err, "finding host running task")
	}
	pRef, err := model.FindBranchProjectRef(t.Project)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "finding project ref '%s'", t.Project)
	}
	if pRef == nil {
		return nil, nil, errors.Errorf("project ref '%s' not found", t.Project)
	}

	const ghTokenLifetime = 50 * time.Minute
	appToken, err := githubapp.CreateGitHubAppAuth(settings).CreateCachedInstallationToken(ctx, pRef.Owner, pRef.Repo, ghTokenLifetime, nil)
	if err != nil {
		return nil, nil, errors.Wrap(err, "creating GitHub app token")
	}

	knownHosts := settings.Expansions[evergreen.GithubKnownHosts]
	expansions, err := model.PopulateExpansions(t, h, appToken, knownHosts)
	if err != nil {
		return nil, nil, errors.Wrap(err, "populating expansions")
	}

	// PopulateExpansions doesn't include build variant expansions, so include
	// them here.
	for _, bv := range project.BuildVariants {
		if bv.Name == t.BuildVariant {
			expansions.Update(bv.Expansions)
		}
	}

	if project == nil {
		project = &model.Project{}
	}
	params := append(project.GetParameters(), v.Parameters...)
	if err = updateExpansions(&expansions, t.Project, params); err != nil {
		return nil, nil, errors.Wrap(err, "updating expansions")
	}

	return project, &expansions, nil
}

// updateExpansions updates expansions with project variables and patch
// parameters.
func updateExpansions(expansions *util.Expansions, projectId string, params []patch.Parameter) error {
	projVars, err := model.FindMergedProjectVars(projectId)
	if err != nil {
		return errors.Wrap(err, "finding project variables")
	}
	if projVars == nil {
		return errors.New("project variables not found")
	}

	expansions.Update(projVars.Vars)

	for _, param := range params {
		expansions.Put(param.Key, param.Value)
	}
	return nil
}

func createHostFromCommand(cmd model.PluginCommandConf) (*apimodels.CreateHost, error) {
	if cmd.Command != evergreen.HostCreateCommandName {
		return nil, nil
	}
	createHost := &apimodels.CreateHost{}
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		WeaklyTypedInput: true,
		Result:           createHost,
	})
	if err != nil {
		return nil, errors.Wrap(err, "constructing mapstructure decoder")
	}
	err = decoder.Decode(cmd.Params)
	if err != nil {
		return nil, errors.Wrap(err, "parsing params")
	}
	return createHost, nil
}

// MakeHost creates a host or container to run for host.create.
func MakeHost(ctx context.Context, env evergreen.Environment, taskID, userID, publicKey string, createHost apimodels.CreateHost, distro distro.Distro) (*host.Host, error) {
	if evergreen.IsDockerProvider(createHost.CloudProvider) {
		return makeDockerIntentHost(ctx, env, taskID, userID, createHost, distro)
	}
	return makeEC2IntentHost(ctx, env, taskID, userID, publicKey, createHost, distro)
}

func makeDockerIntentHost(ctx context.Context, env evergreen.Environment, taskID, userID string, createHost apimodels.CreateHost, d distro.Distro) (*host.Host, error) {
	options, err := getHostCreationOptions(d, taskID, userID, createHost)
	if err != nil {
		return nil, errors.Wrap(err, "making intent host options")
	}

	method := distro.DockerImageBuildTypeImport

	base := path.Base(createHost.Image)
	hasPrefix := strings.HasPrefix(base, "http")
	if !hasPrefix { // not a url
		method = distro.DockerImageBuildTypePull
	}

	envVars := []string{}
	for key, val := range createHost.EnvironmentVars {
		envVars = append(envVars, fmt.Sprintf("%s=%s", key, val))

	}
	options.DockerOptions = host.DockerOptions{
		Image:            createHost.Image,
		Command:          createHost.Command,
		PublishPorts:     createHost.PublishPorts,
		RegistryName:     createHost.Registry.Name,
		RegistryUsername: createHost.Registry.Username,
		RegistryPassword: createHost.Registry.Password,
		StdinData:        createHost.StdinFileContents,
		Method:           method,
		SkipImageBuild:   true,
		EnvironmentVars:  envVars,
		ExtraHosts:       createHost.ExtraHosts,
	}

	containerPool := env.Settings().ContainerPools.GetContainerPool(d.ContainerPool)
	if containerPool == nil {
		return nil, errors.Errorf("distro '%s' doesn't have a container pool", d.Id)
	}
	containerIntents, parentIntents, err := host.MakeContainersAndParents(ctx, d, containerPool, 1, *options)
	if err != nil {
		return nil, errors.Wrap(err, "generating container and parent intent hosts")
	}
	if len(containerIntents) != 1 {
		return nil, errors.Errorf("programmatic error: should have created one new container, not %d", len(containerIntents))
	}
	if err = host.InsertMany(ctx, containerIntents); err != nil {
		return nil, errors.Wrap(err, "inserting container intents")
	}
	if err = host.InsertMany(ctx, parentIntents); err != nil {
		return nil, errors.Wrap(err, "inserting parent intent hosts")
	}

	if err := units.EnqueueHostCreateJobs(ctx, env, append(containerIntents, parentIntents...)); err != nil {
		return nil, errors.Wrapf(err, "enqueueing host create jobs")
	}

	return &containerIntents[0], nil

}

func makeEC2IntentHost(ctx context.Context, env evergreen.Environment, taskID, userID, publicKey string, createHost apimodels.CreateHost, d distro.Distro) (*host.Host, error) {
	if createHost.Region == "" {
		createHost.Region = evergreen.DefaultEC2Region
	}
	ec2Settings := cloud.EC2ProviderSettings{}
	if createHost.Distro != "" {
		if err := ec2Settings.FromDistroSettings(d, createHost.Region); err != nil {
			return nil, errors.Wrapf(err, "getting EC2 provider settings from distro '%s' in region '%s'", createHost.Distro, createHost.Region)
		}
	}

	if publicKey != "" {
		d.Setup += fmt.Sprintf("\necho \"\n%s\" >> %s\n", publicKey, d.GetAuthorizedKeysFile())
	}

	// set provider settings
	if createHost.AMI != "" {
		ec2Settings.AMI = createHost.AMI
	}

	for _, mount := range createHost.EBSDevices {
		ec2Settings.MountPoints = append(ec2Settings.MountPoints, cloud.MountPoint{
			DeviceName: mount.DeviceName,
			Size:       int32(mount.SizeGiB),
			Iops:       int32(mount.IOPS),
			Throughput: int32(mount.Throughput),
			SnapshotID: mount.SnapshotID,
		})
	}
	if createHost.InstanceType != "" {
		ec2Settings.InstanceType = createHost.InstanceType
	}
	if userID == "" {
		ec2Settings.KeyName = "" // never use the distro's key
	}
	if createHost.Subnet != "" {
		ec2Settings.SubnetId = createHost.Subnet
	}
	if createHost.UserdataCommand != "" {
		ec2Settings.UserData = createHost.UserdataCommand
	}

	// Always override distro security group with provided security group.
	if len(createHost.SecurityGroups) > 0 {
		ec2Settings.SecurityGroupIDs = createHost.SecurityGroups
	} else {
		ec2Settings.SecurityGroupIDs = append(ec2Settings.SecurityGroupIDs, evergreen.GetEnvironment().Settings().Providers.AWS.DefaultSecurityGroup)
	}

	if createHost.Tenancy != "" {
		ec2Settings.Tenancy = createHost.Tenancy
	}

	ec2Settings.IPv6 = createHost.IPv6
	ec2Settings.IsVpc = true // task-spawned hosts do not support ec2 classic

	if err := ec2Settings.Validate(); err != nil {
		return nil, errors.Wrap(err, "EC2 settings are invalid")
	}

	// update local distro with modified settings
	doc, err := ec2Settings.ToDocument()
	if err != nil {
		return nil, errors.Wrap(err, "marshalling EC2 settings to BSON document")
	}
	d.ProviderSettingsList = []*birch.Document{doc}

	options, err := getHostCreationOptions(d, taskID, userID, createHost)
	if err != nil {
		return nil, errors.Wrap(err, "making intent host options")
	}
	intent := host.NewIntent(*options)
	if err = intent.Insert(ctx); err != nil {
		return nil, errors.Wrap(err, "inserting intent host")
	}
	event.LogHostCreated(intent.Id)
	grip.Info(message.Fields{
		"message":  "intent host created",
		"host_id":  intent.Id,
		"host_tag": intent.Tag,
		"distro":   intent.Distro.Id,
	})

	if err := units.EnqueueHostCreateJobs(ctx, env, []host.Host{*intent}); err != nil {
		return nil, errors.Wrapf(err, "enqueueing host create job for '%s'", intent.Id)
	}

	return intent, nil
}

func getHostCreationOptions(d distro.Distro, taskID, userID string, createHost apimodels.CreateHost) (*host.CreateOptions, error) {
	options := host.CreateOptions{
		Distro: d,
	}

	if userID != "" {
		options.UserName = userID
		options.UserHost = true
		options.ExpirationTime = time.Now().Add(evergreen.DefaultSpawnHostExpiration)
		options.ProvisionOptions = &host.ProvisionOptions{
			TaskId:  taskID,
			OwnerId: userID,
		}
	} else {
		options.UserName = taskID
		t, err := task.FindOneId(taskID)
		if err != nil {
			return nil, errors.Wrapf(err, "finding task '%s'", taskID)
		}
		if t == nil {
			return nil, errors.Errorf("task '%s' not found", taskID)
		}
		if createHost.Scope == "build" {
			options.SpawnOptions.BuildID = t.BuildId
		}
		if createHost.Scope == "task" {
			options.SpawnOptions.TaskID = taskID
			options.SpawnOptions.TaskExecutionNumber = t.Execution
		}
		options.SpawnOptions.TimeoutTeardown = time.Now().Add(time.Duration(createHost.TeardownTimeoutSecs) * time.Second)
		options.SpawnOptions.TimeoutSetup = time.Now().Add(time.Duration(createHost.SetupTimeoutSecs) * time.Second)
		options.SpawnOptions.Retries = createHost.Retries
		options.SpawnOptions.Respawns = evergreen.SpawnHostRespawns
		options.SpawnOptions.SpawnedByTask = true
	}
	return &options, nil
}

// GetDockerLogs retrieves the logs for the given container.
func GetDockerLogs(ctx context.Context, containerId string, parent *host.Host,
	settings *evergreen.Settings, options types.ContainerLogsOptions) (io.Reader, error) {
	c := cloud.GetDockerClient(settings)

	if err := c.Init(settings.Providers.Docker.APIVersion); err != nil {
		return nil, errors.Wrap(err, "initializing Docker client")
	}

	logs, err := c.GetDockerLogs(ctx, containerId, parent, options)
	if err != nil {
		return nil, errors.Wrapf(err, "getting Docker logs for container '%s'", containerId)
	}
	return logs, nil
}

// GetDockerStatus returns the status of the given Docker container.
func GetDockerStatus(ctx context.Context, containerId string, parent *host.Host, settings *evergreen.Settings) (*cloud.ContainerStatus, error) {
	c := cloud.GetDockerClient(settings)

	if err := c.Init(settings.Providers.Docker.APIVersion); err != nil {
		return nil, errors.Wrap(err, "initializing Docker client")
	}
	status, err := c.GetDockerStatus(ctx, containerId, parent)
	if err != nil {
		return nil, errors.Wrapf(err, "getting status of container '%s'", containerId)
	}
	return status, nil
}
