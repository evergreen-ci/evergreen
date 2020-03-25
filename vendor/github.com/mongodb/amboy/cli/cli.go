package cli

import (
	"context"
	"net/http"
	"strings"

	"github.com/mongodb/amboy/rest"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

// Amboy provides a reusable CLI component for interacting with rest
// management and reporting services.
func Amboy(opts *ServiceOptions) cli.Command {
	return cli.Command{
		Name:  "amboy",
		Usage: "access administrative rest interfaces for an amboy service",
		Subcommands: []cli.Command{
			managementReports(opts),
			abortablePoolManagement(opts),
		},
	}
}

// ServiceOptions makes it possible for users of Amboy to create a cli
// tool with reasonable defaults and with client.
type ServiceOptions struct {
	BaseURL          string
	ReportingPrefix  string
	ManagementPrefix string
	Client           *http.Client
}

const (
	serviceURLFlagName = "service"
	prefixFlagName     = "prefix"
)

func (o *ServiceOptions) managementReportFlags(base ...cli.Flag) []cli.Flag {
	return append(base,
		cli.StringFlag{
			Name:  serviceURLFlagName,
			Usage: "Specify the base URL of the service",
			Value: o.BaseURL,
		},
		cli.StringFlag{
			Name:  prefixFlagName,
			Usage: "Specify the service prefix for the reporting service.",
			Value: o.ReportingPrefix,
		},
	)
}

func (o *ServiceOptions) abortablePoolManagementFlags(base ...cli.Flag) []cli.Flag {
	return append(base,
		cli.StringFlag{
			Name:  serviceURLFlagName,
			Usage: "Specify the base URL of the service.",
			Value: o.BaseURL,
		},
		cli.StringFlag{
			Name:  prefixFlagName,
			Usage: "Specify the service prefix for the management service.",
			Value: o.ManagementPrefix,
		},
	)
}

func (o *ServiceOptions) withManagementClient(ctx context.Context, c *cli.Context, op func(client *rest.ManagementClient) error) error {
	if o.Client == nil {
		o.Client = http.DefaultClient
	}

	client := rest.NewManagementClientFromExisting(o.Client, getCLIPath(c))

	return errors.WithStack(op(client))
}

func (o *ServiceOptions) withAbortablePoolManagementClient(ctx context.Context, c *cli.Context, op func(client *rest.AbortablePoolManagementClient) error) error {
	if o.Client == nil {
		o.Client = http.DefaultClient
	}

	client := rest.NewAbortablePoolManagementClientFromExisting(o.Client, getCLIPath(c))

	return errors.WithStack(op(client))
}

func getCLIPath(c *cli.Context) string {
	return strings.TrimRight(c.String(serviceURLFlagName), "/") + "/" + strings.TrimLeft(c.String(prefixFlagName), "/")
}
