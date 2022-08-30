package operations

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent"
	timberutil "github.com/evergreen-ci/timber/testutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/remote"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	yaml "gopkg.in/20210107192922/yaml.v3"
)

func setupSmokeTest(err error) cli.BeforeFunc {
	return func(c *cli.Context) error {
		if err != nil {
			return errors.Wrap(err, "problem getting working directory")
		}
		grip.GetSender().SetName("evergreen.smoke")
		return nil
	}
}

func startLocalEvergreen() cli.Command {
	return cli.Command{
		Name:  "start-local-evergreen",
		Usage: "start an evergreen for local development",
		Action: func(c *cli.Context) error {
			exit := make(chan error, 1)
			wd, err := os.Getwd()
			if err != nil {
				return errors.Wrap(err, "couldn't get working directory")
			}
			binary := filepath.Join(wd, "clients", runtime.GOOS+"_"+runtime.GOARCH, "evergreen")
			if err := smokeRunBinary(exit, "web.service", wd, binary, "service", "web", "--db", "evergreen_local"); err != nil {
				return errors.Wrap(err, "error running web service")
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
	confPath := filepath.Join(wd, "testdata", "smoke_config.yml")

	return cli.Command{
		Name:    "start-evergreen",
		Aliases: []string{},
		Usage:   "start evergreen web service for smoke tests",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  confFlagName,
				Usage: "path to the (test) service configuration file",
				Value: confPath,
			},
			cli.StringFlag{
				Name:  binaryFlagName,
				Usage: "path to evergreen binary",
				Value: binary,
			},
			cli.BoolFlag{
				Name:  webFlagName,
				Usage: "run the evergreen web service",
			},
			cli.BoolFlag{
				Name:  agentFlagName,
				Usage: "start an evergreen agent",
			},
			cli.BoolFlag{
				Name:  agentMonitorFlagName,
				Usage: "start an evergreen agent monitor",
			},
			cli.StringFlag{
				Name:  distroIDFlagName,
				Usage: "the distro ID of the agent monitor",
			},
			cli.StringFlag{
				Name:  modeFlagName,
				Usage: "run the agent in host or pod more",
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
				if err := smokeRunBinary(exit, "web.service", wd, binary, "service", "web", "--conf", confPath, "--db", "mci_smoke"); err != nil {
					return errors.Wrap(err, "error running web service")
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
					"--log_prefix", evergreen.StandardOutputLoggingOverride,
					"--status_port", statusPort,
					"--working_directory", wd,
				)

				if err != nil {
					return errors.Wrap(err, "error running agent")
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
					return errors.Wrap(err, "error setting up Jasper process manager")
				}
				jasperAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("localhost:%d", jasperPort))
				if err != nil {
					return errors.Wrap(err, "error resolving Jasper network address")
				}
				closeServer, err := remote.StartRPCService(ctx, manager, jasperAddr, nil)
				if err != nil {
					return errors.Wrap(err, "error setting up Jasper RPC service")
				}
				defer func() {
					grip.Warning(closeServer())
				}()

				clientFile, err := ioutil.TempFile("", "evergreen")
				if err != nil {
					return errors.Wrap(err, "error setting up monitor client directory")
				}
				if err = clientFile.Close(); err != nil {
					return errors.Wrap(err, "closing evergreen client binary")
				}

				err = smokeRunBinary(
					exit,
					"monitor",
					wd,
					binary,
					"agent",
					"--mode=host",
					"--host_id", id,
					"--host_secret", secret,
					"--api_server", apiServerURL,
					"--log_prefix", evergreen.StandardOutputLoggingOverride,
					"--status_port", statusPort,
					"--working_directory", wd,
					"monitor",
					"--distro", distroID,
					"--client_path", clientFile.Name(),
					"--log_prefix", evergreen.StandardOutputLoggingOverride,
					"--port", strconv.Itoa(monitorPort),
					"--jasper_port", strconv.Itoa(jasperPort),
				)
				if err != nil {
					return errors.Wrap(err, "error running monitor")
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
		return errors.Wrap(err, "error starting cmd service")
	}
	go func() {
		exit <- cmd.Wait()
		grip.Errorf("%s service exited %s", name, exit)
	}()
	return nil
}

func smokeTestEndpoints() cli.Command {
	const (
		testFileFlagName = "test-file"
		userNameFlagName = "username"
		userKeyFlagName  = "key"
		checkBuildName   = "check-build"
		modeFlagName     = "mode"
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
				Value: filepath.Join(wd, "scripts", "smoke_test.yml"),
			},
			cli.StringFlag{
				Name:  userNameFlagName,
				Usage: "username to use with the API",
			},
			cli.StringFlag{
				Name:  userKeyFlagName,
				Usage: "key to use with the API",
			},
			cli.StringFlag{
				Name:  modeFlagName,
				Usage: "run host or pod build variant",
			},
			cli.BoolFlag{
				Name:  checkBuildName,
				Usage: "verify agent has built latest commit",
			},
		},
		Before: mergeBeforeFuncs(setupSmokeTest(err)),
		Action: func(c *cli.Context) error {
			testFile := c.String(testFileFlagName)
			username := c.String(userNameFlagName)
			key := c.String(userKeyFlagName)
			mode := c.String(modeFlagName)

			defs, err := ioutil.ReadFile(testFile)
			if err != nil {
				return errors.Wrap(err, "error opening test file")
			}
			tests := smokeEndpointTestDefinitions{}
			err = yaml.Unmarshal(defs, &tests)
			if err != nil {
				return errors.Wrap(err, "error unmarshalling yaml")
			}

			if c.Bool(checkBuildName) {
				if mode == string(agent.PodMode) {
					return errors.Wrap(checkContainerTask(username, key), "check task failed")
				}
				return errors.Wrap(checkTaskByCommit(username, key), "check task failed")
			}
			return errors.WithStack(tests.checkEndpoints(username, key))
		},
	}
}
