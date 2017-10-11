// +build go1.7

package gce

import (
	"context"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/jwt"

	compute "google.golang.org/api/compute/v1"
)

const (
	computeScope = compute.ComputeScope
)

// The client interface wraps the Google Compute client interaction.
type client interface {
	Init(*jwt.Config) error
	CreateInstance(*host.Host, *ProviderSettings) (string, error)
	GetInstance(*host.Host) (*compute.Instance, error)
	DeleteInstance(*host.Host) error
}

type clientImpl struct {
	InstancesService *compute.InstancesService
}

// Init establishes a connection to a Google Compute endpoint and creates a client that
// can be used to manage instances.
func (c *clientImpl) Init(config *jwt.Config) error {
	ctx := context.TODO()
	ts := config.TokenSource(ctx)

	client := oauth2.NewClient(ctx, ts)
	grip.Debug("created an OAuth HTTP client to Google services")

	// Connect to Google Compute Engine.
	service, err := compute.New(client)
	if err != nil {
		return errors.Wrap(err, "failed to connect to Google Compute Engine service")
	}
	grip.Debug("connected to Google Compute Engine service")

	// Get a handle to a specific service for instance configuration.
	c.InstancesService = compute.NewInstancesService(service)

	return nil
}

// CreateInstance requests an instance to be provisioned.
//
// API calls to an instance refer to the instance by the user-provided name (which must be unique)
// and not the ID. If successful, CreateInstance returns the name of the provisioned instance.
func (c *clientImpl) CreateInstance(h *host.Host, s *ProviderSettings) (string, error) {
	// Create instance options to spawn an instance
	machineType := makeMachineType(h.Zone, s.MachineName, s.NumCPUs, s.MemoryMB)
	instance := &compute.Instance{
		Name:        h.Id,
		MachineType: machineType,
		Labels:      makeLabels(h),
		Tags:        &compute.Tags{Items: s.NetworkTags},
	}

	grip.Debug(message.Fields{
		"message":      "creating instance",
		"host":         h.Id,
		"machine_type": machineType,
	})

	// Add the disk with the image URL
	var imageURL string
	if s.ImageFamily != "" {
		imageURL = makeImageFromFamily(s.ImageFamily)
	} else {
		imageURL = makeImage(s.ImageName)
	}

	diskType := makeDiskType(h.Zone, s.DiskType)
	instance.Disks = []*compute.AttachedDisk{&compute.AttachedDisk{
		AutoDelete: true,
		Boot:       true,
		DeviceName: instance.Name,
		InitializeParams: &compute.AttachedDiskInitializeParams{
			DiskSizeGb:  s.DiskSizeGB,
			DiskType:    diskType,
			SourceImage: imageURL,
		},
	}}

	grip.Debug(message.Fields{
		"message":      "attaching boot disk",
		"host":         h.Id,
		"disk_size":    s.DiskSizeGB,
		"disk_type":    diskType,
		"source_image": imageURL,
	})

	// Attach a network interface
	instance.NetworkInterfaces = []*compute.NetworkInterface{&compute.NetworkInterface{
		AccessConfigs: []*compute.AccessConfig{&compute.AccessConfig{}},
	}}

	// Add the ssh keys
	keys := s.SSHKeys.String()
	instance.Metadata = &compute.Metadata{
		Items: []*compute.MetadataItems{
			&compute.MetadataItems{Key: "ssh-keys", Value: &keys},
		},
	}

	grip.Debug(message.Fields{
		"message":  "attaching metadata items",
		"host":     h.Id,
		"ssh_keys": keys,
	})

	// Make the API call to insert the instance
	if _, err := c.InstancesService.Insert(h.Project, h.Zone, instance).Do(); err != nil {
		return "", errors.Wrap(err, "API call to insert instance failed")
	}

	return instance.Name, nil
}

// GetInstance requests details on a single instance.
func (c *clientImpl) GetInstance(h *host.Host) (*compute.Instance, error) {
	instance, err := c.InstancesService.Get(h.Project, h.Zone, h.Id).Do()
	if err != nil {
		return nil, errors.Wrap(err, "API call to get instance failed")
	}

	return instance, nil
}

// DeleteInstance requests an instance previously provisioned to be removed.
func (c *clientImpl) DeleteInstance(h *host.Host) error {
	if _, err := c.InstancesService.Delete(h.Project, h.Zone, h.Id).Do(); err != nil {
		return errors.Wrap(err, "API call to delete instance failed")
	}

	return nil
}
