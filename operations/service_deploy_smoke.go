package operations

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	yaml "gopkg.in/yaml.v2"
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

func smokeStartEvergreen() cli.Command {
	const (
		binaryFlagName = "binary"
		runnerFlagName = "runner"
		agentFlagName  = "agent"
		webFlagName    = "web"

		// apiPort is the local port the API will listen on.
		apiPort = "8080"

		hostId     = "localhost"
		hostSecret = "de249183582947721fdfb2ea1796574b"
		statusPort = "2287"
	)

	wd, err := os.Getwd()

	binary := filepath.Join(wd, "clients", runtime.GOOS+"_"+runtime.GOARCH, "evergreen")
	confPath := filepath.Join(wd, "scripts", "smoke_config.yml")

	return cli.Command{
		Name:    "start-evergreen",
		Aliases: []string{},
		Usage:   "start evergreen web service and runner (for smoke tests)",
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
				Name:  runnerFlagName,
				Usage: "run the evergreen runner",
			},
			cli.BoolFlag{
				Name:  webFlagName,
				Usage: "run the evergreen web service",
			},
			cli.BoolFlag{
				Name:  agentFlagName,
				Usage: "start an evergreen agent",
			},
		},
		Before: mergeBeforeFuncs(setupSmokeTest(err), requireFileExists(confFlagName), requireAtLeastOneBool(runnerFlagName, webFlagName)),
		Action: func(c *cli.Context) error {
			confPath := c.String(confFlagName)
			binary := c.String(binaryFlagName)
			startRunner := c.Bool(runnerFlagName)
			startWeb := c.Bool(webFlagName)
			startAgent := c.Bool(agentFlagName)

			exit := make(chan error, 2)

			if startWeb {
				if err := smokeRunBinary(exit, "web.service", binary, "service", "web", "--conf", confPath); err != nil {
					return errors.Wrap(err, "error running web service")
				}
			}

			if startRunner {
				if err := smokeRunBinary(exit, "runner", binary, "service", "runner", "--conf", confPath); err != nil {
					return errors.Wrap(err, "error running web service")
				}
			}

			if startAgent {
				err := smokeRunBinary(exit, "agent",
					"agent",
					"--host_id", hostId,
					"--host_secret", hostSecret,
					"--api_server", smokeUrlPrefix+apiPort,
					"--log_prefix", evergreen.LocalLoggingOverride,
					"--status_port", statusPort,
					"--working_directory", wd)

				if err != nil {
					return errors.Wrap(err, "error running agent")
				}
			}

			<-exit
			return nil

		},
	}
}

func smokeRunBinary(exit chan error, name, bin, wd string, cmdParts ...string) error {
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
		grip.Errorf("%s service exited", name)
	}()
	return nil
}

func smokeTestEndpoints() cli.Command {
	const (
		testFileFlagName = "test-file"
		commitFlagName   = "commit"
		userNameFlagName = "username"
		userKeyFlagName  = "key"
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
				Name:  commitFlagName,
				Usage: "verify a task as run for this commit",
			},
			cli.StringFlag{
				Name:  userNameFlagName,
				Usage: "username to use with the API",
			},
			cli.StringFlag{
				Name:  userKeyFlagName,
				Usage: "key to use with the API",
			},
		},
		Before: mergeBeforeFuncs(setupSmokeTest(err), requireOnlyOneString(testFileFlagName, commitFlagName)),
		Action: func(c *cli.Context) error {
			testFile := c.String(testFileFlagName)
			commit := c.String(commitFlagName)
			username := c.String(userNameFlagName)
			key := c.String(userKeyFlagName)

			defs, err := ioutil.ReadFile(testFile)
			if err != nil {
				return errors.Wrap(err, "error opening test file")
			}
			tests := smokeEndpointTestDefinitions{}
			err = yaml.Unmarshal(defs, &tests)
			if err != nil {
				return errors.Wrap(err, "error unmarshalling yaml")
			}

			if testFileFlagName != "" {
				return errors.WithStack(tests.checkEndpoints())
			}

			return errors.Wrap(checkTaskByCommit(username, key, commit), "check task failed")
		},
	}
}
