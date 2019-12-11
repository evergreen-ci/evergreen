package cli

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/cheynewallace/tabby"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/remote"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
	"github.com/urfave/cli"
)

// RunCMD provides a simple user-centered command-line interface for
// running commands on a remote instance.
func RunCMD() cli.Command {
	const (
		commandFlagName = "command"
		envFlagName     = "env"
		sudoFlagName    = "sudo"
		sudoAsFlagName  = "sudo_as"
		idFlagName      = "id"
		execFlagName    = "exec"
		tagFlagName     = "tag"
		waitFlagName    = "wait"

		logPollInterval = 100 * time.Millisecond
	)

	defaultID := uuid.Must(uuid.NewV4())

	return cli.Command{
		Name:  "run",
		Usage: "run a command with Jasper service",
		Flags: append(clientFlags(),
			cli.StringSliceFlag{
				Name:  joinFlagNames(commandFlagName, "c"),
				Usage: "specify command(s) to run on a remote Jasper service. may specify more than once",
			},
			cli.StringSliceFlag{
				Name:  envFlagName,
				Usage: "specify environment variables, in '<key>=<val>' forms. may specify more than once",
			},
			cli.BoolFlag{
				Name:  sudoFlagName,
				Usage: "run the command with sudo",
			},
			cli.StringFlag{
				Name:  sudoAsFlagName,
				Usage: "run commands as another user as in 'sudo -u <user>'",
			},
			cli.StringFlag{
				Name:  idFlagName,
				Usage: "specify an ID for this command (optional). note: this ID cannot be used to track the subcommands.",
			},
			cli.BoolFlag{
				Name:  execFlagName,
				Usage: "execute commands directly without shell",
			},
			cli.StringSliceFlag{
				Name:  tagFlagName,
				Usage: "specify one or more tag names for the process",
			},
			cli.BoolFlag{
				Name:  waitFlagName,
				Usage: "block until the process returns (subject to service timeouts), propagating the exit code from the process. if set, writes the output of each command to stdout; otherwise, prints the process IDs.",
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

			logger := jasper.NewInMemoryLogger(1000)

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

				ids := cmd.GetProcIDs()

				if wait {
					logDone := make(chan struct{})
					go func() {
						defer recovery.LogStackTraceAndContinue("log handling thread")
						defer close(logDone)
						timer := time.NewTimer(0)
						defer timer.Stop()

						t := tabby.New()
						t.AddHeader("ID", "Logs")
						for _, id := range ids {
						logSingleProcess:
							for {
								select {
								case <-ctx.Done():
									grip.Notice("operation canceled")
									return
								case <-timer.C:
									logLines, err := client.GetLogStream(ctx, id, logger.Options.InMemoryCap)
									if err != nil {
										grip.Error(message.WrapError(err, "problem polling for log lines, aborting log streaming"))
										break logSingleProcess
									}

									for _, ln := range logLines.Logs {
										t.AddLine(id, ln)
										t.Print()
									}

									if logLines.Done {
										timer.Reset(0)
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
					exitCode, err := cmd.Wait(ctx)
					grip.Error(err)
					os.Exit(exitCode)
				} else {
					t := tabby.New()
					t.AddHeader("ID")
					t.Print()
					for _, id := range ids {
						t.AddLine(id)
						t.Print()
					}
				}

				return nil
			})
		},
	}
}

// ListCMD provides a user interface to inspect processes managed by a
// jasper instance and their state. The output of the command is a
// human-readable table.
func ListCMD() cli.Command {
	const (
		filterFlagName = "filter"
		groupFlagName  = "group"
	)
	return cli.Command{
		Name:  "list",
		Usage: "list Jasper managed processes with human readable output",
		Flags: append(clientFlags(),
			cli.StringFlag{
				Name:  filterFlagName,
				Usage: "filter processes by status (all, running, successful, failed, terminated)",
			},
			cli.StringFlag{
				Name:  groupFlagName,
				Usage: "return a list of processes with a tag",
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

// KillCMD terminates a single process by id, sending either TERM or KILL.
func KillCMD() cli.Command {
	const (
		idFlagName   = "id"
		killFlagName = "kill"
	)
	return cli.Command{
		Name:  "kill",
		Usage: "terminate a process",
		Flags: append(clientFlags(),
			cli.StringFlag{
				Name:  joinFlagNames(idFlagName, "i"),
				Usage: "specify the id of the process to kill",
			},
			cli.BoolFlag{
				Name:  killFlagName,
				Usage: "send KILL (9) rather than TERM (15)",
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

// ClearCMD removes all terminated/exited processes from Jasper Manager.
func ClearCMD() cli.Command {
	return cli.Command{
		Name:   "clear",
		Usage:  "clean up terminated processes",
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

// KillAllCMD terminates all processes with a given tag, sending either TERM or
// KILL.
func KillAllCMD() cli.Command {
	const (
		groupFlagName = "group"
		killFlagName  = "kill"
	)

	return cli.Command{
		Name:  "kill-all",
		Usage: "terminate a group of tagged processes",
		Flags: append(clientFlags(),
			cli.StringFlag{
				Name:  groupFlagName,
				Usage: "specify the group of process to kill",
			},
			cli.BoolFlag{
				Name:  killFlagName,
				Usage: "send KILL (9) rather than TERM (15)",
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

// DownloadCMD exposes a simple interface for using jasper to download
// files on the remote jasper.Manager.
func DownloadCMD() cli.Command {
	const (
		urlFlagName         = "url"
		pathFlagName        = "path"
		extractPathFlagName = "extract_to"
	)

	return cli.Command{
		Name: "download",
		Flags: append(clientFlags(),
			cli.StringFlag{
				Name:  joinFlagNames(urlFlagName, "p"),
				Usage: "specify the url of the file to download on the remote.",
			},
			cli.StringFlag{
				Name:  extractPathFlagName,
				Usage: "if specified, attempt to extract the downloaded artifact to the given path.",
			},
			cli.StringFlag{
				Name:  pathFlagName,
				Usage: "specify the remote path to download the file to on the managed system. Required.",
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
