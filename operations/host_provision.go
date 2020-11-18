package operations

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/evergreen-ci/evergreen/cloud/userdata"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func hostProvision() cli.Command {
	const (
		hostIDFlagName       = "host_id"
		hostSecretFlagName   = "host_secret"
		workingDirFlagName   = "working_dir"
		apiServerURLFlagName = "api_server"
	)
	return cli.Command{
		Name:  "provision",
		Usage: "fetch and run the host provisioning script",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  hostIDFlagName,
				Usage: "the host ID",
			},
			cli.StringFlag{
				Name:  hostSecretFlagName,
				Usage: "the host secret",
			},
			cli.StringFlag{
				Name:  workingDirFlagName,
				Usage: "the working directory for the script",
			},
			cli.StringFlag{
				Name:  apiServerURLFlagName,
				Usage: "the base URL for the API server",
			},
		},
		Before: mergeBeforeFuncs(
			requireStringFlag(hostIDFlagName),
			requireStringFlag(hostSecretFlagName),
			requireStringFlag(apiServerURLFlagName),
		),
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			comm := client.NewCommunicator(c.String(apiServerURLFlagName))
			defer comm.Close()

			opts, err := comm.GetHostProvisioningScript(ctx, c.String(hostIDFlagName), c.String(hostSecretFlagName))
			if err != nil {
				return errors.Wrap(err, "failed to get host provisioning script")
			}

			workingDir := c.String(workingDirFlagName)
			scriptPath, err := makeHostProvisioningScriptFile(workingDir, opts)
			if err != nil {
				return errors.Wrap(err, "write host provisioning script to file")
			}
			defer func() {
				grip.Error(errors.Wrap(os.RemoveAll(scriptPath), "removing host provisioning file"))
			}()

			cmd, err := hostProvisioningCommand(opts.Directive, scriptPath)
			if err != nil {
				return errors.Wrap(err, "resolving command to execute script")
			}
			if err := runHostProvisioningCommand(ctx, cmd.Directory(workingDir)); err != nil {
				return errors.Wrap(err, "running host provisioning script")
			}

			return nil
		},
	}
}

func makeHostProvisioningScriptFile(workingDir string, opts *model.APIHostProvisioningScriptOptions) (string, error) {
	if err := os.MkdirAll(workingDir, 0755); err != nil {
		return "", errors.Wrap(err, "creating working directory")
	}

	scriptPath, err := filepath.Abs(filepath.Join(workingDir, "host_provisioning"))
	if err != nil {
		return "", errors.Wrap(err, "making absolute path to the host provisioning script")
	}
	var content string
	if strings.HasPrefix(opts.Directive, string(userdata.ShellScript)) {
		// Include the shebang line, if present.
		content = opts.Directive + "\n"
	}
	content += opts.Content
	if err = ioutil.WriteFile(scriptPath, []byte(content), 0700); err != nil {
		return "", errors.Wrapf(err, "writing script to file '%s'", scriptPath)
	}
	return scriptPath, nil
}

func hostProvisioningCommand(directive, scriptPath string) (*jasper.Command, error) {
	if directive == string(userdata.PowerShellScript) {
		return jasper.NewCommand().AppendArgs("powershell", scriptPath), nil
	}
	if directive == string(userdata.BatchScript) {
		return jasper.NewCommand().AppendArgs("cmd", "/c", scriptPath), nil
	}
	if strings.HasPrefix(directive, string(userdata.ShellScript)) {
		return jasper.NewCommand().AppendArgs(scriptPath), nil
	}

	return nil, errors.Errorf("unrecognized directive '%s', cannot determine how to execute it", directive)
}

func runHostProvisioningCommand(ctx context.Context, cmd *jasper.Command) error {
	cmd.SetOutputWriter(utility.NopWriteCloser(os.Stdout)).SetErrorWriter(utility.NopWriteCloser(os.Stderr))
	if err := cmd.Run(ctx); err != nil {
		return errors.WithStack(err)
	}
	return nil
}
