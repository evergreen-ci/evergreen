package cli

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/cheynewallace/tabby"
	"github.com/google/uuid"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/remote"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

// Run provides a simple user-centered command-line interface for
// running commands on a remote instance.
func Run() cli.Command { //nolint: gocognit
	const (
		commandFlagName = "command"
		envFlagName     = "env"
		sudoFlagName    = "sudo"
		sudoAsFlagName  = "sudo_as"
		idFlagName      = "id"
		execFlagName    = "exec"
		tagFlagName     = "tag"
		waitFlagName    = "wait"
	)

	defaultID := uuid.New()

	return cli.Command{
		Name:  "run",
		Usage: "Run a command with Jasper service.",
		Flags: append(clientFlags(),
			cli.StringSliceFlag{
				Name:  joinFlagNames(commandFlagName, "c"),
				Usage: "Specify command(s) to run on a remote Jasper service. May specify more than once.",
			},
			cli.StringSliceFlag{
				Name:  envFlagName,
				Usage: "Specify environment variables, in '<key>=<val>' forms. May specify more than once.",
			},
			cli.BoolFlag{
				Name:  sudoFlagName,
				Usage: "Run the command with sudo.",
			},
			cli.StringFlag{
				Name:  sudoAsFlagName,
				Usage: "Run commands as another user, as in 'sudo -u <user>'.",
			},
			cli.StringFlag{
				Name:  idFlagName,
				Usage: "Specify an ID for this command (optional). Note: this ID cannot be used to track the subcommands.",
			},
			cli.BoolFlag{
				Name:  execFlagName,
				Usage: "Execute commands directly using exec (i.e. no shell).",
			},
			cli.StringSliceFlag{
				Name:  tagFlagName,
				Usage: "Specify one or more tag names for the process.",
			},
			cli.BoolFlag{
				Name:  waitFlagName,
				Usage: "Wait until the process returns (subject to service timeouts), propagating the exit code from the process. If set, writes the output of each command to stdout; otherwise, prints the process IDs.",
			},
		),
		Before: mergeBeforeFuncs(clientBefore(),
			func(c *cli.Context) error {
				if len(c.StringSlice(commandFlagName)) == 0 {
					if c.NArg() == 0 {
						return errors.New("must specify at least one command")
					}
					return errors.Wrap(c.Set(commandFlagName, strings.Join(c.Args(), " ")), "problem setting command")
				}
				return nil
			},
			func(c *cli.Context) error {
				if c.String(idFlagName) == "" {
					return errors.Wrap(c.Set(idFlagName, defaultID.String()), "problem setting ID")
				}
				return nil
			}),
		Action: func(c *cli.Context) error {
			envvars := c.StringSlice(envFlagName)
			cmds := c.StringSlice(commandFlagName)
			useSudo := c.Bool(sudoFlagName)
			sudoAs := c.String(sudoAsFlagName)
			useExec := c.Bool(execFlagName)
			cmdID := c.String(idFlagName)
			tags := c.StringSlice(tagFlagName)
			wait := c.Bool(waitFlagName)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			inMemoryCap := 1000
			logger, err := jasper.NewInMemoryLogger(inMemoryCap)
			if err != nil {
				return errors.Wrap(err, "problem creating new in memory logger")
			}

			return withConnection(ctx, c, func(client remote.Manager) error {
				cmd := client.CreateCommand(ctx).Sudo(useSudo).ID(cmdID).SetTags(tags)

				if wait {
					cmd.AppendLoggers(logger).RedirectErrorToOutput(true)
				} else {
					cmd.Background(true)
				}

				for _, cmdStr := range cmds {
					if useExec {
						cmd.Append(cmdStr)
					} else {
						cmd.Bash(cmdStr)
					}
				}

				if sudoAs != "" {
					cmd.SudoAs(sudoAs)
				}

				for _, e := range envvars {
					parts := strings.SplitN(e, "=", 2)
					cmd.AddEnv(parts[0], parts[1])
				}

				if err := cmd.Run(ctx); err != nil {
					if wait {
						// Don't return on error, because if any of the
						// sub-commands fail, it will fail to print the log
						// lines.
						grip.Error(errors.Wrap(err, "problem encountered while running commands"))
					} else {
						return errors.Wrap(err, "problem running command")
					}
				}

				if wait {
					exitCode, err := printLogs(ctx, client, cmd, inMemoryCap)
					grip.Error(err)
					os.Exit(exitCode)
				} else {
					t := tabby.New()
					t.AddHeader("ID")
					t.Print()
					for _, id := range cmd.GetProcIDs() {
						t.AddLine(id)
						t.Print()
					}
				}

				return nil
			})
		},
	}
}

// printLogs prints the logs outputted from a process until the process
// terminates.
func printLogs(ctx context.Context, client remote.Manager, cmd *jasper.Command, inMemoryCap int) (int, error) {
	const logPollInterval = 100 * time.Millisecond
	logDone := make(chan struct{})

	go func() {
		defer recovery.LogStackTraceAndContinue("log printing thread")
		defer close(logDone)
		timer := time.NewTimer(0)
		defer timer.Stop()

		t := tabby.New()
		t.AddHeader("ID", "Logs")
		for _, id := range cmd.GetProcIDs() {
		logSingleProcess:
			for {
				select {
				case <-ctx.Done():
					grip.Notice("log fetching canceled")
					return
				case <-timer.C:
					logLines, err := client.GetLogStream(ctx, id, inMemoryCap)
					if err != nil {
						grip.Error(message.WrapError(err, "problem polling for log lines, aborting log streaming"))
						return
					}

					for _, ln := range logLines.Logs {
						t.AddLine(id, ln)
						t.Print()
					}

					if logLines.Done {
						break logSingleProcess
					}

					timer.Reset(randDur(logPollInterval))
				}
			}
			t.AddLine()
			t.Print()
		}
	}()

	<-logDone
	return cmd.Wait(ctx)
}

// List provides a user interface to inspect processes managed by a
// jasper instance and their state. The output of the command is a
// human-readable table.
func List() cli.Command {
	const (
		filterFlagName = "filter"
		groupFlagName  = "group"
	)
	return cli.Command{
		Name:  "list",
		Usage: "List Jasper managed processes with human readable output.",
		Flags: append(clientFlags(),
			cli.StringFlag{
				Name:  filterFlagName,
				Usage: "Filter processes by status (all, running, successful, failed, terminated).",
			},
			cli.StringFlag{
				Name:  groupFlagName,
				Usage: "Return a list of processes matching the tag.",
			},
		),
		Before: mergeBeforeFuncs(clientBefore(),
			func(c *cli.Context) error {
				if c.String(filterFlagName) != "" && c.String(groupFlagName) != "" {
					return errors.New("cannot set both filter and group")
				}
				if c.String(groupFlagName) != "" {
					return nil
				}
				filter := options.Filter(c.String(filterFlagName))
				if filter == "" {
					filter = options.All
					return errors.Wrap(c.Set(filterFlagName, string(filter)), "problem setting default filter")
				}
				return errors.Wrapf(filter.Validate(), "invalid filter '%s'", filter)
			}),
		Action: func(c *cli.Context) error {
			filter := options.Filter(c.String(filterFlagName))
			group := c.String(groupFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			return withConnection(ctx, c, func(client remote.Manager) error {
				var (
					procs []jasper.Process
					err   error
				)

				if group == "" {
					procs, err = client.List(ctx, filter)
				} else {
					procs, err = client.Group(ctx, group)

				}

				if err != nil {
					return errors.Wrap(err, "problem getting list")
				}

				t := tabby.New()
				t.AddHeader("ID", "PID", "Running", "Complete", "Tags", "Command")
				for _, p := range procs {
					info := p.Info(ctx)
					t.AddLine(p.ID(), info.PID, p.Running(ctx), p.Complete(ctx), p.GetTags(), strings.Join(info.Options.Args, " "))
				}
				t.Print()
				return nil
			})
		},
	}
}

// Kill terminates a single process by id, sending either TERM or KILL.
func Kill() cli.Command {
	const (
		idFlagName   = "id"
		killFlagName = "kill"
	)
	return cli.Command{
		Name:  "kill",
		Usage: "Terminate a process.",
		Flags: append(clientFlags(),
			cli.StringFlag{
				Name:  joinFlagNames(idFlagName, "i"),
				Usage: "Specify the ID of the process to kill.",
			},
			cli.BoolFlag{
				Name:  killFlagName,
				Usage: "Send SIGKILL (9) rather than SIGTERM (15).",
			},
		),
		Before: mergeBeforeFuncs(
			clientBefore(),
			func(c *cli.Context) error {
				if len(c.String(idFlagName)) == 0 {
					if c.NArg() != 1 {
						return errors.New("must specify a process ID")
					}
					return errors.Wrap(c.Set(idFlagName, c.Args().First()), "problem setting id from positional flags")
				}
				return nil
			}),
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sendKill := c.Bool(killFlagName)
			procID := c.String(idFlagName)
			return withConnection(ctx, c, func(client remote.Manager) error {
				proc, err := client.Get(ctx, procID)
				if err != nil {
					return errors.WithStack(err)
				}

				if sendKill {
					return errors.WithStack(jasper.Kill(ctx, proc))
				}
				return errors.WithStack(jasper.Terminate(ctx, proc))
			})
		},
	}
}

// Clear removes all terminated/exited processes from Jasper Manager.
func Clear() cli.Command {
	return cli.Command{
		Name:   "clear",
		Usage:  "Clean up terminated processes.",
		Flags:  clientFlags(),
		Before: clientBefore(),
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			return withConnection(ctx, c, func(client remote.Manager) error {
				client.Clear(ctx)
				return nil
			})
		},
	}
}

// KillAll terminates all processes with a given tag, sending either TERM or
// KILL.
func KillAll() cli.Command {
	const (
		groupFlagName = "group"
		killFlagName  = "kill"
	)

	return cli.Command{
		Name:  "kill-all",
		Usage: "Terminate a group of tagged processes.",
		Flags: append(clientFlags(),
			cli.StringFlag{
				Name:  groupFlagName,
				Usage: "Specify the group of process to kill.",
			},
			cli.BoolFlag{
				Name:  killFlagName,
				Usage: "Send SIGKILL (9) rather than SIGTERM (15).",
			},
		),
		Before: mergeBeforeFuncs(
			clientBefore(),
			func(c *cli.Context) error {
				if c.String(groupFlagName) == "" {
					return errors.Errorf("flag '--%s' was not specified", groupFlagName)
				}
				return nil
			}),
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sendKill := c.Bool(killFlagName)
			group := c.String(groupFlagName)
			return withConnection(ctx, c, func(client remote.Manager) error {
				procs, err := client.Group(ctx, group)
				if err != nil {
					return errors.WithStack(err)
				}

				if sendKill {
					return errors.WithStack(jasper.KillAll(ctx, procs))
				}
				return errors.WithStack(jasper.TerminateAll(ctx, procs))
			})
		},
	}
}

// Download exposes a simple interface for using jasper to download
// files on the remote jasper.Manager.
func Download() cli.Command {
	const (
		urlFlagName         = "url"
		pathFlagName        = "path"
		extractPathFlagName = "extract_to"
	)

	return cli.Command{
		Name:  "download",
		Usage: "Download a file on the host running the remote manager.",
		Flags: append(clientFlags(),
			cli.StringFlag{
				Name:  joinFlagNames(urlFlagName, "p"),
				Usage: "Specify the URL of the file to download on the remote.",
			},
			cli.StringFlag{
				Name:  extractPathFlagName,
				Usage: "If specified, attempt to extract the downloaded artifact to the given path.",
			},
			cli.StringFlag{
				Name:  pathFlagName,
				Usage: "Specify the remote path to download the file to on the managed system.",
			}),
		Before: mergeBeforeFuncs(
			clientBefore(),
			requireStringFlag(pathFlagName),
			func(c *cli.Context) error {
				if c.String(urlFlagName) == "" {
					if c.NArg() != 1 {
						return errors.New("must specify a URL")
					}
					return errors.Wrap(c.Set(urlFlagName, c.Args().First()), "problem setting URL from positional flags")
				}
				return nil
			}),
		Action: func(c *cli.Context) error {
			opts := options.Download{
				URL:  c.String(urlFlagName),
				Path: c.String(pathFlagName),
			}

			if path := c.String(extractPathFlagName); path != "" {
				opts.ArchiveOpts = options.Archive{
					ShouldExtract: true,
					Format:        options.ArchiveAuto,
					TargetPath:    path,
				}
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			return withConnection(ctx, c, func(client remote.Manager) error {
				return errors.WithStack(client.DownloadFile(ctx, opts))
			})
		},
	}

}
