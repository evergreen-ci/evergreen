package operations

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/globals"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/remote"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func setupSmokeTest(err error) cli.BeforeFunc {
	return func(c *cli.Context) error {
		if err != nil {
			return errors.Wrap(err, "getting working directory")
		}
		grip.GetSender().SetName("evergreen.smoke")
		return nil
	}
}

func startLocalEvergreen() cli.Command {
	return cli.Command{
		Name:  "start-local-evergreen",
		Usage: "start an Evergreen for local development",
		Action: func(c *cli.Context) error {
			exit := make(chan error, 1)
			wd, err := os.Getwd()
			if err != nil {
				return errors.Wrap(err, "getting working directory")
			}
			binary := filepath.Join(wd, "clients", runtime.GOOS+"_"+runtime.GOARCH, "evergreen")
			if err := smokeRunBinary(exit, "web.service", wd, nil, binary, "service", "web", "--db", "evergreen_local", "--testing-env"); err != nil {
				return errors.Wrap(err, "running web service")
			}
			<-exit
			return nil
		},
	}
}

func smokeStartEvergreen() cli.Command {
	const (
		binaryFlagName         = "binary"
		agentFlagName          = "agent"
		webFlagName            = "web"
		agentMonitorFlagName   = "monitor"
		distroIDFlagName       = "distro"
		apiServerURLFlagName   = "api_server"
		modeFlagName           = "mode"
		execModeIDFlagName     = "exec_mode_id"
		execModeSecretFlagName = "exec_mode_secret"
		statusPort             = "2287"
		monitorPort            = 2288
		jasperPort             = 2289
	)

	wd, err := os.Getwd()

	binary := filepath.Join(wd, "clients", runtime.GOOS+"_"+runtime.GOARCH, "evergreen")
	confPath := filepath.Join(wd, "smoke", "internal", "testdata", "admin_settings.yml")

	return cli.Command{
		Name:    "start-evergreen",
		Aliases: []string{},
		Usage:   "run Evergreen binary for smoke tests",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  ConfFlagName,
				Usage: "path to the (test) service configuration file",
				Value: confPath,
			},
			cli.StringFlag{
				Name:  binaryFlagName,
				Usage: "path to Evergreen binary",
				Value: binary,
			},
			cli.BoolFlag{
				Name:  webFlagName,
				Usage: "run the Evergreen web service",
			},
			cli.StringFlag{
				Name:  apiServerURLFlagName,
				Usage: "the URL of the app server",
			},
			cli.BoolFlag{
				Name:  agentFlagName,
				Usage: "start an Evergreen agent",
			},
			cli.BoolFlag{
				Name:  agentMonitorFlagName,
				Usage: "start an Evergreen agent monitor",
			},
			cli.StringFlag{
				Name:  distroIDFlagName,
				Usage: "the distro ID of the agent monitor",
			},
			cli.StringFlag{
				Name:  modeFlagName,
				Usage: "run the agent in host mode",
			},
			cli.StringFlag{
				Name:  execModeIDFlagName,
				Usage: "the ID of the host running the agent",
			},
			cli.StringFlag{
				Name:  execModeSecretFlagName,
				Usage: "the secret of the host running the agent",
			},
		},
		Before: mergeBeforeFuncs(setupSmokeTest(err), requireFileExists(ConfFlagName), requireAtLeastOneBool(webFlagName, agentFlagName, agentMonitorFlagName)),
		Action: func(c *cli.Context) error {
			confPath := c.String(ConfFlagName)
			binary := c.String(binaryFlagName)
			startWeb := c.Bool(webFlagName)
			startAgent := c.Bool(agentFlagName)
			startAgentMonitor := c.Bool(agentMonitorFlagName)
			execModeID := c.String(execModeIDFlagName)
			execModeSecret := c.String(execModeSecretFlagName)
			distroID := c.String(distroIDFlagName)
			mode := c.String(modeFlagName)
			apiServerURL := c.String(apiServerURLFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			exit := make(chan error, 3)

			if startWeb {
				if err := smokeRunBinary(exit, "web.service", wd, nil, binary, "service", "web", "--testing-env", "--conf", confPath); err != nil {
					return errors.Wrap(err, "running web service")
				}
			}

			if startAgent {

				var envVars []string
				switch mode {
				case string(globals.HostMode):
					envVars = makeHostAuthEnvVars(execModeID, execModeSecret)
				}

				err := smokeRunBinary(exit, "agent",
					wd,
					envVars,
					binary,
					"agent",
					fmt.Sprintf("--mode=%s", mode),
					"--api_server", apiServerURL,
					"--log_output", string(globals.LogOutputFile),
					"--log_prefix", "smoke.agent",
					"--global_task_logs",
					"--status_port", statusPort,
					"--working_directory", wd,
				)

				if err != nil {
					return errors.Wrap(err, "running agent")
				}
			} else if startAgentMonitor {
				if distroID == "" {
					return errors.New("distro ID URL cannot be empty when starting agent monitor")
				}
				manager, err := jasper.NewSynchronizedManager(false)
				if err != nil {
					return errors.Wrap(err, "setting up Jasper process manager")
				}
				jasperAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("localhost:%d", jasperPort))
				if err != nil {
					return errors.Wrap(err, "resolving Jasper network address")
				}
				closeServer, err := remote.StartRPCService(ctx, manager, jasperAddr, nil)
				if err != nil {
					return errors.Wrap(err, "setting up Jasper RPC service")
				}
				defer func() {
					grip.Warning(closeServer())
				}()

				clientFile, err := os.CreateTemp("", "evergreen")
				if err != nil {
					return errors.Wrap(err, "setting up agent monitor client directory")
				}
				if err = clientFile.Close(); err != nil {
					return errors.Wrap(err, "closing Evergreen client binary")
				}

				err = smokeRunBinary(
					exit,
					"agent.monitor",
					wd,
					makeHostAuthEnvVars(execModeID, execModeSecret),
					binary,
					"agent",
					fmt.Sprintf("--mode=%s", globals.HostMode),
					"--api_server", apiServerURL,
					"--log_output", string(globals.LogOutputFile),
					"--global_task_logs",
					"--log_prefix", "smoke.agent",
					"--status_port", statusPort,
					"--working_directory", wd,
					"monitor",
					"--distro", distroID,
					"--client_path", clientFile.Name(),
					"--log_output", string(globals.LogOutputFile),
					"--log_prefix", "smoke.agent.monitor",
					"--port", strconv.Itoa(monitorPort),
					"--jasper_port", strconv.Itoa(jasperPort),
				)
				if err != nil {
					return errors.Wrap(err, "running agent monitor")
				}
			}

			<-exit
			return nil

		},
	}
}

func makeHostAuthEnvVars(hostID, secret string) []string {
	return []string{
		fmt.Sprintf("%s=%s", evergreen.HostIDEnvVar, hostID),
		fmt.Sprintf("%s=%s", evergreen.HostSecretEnvVar, secret),
	}
}

func smokeRunBinary(exit chan error, name string, wd string, envVars []string, bin string, cmdParts ...string) error {
	cmd := exec.Command(bin, cmdParts...)
	cmd.Env = append(os.Environ(), fmt.Sprintf("EVGHOME=%s", wd))
	cmd.Env = append(cmd.Env, envVars...)
	cmdSender := send.NewWriterSender(send.MakeNative())
	cmdSender.SetName(name)
	cmd.Stdout = cmdSender
	cmd.Stderr = cmdSender
	if err := cmd.Start(); err != nil {
		return errors.Wrap(err, "starting Evergreen binary command")
	}
	go func() {
		exit <- cmd.Wait()
		grip.Errorf("%s exited", name)
	}()
	return nil
}
