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

	if _, err := os.Stat(evergreen.SetupScriptName); os.IsNotExist(err) {
		return nil
	}
	if err := os.Rename(evergreen.SetupScriptName, evergreen.TempSetupScriptName); os.IsNotExist(err) {
		return nil
	}
}

func runShellScript(ctx context.Context, wd, scriptFile, tempFile string, sudo bool) error {
	if err := os.Rename(scriptFile, tempFile); os.IsNotExist(err) {
		return nil
	}
	chmod := host.ChmodCommandWithSudo(ctx, tempFile, sudo)
	out, err := chmod.CombinedOutput()
	if err != nil {
		return errors.Wrap(err, string(out))
	}

	catcher := grip.NewSimpleCatcher()

	cmd := host.ShCommandWithSudo(ctx, scriptFile, sudo)
	out, err = cmd.CombinedOutput()

	catcher.Add(err)
	// catcher.Add(os.Remove(tempFile))

	grip.Warning(os.MkdirAll(wd, 0777))

	return errors.Wrap(catcher.Resolve(), string(out))
}

func runPowerShellScript(ctx context.Context, wd, scriptFile, tempFile string) error {
	if err := os.Rename(scriptFile, tempFile); os.IsNotExist(err) {
		return nil
	}
	chmod := host.ChmodCommandWithSudo(ctx, tempFile, false)
	out, err := chmod.CombinedOutput()
	if err != nil {
		return errors.Wrap(err, string(out))
	}

	catcher := grip.NewSimpleCatcher()

	cmd := exec.CommandContext(ctx, "powershell", "./"+scriptFile)
	out, err = cmd.CombinedOutput()

	catcher.Add(err)
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
