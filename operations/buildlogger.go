package operations

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/evergreen-ci/timber"
	"github.com/evergreen-ci/timber/buildlogger"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func Buildlogger() cli.Command {
	return cli.Command{
		Name:  "buildlogger",
		Usage: "operations for buildlogger logs",
		Subcommands: []cli.Command{
			fetch(),
		},
	}
}

func fetch() cli.Command {
	const (
		cedarBaseURLFlagName  = "cedar_base_url"
		taskIDFlagName        = "task_id"
		testNameFlagName      = "test_name"
		groupIDFlagName       = "group_id"
		startFlagName         = "start"
		endFlagName           = "end"
		executionFlagName     = "execution"
		processNameFlagName   = "proc_name"
		tagsFlagName          = "tags"
		printTimeFlagName     = "print_time"
		printPriorityFlagName = "print_priority"
		tailFlagName          = "tail"
		limitFlagName         = "limit"
		outputFileFlagName    = "out"
	)

	return cli.Command{
		Name:  "fetch",
		Usage: "fetch buildlogger log(s) from cedar",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  cedarBaseURLFlagName,
				Usage: "The base URL for the cedar service",
				Value: "https://cedar.mongodb.com",
			},
			cli.StringFlag{
				Name:  taskIDFlagName,
				Usage: "The task id of the log(s) you would like to download. (Required)",
			},
			cli.StringFlag{
				Name:  testNameFlagName,
				Usage: "The test name of the log(s) you would like to download.",
			},
			cli.StringFlag{
				Name:  groupIDFlagName,
				Usage: "The group id of the log(s) you would like to download.",
			},
			cli.StringFlag{
				Name:  startFlagName,
				Usage: "The start time in RFC3339 format.",
				Value: "0001-01-01T00:00:00Z",
			},
			cli.StringFlag{
				Name:  endFlagName,
				Usage: "The end time in RFC3339 format.",
				Value: "0001-01-01T00:00:00Z",
			},
			cli.IntFlag{
				Name:  executionFlagName,
				Usage: "The execution of the task id. Defaults to the latest.",
			},
			cli.StringFlag{
				Name:  processNameFlagName,
				Usage: "The name of the process the logs are following.",
			},
			cli.StringFlag{
				Name:  tagsFlagName,
				Usage: "Comma separated list of tags to used for filtering logs.",
			},
			cli.BoolFlag{
				Name:  printTimeFlagName,
				Usage: "Print timestamps of log lines.",
			},
			cli.BoolFlag{
				Name:  printPriorityFlagName,
				Usage: "Print priority of log lines.",
			},
			cli.IntFlag{
				Name:  tailFlagName,
				Usage: "Print last N lines of the log(s).",
			},
			cli.IntFlag{
				Name:  limitFlagName,
				Usage: "Print first N lines of the log(s). If tail is set, this is ignored.",
			},
			cli.StringFlag{
				Name:  fmt.Sprintf("%s,o", outputFileFlagName),
				Usage: "Optional output file, defaults to stdout.",
			},
		},
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			confPath := c.Parent().Parent().String(confFlagName)
			conf, err := NewClientSettings(confPath)
			if err != nil {
				return errors.Wrap(err, "problem loading configuration")
			}

			var execution *int
			if c.IsSet(executionFlagName) {
				execution = utility.ToIntPtr(c.Int(executionFlagName))
			}
			start, err := time.Parse(time.RFC3339, c.String(startFlagName))
			if err != nil {
				return errors.Wrapf(err, "unable to parse start time '%s' from RFC3339 format", c.String(startFlagName))
			}
			end, err := time.Parse(time.RFC3339, c.String(endFlagName))
			if err != nil {
				return errors.Wrapf(err, "unable to parse end time '%s' from RFC3339 format", c.String(endFlagName))
			}
			var tags []string
			if c.String(tagsFlagName) != "" {
				tags = strings.Split(c.String(tagsFlagName), ",")
				for i := range tags {
					tags[i] = strings.TrimSpace(tags[i])
				}
			}

			opts := buildlogger.GetOptions{
				Cedar: timber.GetOptions{
					BaseURL:  c.String(cedarBaseURLFlagName),
					UserKey:  conf.APIKey,
					UserName: conf.User,
				},
				TaskID:        c.String(taskIDFlagName),
				TestName:      c.String(testNameFlagName),
				Execution:     execution,
				GroupID:       c.String(groupIDFlagName),
				Start:         start,
				End:           end,
				ProcessName:   c.String(processNameFlagName),
				Tags:          tags,
				PrintTime:     c.Bool(printTimeFlagName),
				PrintPriority: c.Bool(printPriorityFlagName),
				Tail:          c.Int(tailFlagName),
				Limit:         c.Int(limitFlagName),
			}
			r, err := buildlogger.Get(ctx, opts)
			if err != nil {
				return errors.Wrap(err, "problem fetching log(s)")
			}
			defer r.Close()

			out := os.Stdout
			fn := c.String(outputFileFlagName)
			if fn != "" {
				out, err = os.Create(fn)
				if err != nil {
					return errors.Wrap(err, "problem creating output file")
				}
				defer out.Close()
			}

			_, err = io.Copy(out, r)
			return errors.Wrap(err, "problem reading log(s)")

		},
	}
}
