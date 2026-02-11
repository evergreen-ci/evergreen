package evergreenlocal

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/operations"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

var (
	validationOnce   sync.Once
	validationResult error
)

// BuildDebugger creates and configures the evergreen-local CLI application.
// This function is exported so it can be called from the main evergreen CLI
// to integrate debugger commands into a single binary.
func BuildDebugger() *cli.App {
	app := cli.NewApp()
	app.Name = "evergreen-local"
	app.Usage = "Run Evergreen tasks locally from YAML configuration files"
	app.Version = fmt.Sprintf("%s (%s)", evergreen.AgentVersion, evergreen.BuildRevision)

	SetupLogging()

	app.Commands = operations.DaemonCommands()

	// Add validation as a Before hook only to the daemon start command
	for i := range app.Commands {
		if app.Commands[i].Name == "daemon" {
			for j := range app.Commands[i].Subcommands {
				if app.Commands[i].Subcommands[j].Name == "start" {
					originalBefore := app.Commands[i].Subcommands[j].Before
					app.Commands[i].Subcommands[j].Before = func(c *cli.Context) error {
						// Run validation first
						if err := CheckDebugSpawnHostEnabled(c); err != nil {
							return err
						}
						// Then run original Before hook if it exists
						if originalBefore != nil {
							return originalBefore(c)
						}
						return nil
					}
					break
				}
			}
			break
		}
	}

	return app
}

// SetupLogging configures the logging settings for the evergreen-local CLI.
func SetupLogging() {
	sender := grip.GetSender()
	sender.SetLevel(send.LevelInfo{
		Default:   level.Info,
		Threshold: level.Debug,
	})
	grip.SetSender(sender)
}

// CheckDebugSpawnHostEnabled validates that the current environment is authorized
// to run debug spawn host commands. It checks service flags and that the host
// was spawned by a task.
func CheckDebugSpawnHostEnabled(c *cli.Context) error {
	// Use a longer timeout to allow for OAuth authentication flow
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	confPath := c.String(operations.ConfFlagName)
	conf, err := operations.NewClientSettings(confPath)
	if err != nil {
		return errors.Wrapf(err, "finding configuration at '%s'", confPath)
	}

	if conf.SpawnHostID == "" {
		return errors.New("Could not find spawn host ID in configuration. This command must be run from a spawn host.")
	}

	// Use user authentication to access the spawn host info via REST API
	restClient, err := conf.SetupRestCommunicator(ctx, false)
	if err != nil {
		return errors.Wrap(err, "setting up REST communicator")
	}
	defer restClient.Close()

	flags, err := restClient.GetServiceFlags(ctx)
	if err != nil {
		return errors.Wrap(err, "getting service flags for debug spawn host")
	}

	if flags.DebugSpawnHostDisabled {
		return errors.New("Debug spawn hosts currently disabled.")
	}

	currentHost, err := restClient.GetSpawnHost(ctx, conf.SpawnHostID)
	if err != nil {
		return errors.Wrapf(err, "getting current host '%s' for debug spawn host", conf.SpawnHostID)
	}

	taskID := utility.FromStringPtr(currentHost.ProvisionOptions.TaskID)
	if taskID == "" {
		return errors.New("Only hosts spawned by tasks are allowed to use debugger")
	}

	// Verify we have project information
	if conf.ProjectID == "" {
		return errors.New("Project ID not found in configuration. Debug spawn host validation requires project information.")
	}

	// Fetch the project settings via REST API to check if debug spawn hosts are enabled
	project, err := restClient.GetProject(ctx, conf.ProjectID)
	if err != nil {
		return errors.Wrapf(err, "getting project '%s' settings", conf.ProjectID)
	}
	if project == nil {
		return errors.Errorf("project '%s' not found", conf.ProjectID)
	}

	// Check if debug spawn hosts are enabled for this project
	debugSpawnHostsDisabled := utility.FromBoolPtr(project.DebugSpawnHostsDisabled)
	if debugSpawnHostsDisabled {
		return errors.Errorf("debug spawn hosts are disabled for project '%s'", conf.ProjectID)
	}

	return nil
}
