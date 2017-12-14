package cli

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

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v2"
)

const (
	// defaultBinaryName is the default path to the Evergreen binary.
	defaultBinaryName = "evergreen"

	// defaultConfigFile is the default configuration file for the web service and runner.
	defaultConfigFile = "smoke_config.yml"

	// uiPort is the local port the UI will listen on.
	uiPort = "9090"

	// apiPort is the local port the API will listen on.
	apiPort = "8080"

	// urlPrefix is the localhost prefix for accessing local Evergreen.
	urlPrefix = "http://localhost:"
)

// StartEvergreenCommand starts the Evergreen web service and runner.
type StartEvergreenCommand struct {
	Binary string `long:"binary" default:"" description:"path to Evergreen binary"`
	Conf   string `long:"conf" default:"" description:"Evergreen configuration file"`
	Runner bool   `long:"runner" description:"Run only the Evergreen runner"`
	Web    bool   `long:"web" description:"Run only the Evergreen web service"`
}

// SmokeTestEndpointCommand runs tests against UI and API endpoints.
type SmokeTestEndpointCommand struct {
	TestFile string `long:"test-file" description:"file with test endpoints definitions"`

	client http.Client
	tests  EndpointTestDefinitions
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
		c.Binary = filepath.Join(wd, "clients", runtime.GOOS+"_"+runtime.GOARCH, defaultBinaryName)
	}
	if c.Conf == "" {
		c.Conf = filepath.Join(wd, "scripts", defaultConfigFile)
	}
	if !c.Runner && !c.Web {
		return errors.New("Must specify --web or --runner (or both)")
	}

	exit := make(chan error, 2)
	if c.Web {
		web := exec.Command(c.Binary, "service", "web", "--conf", c.Conf)
		web.Env = []string{fmt.Sprintf("EVGHOME=%s", wd), "PATH=" + strings.Replace(os.Getenv("PATH"), `\`, `\\`, -1), "EVR_TASK_ID=" + os.Getenv("EVR_TASK_ID"), "EVR_AGENT_PID" + os.Getenv("EVR_AGENT_PID")}
		webSender := send.NewWriterSender(send.MakeNative())
		defer webSender.Close()
		webSender.SetName("web.service")
		web.Stdout = webSender
		web.Stderr = webSender
		if err = web.Start(); err != nil {
			return errors.Wrap(err, "error starting web service")
		}
		defer web.Process.Kill()
		go func() {
			exit <- web.Wait()
			grip.Errorf("web service exited: %s", err)
		}()
	}

	if c.Runner {
		runner := exec.Command(c.Binary, "service", "runner", "--conf", c.Conf)
		runner.Env = []string{fmt.Sprintf("EVGHOME=%s", wd), "PATH=" + strings.Replace(os.Getenv("PATH"), `\`, `\\`, -1), "EVR_TASK_ID=" + os.Getenv("EVR_TASK_ID"), "EVR_AGENT_PID" + os.Getenv("EVR_AGENT_PID")}
		runnerSender := send.NewWriterSender(send.MakeNative())
		defer runnerSender.Close()
		runnerSender.SetName("runner")
		runner.Stdout = runnerSender
		runner.Stderr = runnerSender
		if err = runner.Start(); err != nil {
			return errors.Wrap(err, "error starting runner")
		}
		defer runner.Process.Kill()
		go func() {
			exit <- runner.Wait()
			grip.Errorf("runner exited: %s", err)
		}()
	}

	<-exit
	return nil
}

// Execute runs tests against UI and API endpoints.
func (c *SmokeTestEndpointCommand) Execute(_ []string) error {
	setSenderNameToSmoke()
	if c.TestFile == "" {
		return errors.New("must specify --test-file")

	}
	// wait for web service to start
	c.client = http.Client{}
	c.client.Timeout = time.Second
	attempts := 10
	for i := 1; i <= attempts; i++ {
		grip.Infof("checking if Evergreen is up (attempt %d of %d)", i, attempts)
		_, err := c.client.Get(urlPrefix + uiPort)
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
		break
	}
	grip.Info("Evergreen is up")

	if err := c.checkEndpointsFromFile(); err != nil {
		return errors.Wrap(err, "test endpoints failed")
	}
	grip.Info("success: all endpoints accessible")
	return nil
}

func setSenderNameToSmoke() {
	sender := grip.GetSender()
	sender.SetName("evergreen.smoke")
}

func (c *SmokeTestEndpointCommand) checkEndpointsFromFile() error {
	wd, err := os.Getwd()
	if err != nil {
		return errors.Wrap(err, "error getting current directory")
	}
	c.TestFile = filepath.Join(wd, "scripts", c.TestFile)
	defs, err := ioutil.ReadFile(c.TestFile)
	if err != nil {
		return errors.Wrap(err, "error opening test file")
	}
	c.tests = EndpointTestDefinitions{}
	err = yaml.Unmarshal(defs, &c.tests)
	if err != nil {
		return errors.Wrap(err, "error unmarshalling yaml")
	}

	return c.checkEndpoints()
}

func (c *SmokeTestEndpointCommand) checkEndpoints() error {
	catcher := grip.NewSimpleCatcher()
	for url, expected := range c.tests.UI {
		grip.Infof("Getting endpoint '%s'", url)
		resp, err := c.client.Get(urlPrefix + uiPort + url)
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
	for url, expected := range c.tests.API {
		grip.Infof("Getting endpoint '%s'", url)
		resp, err := c.client.Get(urlPrefix + apiPort + url)
		if err != nil {
			catcher.Add(errors.Errorf("error getting API endpoint '%s'", url))
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
				grip.Infof("found '%s' in API endpoint '%s'", text, url)
			} else {
				grip.Infof("did not find '%s' in API endpoint '%s'", text, url)
				catcher.Add(errors.Errorf("'%s' not in API endpoint '%s'", text, url))
			}
		}
	}

	if catcher.HasErrors() {
		grip.Error(catcher.String())
		grip.ErrorWhenf(catcher.HasErrors(), "failed to get %d endpoints", catcher.Len())
	}
	return catcher.Resolve()
}
