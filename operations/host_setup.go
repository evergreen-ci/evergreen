package operations

import (
	"context"
	"os"
	"os/exec"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
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
			wd := c.String("working_directory")
			setupAsSudo := c.Bool("setup_as_sudo")
			ctx, cancel := context.WithTimeout(context.Background(), setupTimeout)
			defer cancel()

			return errors.WithStack(runSetupScript(ctx, wd, setupAsSudo))
		},
	}
}

func runSetupScript(ctx context.Context, wd string, setupAsSudo bool) error {
	grip.Warning(os.MkdirAll(wd, 0777))

	if _, err := os.Stat(evergreen.SetupScriptName); err == nil {
		cmd := host.ShCommandWithSudo(ctx, evergreen.TempSetupScriptName, setupAsSudo)
		return runScript(ctx, wd, evergreen.SetupScriptName, evergreen.TempSetupScriptName, cmd, setupAsSudo)
	} else if _, err = os.Stat(evergreen.PowerShellSetupScriptName); err == nil {
		cmd := exec.CommandContext(ctx, "powershell", "./"+evergreen.PowerShellTempSetupScriptName)
		return runScript(ctx, wd, evergreen.PowerShellSetupScriptName, evergreen.PowerShellTempSetupScriptName, cmd, setupAsSudo)
	}
	return nil
}

func runScript(ctx context.Context, wd, scriptFileName, tempFileName string, runScript *exec.Cmd, sudo bool) error {
	if err := os.Rename(scriptFileName, tempFileName); os.IsNotExist(err) {
		return nil
	}

	chmod := host.ChmodCommandWithSudo(ctx, tempFileName, sudo)
	out, err := chmod.CombinedOutput()
	if err != nil {
		return errors.Wrap(err, string(out))
	}

	catcher := grip.NewSimpleCatcher()

	out, err = runScript.CombinedOutput()
	catcher.Add(err)
	catcher.Add(os.Remove(tempFileName))

	grip.Warning(os.MkdirAll(wd, 0777))

	return errors.Wrap(catcher.Resolve(), string(out))
}

func hostTeardown() cli.Command {
	return cli.Command{
		Name:  "teardown",
		Usage: "run a teardown script on a build host",
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithTimeout(context.Background(), setupTimeout)
			defer cancel()

			return errors.WithStack(runHostTeardownScript(ctx))
		},
	}
}

func runHostTeardownScript(ctx context.Context) error {
	if _, err := os.Stat(evergreen.TeardownScriptName); os.IsNotExist(err) {
		return errors.Errorf("no teardown script '%s' found", evergreen.TeardownScriptName)
	}

	chmod := host.ChmodCommandWithSudo(ctx, evergreen.TeardownScriptName, false)
	out, err := chmod.CombinedOutput()
	if err != nil {
		return errors.Wrap(err, string(out))
	}

	cmd := host.ShCommandWithSudo(ctx, evergreen.TeardownScriptName, false)
	out, err = cmd.CombinedOutput()
	if err != nil {
		return errors.Wrap(err, string(out))
	}

	return nil
}
