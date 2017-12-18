package cli

import (
	"encoding/json"
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
	"github.com/evergreen-ci/evergreen/apimodels"
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

	userHeader = "Auth-Username"
	keyHeader  = "Api-Key"

	hostId     = "localhost"
	hostSecret = "de249183582947721fdfb2ea1796574b"
	statusPort = "2287"
)

// StartEvergreenCommand starts the Evergreen web service and runner.
type StartEvergreenCommand struct {
	Binary string `long:"binary" default:"" description:"path to Evergreen binary"`
	Conf   string `long:"conf" default:"" description:"Evergreen configuration file"`
	Runner bool   `long:"runner" description:"Run the Evergreen runner"`
	Web    bool   `long:"web" description:"Run the Evergreen web service"`
	Agent  bool   `long:"agent" description:"Run the Evergreen agent"`

	wd string
}

// SmokeTestEndpointCommand runs tests against UI and API endpoints.
type SmokeTestEndpointCommand struct {
	TestFile string `long:"test-file" description:"file with test endpoints definitions"`
	Commit   string `long:"commit" description:"verify that a task has run for this commit"`
	UserName string `long:"username" description:"username to use with api"`
	UserKey  string `long:"key" description:"key to use with api"`

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
	var err error
	c.wd, err = os.Getwd()
	if err != nil {
		return errors.Wrap(err, "error getting current directory")
	}
	if !c.Runner && !c.Web && !c.Agent {
		return errors.New("Must specify at least one of --agent, --runner, or --web")
	}
	if c.Binary == "" {
		c.Binary = filepath.Join(c.wd, "clients", runtime.GOOS+"_"+runtime.GOARCH, defaultBinaryName)
	}
	if c.Conf == "" {
		c.Conf = filepath.Join(c.wd, "scripts", defaultConfigFile)
	}

	exit := make(chan error, 3)

	if c.Agent {
		if err := c.runBinary(exit, "agent", []string{
			"agent",
			"--host_id",
			hostId,
			"--host_secret",
			hostSecret,
			"--api_server",
			urlPrefix + apiPort,
			"--log_prefix",
			evergreen.LocalLoggingOverride,
			"--status_port",
			statusPort,
			"--working_directory",
			c.wd,
		}); err != nil {
			return errors.Wrap(err, "error running agent")
		}
	}
	if c.Runner {
		if err := c.runBinary(exit, "runner", []string{"service", "runner", "--conf", c.Conf}); err != nil {
			return errors.Wrap(err, "error running runner")
		}
	}
	if c.Web {
		if err := c.runBinary(exit, "web.service", []string{"service", "web", "--conf", c.Conf}); err != nil {
			return errors.Wrap(err, "error running web service")
		}
	}

	<-exit
	return nil
}

func (c *StartEvergreenCommand) runBinary(exit chan error, name string, cmdParts []string) error {
	cmd := exec.Command(c.Binary, cmdParts...)
	cmd.Env = append(os.Environ(), fmt.Sprintf("EVGHOME=%s", c.wd))
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

// Execute runs tests against UI and API endpoints.
func (c *SmokeTestEndpointCommand) Execute(_ []string) error {
	setSenderNameToSmoke()
	err := errors.New("must specify either --test-file or --commit")
	if c.TestFile == "" && c.Commit == "" {
		return err
	}
	if c.TestFile != "" && c.Commit != "" {
		return err
	}

	// wait for web service to start
	c.client = http.Client{}
	c.client.Timeout = time.Second
	attempts := 10
	for i := 1; i <= attempts; i++ {
		grip.Infof("checking if Evergreen is up (attempt %d of %d)", i, attempts)
		_, err = c.client.Get(urlPrefix + uiPort)
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

	if c.TestFile != "" {
		if err := c.checkEndpointsFromFile(); err != nil {
			return errors.Wrap(err, "test endpoints failed")
		}
		grip.Info("success: all endpoints accessible")
		return nil
	}

	if err := c.checkTaskByCommit(); err != nil {
		return errors.Wrap(err, "check task failed")
	}
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

func (c *SmokeTestEndpointCommand) checkTaskByCommit() error {
	var builds []apimodels.APIBuild
	var build apimodels.APIBuild
	for i := 0; i <= 30; i++ {
		// get task id
		if i == 30 {
			return errors.New("error getting builds for version")
		}
		time.Sleep(10 * time.Second)
		grip.Infof("checking for a build of %s (%d/30)", c.Commit, i+1)
		resp, err := c.client.Get(urlPrefix + uiPort + "/rest/v2/versions/evergreen_" + c.Commit + "/builds")
		if err != nil {
			grip.Info(err)
			continue
		}
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			err = errors.Wrap(err, "error reading response body")
			grip.Error(err)
			return err
		}
		resp.Body.Close()
		err = json.Unmarshal(body, &builds)
		if err != nil {
			err = json.Unmarshal(body, &build)
			if err != nil {
				return errors.Wrap(err, "error unmarshaling json")
			}
		}
		if len(builds) == 0 {
			builds = []apimodels.APIBuild{build}
		}
		if len(builds[0].Tasks) == 0 {
			builds = []apimodels.APIBuild{}
			build = apimodels.APIBuild{}
			continue
		}
		break
	}

	var task apimodels.APITask
	for i := 0; i <= 30; i++ {
		// check task
		if i == 30 {
			return errors.Errorf("task status is %s (expected %s)", task.Status, evergreen.TaskSucceeded)
		}
		time.Sleep(10 * time.Second)
		grip.Infof("checking for task %s (%d/30)", builds[0].Tasks[0], i+1)
		r, err := http.NewRequest("GET", urlPrefix+uiPort+"/rest/v2/tasks/"+builds[0].Tasks[0], nil)
		if err != nil {
			return errors.Wrap(err, "failed to make request")
		}
		r.Header.Add(userHeader, c.UserName)
		r.Header.Add(keyHeader, c.UserKey)
		resp, err := c.client.Do(r)
		if err != nil {
			return errors.Wrap(err, "error getting task data")
		}
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			err = errors.Wrap(err, "error reading response body")
			grip.Error(err)
			return err
		}
		resp.Body.Close()
		err = json.Unmarshal(body, &task)
		if err != nil {
			return errors.Wrap(err, "error unmarshaling json")
		}

		if task.Status != evergreen.TaskSucceeded {
			grip.Infof("found task is status %s", task.Status)
			task = apimodels.APITask{}
			continue
		}
		break
	}
	grip.Infof("checking for log %s", task.Logs["task_log"])
	resp, err := c.client.Get(task.Logs["task_log"] + "&text=true")
	if err != nil {
		return errors.Wrap(err, "error getting log data")
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		err = errors.Wrap(err, "error reading response body")
		return err
	}
	page := string(body)
	if strings.Contains(page, "Task completed - SUCCESS") {
		grip.Infof("Found task completed message in log:\n%s", page)
	} else {
		grip.Errorf("did not find task completed message in log:\n%s", page)
		return errors.New("did not find task completed message in log")
	}

	grip.Info("Successfully checked task by commit")
	return nil
}
