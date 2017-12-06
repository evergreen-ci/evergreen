// +build go1.7

package cloud

import (
	"context"
	"net/url"
	"time"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"
)

const (
	timeout = 1000 * time.Millisecond
)

type authOptions struct {
	Host     string `yaml:"host"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// The client interface wraps interaction with the vCenter server.
type vsphereClient interface {
	Init(*authOptions) error
	GetIP(*host.Host) (string, error)
	GetPowerState(*host.Host) (types.VirtualMachinePowerState, error)
	CreateInstance(*host.Host, *ProviderSettings) (string, error)
	DeleteInstance(*host.Host) error
}

type vsphereClientImpl struct {
	Client     *govmomi.Client
	Datacenter *object.Datacenter
	Finder     *find.Finder
}

func (c *vsphereClientImpl) Init(ao *authOptions) error {
	ctx := context.TODO()
	u := &url.URL{
		Scheme: "https",
		User:   url.UserPassword(ao.Username, ao.Password),
		Host:   ao.Host,
		Path:   "sdk",
	}

	// Note: this turns off certificate validation.
	insecureSkipVerify := true
	client, err := govmomi.NewClient(ctx, u, insecureSkipVerify)
	if err != nil {
		return errors.Wrapf(err, "could not connect to vmware host")
	}

	if !client.IsVC() {
		return errors.New("successfully connected to host," +
			"but host is not a vCenter server")
	}

	grip.Debug("connected to vCenter server")
	c.Client = client
	c.Finder = find.NewFinder(client.Client, true)

	// Set datacenter path for finder to search for objects.
	dc, err := c.Finder.DefaultDatacenter(ctx)
	if err != nil {
		return errors.Wrap(err, "could not find default datacenter")
	}
	c.Finder.SetDatacenter(dc)
	c.Datacenter = dc
	grip.Debugf("finder will look in datacenter %s", dc.Common.InventoryPath)

	return nil
}

func (c *vsphereClientImpl) GetIP(h *host.Host) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	vm, err := c.getInstance(ctx, h.Id)
	if err != nil {
		return "", errors.Wrap(err, "API call to get instance failed")
	}

	ip, err := vm.WaitForIP(ctx)
	if err != nil {
		return "", errors.Wrapf(err, "could not read ip for host %s", h.Id)
	}

	return ip, nil
}

func (c *vsphereClientImpl) GetPowerState(h *host.Host) (types.VirtualMachinePowerState, error) {
	ctx := context.TODO()

	vm, err := c.getInstance(ctx, h.Id)
	if err != nil {
		err = errors.Wrap(err, "API call to get instance failed")
		return types.VirtualMachinePowerState(""), err
	}

	state, err := vm.PowerState(ctx)
	if err != nil {
		err = errors.Wrapf(err, "could not read power state for host %s", h.Id)
		return types.VirtualMachinePowerState(""), err
	}

	return state, nil
}

func (c *vsphereClientImpl) CreateInstance(h *host.Host, s *ProviderSettings) (string, error) {
	ctx := context.TODO()

	// Locate and organize resources for creating a virtual machine.
	grip.Info(message.Fields{
		"message":       "locating and organizing resources for creating a vm",
		"datacenter":    c.Datacenter,
		"template":      s.Template,
		"datastore":     s.Datastore,
		"resource_pool": s.ResourcePool,
		"num_cpus":      s.NumCPUs,
		"memory_mb":     s.MemoryMB,
	})

	t, err := c.getInstance(ctx, s.Template)
	if err != nil {
		return "", errors.Wrapf(err, "error finding template %s", s.Template)
	}

	folders, err := c.Datacenter.Folders(ctx)
	if err != nil {
		return "", errors.Wrapf(err, "error getting folders from datacenter %v", c.Datacenter)
	}

	spec, err := c.cloneSpec(s)
	if err != nil {
		err = errors.Wrap(err, "error making spec to clone vm")
		grip.Error(err)
		return "", err
	}

	// Deploy the virtual machine from a template as a virtual machine.
	if _, err = t.Clone(ctx, folders.VmFolder, h.Id, spec); err != nil {
		err = errors.Wrapf(err, "error making task to clone vm %s", t)
		grip.Error(err)
		return "", err
	}

	grip.Info(message.Fields{
		"message":  "cloning vm, may take a few minutes to start up...",
		"template": s.Template,
		"host_id":  h.Id,
	})

	return h.Id, nil
}

func (c *vsphereClientImpl) DeleteInstance(h *host.Host) error {
	ctx := context.TODO()

	vm, err := c.getInstance(ctx, h.Id)
	if err != nil {
		return errors.Wrap(err, "API call to get instance failed")
	}

	// make sure the instance is powered off before removing
	state, err := vm.PowerState(ctx)
	if err != nil {
		return errors.Wrapf(err, "could not read power state", h.Id)
	}

	if state == types.VirtualMachinePowerStatePoweredOn {
		task, err := vm.PowerOff(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to create power off task")
		}

		if err = task.Wait(ctx); err != nil {
			return errors.Wrap(err, "power off task failed to execute")
		}
	}

	// remove the instance
	if _, err = vm.Destroy(ctx); err != nil {
		return errors.Wrapf(err, "error destroying vm %v", vm)
	}

	return nil
}
