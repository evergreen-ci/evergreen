package data

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"time"

	"github.com/docker/docker/api/types"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// DBCreateHostConnector supports `host.create` commands from the agent.
type DBCreateHostConnector struct{}

// ListHostsForTask lists running hosts scoped to the task or the task's build.
func (dc *DBCreateHostConnector) ListHostsForTask(taskID string) ([]host.Host, error) {
	t, err := task.FindOneId(taskID)
	if err != nil {
		return nil, gimlet.ErrorResponse{StatusCode: http.StatusInternalServerError, Message: "error finding task"}
	}
	if t == nil {
		return nil, gimlet.ErrorResponse{StatusCode: http.StatusInternalServerError, Message: "no task found"}
	}

	catcher := grip.NewBasicCatcher()
	hostsSpawnedByTask, err := host.FindHostsSpawnedByTask(t.Id)
	catcher.Add(err)
	hostsSpawnedByBuild, err := host.FindHostsSpawnedByBuild(t.BuildId)
	catcher.Add(err)
	if catcher.HasErrors() {
		return nil, gimlet.ErrorResponse{StatusCode: http.StatusInternalServerError, Message: catcher.String()}
	}
	hosts := []host.Host{}
	hosts = append(hosts, hostsSpawnedByBuild...)
	hosts = append(hosts, hostsSpawnedByTask...)

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

	tc, err := model.MakeConfigFromTask(t)
	if err != nil {
		return err
	}

	projectTask := tc.Project.FindProjectTask(tc.Task.DisplayName)
	if projectTask == nil {
		return errors.Errorf("unable to find configuration for task %s", tc.Task.Id)
	}

	createHostCmds := []apimodels.CreateHost{}
	catcher := grip.NewBasicCatcher()
	for _, commandConf := range projectTask.Commands {
		var createHost *apimodels.CreateHost
		if commandConf.Function != "" {
			cmds := tc.Project.Functions[commandConf.Function]
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
		err = createHost.Expand(tc.Expansions)
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
	if catcher.HasErrors() {
		return catcher.Resolve()
	}

	return errors.Wrap(host.InsertMany(hosts), "error inserting host documents")
}

func createHostFromCommand(cmd model.PluginCommandConf) (*apimodels.CreateHost, error) {
	if cmd.Command != evergreen.CreateHostCommandName {
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
	if createHost.CloudProvider == evergreen.ProviderNameDocker {
		return makeDockerIntentHost(taskID, userID, createHost)
	}
	return makeEC2IntentHost(taskID, userID, publicKey, createHost)
}

func makeDockerIntentHost(taskID, userID string, createHost apimodels.CreateHost) (*host.Host, error) {
	d := distro.Distro{}
	var err error

	distroID := createHost.Distro
	if distroID != "" {
		d, err = distro.FindOne(distro.ById(distroID))
		if err != nil {
			return nil, errors.Wrapf(err, "problem finding distro %s", distroID)
		}
	}

	options, err := getAgentOptions(taskID, userID, createHost)
	if err != nil {
		return nil, errors.Wrap(err, "error making host options for docker")
	}

	method := distro.DockerImageBuildTypeImport
	// not a URL
	if path.Base(createHost.Image) == createHost.Image {
		method = distro.DockerImageBuildTypePull
	}

	options.DockerOptions = host.DockerOptions{
		Image:            createHost.Image,
		Command:          createHost.Command,
		RegistryName:     createHost.Registry.Name,
		RegistryUsername: createHost.Registry.Username,
		RegistryPassword: createHost.Registry.Password,
		Method:           method,
		SkipImageBuild:   true,
	}

	return cloud.NewIntent(d, d.GenerateName(), d.Provider, *options), nil

}

func makeEC2IntentHost(taskID, userID, publicKey string, createHost apimodels.CreateHost) (*host.Host, error) {
	provider := evergreen.ProviderNameEc2OnDemand
	if createHost.Spot {
		provider = evergreen.ProviderNameEc2Spot
	}

	// get distro if it is set
	d := distro.Distro{}
	ec2Settings := cloud.EC2ProviderSettings{}
	var err error
	if distroID := createHost.Distro; distroID != "" {
		d, err = distro.FindOne(distro.ById(distroID))
		if err != nil {
			return nil, errors.Wrap(err, "problem finding distro")
		}
		if err = mapstructure.Decode(d.ProviderSettings, &ec2Settings); err != nil {
			return nil, errors.Wrap(err, "problem unmarshaling provider settings")
		}
	}

	// set provider
	d.Provider = provider

	if publicKey != "" {
		d.Setup += fmt.Sprintf("\necho \"\n%s\" >> ~%s/.ssh/authorized_keys\n", publicKey, d.User)
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
	if createHost.Region != "" {
		ec2Settings.Region = createHost.Region
	}
	if len(createHost.SecurityGroups) > 0 {
		ec2Settings.SecurityGroupIDs = createHost.SecurityGroups
	}
	if createHost.Subnet != "" {
		ec2Settings.SubnetId = createHost.Subnet
	}
	if createHost.UserdataCommand != "" {
		ec2Settings.UserData = createHost.UserdataCommand
	}
	ec2Settings.IPv6 = createHost.IPv6
	ec2Settings.IsVpc = true // task-spawned hosts do not support ec2 classic
	if err = mapstructure.Decode(ec2Settings, &d.ProviderSettings); err != nil {
		return nil, errors.Wrap(err, "error marshaling provider settings")
	}

	options, err := getAgentOptions(taskID, userID, createHost)
	if err != nil {
		return nil, errors.Wrap(err, "error making host options for EC2")
	}

	return cloud.NewIntent(d, d.GenerateName(), provider, *options), nil
}

func getAgentOptions(taskID, userID string, createHost apimodels.CreateHost) (*cloud.HostOptions, error) {
	options := cloud.HostOptions{}
	if userID != "" {
		options.UserName = userID
		options.UserHost = true
		expiration := cloud.DefaultSpawnHostExpiration
		options.ExpirationDuration = &expiration
		options.ProvisionOptions = &host.ProvisionOptions{
			TaskId:  taskID,
			OwnerId: userID,
		}
	} else {
		options.UserName = taskID
		if createHost.Scope == "build" {
			t, err := task.FindOneId(taskID)
			if err != nil {
				return nil, errors.Wrap(err, "could not find task")
			}
			if t == nil {
				return nil, errors.New("no task returned")
			}
			options.SpawnOptions.BuildID = t.BuildId
		}
		if createHost.Scope == "task" {
			options.SpawnOptions.TaskID = taskID
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
func (*MockCreateHostConnector) ListHostsForTask(taskID string) ([]host.Host, error) {
	return nil, errors.New("method not implemented")
}

func (*MockCreateHostConnector) MakeIntentHost(taskID, userID, publicKey string, createHost apimodels.CreateHost) (*host.Host, error) {
	connector := DBCreateHostConnector{}
	return connector.MakeIntentHost(taskID, userID, publicKey, createHost)
}

func (*MockCreateHostConnector) CreateHostsFromTask(t *task.Task, user user.DBUser, keyNameOrVal string) error {
	return errors.New("CreateHostsFromTask not implemented")
}
