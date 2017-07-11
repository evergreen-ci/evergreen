// +build go1.7

package vsphere

import (
	"context"
	"net/url"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/vmware/govmomi"
)

type authOptions struct {
	Host     string `yaml:"host"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// The client interface wraps interaction with the vCenter server.
type client interface {
	Init(*authOptions) error
}

type clientImpl struct {
	Client *govmomi.Client
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

	return nil
}
