package operations

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/jasper"
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
		setup := host.ShCommandWithSudo(evergreen.TempSetupScriptName, setupAsSudo)
		return runScript(ctx, evergreen.SetupScriptName, evergreen.TempSetupScriptName, setup, setupAsSudo)
	} else if _, err = os.Stat(evergreen.PowerShellSetupScriptName); err == nil {
		setup := []string{"powershell", "./" + evergreen.PowerShellTempSetupScriptName}
		return runScript(ctx, evergreen.PowerShellSetupScriptName, evergreen.PowerShellTempSetupScriptName, setup, setupAsSudo)
	}
	return nil
}

// runScript ensures a shell script has proper permissions and runs it. The
// script is deleted.
func runScript(ctx context.Context, scriptFileName, tempFileName string, runScriptArgs []string, sudo bool) error {
	if err := os.Rename(scriptFileName, tempFileName); os.IsNotExist(err) {
		return nil
	}

	chmod := host.ChmodCommandWithSudo(tempFileName, sudo)
	if output, err := runCmd(ctx, chmod); err != nil {
		return errors.Wrap(err, output)
	}

	catcher := grip.NewSimpleCatcher()

	output, err := runCmd(ctx, runScriptArgs)
	catcher.Add(err)
	if err == nil {
		fmt.Println(output)
	}
	catcher.Add(os.Remove(tempFileName))

	return errors.Wrap(catcher.Resolve(), output)
}

// runCmd runs the given command and returns the output.
func runCmd(ctx context.Context, args []string) (string, error) {
	output := util.NewMBCappedWriter()
	cmd := jasper.NewCommand().Add(args).SetCombinedWriter(output)
	err := cmd.Run(ctx)
	return output.String(), errors.Wrapf(err, "running command: %s", output.String())
}
