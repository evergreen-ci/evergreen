package operations

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/grip"
	"github.com/urfave/cli"
)

const (
	setupTimeout = 2 * time.Minute
)

func hostSetup() cli.Command {
	return cli.Command{
		Name:  "setup",
		Usage: "run setup script on a build host",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "working_directory",
				Usage: "working directory for the script",
			},
			cli.BoolFlag{
				Name:  "setup_as_sudo",
				Usage: "run the setup script with sudo",
			},
		},
		Action: func(c *cli.Context) error {
			setupAsSudo := c.Bool("setup_as_sudo")
			wd := c.String("working_directory")

			grip.Warning(os.MkdirAll(wd, 0777))
			if _, err := os.Stat(evergreen.SetupScriptName); os.IsNotExist(err) {
				return nil
			}

			ctx, cancel := context.WithTimeout(ctx, setupTimeout)
			defer cancel()

			chmod := getChmodCommandWithSudo(ctx, evergreen.SetupScriptName, setupAsSudo)
			out, err := chmod.CombinedOutput()
			if err != nil {
				return errors.Wrap(err, string(out))
			}

			cmd := getShCommandWithSudo(ctx, evergreen.SetupScriptName, setupAsSudo)
			out, err = cmd.CombinedOutput()

			catcher := grip.NewSimpleCatcher()
			catcher.Add(err)
			catcher.Add(os.Remove(evergreen.SetupScriptName))

			grip.Warning(os.MkdirAll(wd, 0777))

			return errors.Wrap(catcher.Resolve(), string(out))
		},
	}

}

func hostTeardown() cli.Command {
	return cli.Command{
		Name:  "teardown",
		Usage: "run a teardown script on a build host",
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithTimeout(context.Background(), setupTimeout)
			defer cancel()

			chmod := getChmodCommandWithSudo(ctx, evergreen.TeardownScriptName, false)
			out, err := chmod.CombinedOutput()
			if err != nil {
				return errors.Wrap(err, string(out))
			}

			cmd := getShCommandWithSudo(ctx, evergreen.TeardownScriptName, false)
			out, err = cmd.CombinedOutput()
			if err != nil {
				return errors.Wrap(err, string(out))
			}

			return nil
		},
	}

}

func getShCommandWithSudo(ctx context.Context, script string, sudo bool) *exec.Cmd {
	if sudo {
		return exec.CommandContext(ctx, "sudo", "sh", script)
	}
	return exec.CommandContext(ctx, "sh", script)
}

func getChmodCommandWithSudo(ctx context.Context, script string, sudo bool) *exec.Cmd {
	args := []string{}
	if sudo {
		args = append(args, "sudo")
	}
	args = append(args, "chmod", "+x", script)
	return exec.CommandContext(ctx, args[0], args[1:]...)
}
