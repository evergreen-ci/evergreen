package cli

import (
	"context"

	"github.com/cheynewallace/tabby"
	"github.com/mongodb/amboy/management"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

const (
	jobNameFlagName      = "name"
	jobTypeFlagName      = "type"
	jobPatternFlagName   = "job-pattern"
	statusFilterFlagName = "filter"
)

func queueManagement(opts *ServiceOptions) cli.Command {
	return cli.Command{
		Name: "management",
		Subcommands: []cli.Command{
			managementReportJobStatus(opts),
			managementReportJobIDs(opts),
			managementCompleteJob(opts),
			managementCompleteJobsByType(opts),
			managementCompleteJobsByStatus(opts),
			managementCompleteJobsByPattern(opts),
		},
	}
}

func managementReportJobStatus(opts *ServiceOptions) cli.Command {
	return cli.Command{
		Name: "status",
		Flags: opts.managementReportFlags(
			cli.StringFlag{
				Name:  statusFilterFlagName,
				Value: "in-progress",
				Usage: "specify the job status filter. Valid values: 'pending', 'in-progress', 'stale', 'completed', 'retrying', 'stale-retrying', 'all'",
			},
		),
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			filter := management.StatusFilter(c.String(statusFilterFlagName))
			if err := filter.Validate(); err != nil {
				return errors.WithStack(err)
			}

			return opts.withManagementClient(ctx, c, func(client management.Manager) error {
				counts, err := client.JobStatus(ctx, filter)
				if err != nil {
					return errors.WithStack(err)
				}

				t := tabby.New()
				t.AddHeader("Job Type", "Count", "Group")
				for _, c := range counts {
					t.AddLine(c.Type, c.Count, c.Group)
				}
				t.Print()

				return nil
			})
		},
	}
}

func managementReportJobIDs(opts *ServiceOptions) cli.Command {
	return cli.Command{
		Name: "jobs",
		Flags: opts.managementReportFlags(
			cli.StringFlag{
				Name:  "filter",
				Value: "in-progress",
				Usage: "specify the job status filter. Valid values: 'pending', 'in-progress', 'stale', 'completed', 'retrying', 'stale-retrying', 'all'",
			},
		),
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			filter := management.StatusFilter(c.String("filter"))
			if err := filter.Validate(); err != nil {
				return errors.WithStack(err)
			}

			jobTypes := c.StringSlice("type")

			return opts.withManagementClient(ctx, c, func(client management.Manager) error {

				t := tabby.New()
				t.AddHeader("Job Type", "Group", "ID")

				for _, jt := range jobTypes {
					groupedIDs, err := client.JobIDsByState(ctx, jt, filter)
					if err != nil {
						return errors.WithStack(err)
					}
					for _, gid := range groupedIDs {
						t.AddLine(jt, gid.Group, gid.ID)
					}
				}
				t.Print()

				return nil
			})
		},
	}
}
func managementCompleteJob(opts *ServiceOptions) cli.Command {
	return cli.Command{
		Name: "complete-job",
		Flags: opts.managementReportFlags(
			cli.StringFlag{
				Name:  jobNameFlagName,
				Usage: "name of job to complete",
			},
		),
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			name := c.String(jobNameFlagName)

			return opts.withManagementClient(ctx, c, func(client management.Manager) error {
				if err := client.CompleteJob(ctx, name); err != nil {
					return errors.Wrap(err, "problem marking job complete")
				}
				return nil
			})
		},
	}

}

func managementCompleteJobsByStatus(opts *ServiceOptions) cli.Command {
	return cli.Command{
		Name: "complete-jobs",
		Flags: opts.managementReportFlags(
			cli.StringFlag{
				Name:  statusFilterFlagName,
				Value: "in-progress",
				Usage: "specify the process filter, can be 'all', 'completed', 'in-progress', 'pending', or 'stale'",
			},
		),
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			filter := management.StatusFilter(c.String(statusFilterFlagName))

			return opts.withManagementClient(ctx, c, func(client management.Manager) error {
				if err := client.CompleteJobs(ctx, filter); err != nil {
					return errors.Wrap(err, "problem marking jobs complete")
				}
				return nil
			})
		},
	}

}

func managementCompleteJobsByType(opts *ServiceOptions) cli.Command {
	return cli.Command{
		Name: "complete-jobs-by-type",
		Flags: opts.managementReportFlags(
			cli.StringFlag{
				Name:  jobTypeFlagName,
				Usage: "job type to filter by",
			},
			cli.StringFlag{
				Name:  statusFilterFlagName,
				Value: "in-progress",
				Usage: "specify the process filter, can be 'all', 'completed', 'in-progress', 'pending', or 'stale'",
			},
		),
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			jobType := c.String(jobTypeFlagName)
			filter := management.StatusFilter(c.String(statusFilterFlagName))

			return opts.withManagementClient(ctx, c, func(client management.Manager) error {
				if err := client.CompleteJobsByType(ctx, filter, jobType); err != nil {
					return errors.Wrap(err, "problem marking jobs complete")
				}
				return nil
			})
		},
	}
}

func managementCompleteJobsByPattern(opts *ServiceOptions) cli.Command {
	return cli.Command{
		Name: "complete-jobs-by-pattern",
		Flags: opts.managementReportFlags(
			cli.StringFlag{
				Name:  jobPatternFlagName,
				Usage: "job pattern to filter by",
			},
			cli.StringFlag{
				Name:  statusFilterFlagName,
				Value: "in-progress",
				Usage: "specify the process filter, can be 'all', 'completed', 'in-progress', 'pending', or 'stale'",
			},
		),
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			pattern := c.String(jobPatternFlagName)
			filter := management.StatusFilter(c.String(statusFilterFlagName))

			return opts.withManagementClient(ctx, c, func(client management.Manager) error {
				if err := client.CompleteJobsByPattern(ctx, filter, pattern); err != nil {
					return errors.Wrap(err, "problem marking jobs complete")
				}
				return nil
			})
		},
	}
}
