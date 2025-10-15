package operations

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/evergreen-ci/evergreen"
	agentutil "github.com/evergreen-ci/evergreen/agent/util"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/rest/client"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func hostProvision() cli.Command {
	const (
		hostIDFlagName        = "host_id"
		hostSecretFlagName    = "host_secret"
		cloudProviderFlagName = "provider"
		workingDirFlagName    = "working_dir"
		apiServerURLFlagName  = "api_server"
		shellPathFlagName     = "shell_path"
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
				Name:  cloudProviderFlagName,
				Usage: "the cloud provider that manages this host",
			},
			cli.StringFlag{
				Name:  workingDirFlagName,
				Usage: "the working directory for the script",
			},
			cli.StringFlag{
				Name:  apiServerURLFlagName,
				Usage: "the base URL for the API server",
			},
			cli.StringFlag{
				Name:  shellPathFlagName,
				Usage: "the path to the shell to use",
			},
		},
		Before: mergeBeforeFuncs(
			requireStringFlag(hostIDFlagName),
			requireStringFlag(hostSecretFlagName),
			requireStringFlag(apiServerURLFlagName),
			requireStringFlag(shellPathFlagName),
		),
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			comm, err := client.NewCommunicator(c.String(apiServerURLFlagName))
			if err != nil {
				return errors.Wrap(err, "initializing client")
			}
			defer comm.Close()

			hostID := c.String(hostIDFlagName)
			hostSecret := c.String(hostSecretFlagName)
			comm.SetHostID(hostID)
			comm.SetHostSecret(hostSecret)

			cloudProvider := c.String(cloudProviderFlagName)
			h, err := postHostIsUp(ctx, comm, hostID, cloudProvider)
			if err != nil {
				return errors.Wrap(err, "posting that the host is up")
			}
			if h != nil {
				// If the host was an intent host when it was first started up,
				// the host ID can change. Use the most up-to-date host ID when
				// starting the agent.
				if updatedHostID := utility.FromStringPtr(h.Id); updatedHostID != "" {
					hostID = updatedHostID
					comm.SetHostID(hostID)
				}
			}

			opts, err := comm.GetHostProvisioningOptions(ctx)
			if err != nil {
				return errors.Wrap(err, "getting host provisioning script")
			}

			workingDir := c.String(workingDirFlagName)
			scriptPath, err := makeHostProvisioningScriptFile(workingDir, opts.Content)
			if err != nil {
				return errors.Wrap(err, "writing host provisioning script file")
			}
			defer func() {
				grip.Error(errors.Wrap(os.RemoveAll(scriptPath), "removing host provisioning file"))
			}()

			if err := runHostProvisioningScript(ctx, c.String(shellPathFlagName), scriptPath, workingDir); err != nil {
				return errors.Wrap(err, "running host provisioning script")
			}

			return nil
		},
	}
}

func postHostIsUp(ctx context.Context, comm client.Communicator, hostID, cloudProvider string) (*restmodel.APIHost, error) {
	ec2Metadata := host.HostMetadataOptions{}
	if cloud.IsEC2InstanceID(hostID) {
		ec2Metadata.Hostname = hostID
	} else if evergreen.IsEc2Provider(cloudProvider) {
		fetchEC2InfoCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		var err error

		ec2Metadata, err = agentutil.GetEC2Metadata(fetchEC2InfoCtx)
		grip.Error(message.WrapError(err, message.Fields{
			"message":        "could not fetch EC2 metadata dynamically",
			"host_id":        hostID,
			"cloud_provider": cloudProvider,
		}))
	}
	h, err := comm.PostHostIsUp(ctx, ec2Metadata)
	if err != nil {
		return nil, errors.Wrap(err, "posting that the host is up")
	}
	return h, nil
}

// makeHostProvisioningScriptFile creates the working directory with the host
// provisioning script in it. Returns the absolute path to the script.
// Note: we have to write the host provisioning script to a file instead of
// running it directly (like with 'sh -c "<script>"') because the script will
// exit before it finishes executing on Windows.
func makeHostProvisioningScriptFile(workingDir string, content string) (string, error) {
	if err := os.MkdirAll(workingDir, 0755); err != nil {
		return "", errors.Wrapf(err, "creating working directory '%s'", workingDir)
	}

	scriptPath, err := filepath.Abs(filepath.Join(workingDir, "host_provisioning"))
	if err != nil {
		return "", errors.Wrap(err, "making absolute path to the host provisioning script")
	}
	// Cygwin shell requires back slashes ('\') to be escaped, so use forward
	// slashes ('/') instead as the path separator.
	scriptPath = util.ConsistentFilepath(scriptPath)
	if err = os.WriteFile(scriptPath, []byte(content), 0700); err != nil {
		return "", errors.Wrapf(err, "writing script file '%s'", scriptPath)
	}
	return scriptPath, nil
}

func runHostProvisioningScript(ctx context.Context, shellPath, scriptPath, workingDir string) error {
	cmd := jasper.NewCommand().AppendArgs(shellPath, "-l", scriptPath).Directory(workingDir)
	if runtime.GOOS != "windows" {
		// For non-Windows distros, it is beneficial to have output from the
		// script. However, on Windows, we have to suppress it because it can
		// cause the PowerShell environment that it's executing in to hang if it
		// produces too much output.
		cmd.SetOutputWriter(utility.NopWriteCloser(os.Stdout)).SetErrorWriter(utility.NopWriteCloser(os.Stderr))
	}
	if err := cmd.Run(ctx); err != nil {
		return errors.WithStack(err)
	}
	return nil
}
