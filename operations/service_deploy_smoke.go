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

	"github.com/evergreen-ci/evergreen/agent"
	timberutil "github.com/evergreen-ci/timber/testutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/remote"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	"gopkg.in/yaml.v3"
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
			if err := smokeRunBinary(exit, "web.service", wd, binary, "service", "web", "--db", "evergreen_local"); err != nil {
				return errors.Wrap(err, "running web service")
			}
			<-exit
			return nil
		},
	}
}

func smokeStartEvergreen() cli.Command {
	const (
		binaryFlagName       = "binary"
		agentFlagName        = "agent"
		webFlagName          = "web"
		agentMonitorFlagName = "monitor"
		distroIDFlagName     = "distro"
		modeFlagName         = "mode"

		// apiPort is the local port the API will listen on.
		apiPort = ":9090"

		cedarPort = 7070

		id         = "localhost"
		secret     = "de249183582947721fdfb2ea1796574b"
		statusPort = "2287"

		monitorPort = 2288
		jasperPort  = 2289
	)

	wd, err := os.Getwd()

	binary := filepath.Join(wd, "clients", runtime.GOOS+"_"+runtime.GOARCH, "evergreen")
	confPath := filepath.Join(wd, "testdata", "smoke", "admin_settings.yml")

	return cli.Command{
		Name:    "start-evergreen",
		Aliases: []string{},
		Usage:   "start Evergreen web service for smoke tests",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  confFlagName,
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
				Usage: "run the agent in host or pod mode",
			},
		},
		Before: mergeBeforeFuncs(setupSmokeTest(err), requireFileExists(confFlagName), requireAtLeastOneBool(webFlagName, agentFlagName, agentMonitorFlagName)),
		Action: func(c *cli.Context) error {
			confPath := c.String(confFlagName)
			binary := c.String(binaryFlagName)
			startWeb := c.Bool(webFlagName)
			startAgent := c.Bool(agentFlagName)
			startAgentMonitor := c.Bool(agentMonitorFlagName)
			distroID := c.String(distroIDFlagName)
			mode := c.String(modeFlagName)

			exit := make(chan error, 3)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if startWeb {
				if err := smokeRunBinary(exit, "web.service", wd, binary, "service", "web", "--conf", confPath); err != nil {
					return errors.Wrap(err, "running web service")
				}
			}
			apiServerURL := smokeUrlPrefix + apiPort

			if startAgent {
				_, err = timberutil.NewMockCedarServer(ctx, cedarPort)
				if err != nil {
					return errors.Wrap(err, "starting mock Cedar service")
				}

				err := smokeRunBinary(exit, "agent",
					wd,
					binary,
					"agent",
					fmt.Sprintf("--mode=%s", mode),
					fmt.Sprintf("--%s_id", mode), id,
					fmt.Sprintf("--%s_secret", mode), secret,
					"--api_server", apiServerURL,
					"--log_output", string(agent.LogOutputFile),
					"--log_prefix", "smoke.agent",
					"--global_task_logs",
					"--status_port", statusPort,
					"--working_directory", wd,
				)

				if err != nil {
					return errors.Wrap(err, "running agent")
				}
			} else if startAgentMonitor {
				_, err = timberutil.NewMockCedarServer(ctx, cedarPort)
				if err != nil {
					return errors.Wrap(err, "starting mock Cedar service")
				}

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
					binary,
					"agent",
					fmt.Sprintf("--mode=%s", agent.HostMode),
					"--host_id", id,
					"--host_secret", secret,
					"--api_server", apiServerURL,
					"--log_output", string(agent.LogOutputFile),
					"--global_task_logs",
					"--log_prefix", "smoke.agent",
					"--status_port", statusPort,
					"--working_directory", wd,
					"monitor",
					"--distro", distroID,
					"--client_path", clientFile.Name(),
					"--log_output", string(agent.LogOutputFile),
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

func smokeRunBinary(exit chan error, name, wd, bin string, cmdParts ...string) error {
	cmd := exec.Command(bin, cmdParts...)
	cmd.Env = append(os.Environ(), fmt.Sprintf("EVGHOME=%s", wd))
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

func smokeTestEndpoints() cli.Command {
	const (
		testFileFlagName      = "test-file"
		projectNameFlagName   = "project"
		userNameFlagName      = "username"
		userKeyFlagName       = "key"
		checkBuildFlagName    = "check-build"
		modeFlagName          = "mode"
		bvFlagName            = "buildvariant"
		cliPathFlagName       = "cli"
		cliConfigPathFlagName = "cli-config"
	)

	wd, err := os.Getwd()

	return cli.Command{
		Name:    "test-endpoints",
		Aliases: []string{"smoke-test"},
		Usage:   "run smoke tests against ",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  testFileFlagName,
				Usage: "file with test endpoints definitions",
				Value: filepath.Join(wd, "testdata", "smoke", "smoke_test_endpoints.yml"),
			},
			cli.StringFlag{
				Name:  projectNameFlagName,
				Usage: "ID of project in which smoke test should run",
				Value: "evergreen",
			},
			cli.StringFlag{
				Name:  bvFlagName,
				Usage: "build variant to run in the smoke test",
				Value: "localhost",
			},
			cli.StringFlag{
				Name:  cliPathFlagName,
				Usage: "path to the Evergreen CLI to use for the smoke test",
				Value: filepath.Join(wd, "clients", runtime.GOOS+"_"+runtime.GOARCH, "evergreen"),
			},
			cli.StringFlag{
				Name:  cliConfigPathFlagName,
				Usage: "path to the Evergreen CLI config to use for smoke test",
				Value: filepath.Join(wd, "testdata", "smoke", "cli.yml"),
			},
			cli.StringFlag{
				Name:  userNameFlagName,
				Usage: "username to use with the Evergreen API",
				Value: "admin",
			},
			cli.StringFlag{
				Name:  userKeyFlagName,
				Usage: "key to use with the Evergreen API",
				Value: "abb623665fdbf368a1db980dde6ee0f0",
			},
			cli.StringFlag{
				Name:  modeFlagName,
				Usage: "run host or pod build variant",
			},
			cli.BoolFlag{
				Name:  checkBuildFlagName,
				Usage: "run the smoke test and verify the tasks run",
			},
		},
		Before: mergeBeforeFuncs(setupSmokeTest(err)),
		Action: func(c *cli.Context) error {
			testFile := c.String(testFileFlagName)
			projectName := c.String(projectNameFlagName)
			cliPath := c.String(cliPathFlagName)
			cliConfigPath := c.String(cliConfigPathFlagName)
			username := c.String(userNameFlagName)
			key := c.String(userKeyFlagName)
			mode := c.String(modeFlagName)
			bv := c.String(bvFlagName)

			defs, err := os.ReadFile(testFile)
			if err != nil {
				return errors.Wrapf(err, "opening smoke endpoint test file '%s'", testFile)
			}
			tests := smokeEndpointTestDefinitions{}
			err = yaml.Unmarshal(defs, &tests)
			if err != nil {
				return errors.Wrapf(err, "unmarshalling smoke endpoint test YAML '%s'", testFile)
			}

			if c.Bool(checkBuildFlagName) {
				if mode == string(agent.PodMode) {
					return errors.Wrap(checkContainerTask(username, key), "checking container task")
				}
				return errors.Wrap(checkHostTaskByPatch(projectName, bv, cliPath, cliConfigPath, username, key), "checking host task")
			}
			return errors.WithStack(tests.checkEndpoints(username, key))
		},
	}
}
