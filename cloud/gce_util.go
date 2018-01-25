// +build go1.7

package cloud

import (
	"fmt"
	"os"
	"os/user"
	"regexp"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
)

const (
	// statusProvisioning means resources are being reserved for the instance.
	statusProvisioning = "PROVISIONING"
	// statusStaging means resources have been acquired and the instance is
	// preparing for launch.
	statusStaging = "STAGING"
	// statusRunning means the instance is booting up or running. You should be
	// able to SSH into the instance soon, though not immediately, after it
	// enters this state.
	statusRunning = "RUNNING"
	// statusStopping means the instance is being stopped either due to a
	// failure, or the instance is being shut down. This is a temporary status
	// before it becomes terminated.
	statusStopping = "STOPPING"
	// statusTerminated means the instance was shut down or encountered a
	// failure, either through the API or from inside the guest.
	statusTerminated = "TERMINATED"
	// maxNameLength is the maximum length of an instance name permitted by GCE.
	maxNameLength = 63
)

// SSHKey is the ssh key information to add to a VM instance.
type sshKey struct {
	Username  string `mapstructure:"username"`
	PublicKey string `mapstructure:"public_key"`
}

type sshKeyGroup []sshKey

// Formats the ssh key in "username:publickey" format.
// This is the format required by Google Compute's REST API.
func (key sshKey) String() string {
	return fmt.Sprintf("%s:%s", key.Username, key.PublicKey)
}

// Formats the ssh keys in "username:publickey" format joined by newlines.
// This is the format required by Google Compute's REST API.
func (keys sshKeyGroup) String() string {
	arr := make([]string, len(keys))
	for i, key := range keys {
		arr[i] = key.String()
	}
	return strings.Join(arr, "\n")
}

func gceToEvgStatus(status string) CloudStatus {
	switch status {
	case statusProvisioning, statusStaging:
		return StatusInitializing
	case statusRunning:
		return StatusRunning
	case statusStopping:
		return StatusStopped
	case statusTerminated:
		return StatusTerminated
	default:
		return StatusUnknown
	}
}

// Returns a machine type URL for the given the zone
func makeMachineType(zone, machineName string, cpus, memory int64) string {
	if machineName != "" {
		return fmt.Sprintf("zones/%s/machineTypes/%s", zone, machineName)
	}
	return fmt.Sprintf("zones/%s/machineTypes/custom-%d-%d", zone, cpus, memory)
}

// Returns a disk type URL given the zone
func makeDiskType(zone, disk string) string {
	return fmt.Sprintf("zones/%s/diskTypes/%s", zone, disk)
}

// Returns an image source URL for a private image family. The URL refers to
// the newest image version associated with the given family.
func makeImageFromFamily(family string) string {
	return fmt.Sprintf("global/images/family/%s", family)
}

// Returns an image source URL for a private image.
func makeImage(name string) string {
	return fmt.Sprintf("global/images/%s", name)
}

// Generates a unique instance name for an instance based on the distro ID.
// Must be a match of regex '(?:[a-z](?:[-a-z0-9]{0,61}[a-z0-9])?)'
func generateName(d *distro.Distro) string {
	name := d.GenerateName()

	// Ensure all characters in tags are on the whitelist
	r, _ := regexp.Compile("[^a-z0-9_-]+")
	name = string(r.ReplaceAll([]byte(strings.ToLower(name)), []byte("")))

	// Ensure the new name's is no longer than maxNameLength
	if len(name) > maxNameLength {
		return name[:maxNameLength]
	}
	return name
}

// Makes labels to attach to the VM instance. Only hyphens (-),
// underscores (_), lowercase characters, and numbers are allowed.
func makeLabels(intent *host.Host) map[string]string {
	// Get requester host name
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	// Get requester user name
	var username string
	user, err := user.Current()
	if err != nil {
		username = "unknown"
	} else {
		username = user.Name
	}

	tags := map[string]string{
		"distro":            intent.Distro.Id,
		"evergreen-service": hostname,
		"username":          username,
		"owner":             intent.StartedBy,
		"mode":              "production",
		"start-time":        intent.CreationTime.Format(evergreen.NameTimeFormat),
	}

	// Ensure all characters in tags are on the whitelist
	for k, v := range tags {
		r, _ := regexp.Compile("[^a-z0-9_-]+")
		tags[k] = string(r.ReplaceAll([]byte(strings.ToLower(v)), []byte("")))
	}

	if intent.UserHost {
		tags["mode"] = "testing"
	}
	return tags
}
