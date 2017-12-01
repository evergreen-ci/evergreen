package cli

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v2"
)

const (
	// DefaultBinaryName is the default path to the Evergreen binary.
	DefaultBinaryName = "evergreen"

	// DefaultConfigFile is the default configuration file for the web service and runner.
	DefaultConfigFile = "smoke_config.yml"

	// DefaultTestFile contains definitions of endpoints to check.
	DefaultTestFile = "smoke_test.yml"

	// UIPort is the local port the UI will listen on.
	UIPort = "9090"

	// APIPort is the local port the API will listen on.
	APIPort = "9090"

	// URLPrefix is the localhost prefix for accessing local Evergreen.
	URLPrefix = "http://localhost:"
)

// StartEvergreenCommand starts the Evergreen web service and runner.
type StartEvergreenCommand struct {
	Binary string `long:"binary" default:"" description:"path to Evergreen binary"`
	Conf   string `long:"conf" default:"" description:"Evergreen configuration file"`
}

// SmokeTestEndpointCommand runs tests against UI and API endpoints.
type SmokeTestEndpointCommand struct {
	TestFile string `long:"test-file" short:"t" description:"file with test endpoints definitions"`
}

// EndpointTestDefinitions describes the UI and API endpoints to verify are up.
type EndpointTestDefinitions struct {
	UI  map[string][]string `yaml:"ui,omitempty"`
	API map[string][]string `yaml:"api,omitempty"`
}

// Execute starts the Evergreen web service and runner.
func (c *StartEvergreenCommand) Execute(_ []string) error {
	setSenderNameToSmoke()
	wd, err := os.Getwd()
	if err != nil {
		return errors.Wrap(err, "error getting current directory")
	}
	if c.Binary == "" {
		c.Binary = filepath.Join(wd, "scripts", DefaultBinaryName)
	}
	if c.Conf == "" {
		c.Conf = filepath.Join(wd, "scripts", DefaultConfigFile)
	}
	web := exec.Command(c.Binary, "service", "web", "--conf", c.Conf)
	web.Env = []string{fmt.Sprintf("EVGHOME=%s", wd), "PATH=" + strings.Replace(os.Getenv("PATH"), `\`, `\\`, -1)}
	web.Stdout = os.Stdout
	web.Stderr = os.Stderr

	runner := exec.Command(c.Binary, "service", "runner", "--conf", c.Conf)
	runner.Env = []string{fmt.Sprintf("EVGHOME=%s", wd), "PATH=" + strings.Replace(os.Getenv("PATH"), `\`, `\\`, -1)}
	runner.Stdout = os.Stdout
	runner.Stderr = os.Stderr

	if err := web.Start(); err != nil {
		return errors.Wrap(err, "error starting web service")
	}
	if err := runner.Start(); err != nil {
		return errors.Wrap(err, "error starting runner")
	}

	exit := make(chan error)
	interrupt := make(chan os.Signal)
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
		grip.Error("problem running Evergreen")
		os.Exit(1)
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
}

// Execute runs tests against UI and API endpoints.
func (c *SmokeTestEndpointCommand) Execute(_ []string) error {
	setSenderNameToSmoke()
	wd, err := os.Getwd()
	if err != nil {
		return errors.Wrap(err, "error getting current directory")
	}

	if c.TestFile == "" {
		c.TestFile = filepath.Join(wd, "scripts", DefaultTestFile)
	}

	defs, err := ioutil.ReadFile(c.TestFile)
	if err != nil {
		return errors.Wrap(err, "error opening test file")
	}
	tests := EndpointTestDefinitions{}
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
		_, err = client.Get(URLPrefix + UIPort)
		if err != nil {
			if i == attempts {
				err = errors.Wrapf(err, "could not connect to Evergreen after %d attempts", attempts)
				grip.Error(err)
				return err
			}
			grip.Infof("could not connect to Evergreen (attempt %d of %d)", i, attempts)
			time.Sleep(time.Second)
			continue
		} else {
			grip.Info("Evergreen is up")
			break
		}
	}

	// check endpoints
	catcher := grip.NewSimpleCatcher()
	for url, expected := range tests.UI {
		grip.Infof("Getting endpoint '%s'", url)
		resp, err := client.Get(URLPrefix + UIPort + url)
		if err != nil {
			catcher.Add(errors.Errorf("error getting UI endpoint '%s'", url))
		}
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
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
	if catcher.HasErrors() {
		grip.Error(catcher.String())
		grip.Errorf("failed to get %d endpoints", catcher.Len())
		os.Exit(1)
	} else {
		grip.Info("success: all endpoints accessible")
	}

	return nil
}

func setSenderNameToSmoke() {
	sender := grip.GetSender()
	sender.SetName("smoke test")
	if err := grip.SetSender(sender); err != nil {
		grip.Error(errors.Wrap(err, "error setting sender"))
	}
}
