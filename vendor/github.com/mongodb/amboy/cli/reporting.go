package cli

import (
	"context"
	"time"

	"github.com/cheynewallace/tabby"
	"github.com/mongodb/amboy/reporting"
	"github.com/mongodb/amboy/rest"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func reports(opts *ServiceOptions) cli.Command {
	return cli.Command{
		Name: "report",
		Subcommands: []cli.Command{
			reportingJobStatus(opts),
			reportingRecentTiming(opts),
			reportingJobIDs(opts),
			reportingRecentErrors(opts),
		},
	}
}

func reportingJobStatus(opts *ServiceOptions) cli.Command {
	return cli.Command{
		Name: "status",
		Flags: opts.reportingFlags(
			cli.StringFlag{
				Name:  "filter",
				Value: "in-progress",
				Usage: "specify the process filter, can be 'in-progress', 'pending', or 'stale'",
			},
		),
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			filter := reporting.CounterFilter(c.String("filter"))
			if err := filter.Validate(); err != nil {
				return errors.WithStack(err)
			}

			return opts.withReportingClient(ctx, c, func(client *rest.ReportingClient) error {
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

func reportingRecentTiming(opts *ServiceOptions) cli.Command {
	return cli.Command{
		Name: "timing",
		Flags: opts.reportingFlags(
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
			filter := reporting.RuntimeFilter(c.String("filter"))
			if err := filter.Validate(); err != nil {
				return errors.WithStack(err)
			}

			return opts.withReportingClient(ctx, c, func(client *rest.ReportingClient) error {
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

func reportingJobIDs(opts *ServiceOptions) cli.Command {
	return cli.Command{
		Name: "jobs",
		Flags: opts.reportingFlags(
			cli.StringFlag{
				Name:  "filter",
				Value: "in-progress",
				Usage: "specify the process filter, can be 'in-progress', 'pending', or 'stale'",
			},
		),
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			filter := reporting.CounterFilter(c.String("filter"))
			if err := filter.Validate(); err != nil {
				return errors.WithStack(err)
			}

			jobTypes := c.StringSlice("type")

			return opts.withReportingClient(ctx, c, func(client *rest.ReportingClient) error {

				t := tabby.New()
				t.AddHeader("Job Type", "ID", "Group")

				for _, jt := range jobTypes {
					report, err := client.JobIDsByState(ctx, jt, filter)
					if err != nil {
						return errors.WithStack(err)
					}
					for _, j := range report.IDs {
						t.AddLine(jt, j, report.Group)
					}
				}
				t.Print()

				return nil
			})
		},
	}
}

func reportingRecentErrors(opts *ServiceOptions) cli.Command {
	return cli.Command{
		Name: "errors",
		Flags: opts.reportingFlags(
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

			filter := reporting.ErrorFilter(c.String("filter"))
			if err := filter.Validate(); err != nil {
				return errors.WithStack(err)
			}

			jobTypes := c.StringSlice("type")

			return opts.withReportingClient(ctx, c, func(client *rest.ReportingClient) error {
				reports := []*reporting.JobErrorsReport{}

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
