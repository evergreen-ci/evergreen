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
	Init(context.Context, *authOptions) error
	GetIP(context.Context, *host.Host) (string, error)
	GetPowerState(context.Context, *host.Host) (types.VirtualMachinePowerState, error)
	CreateInstance(context.Context, *host.Host, *vsphereSettings) (string, error)
	DeleteInstance(context.Context, *host.Host) error
}

type vsphereClientImpl struct {
	Client     *govmomi.Client
	Datacenter *object.Datacenter
	Finder     *find.Finder
}

func (c *vsphereClientImpl) Init(ctx context.Context, ao *authOptions) error {
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
		return errors.Wrapf(err, "connecting to to VMware host")
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
		return errors.Wrap(err, "finding default datacenter")
	}
	c.Finder.SetDatacenter(dc)
	c.Datacenter = dc
	grip.Debugf("finder will look in datacenter '%s'", dc.Common.InventoryPath)

	return nil
}

func (c *vsphereClientImpl) GetIP(ctx context.Context, h *host.Host) (string, error) {
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, timeout)
	defer cancel()

	vm, err := c.getInstance(ctx, h.Id)
	if err != nil {
		return "", errors.Wrap(err, "getting instance")
	}

	ip, err := vm.WaitForIP(ctx)
	if err != nil {
		return "", errors.Wrapf(err, "waiting for IP for host '%s'", h.Id)
	}

	return ip, nil
}

func (c *vsphereClientImpl) GetPowerState(ctx context.Context, h *host.Host) (types.VirtualMachinePowerState, error) {
	vm, err := c.getInstance(ctx, h.Id)
	if err != nil {
		return types.VirtualMachinePowerState(""), errors.Wrap(err, "getting instance")
	}

	state, err := vm.PowerState(ctx)
	if err != nil {
		return types.VirtualMachinePowerState(""), errors.Wrapf(err, "reading power state for host '%s'", h.Id)
	}

	return state, nil
}

func (c *vsphereClientImpl) CreateInstance(ctx context.Context, h *host.Host, s *vsphereSettings) (string, error) {
	// Locate and organize resources for creating a virtual machine.
	grip.Info(message.Fields{
		"message":       "locating and organizing resources for creating a VM",
		"datacenter":    c.Datacenter,
		"template":      s.Template,
		"datastore":     s.Datastore,
		"resource_pool": s.ResourcePool,
		"num_cpus":      s.NumCPUs,
		"memory_mb":     s.MemoryMB,
	})

	t, err := c.getInstance(ctx, s.Template)
	if err != nil {
		return "", errors.Wrapf(err, "finding template '%s'", s.Template)
	}

	folders, err := c.Datacenter.Folders(ctx)
	if err != nil {
		return "", errors.Wrapf(err, "getting folders from datacenter %v", c.Datacenter)
	}

	spec, err := c.cloneSpec(ctx, s)
	if err != nil {
		err = errors.Wrap(err, "making spec to clone VM")
		grip.Error(err)
		return "", err
	}

	// Deploy the virtual machine from a template as a virtual machine.
	if _, err = t.Clone(ctx, folders.VmFolder, h.Id, spec); err != nil {
		err = errors.Wrapf(err, "making task to clone VM %v", t)
		grip.Error(err)
		return "", err
	}

	grip.Info(message.Fields{
		"message":  "cloning VM, may take a few minutes to start up...",
		"template": s.Template,
		"host_id":  h.Id,
	})

	return h.Id, nil
}

func (c *vsphereClientImpl) DeleteInstance(ctx context.Context, h *host.Host) error {
	vm, err := c.getInstance(ctx, h.Id)
	if err != nil {
		return errors.Wrap(err, "getting instance")
	}

	// make sure the instance is powered off before removing
	state, err := vm.PowerState(ctx)
	if err != nil {
		return errors.Wrapf(err, "reading power state of host '%s'", h.Id)
	}

	if state == types.VirtualMachinePowerStatePoweredOn {
		var task *object.Task
		task, err = vm.PowerOff(ctx)
		if err != nil {
			return errors.Wrap(err, "creating power off task")
		}

		if err = task.Wait(ctx); err != nil {
			return errors.Wrap(err, "waiting for VM to power off")
		}
	}

	// remove the instance
	if _, err = vm.Destroy(ctx); err != nil {
		return errors.Wrapf(err, "destroying VM %v", vm)
	}

	return nil
}
