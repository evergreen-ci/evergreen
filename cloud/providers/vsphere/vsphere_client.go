// +build go1.7

package vsphere

import (
	"net/url"
	"time"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"golang.org/x/net/context"

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
type client interface {
	Init(*authOptions) error
	GetIP(*host.Host) (string, error)
	GetPowerState(*host.Host) (types.VirtualMachinePowerState, error)
}

type clientImpl struct {
	Client *govmomi.Client
	Finder *find.Finder
}

func (c *clientImpl) Init(ao *authOptions) error {
	ctx := context.TODO()
	u := &url.URL{
		Scheme: "https",
		User: url.UserPassword(ao.Username, ao.Password),
		Host: ao.Host,
		Path: "sdk",
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
	grip.Debugf("finder will look in datacenter %v", dc)

	return nil
}

func (c *clientImpl) getInstance(ctx context.Context, h *host.Host) (*object.VirtualMachine, error) {
	vm, err := c.Finder.VirtualMachine(ctx, h.Id)
	if err != nil {
		return nil, errors.Wrapf(err, "could not find vm for host %s", h.Id)
	}

	return vm, err
}

func (c *clientImpl) GetIP(h *host.Host) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	vm, err := c.getInstance(ctx, h)
	if err != nil {
		return "", errors.Wrap(err, "API call to get instance failed")
	}

	ip, err := vm.WaitForIP(ctx)
	if err != nil {
		return "", errors.Wrapf(err, "could not read ip for host %s", h.Id)
	}

	return ip, nil
}

func (c *clientImpl) GetPowerState(h *host.Host) (types.VirtualMachinePowerState, error) {
	ctx := context.TODO()

	vm, err := c.getInstance(ctx, h)
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
