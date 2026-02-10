package evergreenlocal

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/operations"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
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

	app.Before = func(c *cli.Context) error {
		return CheckDebugSpawnHostEnabled(c)
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
// to run debug spawn host commands. It checks service flags, host credentials,
// and project-level settings.
func CheckDebugSpawnHostEnabled(c *cli.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	hostID := os.Getenv(evergreen.HostIDEnvVar)
	hostSecret := os.Getenv(evergreen.HostSecretEnvVar)
	if hostID == "" || hostSecret == "" {
		return errors.New("Could not get Evergreen host credentials")
	}

	confPath := c.String(operations.ConfFlagName)
	conf, err := operations.NewClientSettings(confPath)
	if err != nil {
		return errors.Wrapf(err, "finding configuration at '%s'", confPath)
	}

	restClient, err := client.NewCommunicator(conf.APIServerHost)
	if err != nil {
		return errors.Wrap(err, "initializing communicator for debug spawn host")
	}
	defer restClient.Close()

	restClient.SetHostID(hostID)
	restClient.SetHostSecret(hostSecret)

	flags, err := restClient.GetServiceFlags(ctx)
	if err != nil {
		return errors.Wrap(err, "getting service flags for debug spawn host")
	}

	if flags.DebugSpawnHostDisabled {
		return errors.New("Debug spawn hosts not allowed")
	}

	currentHost, err := restClient.GetSpawnHost(ctx, hostID)
	if err != nil {
		return errors.Wrapf(err, "getting current host '%s' for debug spawn host", hostID)
	}

	taskID := utility.FromStringPtr(currentHost.ProvisionOptions.TaskID)
	if taskID == "" {
		return errors.New("Only hosts spawned by tasks are allowed to use debugger")
	}

	provisionedTask, err := task.FindByIdExecution(ctx, taskID, nil)
	if err != nil {
		return errors.Wrapf(err, "getting task '%s' for debug spawn host", taskID)
	}
	if provisionedTask == nil {
		return errors.Errorf("task '%s' not found for debug spawn host", taskID)
	}

	projectSettings, err := model.GetProjectSettingsById(ctx, provisionedTask.Project, false)
	if err != nil {
		return errors.Wrapf(err, "getting project settings for project '%s'", provisionedTask.Project)
	}
	if projectSettings == nil {
		return errors.Errorf("project settings for project '%s' not found", provisionedTask.Project)
	}

	if !projectSettings.ProjectRef.IsDebugSpawnHostsEnabled() {
		return errors.Errorf("debug spawn hosts are not enabled for project '%s'", provisionedTask.Project)
	}

	return nil
}
