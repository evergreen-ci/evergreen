package cli

import (
	"context"

	"github.com/cheynewallace/tabby"
	"github.com/mongodb/amboy/rest"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func management(opts *ServiceOptions) cli.Command {
	return cli.Command{
		Name: "manage",
		Subcommands: []cli.Command{
			manageListJobs(opts),
			manageAbortAllJobs(opts),
			manageCheckJob(opts),
			manageAbortJob(opts),
		},
	}
}

func manageListJobs(opts *ServiceOptions) cli.Command {
	return cli.Command{
		Name:  "list",
		Flags: opts.managementFlags(),
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			return opts.withManagementClient(ctx, c, func(client *rest.ManagementClient) error {
				jobs, err := client.ListJobs(ctx)
				if err != nil {
					return errors.WithStack(err)
				}

				t := tabby.New()
				t.AddHeader("Job ID")
				for _, j := range jobs {
					t.AddLine(j)
				}
				t.Print()

				return nil
			})

		},
	}
}

func manageAbortAllJobs(opts *ServiceOptions) cli.Command {
	return cli.Command{
		Name:  "abort-all",
		Flags: opts.managementFlags(),
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			return opts.withManagementClient(ctx, c, func(client *rest.ManagementClient) error {
				return errors.WithStack(client.AbortAllJobs(ctx))
			})

		},
	}
}

func manageCheckJob(opts *ServiceOptions) cli.Command {
	const jobIDFlagName = "id"

	return cli.Command{
		Name: "check",
		Flags: opts.managementFlags(
			cli.StringSliceFlag{
				Name:  jobIDFlagName,
				Usage: "specify the name of the job to check. May specify more than once.",
			},
		),
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			return opts.withManagementClient(ctx, c, func(client *rest.ManagementClient) error {
				t := tabby.New()
				t.AddHeader("Job ID", "Is Running")
				for _, j := range c.StringSlice(jobIDFlagName) {
					isRunning, err := client.IsRunning(ctx, j)
					if err != nil {
						return errors.WithStack(err)
					}

					t.AddLine(j, isRunning)
				}
				t.Print()

				return nil
			})

		},
	}
}

func manageAbortJob(opts *ServiceOptions) cli.Command {
	const jobIDFlagName = "id"

	return cli.Command{
		Name: "abort",
		Flags: opts.managementFlags(
			cli.StringSliceFlag{
				Name:  jobIDFlagName,
				Usage: "specify the name of the job to abort. May specify more than once.",
			},
		),
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			return opts.withManagementClient(ctx, c, func(client *rest.ManagementClient) error {
				var hasErrors bool
				t := tabby.New()
				t.AddHeader("Job ID", "Aborted", "Error")
				for _, j := range c.StringSlice(jobIDFlagName) {
					err := client.AbortJob(ctx, j)
					if err == nil {
						t.AddLine(j, true, "")
					} else {
						hasErrors = true
						t.AddLine(j, false, err.Error())
					}
				}
				t.Print()

				if hasErrors {
					return errors.New("problem aborting some jobs")
				}

				return nil
			})

		},
	}
}
