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

	// uiPort is the local port the UI will listen on.
	apiPort = "8080"

	// urlPrefix is the localhost prefix for accessing local Evergreen.
	urlPrefix = "http://localhost:"

	userHeader = "Auth-Username"
	keyHeader  = "Api-Key"
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

// APIBuild represents part of a build from the REST API
type APIBuild struct {
	Tasks []string `json:"tasks"`
}

// APITask represents part of a task from the REST API
type APITask struct {
	Status string            `json:"status"`
	Logs   map[string]string `json:"logs"`
}

func (c *SmokeTestEndpointCommand) checkTaskByCommit() error {
	var builds []APIBuild
	var build APIBuild
	for i := 0; i <= 300; i++ {
		// get task id
		if i == 300 {
			return errors.New("error getting builds for version")
		}
		time.Sleep(time.Second)
		grip.Infof("checking for a build of %s (%d/300)", c.Commit, i+1)
		resp, err := c.client.Get(urlPrefix + uiPort + "/rest/v2/versions/evergreen_" + c.Commit + "/builds")
		if err != nil {
			grip.Info(err)
			continue
		}
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			err = errors.Wrap(err, "error reading response body")
			grip.Error(err)
			return err
		}
		err = json.Unmarshal(body, &builds)
		if err != nil {
			err = json.Unmarshal(body, &build)
			if err != nil {
				return errors.Wrap(err, "error unmarshaling json")
			}
		}
		if len(builds) == 0 {
			builds = []APIBuild{build}
		}
		if len(builds[0].Tasks) == 0 {
			grip.Info("no tasks found")
			continue
		}
		break
	}

	var task APITask
	for i := 0; i <= 300; i++ {
		// check task
		if i == 300 {
			return errors.Errorf("task status is %s (expected %s)", task.Status, evergreen.TaskSucceeded)
		}
		time.Sleep(time.Second)
		grip.Infof("checking for task %s (%d/300)", builds[0].Tasks[0], i+1)
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
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			err = errors.Wrap(err, "error reading response body")
			grip.Error(err)
			return err
		}
		err = json.Unmarshal(body, &task)
		if err != nil {
			return errors.Wrap(err, "error unmarshaling json")
		}

		if task.Status != evergreen.TaskSucceeded {
			grip.Infof("found task is status %s", task.Status)
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
		grip.Error(err)
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
