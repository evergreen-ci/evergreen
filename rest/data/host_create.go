package data

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"path/filepath"
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
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// DBCreateHostConnector supports `host.create` commands from the agent.
type DBCreateHostConnector struct{}

// ListHostsForTask lists running hosts scoped to the task or the task's build.
func (dc *DBCreateHostConnector) ListHostsForTask(ctx context.Context, taskID string) ([]host.Host, error) {
	env := evergreen.GetEnvironment()
	t, err := task.FindOneId(taskID)
	if err != nil {
		return nil, gimlet.ErrorResponse{StatusCode: http.StatusInternalServerError, Message: "error finding task"}
	}
	if t == nil {
		return nil, gimlet.ErrorResponse{StatusCode: http.StatusInternalServerError, Message: "no task found"}
	}

	catcher := grip.NewBasicCatcher()
	hostsSpawnedByTask, err := host.FindHostsSpawnedByTask(t.Id, t.Execution)
	catcher.Add(err)
	hostsSpawnedByBuild, err := host.FindHostsSpawnedByBuild(t.BuildId)
	catcher.Add(err)
	if catcher.HasErrors() {
		return nil, gimlet.ErrorResponse{StatusCode: http.StatusInternalServerError, Message: catcher.String()}
	}
	hosts := []host.Host{}
	hosts = append(hosts, hostsSpawnedByBuild...)
	hosts = append(hosts, hostsSpawnedByTask...)
	for idx, h := range hosts {
		if h.IsContainer() {
			p, err := h.GetParent()
			if err != nil {
				return nil, gimlet.ErrorResponse{
					StatusCode: http.StatusInternalServerError,
					Message:    fmt.Sprintf("error getting parent for container '%s'", h.Id)}
			}
			if p != nil {
				hosts[idx].Host = p.Host
				hosts[idx].IP = p.IP
			}
			// update port binding if instance status not yet called
			if h.NeedsPortBindings() {
				mgrOpts, err := cloud.GetManagerOptions(h.Distro)
				if err != nil {
					return nil, errors.Wrap(err, "error getting manager options for host.list")
				}
				mgr, err := cloud.GetManager(ctx, env, mgrOpts)
				if err != nil {
					return nil, errors.Wrap(err, "error getting cloud manager for host.list")
				}
				if err = mgr.SetPortMappings(ctx, &hosts[idx], p); err != nil {
					return nil, errors.Wrapf(err, "error getting status for container '%s'", h.Id)
				}
			}
		}
	}
	return hosts, nil
}

func (dc *DBCreateHostConnector) CreateHostsFromTask(t *task.Task, user user.DBUser, keyNameOrVal string) error {
	if t == nil {
		return errors.New("no task to create hosts from")
	}
	keyVal, err := user.GetPublicKey(keyNameOrVal)
	if err != nil {
		keyVal = keyNameOrVal
	}

	proj, expansions, err := makeProjectAndExpansionsFromTask(t)
	if err != nil {
		return errors.WithStack(err)
	}

	projectTask := proj.FindProjectTask(t.DisplayName)
	if projectTask == nil {
		return errors.Errorf("unable to find configuration for task %s", t.Id)
	}

	createHostCmds := []apimodels.CreateHost{}
	catcher := grip.NewBasicCatcher()
	for _, commandConf := range projectTask.Commands {
		var createHost *apimodels.CreateHost
		if commandConf.Function != "" {
			cmds := proj.Functions[commandConf.Function]
			for _, cmd := range cmds.List() {
				createHost, err = createHostFromCommand(cmd)
				if err != nil {
					return err
				}
				if createHost == nil {
					continue
				}
				createHostCmds = append(createHostCmds, *createHost)
			}
		} else {
			createHost, err = createHostFromCommand(commandConf)
			if err != nil {
				return err
			}
			if createHost == nil {
				continue
			}
			createHostCmds = append(createHostCmds, *createHost)
		}
	}
	if catcher.HasErrors() {
		return catcher.Resolve()
	}

	hosts := []host.Host{}
	for _, createHost := range createHostCmds {
		err = createHost.Expand(expansions)
		if err != nil {
			catcher.Add(err)
			continue
		}
		err = createHost.Validate()
		if err != nil {
			catcher.Add(err)
			continue
		}
		numHosts, err := strconv.Atoi(createHost.NumHosts)
		if err != nil {
			catcher.Add(errors.Wrapf(err, "problem parsing '%s' as int", createHost.NumHosts))
			continue
		}
		for i := 0; i < numHosts; i++ {
			intent, err := dc.MakeIntentHost(t.Id, user.Username(), keyVal, createHost)
			if err != nil {
				return errors.Wrap(err, "error creating host document")
			}
			hosts = append(hosts, *intent)
		}
	}

	return catcher.Resolve()
}

func makeProjectAndExpansionsFromTask(t *task.Task) (*model.Project, *util.Expansions, error) {
	v, err := model.VersionFindOne(model.VersionById(t.Version))
	if err != nil {
		return nil, nil, errors.Wrap(err, "error finding version")
	}
	if v == nil {
		return nil, nil, errors.New("version doesn't exist")
	}
	proj, _, err := model.LoadProjectForVersion(v, v.Identifier, true)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error loading project")
	}
	h, err := host.FindOne(host.ById(t.HostId))
	if err != nil {
		return nil, nil, errors.Wrap(err, "error finding host")
	}
	settings, err := evergreen.GetConfig()
	if err != nil {
		return nil, nil, errors.Wrap(err, "error getting evergreen config")
	}
	oauthToken, err := settings.GetGithubOauthToken()
	if err != nil {
		return nil, nil, errors.Wrap(err, "error getting oauth token")
	}
	expansions, err := model.PopulateExpansions(t, h, oauthToken)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error populating expansions")
	}
	params := append(proj.GetParameters(), v.Parameters...)
	if err = updateExpansions(&expansions, t.Project, params); err != nil {
		return nil, nil, errors.Wrap(err, "error updating expansions")
	}

	return proj, &expansions, nil
}

// updateExpansions updates expansions with project variables and patch
// parameters.
func updateExpansions(expansions *util.Expansions, projectId string, params []patch.Parameter) error {
	projVars, err := model.FindMergedProjectVars(projectId)
	if err != nil {
		return errors.Wrap(err, "error finding project vars")
	}
	if projVars == nil {
		return errors.New("project vars not found")
	}

	expansions.Update(projVars.GetUnrestrictedVars())

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
		return nil, errors.WithStack(err)
	}
	err = decoder.Decode(cmd.Params)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return createHost, nil
}

func (dc *DBCreateHostConnector) MakeIntentHost(taskID, userID, publicKey string, createHost apimodels.CreateHost) (*host.Host, error) {
	if cloud.IsDockerProvider(createHost.CloudProvider) {
		return makeDockerIntentHost(taskID, userID, createHost)
	}
	return makeEC2IntentHost(taskID, userID, publicKey, createHost)
}

func makeDockerIntentHost(taskID, userID string, createHost apimodels.CreateHost) (*host.Host, error) {
	var d *distro.Distro
	var err error

	d, err = distro.FindByID(createHost.Distro)
	if err != nil {
		return nil, errors.Wrapf(err, "problem finding distro '%s'", createHost.Distro)
	}
	if d == nil {
		return nil, errors.Errorf("distro '%s' not found", createHost.Distro)
	}
	if !cloud.IsDockerProvider(d.Provider) {
		return nil, errors.Errorf("distro '%s' provider must be docker (provider is '%s')", d.Id, d.Provider)
	}

	// Do not provision task-spawned hosts.
	d.BootstrapSettings.Method = distro.BootstrapMethodNone

	options, err := getAgentOptions(taskID, userID, createHost)
	if err != nil {
		return nil, errors.Wrap(err, "error making host options for docker")
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
		Method:           method,
		SkipImageBuild:   true,
		EnvironmentVars:  envVars,
	}

	config, err := evergreen.GetConfig()
	if err != nil {
		return nil, errors.Wrap(err, "error getting config")
	}
	containerPool := config.ContainerPools.GetContainerPool(d.ContainerPool)
	if containerPool == nil {
		return nil, errors.Errorf("distro '%s' doesn't have a container pool", d.Id)
	}
	containerIntents, parentIntents, err := host.MakeContainersAndParents(*d, containerPool, 1, *options)
	if err != nil {
		return nil, errors.Wrap(err, "error generating host intent")
	}
	if len(containerIntents) != 1 {
		return nil, errors.Errorf("Programmer error: should have created one new container, not %d", len(containerIntents))
	}
	if err = host.InsertMany(containerIntents); err != nil {
		return nil, errors.Wrap(err, "unable to insert container intents")
	}
	if err = host.InsertMany(parentIntents); err != nil {
		return nil, errors.Wrap(err, "unable to insert parent intents")
	}
	return &containerIntents[0], nil

}

func makeEC2IntentHost(taskID, userID, publicKey string, createHost apimodels.CreateHost) (*host.Host, error) {
	provider := evergreen.ProviderNameEc2OnDemand
	if createHost.Spot {
		provider = evergreen.ProviderNameEc2Spot
	}

	if createHost.Region == "" {
		createHost.Region = evergreen.DefaultEC2Region
	}
	// get distro if it is set
	d := distro.Distro{}
	ec2Settings := cloud.EC2ProviderSettings{}
	var err error
	if distroID := createHost.Distro; distroID != "" {
		var dat distro.AliasLookupTable
		dat, err = distro.NewDistroAliasesLookupTable()
		if err != nil {
			return nil, errors.Wrap(err, "could not get distro lookup table")
		}
		distroIDs := dat.Expand([]string{distroID})
		if len(distroIDs) == 0 {
			return nil, errors.Wrap(err, "distro lookup returned no matching distro IDs")
		}
		d, err = distro.FindOne(distro.ById(distroIDs[0]))
		if err != nil {
			return nil, errors.Wrapf(err, "problem finding distro '%s'", distroID)
		}
		if err = ec2Settings.FromDistroSettings(d, createHost.Region); err != nil {
			return nil, errors.Wrapf(err, "error getting ec2 provider from distro")
		}
	}

	// Do not provision task-spawned hosts.
	d.BootstrapSettings.Method = distro.BootstrapMethodNone

	// set provider
	d.Provider = provider

	if publicKey != "" {
		d.Setup += fmt.Sprintf("\necho \"\n%s\" >> %s\n", publicKey, filepath.Join(d.HomeDir(), ".ssh", "authorized_keys"))
	}

	// set provider settings
	if createHost.AMI != "" {
		ec2Settings.AMI = createHost.AMI
	}
	if createHost.AWSKeyID != "" {
		ec2Settings.AWSKeyID = createHost.AWSKeyID
		ec2Settings.AWSSecret = createHost.AWSSecret
	}

	for _, mount := range createHost.EBSDevices {
		ec2Settings.MountPoints = append(ec2Settings.MountPoints, cloud.MountPoint{
			DeviceName: mount.DeviceName,
			Size:       int64(mount.SizeGiB),
			Iops:       int64(mount.IOPS),
			SnapshotID: mount.SnapshotID,
		})
	}
	if createHost.InstanceType != "" {
		ec2Settings.InstanceType = createHost.InstanceType
	}
	if userID == "" {
		ec2Settings.KeyName = createHost.KeyName // never use the distro's key
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

	ec2Settings.IPv6 = createHost.IPv6
	ec2Settings.IsVpc = true // task-spawned hosts do not support ec2 classic

	if err = ec2Settings.Validate(); err != nil {
		return nil, errors.Wrap(err, "EC2 settings are invalid")
	}

	// update local distro with modified settings
	doc, err := ec2Settings.ToDocument()
	if err != nil {
		return nil, errors.Wrap(err, "error marshalling ec2 settings to document")
	}
	d.ProviderSettingsList = []*birch.Document{doc}

	options, err := getAgentOptions(taskID, userID, createHost)
	if err != nil {
		return nil, errors.Wrap(err, "error making host options for EC2")
	}
	intent := host.NewIntent(d, d.GenerateName(), provider, *options)
	if err = intent.Insert(); err != nil {
		return nil, errors.Wrap(err, "unable to insert host intent")
	}

	return intent, nil
}

func getAgentOptions(taskID, userID string, createHost apimodels.CreateHost) (*host.CreateOptions, error) {
	options := host.CreateOptions{}
	if userID != "" {
		options.UserName = userID
		options.UserHost = true
		expiration := evergreen.DefaultSpawnHostExpiration
		options.ExpirationDuration = &expiration
		options.ProvisionOptions = &host.ProvisionOptions{
			TaskId:  taskID,
			OwnerId: userID,
		}
	} else {
		options.UserName = taskID
		t, err := task.FindOneId(taskID)
		if err != nil {
			return nil, errors.Wrap(err, "could not find task")
		}
		if t == nil {
			return nil, errors.New("no task returned")
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
		options.SpawnOptions.SpawnedByTask = true
	}
	return &options, nil
}

// GetDockerLogs is used by the /host/{host_id}/logs route to retrieve the logs for the given container.
func (dc *DBCreateHostConnector) GetDockerLogs(ctx context.Context, containerId string, parent *host.Host,
	settings *evergreen.Settings, options types.ContainerLogsOptions) (io.Reader, error) {
	c := cloud.GetDockerClient(settings)

	if err := c.Init(settings.Providers.Docker.APIVersion); err != nil {

		return nil, errors.Wrap(err, "error initializing client")
	}

	logs, err := c.GetDockerLogs(ctx, containerId, parent, options)
	if err != nil {
		return nil, errors.Wrapf(err, "error getting logs for container %s", containerId)
	}
	return logs, nil
}

func (db *DBCreateHostConnector) GetDockerStatus(ctx context.Context, containerId string, parent *host.Host, settings *evergreen.Settings) (*cloud.ContainerStatus, error) {
	c := cloud.GetDockerClient(settings)

	if err := c.Init(settings.Providers.Docker.APIVersion); err != nil {

		return nil, errors.Wrap(err, "error initializing client")
	}
	status, err := c.GetDockerStatus(ctx, containerId, parent)
	if err != nil {
		return nil, errors.Wrapf(err, "error getting status of container %s", containerId)
	}
	return status, nil
}

// MockCreateHostConnector mocks `DBCreateHostConnector`.
type MockCreateHostConnector struct{}

func (dc *MockCreateHostConnector) GetDockerLogs(ctx context.Context, containerId string, parent *host.Host,
	settings *evergreen.Settings, options types.ContainerLogsOptions) (io.Reader, error) {
	c := cloud.GetMockClient()
	logs, err := c.GetDockerLogs(ctx, containerId, parent, options)
	if err != nil {
		return nil, errors.Wrapf(err, "error getting logs for container %s", containerId)
	}
	return logs, nil
}

func (dc *MockCreateHostConnector) GetDockerStatus(ctx context.Context, containerId string, parent *host.Host,
	_ *evergreen.Settings) (*cloud.ContainerStatus, error) {
	c := cloud.GetMockClient()
	status, err := c.GetDockerStatus(ctx, containerId, parent)
	if err != nil {
		return nil, errors.Wrapf(err, "error getting status of container %s", containerId)
	}
	return status, nil
}

// ListHostsForTask lists running hosts scoped to the task or the task's build.
func (*MockCreateHostConnector) ListHostsForTask(ctx context.Context, taskID string) ([]host.Host, error) {
	return nil, errors.New("method not implemented")
}

func (*MockCreateHostConnector) MakeIntentHost(taskID, userID, publicKey string, createHost apimodels.CreateHost) (*host.Host, error) {
	connector := DBCreateHostConnector{}
	return connector.MakeIntentHost(taskID, userID, publicKey, createHost)
}

func (*MockCreateHostConnector) CreateHostsFromTask(t *task.Task, user user.DBUser, keyNameOrVal string) error {
	return errors.New("CreateHostsFromTask not implemented")
}
