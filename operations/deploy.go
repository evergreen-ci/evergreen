package operations

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	yaml "gopkg.in/yaml.v2"
)

func Deploy() cli.Command {
	return cli.Command{
		Name:  "deploy",
		Usage: "deployment helpers for evergreen site administration",
		Subcommands: []cli.Command{
			deployMigration(),
			smokeStartEvergreen(),
			smokeTestEndpoints(),
		},
	}
}

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
				if err := c.smokeRunBinary(exit, "web.service", binary, "service", "web", "--conf", confPath); err != nil {
					return errors.Wrap(err, "error running web service")
				}
			}

			if startRunner {
				if err := c.smokeRunBinary(exit, "runner", binary, "service", "runner", "--conf", confPath); err != nil {
					return errors.Wrap(err, "error running web service")
				}
			}

			if startAgent {
				err := smokeRunBinary(exit, "agent",
					"agent",
					"--host_id", hostId,
					"--host_secret", hostSecret,
					"--api_server", urlPrefix+apiPort,
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

// smokeEndpointTestDefinitions describes the UI and API endpoints to verify are up.
type smokeEndpointTestDefinitions struct {
	UI  map[string][]string `yaml:"ui,omitempty"`
	API map[string][]string `yaml:"api,omitempty"`
}

func deploySmokeTest() cli.Command {
	const (
		// uiPort is the local port the UI will listen on.
		uiPort = ":9090"
		// urlPrefix is the localhost prefix for accessing local Evergreen.
		urlPrefix = "http://localhost"

		testFileFlagName = "test-file"
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
		},
		Before: setupSmokeTest(err),
		Action: func(c *cli.Context) error {
			testFile := c.String(testFileFlagName)

			defs, err := ioutil.ReadFile(testFile)
			if err != nil {
				return errors.Wrap(err, "error opening test file")
			}
			tests := smokeEndpointTestDefinitions{}
			err = yaml.Unmarshal(defs, &tests)
			if err != nil {
				return errors.Wrap(err, "error unmarshalling yaml")
			}

			client := http.Client{}
			client.Timeout = time.Second

			// wait for web service to start
			attempts := 10
			for i := 1; i <= attempts; i++ {
				grip.Infof("checking if Evergreen is up (attempt %d of %d)", i, attempts)
				_, err = client.Get(urlPrefix + uiPort)
				if err != nil {
					if i == attempts {
						err = errors.Wrapf(err, "could not connect to Evergreen after %d attempts", attempts)
						grip.Error(err)
						return err
					}
					grip.Infof("could not connect to Evergreen (attempt %d of %d)", i, attempts)
					time.Sleep(time.Second)
					continue
				}
			}
			grip.Info("Evergreen is up")

			// check endpoints
			catcher := grip.NewSimpleCatcher()
			for url, expected := range tests.UI {
				grip.Infof("Getting endpoint '%s'", url)
				resp, err := client.Get(urlPrefix + uiPort + url)
				if err != nil {
					catcher.Add(errors.Errorf("error getting UI endpoint '%s'", url))
				}
				defer resp.Body.Close()
				body, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					err = errors.Wrap(err, "error reading response body")
					grip.Error(err)
					return err
				}
				page := string(body)
				for _, text := range expected {
					if strings.Contains(page, text) {
						grip.Infof("found '%s' in UI endpoint '%s'", text, url)
					} else {
						grip.Infof("did not find '%s' in UI endpoint '%s'", text, url)
						catcher.Add(errors.Errorf("'%s' not in UI endpoint '%s'", text, url))
					}
				}
			}

			grip.InfoWhen(!catcher.HasErrors(), "success: all endpoints accessible")

			return errors.Wrapf(catcher.Resolve(), "failed to get %d endpoints", catcher.Len())
		},
	}
}
