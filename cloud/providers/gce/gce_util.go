// +build go1.7

package gce

import (
	"os"
	"os/user"
	"fmt"
	"regexp"
	"strings"

	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/host"
)

const (
	// nameTimeFormat is the format in which to log times like start time.
	nameTimeFormat = "20060102150405"
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

func toEvgStatus(status string) cloud.CloudStatus {
	switch status {
	case statusProvisioning, statusStaging:
		return cloud.StatusInitializing
	case statusRunning:
		return cloud.StatusRunning
	case statusStopping:
		return cloud.StatusStopped
	case statusTerminated:
		return cloud.StatusTerminated
	default:
		return cloud.StatusUnknown
	}
}

// Returns a machine type URL for the given the zone
func makeMachineType(zone, machineType string) string {
	return fmt.Sprintf("zones/%s/machineTypes/%s", zone, machineType)
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

// Makes labels to attach to the VM instance. Only hyphens (-),
// underscores (_), lowercase characters, and numbers are allowed.
func makeTags(intent *host.Host) map[string]string {
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
		"start-time":        intent.CreationTime.Format(nameTimeFormat),
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
