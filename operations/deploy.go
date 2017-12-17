package operations

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/migrations"
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
			migration(),
			startEvergreen(),
			smokeTestEndpoints(),
		},
	}
}

func migration() cli.Command {
	return cli.Command{
		Name:    "anser",
		Aliases: []string{"migrations", "migrate"},
		Usage:   "database migration tool",
		Flags: serviceConfigFlags(
			cli.BoolFlag{
				Name:  "dry-run, n",
				Usage: "run migration in a dry-run mode",
			},
			cli.IntFlag{
				Name:  "limit, l",
				Usage: "limit the number of migration jobs to process",
			},
			cli.IntFlag{
				Name:  "target, t",
				Usage: "target number of migrations",
				Value: 60,
			},
			cli.IntFlag{
				Name:  "workers, j",
				Usage: "total number of parallel migration workers",
				Value: 4,
			},
			cli.DurationFlag{
				Name:  "period, p",
				Usage: "length of scheduling window",
				Value: time.Minute,
			},
		),
		Action: func(c *cli.Context) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			env := evergreen.GetEnvironment()
			err := env.Configure(ctx, c.String(confFlagName))

			grip.CatchEmergencyFatal(errors.Wrap(err, "problem configuring application environment"))
			settings := env.Settings()

			opts := migrations.Options{
				Period:   c.Duration("period"),
				Target:   c.Int("target"),
				Limit:    c.Int("limit"),
				Session:  env.Session(),
				Workers:  c.Int("workers"),
				Database: settings.Database.DB,
			}

			anserEnv, err := opts.Setup(ctx)
			if err != nil {
				return errors.Wrap(err, "problem setting up migration environment")
			}
			defer anserEnv.Close()

			app, err := opts.Application(anserEnv)
			if err != nil {
				return errors.Wrap(err, "problem configuring migration application")
			}
			app.DryRun = c.Bool("dry-run")

			return errors.Wrap(app.Run(ctx), "problem running migration operation")
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

func startEvergreen() cli.Command {
	const (
		binaryFlagName = "binary"
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
		},
		Before: setupSmokeTest(err),
		Action: func(c *cli.Context) error {
			confPath := c.String(confFlagName)
			binary := c.String(binaryFlagName)

			web := exec.Command(binary, "service", "web", "--conf", confPath)
			web.Env = []string{fmt.Sprintf("EVGHOME=%s", wd), "PATH=" + strings.Replace(os.Getenv("PATH"), `\`, `\\`, -1)}
			webSender := send.NewWriterSender(send.MakeNative())
			defer webSender.Close()
			webSender.SetName("evergreen.smoke.web.service")
			web.Stdout = webSender
			web.Stderr = webSender

			runner := exec.Command(binary, "service", "runner", "--conf", confPath)
			runner.Env = []string{fmt.Sprintf("EVGHOME=%s", wd), "PATH=" + strings.Replace(os.Getenv("PATH"), `\`, `\\`, -1)}
			runnerSender := send.NewWriterSender(send.MakeNative())
			defer runnerSender.Close()
			runnerSender.SetName("evergreen.smoke.runner")
			runner.Stdout = runnerSender
			runner.Stderr = runnerSender

			if err = web.Start(); err != nil {
				return errors.Wrap(err, "error starting web service")
			}
			if err = runner.Start(); err != nil {
				return errors.Wrap(err, "error starting runner")
			}

			exit := make(chan error)
			interrupt := make(chan os.Signal, 1)
			signal.Notify(interrupt, os.Interrupt)
			go func() {
				exit <- web.Wait()
				grip.Errorf("web service exited: %s", err)
			}()
			go func() {
				exit <- runner.Wait()
				grip.Errorf("runner exited: %s", err)
			}()

			select {
			case <-exit:
				err = errors.New("problem running Evergreen")
				grip.Error(err)
				return err
			case <-interrupt:
				grip.Info("received SIGINT, killing Evergreen")
				if err := web.Process.Kill(); err != nil {
					grip.Errorf("error killing evergreen web service: %s", err)
				}
				if err := runner.Process.Kill(); err != nil {
					grip.Errorf("error killing evergreen runner: %s", err)
				}
			}

			return nil

		},
	}
}

// smokeEndpointTestDefinitions describes the UI and API endpoints to verify are up.
type smokeEndpointTestDefinitions struct {
	UI  map[string][]string `yaml:"ui,omitempty"`
	API map[string][]string `yaml:"api,omitempty"`
}

func smokeTestEndpoints() cli.Command {
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
