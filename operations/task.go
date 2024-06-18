package operations

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

const (
	taskIDFlagName    = "task_id"
	executionFlagName = "execution"

	logStartFlagName         = "start"
	logEndFlagName           = "end"
	logLineLimitFlagName     = "line_limit"
	logTailLimitFlagName     = "tail_limit"
	logPrintTimeFlagName     = "print_time"
	logPrintPriorityFlagName = "print_priority"
	logPaginateFlagName      = "paginate"
	logOutputFileFlagName    = "out"
)

func Task() cli.Command {
	flags := []cli.Flag{
		cli.StringFlag{
			Name:     taskIDFlagName,
			Usage:    "ID of the task.",
			Required: true,
		},
		cli.IntFlag{
			Name:  executionFlagName,
			Usage: "The 0-based number corresponding to the execution of the task ID.",
		},
	}

	return cli.Command{
		Name:  "task",
		Usage: "operations for Evergreen tasks",
		Subcommands: []cli.Command{
			taskBuild(flags),
		},
	}
}

func taskBuild(flags []cli.Flag) cli.Command {
	return cli.Command{
		Name:  "build",
		Usage: "operations for downloading task (build) output",
		Subcommands: []cli.Command{
			taskLogs(flags),
			testLogs(flags),
		},
	}
}

func taskLogs(flags []cli.Flag) cli.Command {
	const logTypeFlagName = "type"

	return cli.Command{
		Name:  "TaskLogs",
		Usage: "download task logs",
		Flags: mergeFlagSlices(
			flags,
			[]cli.Flag{
				cli.StringFlag{
					Name:  logTypeFlagName,
					Usage: "Task log type. Must be one of: \"agent_log\", \"system_log\", \"task_log\", \"all_logs\".",
					Value: "all_logs",
				},
			},
			taskOutputLogFlags(),
		),
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(c.Parent().String(confFlagName))
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}

			restClient, err := conf.setupRestCommunicator(ctx, false)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer restClient.Close()

			var execution *int
			if c.IsSet(executionFlagName) {
				execution = utility.ToIntPtr(c.Int(executionFlagName))
			}

			r, err := restClient.GetTaskLogs(ctx, client.GetTaskLogsOptions{
				TaskID: c.String(taskIDFlagName),
				//Execution:     utility.ToIntPtr(c.Int(executionFlagName)),
				Execution:     execution,
				Type:          c.String(logTypeFlagName),
				Start:         c.String(logStartFlagName),
				End:           c.String(logEndFlagName),
				LineLimit:     c.Int(logLineLimitFlagName),
				TailLimit:     c.Int(logTailLimitFlagName),
				PrintTime:     c.Bool(logPrintTimeFlagName),
				PrintPriority: c.Bool(logPrintPriorityFlagName),
				Paginate:      c.Bool(logPaginateFlagName),
			})
			if err != nil {
				return errors.Wrap(err, "getting task logs")
			}
			defer r.Close()

			out := os.Stdout
			fmt.Println(c.String(logOutputFileFlagName))
			if fn := c.String(logOutputFileFlagName); fn != "" {
				out, err = os.Create(fn)
				if err != nil {
					return errors.Wrapf(err, "creating output file '%s'", fn)
				}
				defer out.Close()
			}

			_, err = io.Copy(out, r)
			return errors.Wrap(err, "writing task logs out")
		},
	}
}

func testLogs(flags []cli.Flag) cli.Command {
	const (
		logPathFlagName     = "log_path"
		logsToMergeFlagName = "logs_to_merge"
	)

	return cli.Command{
		Name:  "TestLogs",
		Usage: "download test logs",
		Flags: mergeFlagSlices(
			flags,
			[]cli.Flag{
				cli.StringSliceFlag{
					Name:     logPathFlagName,
					Usage:    "Test log path relative to the task's test logs directory.",
					Required: true,
				},
				cli.StringSliceFlag{
					Name:  logsToMergeFlagName,
					Usage: "Test log path, relative to the task's test log directory, to merge with test log specified in the URL path. Can be a prefix. Merging is stable and timestamp-based. Repeat the option flag if more than one value.",
				},
			},
			taskOutputLogFlags(),
		),
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := NewClientSettings(c.Parent().String(confFlagName))
			if err != nil {
				return errors.Wrap(err, "loading configuration")
			}

			restClient, err := conf.setupRestCommunicator(ctx, false)
			if err != nil {
				return errors.Wrap(err, "setting up REST communicator")
			}
			defer restClient.Close()

			var execution *int
			if c.IsSet(executionFlagName) {
				execution = utility.ToIntPtr(c.Int(executionFlagName))
			}

			r, err := restClient.GetTestLogs(ctx, client.GetTestLogsOptions{
				TaskID:    c.String(taskIDFlagName),
				Path:      c.String(logPathFlagName),
				Execution: execution,
				//Execution:     utility.ToIntPtr(c.Int(executionFlagName)),
				LogsToMerge:   c.StringSlice(logsToMergeFlagName),
				Start:         c.String(logStartFlagName),
				End:           c.String(logEndFlagName),
				LineLimit:     c.Int(logLineLimitFlagName),
				TailLimit:     c.Int(logTailLimitFlagName),
				PrintTime:     c.Bool(logPrintTimeFlagName),
				PrintPriority: c.Bool(logPrintPriorityFlagName),
				Paginate:      c.Bool(logPaginateFlagName),
			})
			if err != nil {
				return errors.Wrap(err, "getting test logs")
			}
			defer r.Close()

			out := os.Stdout
			if fn := c.String(logOutputFileFlagName); fn != "" {
				out, err = os.Create(fn)
				if err != nil {
					return errors.Wrapf(err, "creating output file '%s'", fn)
				}
				defer out.Close()
			}

			_, err = io.Copy(out, r)
			return errors.Wrap(err, "writing test logs out")
		},
	}
}

func taskOutputLogFlags() []cli.Flag {
	return []cli.Flag{
		cli.StringFlag{
			Name:  logStartFlagName,
			Usage: "Start of targeted time interval (inclusive) in RFC3339 format. Defaults to the first timestamp of the requested logs.",
		},
		cli.StringFlag{
			Name:  logEndFlagName,
			Usage: "End of targeted time interval (inclusive) in RFC3339 format. Defaults to the last timestamp of the requested logs.",
		},
		cli.IntFlag{
			Name:  logLineLimitFlagName,
			Usage: "If set greater than 0, limits the number of log lines returned.",
		},
		cli.IntFlag{
			Name:  fmt.Sprintf("%s,n", logTailLimitFlagName),
			Usage: "If set greater than 0, returns the last N log lines.",
		},
		cli.BoolFlag{
			Name:  logPrintTimeFlagName,
			Usage: "If set, returns log lines prefixed with their timestamp.",
		},
		cli.BoolFlag{
			Name:  logPrintPriorityFlagName,
			Usage: "If set, returns log lines prefixed with their priority.",
		},
		cli.BoolFlag{
			Name:  logPaginateFlagName,
			Usage: "If set, paginates the download.",
		},
		cli.StringFlag{
			Name:  fmt.Sprintf("%s,o", logOutputFileFlagName),
			Usage: "Output file. Defaults to stdout.",
		},
	}
}
