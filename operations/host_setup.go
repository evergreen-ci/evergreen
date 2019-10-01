package operations

import (
	"context"
	"os"
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

	_, err := os.Stat(evergreen.SetupScriptName)
	if err == nil {
		return runScript(ctx, wd, evergreen.SetupScriptName, evergreen.TempSetupScriptName, setupAsSudo)
	} else if _, err = os.Stat(evergreen.PowerShellSetupScriptName); err == nil {
		return runScript(ctx, wd, evergreen.PowerShellSetupScriptName, evergreen.TempPowerShellSetupScriptName, setupAsSudo)
	}
	return nil
}

func runScript(ctx context.Context, wd, scriptFileName, tempFileName string, sudo bool) error {
	if err := os.Rename(scriptFileName, tempFileName); os.IsNotExist(err) {
		return nil
	}
	chmod := host.ChmodCommandWithSudo(ctx, tempFileName, sudo)
	out, err := chmod.CombinedOutput()
	if err != nil {
		return errors.Wrap(err, string(out))
	}

	catcher := grip.NewSimpleCatcher()

	var cmd *exec.Cmd
	if sudo {
		cmd = exec.CommandContext(ctx, "sudo", "./"+tempFileName)
	} else {
		cmd = exec.CommandContext(ctx, "./"+tempFileName)
	}
	out, err = cmd.CombinedOutput()

	catcher.Add(err)
	// kim: TODO: uncomment this line when the script works 100%.
	// catcher.Add(os.Remove(tempFile))

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
