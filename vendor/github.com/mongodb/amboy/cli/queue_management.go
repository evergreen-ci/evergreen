package cli

import (
	"context"
	"time"

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
			managementReportRecentTiming(opts),
			managementReportJobIDs(opts),
			managementReportRecentErrors(opts),
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
				Usage: "specify the process filter, can be 'all', 'completed', 'in-progress', 'pending', or 'stale'",
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
				report, err := client.JobStatus(ctx, filter)
				if err != nil {
					return errors.WithStack(err)
				}

				t := tabby.New()
				t.AddHeader("Job Type", "Count", "Group")
				for _, r := range report.Stats {
					t.AddLine(r.ID, r.Count, r.Group)
				}
				t.Print()

				return nil
			})
		},
	}
}

func managementReportRecentTiming(opts *ServiceOptions) cli.Command {
	return cli.Command{
		Name: "timing",
		Flags: opts.managementReportFlags(
			cli.DurationFlag{
				Name:  "duration, d",
				Value: time.Minute,
				Usage: "specify a duration in string form (e.g. 100ms, 1s, 1m, 1h) to limit the report",
			},
			cli.StringFlag{
				Name:  "filter",
				Value: "completed",
				Usage: "specify the runtime filter, can be 'completed', 'latency', or 'running'",
			},
		),
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			dur := c.Duration("duration")
			filter := management.RuntimeFilter(c.String("filter"))
			if err := filter.Validate(); err != nil {
				return errors.WithStack(err)
			}

			return opts.withManagementClient(ctx, c, func(client management.Manager) error {
				report, err := client.RecentTiming(ctx, dur, filter)
				if err != nil {
					return errors.WithStack(err)
				}

				t := tabby.New()
				t.AddHeader("Job ID", "Duration (sec)", "Group")
				for _, r := range report.Stats {
					t.AddLine(r.ID, r.Duration.Seconds(), r.Group)
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
				Usage: "specify the process filter, can be 'all', 'in-progress', 'completed', 'pending', or 'stale'",
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
					report, err := client.JobIDsByState(ctx, jt, filter)
					if err != nil {
						return errors.WithStack(err)
					}
					for _, j := range report.GroupedIDs {
						t.AddLine(jt, j.Group, j.ID)
					}
				}
				t.Print()

				return nil
			})
		},
	}
}

func managementReportRecentErrors(opts *ServiceOptions) cli.Command {
	return cli.Command{
		Name: "errors",
		Flags: opts.managementReportFlags(
			cli.DurationFlag{
				Name:  "duration, d",
				Value: time.Minute,
				Usage: "specify a duration in string form (e.g. 100ms, 1s, 1m, 1h) to limit the report",
			},
			cli.StringFlag{
				Name:  "filter",
				Value: "unique-errors",
				Usage: "specify the process filter, can be 'unique-errors', 'all-errors', or 'stats-only'",
			},
		),
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			dur := c.Duration("duration")

			filter := management.ErrorFilter(c.String("filter"))
			if err := filter.Validate(); err != nil {
				return errors.WithStack(err)
			}

			jobTypes := c.StringSlice("type")

			return opts.withManagementClient(ctx, c, func(client management.Manager) error {
				reports := []*management.JobErrorsReport{}

				if len(jobTypes) == 0 {
					report, err := client.RecentErrors(ctx, dur, filter)
					if err != nil {
						return errors.WithStack(err)
					}
					reports = append(reports, report)
				} else {
					for _, jt := range jobTypes {
						report, err := client.RecentJobErrors(ctx, jt, dur, filter)
						if err != nil {
							return errors.WithStack(err)
						}
						reports = append(reports, report)
					}
				}

				t := tabby.New()
				t.AddHeader("Job Type", "Count", "Total", "Average", "First Error", "Group")
				for _, report := range reports {
					for _, d := range report.Data {
						if len(d.Errors) > 0 {
							t.AddLine(d.ID, d.Count, d.Total, d.Average, d.Errors[0], d.Group)
						} else {
							t.AddLine(d.ID, d.Count, d.Total, d.Average, "", d.Group)
						}
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
