package docker

import (
	"bytes"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db/bsonutil"
	"github.com/evergreen-ci/evergreen/hostutil"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/fsouza/go-dockerclient"
	"github.com/mitchellh/mapstructure"
	"gopkg.in/mgo.v2/bson"
	"math/rand"
	"time"
)

const (
	DockerStatusRunning = iota
	DockerStatusPaused
	DockerStatusRestarting
	DockerStatusKilled
	DockerStatusUnknown

	ProviderName   = "docker"
	TimeoutSeconds = 5
)

type DockerManager struct {
}

type portRange struct {
	MinPort int64 `mapstructure:"min_port" json:"min_port" bson:"min_port"`
	MaxPort int64 `mapstructure:"max_port" json:"max_port" bson:"max_port"`
}

type auth struct {
	Cert string `mapstructure:"cert" json:"cert" bson:"cert"`
	Key  string `mapstructure:"key" json:"key" bson:"key"`
	Ca   string `mapstructure:"ca" json:"ca" bson:"ca"`
}

type Settings struct {
	HostIp     string     `mapstructure:"host_ip" json:"host_ip" bson:"host_ip"`
	ImageId    string     `mapstructure:"image_name" json:"image_name" bson:"image_name"`
	ClientPort int        `mapstructure:"client_port" json:"client_port" bson:"client_port"`
	PortRange  *portRange `mapstructure:"port_range" json:"port_range" bson:"port_range"`
	Auth       *auth      `mapstructure:"auth" json:"auth" bson:"auth"`
}

var (
	// bson fields for the Settings struct
	HostIp     = bsonutil.MustHaveTag(Settings{}, "HostIp")
	ImageId    = bsonutil.MustHaveTag(Settings{}, "ImageId")
	ClientPort = bsonutil.MustHaveTag(Settings{}, "ClientPort")
	PortRange  = bsonutil.MustHaveTag(Settings{}, "PortRange")
	Auth       = bsonutil.MustHaveTag(Settings{}, "Auth")

	// bson fields for the portRange struct
	MinPort = bsonutil.MustHaveTag(portRange{}, "MinPort")
	MaxPort = bsonutil.MustHaveTag(portRange{}, "MaxPort")

	// bson fields for the auth struct
	Cert = bsonutil.MustHaveTag(auth{}, "Cert")
	Key  = bsonutil.MustHaveTag(auth{}, "Key")
	Ca   = bsonutil.MustHaveTag(auth{}, "Ca")

	// exposed port (set to 22/tcp, default ssh port)
	SSHDPort docker.Port = "22/tcp"
)

//*********************************************************************************
// Helper Functions
//*********************************************************************************

func generateClient(d *distro.Distro) (*docker.Client, *Settings, error) {
	// Populate and validate settings
	settings := &Settings{} // Instantiate global settings
	if err := mapstructure.Decode(d.ProviderSettings, settings); err != nil {
		return nil, settings, fmt.Errorf("Error decoding params for distro %v: %v", d.Id, err)
	}

	if err := settings.Validate(); err != nil {
		return nil, settings, fmt.Errorf("Invalid Docker settings in distro %v: %v", d.Id, err)
	}

	// Convert authentication strings to byte arrays
	cert := bytes.NewBufferString(settings.Auth.Cert).Bytes()
	key := bytes.NewBufferString(settings.Auth.Key).Bytes()
	ca := bytes.NewBufferString(settings.Auth.Ca).Bytes()

	// Create client
	endpoint := fmt.Sprintf("tcp://%s:%v", settings.HostIp, settings.ClientPort)
	client, err := docker.NewTLSClientFromBytes(endpoint, cert, key, ca)
	if err != nil {
		evergreen.Logger.Logf(slogger.ERROR, "Docker initialize client API call failed for host '%s': %v", endpoint, err)
	}
	return client, settings, err
}

func populateHostConfig(hostConfig *docker.HostConfig, d *distro.Distro) error {
	// Retrieve client for API call and settings
	client, settings, err := generateClient(d)
	if err != nil {
		return err
	}
	minPort := settings.PortRange.MinPort
	maxPort := settings.PortRange.MaxPort

	// Get all the things!
	containers, err := client.ListContainers(docker.ListContainersOptions{})
	if err != nil {
		evergreen.Logger.Logf(slogger.ERROR, "Docker list containers API call failed. %v", err)
		return err
	}
	reservedPorts := make(map[int64]bool)
	for _, c := range containers {
		for _, p := range c.Ports {
			reservedPorts[p.PublicPort] = true
		}
	}

	// If unspecified, let Docker choose random port
	if minPort == 0 && maxPort == 0 {
		hostConfig.PublishAllPorts = true
		return nil
	}

	hostConfig.PortBindings = make(map[docker.Port][]docker.PortBinding)
	for i := minPort; i <= maxPort; i++ {
		// if port is not already in use, bind it to sshd exposed container port
		if !reservedPorts[i] {
			hostConfig.PortBindings[SSHDPort] = []docker.PortBinding{
				docker.PortBinding{
					HostIP:   settings.HostIp,
					HostPort: fmt.Sprintf("%v", i),
				},
			}
			break
		}
	}

	// If map is empty, no ports were available.
	if len(hostConfig.PortBindings) == 0 {
		return evergreen.Logger.Errorf(slogger.ERROR, "No available ports in specified range.")
	}
	return nil
}

func retrieveOpenPortBinding(containerPtr *docker.Container) (string, error) {
	exposedPorts := containerPtr.Config.ExposedPorts
	ports := containerPtr.NetworkSettings.Ports
	for k, _ := range exposedPorts {
		portBindings := ports[k]
		if len(portBindings) > 0 {
			return portBindings[0].HostPort, nil
		}
	}
	return "", fmt.Errorf("No available ports")
}

//*********************************************************************************
// Public Functions
//*********************************************************************************

//Validate checks that the settings from the config file are sane.
func (settings *Settings) Validate() error {
	if settings.HostIp == "" {
		return fmt.Errorf("HostIp must not be blank")
	}

	if settings.ImageId == "" {
		return fmt.Errorf("ImageName must not be blank")
	}

	if settings.ClientPort == 0 {
		return fmt.Errorf("Port must not be blank")
	}

	if settings.PortRange != nil {
		min := settings.PortRange.MinPort
		max := settings.PortRange.MaxPort

		if max < min {
			return fmt.Errorf("Container port range must be valid")
		}
	}

	if settings.Auth == nil {
		return fmt.Errorf("Authentication materials must not be blank")
	} else if settings.Auth.Cert == "" {
		return fmt.Errorf("Certificate must not be blank")
	} else if settings.Auth.Key == "" {
		return fmt.Errorf("Key must not be blank")
	} else if settings.Auth.Ca == "" {
		return fmt.Errorf("Certificate authority must not be blank")
	}

	return nil
}

func (_ *DockerManager) GetSettings() cloud.ProviderSettings {
	return &Settings{}
}

// SpawnInstance creates and starts a new Docker container
func (dockerMgr *DockerManager) SpawnInstance(d *distro.Distro, owner string, userHost bool) (*host.Host, error) {
	var err error

	if d.Provider != ProviderName {
		return nil, fmt.Errorf("Can't spawn instance of %v for distro %v: provider is %v", ProviderName, d.Id, d.Provider)
	}

	// Initialize client
	dockerClient, settings, err := generateClient(d)
	if err != nil {
		return nil, err
	}

	// Create HostConfig structure
	hostConfig := &docker.HostConfig{}
	err = populateHostConfig(hostConfig, d)
	if err != nil {
		evergreen.Logger.Logf(slogger.ERROR, "Unable to populate docker host config for host '%s': %v", settings.HostIp, err)
		return nil, err
	}

	// Build container
	containerName := "docker-" + bson.NewObjectId().Hex()
	newContainer, err := dockerClient.CreateContainer(
		docker.CreateContainerOptions{
			Name: containerName,
			Config: &docker.Config{
				Cmd: []string{"/usr/sbin/sshd", "-D"},
				ExposedPorts: map[docker.Port]struct{}{
					SSHDPort: struct{}{},
				},
				Image: settings.ImageId,
			},
			HostConfig: hostConfig,
		},
	)
	if err != nil {
		evergreen.Logger.Logf(slogger.ERROR, "Docker create container API call failed for host '%s': %v", settings.HostIp, err)
		return nil, err
	}

	// Start container
	err = dockerClient.StartContainer(newContainer.ID, nil)
	if err != nil {
		// Clean up
		err2 := dockerClient.RemoveContainer(
			docker.RemoveContainerOptions{
				ID:    newContainer.ID,
				Force: true,
			},
		)
		if err2 != nil {
			err = fmt.Errorf("%v. And was unable to clean up container %v: %v", err, newContainer.ID, err2)
		}
		evergreen.Logger.Logf(slogger.ERROR, "Docker start container API call failed for host '%s': %v", settings.HostIp, err)
		return nil, err
	}

	// Retrieve container details
	newContainer, err = dockerClient.InspectContainer(newContainer.ID)
	if err != nil {
		evergreen.Logger.Logf(slogger.ERROR, "Docker inspect container API call failed for host '%s': %v", settings.HostIp, err)
		return nil, err
	}

	hostPort, err := retrieveOpenPortBinding(newContainer)
	if err != nil {
		evergreen.Logger.Logf(slogger.ERROR, "Error with docker container '%v': %v", newContainer.ID, err)
		return nil, err
	}
	hostStr := fmt.Sprintf("%s:%s", settings.HostIp, hostPort)

	// Add host info to db
	instanceName := "container-" +
		fmt.Sprintf("%d", rand.New(rand.NewSource(time.Now().UnixNano())).Int())
	host := &host.Host{
		Id:               newContainer.ID,
		Host:             hostStr,
		User:             d.User,
		Tag:              instanceName,
		Distro:           *d,
		CreationTime:     newContainer.Created,
		Status:           evergreen.HostUninitialized,
		TerminationTime:  model.ZeroTime,
		TaskDispatchTime: model.ZeroTime,
		Provider:         ProviderName,
		StartedBy:        owner,
	}

	err = host.Insert()
	if err != nil {
		return nil, evergreen.Logger.Errorf(slogger.ERROR, "Failed to insert new host '%s': %v", host.Id, err)
	}

	evergreen.Logger.Logf(slogger.DEBUG, "Successfully inserted new host '%v' for distro '%v'", host.Id, d.Id)

	return host, nil
}

// getStatus is a helper function which returns the enum representation of the status
// contained in a container's state
func getStatus(s *docker.State) int {
	if s.Running {
		return DockerStatusRunning
	} else if s.Paused {
		return DockerStatusPaused
	} else if s.Restarting {
		return DockerStatusRestarting
	} else if s.OOMKilled {
		return DockerStatusKilled
	}

	return DockerStatusUnknown
}

// GetInstanceStatus returns a universal status code representing the state
// of a container.
func (dockerMgr *DockerManager) GetInstanceStatus(host *host.Host) (cloud.CloudStatus, error) {
	dockerClient, _, err := generateClient(&host.Distro)
	if err != nil {
		return cloud.StatusUnknown, err
	}

	container, err := dockerClient.InspectContainer(host.Id)
	if err != nil {
		return cloud.StatusUnknown, fmt.Errorf("Failed to get container information for host '%v': %v", host.Id, err)
	}

	switch getStatus(&container.State) {
	case DockerStatusRestarting:
		return cloud.StatusInitializing, nil
	case DockerStatusRunning:
		return cloud.StatusRunning, nil
	case DockerStatusPaused:
		return cloud.StatusStopped, nil
	case DockerStatusKilled:
		return cloud.StatusTerminated, nil
	default:
		return cloud.StatusUnknown, nil
	}
}

//GetDNSName gets the DNS hostname of a container by reading it directly from
//the Docker API
func (dockerMgr *DockerManager) GetDNSName(host *host.Host) (string, error) {
	return host.Host, nil
}

//CanSpawn returns if a given cloud provider supports spawning a new host
//dynamically. Always returns true for Docker.
func (dockerMgr *DockerManager) CanSpawn() (bool, error) {
	return true, nil
}

//TerminateInstance destroys a container.
func (dockerMgr *DockerManager) TerminateInstance(host *host.Host) error {
	dockerClient, _, err := generateClient(&host.Distro)
	if err != nil {
		return err
	}

	err = dockerClient.StopContainer(host.Id, TimeoutSeconds)
	if err != nil {
		return evergreen.Logger.Errorf(slogger.ERROR, "Failed to stop container '%v': %v", host.Id, err)
	}

	err = dockerClient.RemoveContainer(
		docker.RemoveContainerOptions{
			ID: host.Id,
		},
	)
	if err != nil {
		return evergreen.Logger.Errorf(slogger.ERROR, "Failed to remove container '%v': %v", host.Id, err)
	}

	return host.Terminate()
}

//Configure populates a DockerManager by reading relevant settings from the
//config object.
func (dockerMgr *DockerManager) Configure(settings *evergreen.Settings) error {
	return nil
}

//IsSSHReachable checks if a container appears to be reachable via SSH by
//attempting to contact the host directly.
func (dockerMgr *DockerManager) IsSSHReachable(host *host.Host, keyPath string) (bool, error) {
	sshOpts, err := dockerMgr.GetSSHOptions(host, keyPath)
	if err != nil {
		return false, err
	}
	return hostutil.CheckSSHResponse(host, sshOpts)
}

//IsUp checks the container's state by querying the Docker API and
//returns true if the host should be available to connect with SSH.
func (dockerMgr *DockerManager) IsUp(host *host.Host) (bool, error) {
	cloudStatus, err := dockerMgr.GetInstanceStatus(host)
	if err != nil {
		return false, err
	}
	if cloudStatus == cloud.StatusRunning {
		return true, nil
	}
	return false, nil
}

func (dockerMgr *DockerManager) OnUp(host *host.Host) error {
	return nil
}

//GetSSHOptions returns an array of default SSH options for connecting to a
//container.
func (dockerMgr *DockerManager) GetSSHOptions(host *host.Host, keyPath string) ([]string, error) {
	if keyPath == "" {
		return []string{}, fmt.Errorf("No key specified for Docker host")
	}

	opts := []string{"-i", keyPath}
	for _, opt := range host.Distro.SSHOptions {
		opts = append(opts, "-o", opt)
	}
	return opts, nil
}

// TimeTilNextPayment returns the amount of time until the next payment is due
// for the host. For Docker this is not relevant.
func (dockerMgr *DockerManager) TimeTilNextPayment(host *host.Host) time.Duration {
	return time.Duration(0)
}
