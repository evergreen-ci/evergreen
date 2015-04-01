package plugin_test

import (
	"10gen.com/mci"
	"10gen.com/mci/agent"
	"10gen.com/mci/apiserver"
	"10gen.com/mci/db"
	"10gen.com/mci/model"
	"10gen.com/mci/model/distro"
	"10gen.com/mci/model/host"
	"10gen.com/mci/model/version"
	"10gen.com/mci/plugin"
	"10gen.com/mci/plugin/builtin/expansions"
	"10gen.com/mci/plugin/builtin/shell"
	_ "10gen.com/mci/plugin/config"
	"10gen.com/mci/util"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/gorilla/mux"
	"github.com/mitchellh/mapstructure"
	. "github.com/smartystreets/goconvey/convey"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"labix.org/v2/mgo/bson"
	"net/http"
	"strconv"
	"testing"
)

var Port = 8181

type MockPlugin struct {
}

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(mci.TestConfig()))
}

func MockPluginEcho(w http.ResponseWriter, request *http.Request) {
	arg1 := mux.Vars(request)["param1"]
	arg2, err := strconv.Atoi(mux.Vars(request)["param2"])
	if err != nil {
		http.Error(w, "bad val for param2", http.StatusBadRequest)
		return
	}

	task := plugin.GetTask(request)
	if task != nil {
		//task should have been populated for us, by the API server
		plugin.WriteJSON(w, http.StatusOK, map[string]string{
			"echo": fmt.Sprintf("%v/%v/%v", arg1, arg2, task.Id),
		})
		return
	}
	http.Error(w, "couldn't get task from context", http.StatusInternalServerError)
}

func (self *MockPlugin) Configure(conf map[string]interface{}) error {
	return nil
}

func (self *MockPlugin) GetAPIHandler() http.Handler {
	r := mux.NewRouter()
	r.Path("/blah/{param1}/{param2}").Methods("GET").HandlerFunc(MockPluginEcho)
	return r
}

func (self *MockPlugin) GetUIHandler() http.Handler {
	return nil
}

func (self *MockPlugin) GetPanelConfig() (*plugin.PanelConfig, error) {
	return nil, nil
}

func (self *MockPlugin) Name() string {
	return "mock"
}

func (self *MockPlugin) NewCommand(commandName string) (plugin.Command, error) {
	if commandName != "foo" {
		return nil, &plugin.ErrUnknownCommand{commandName}
	}
	return &MockCommand{}, nil
}

type MockCommand struct {
	Param1 string
	Param2 int64
}

func (self *MockCommand) Name() string {
	return "mock"
}

func (self *MockCommand) Plugin() string {
	return "mock"
}

func (self *MockCommand) ParseParams(params map[string]interface{}) error {
	err := mapstructure.Decode(params, self)
	if err != nil {
		return err
	}
	if self.Param1 == "" {
		return fmt.Errorf("Param1 must be a non-blank string.")
	}
	if self.Param2 == 0 {
		return fmt.Errorf("Param2 must be a non-zero integer.")
	}
	return nil
}

func (self *MockCommand) Execute(logger plugin.Logger,
	pluginCom plugin.PluginCommunicator, conf *model.TaskConfig, stop chan bool) error {
	resp, err := pluginCom.TaskGetJSON(fmt.Sprintf("blah/%s/%d", self.Param1, self.Param2))
	if resp != nil {
		defer resp.Body.Close()
	}

	if resp == nil {
		return fmt.Errorf("Received nil HTTP response from api server")
	}

	jsonReply := map[string]string{}
	err = util.ReadJSONInto(resp.Body, &jsonReply)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Got bad status code from API response: %v, body: %v", resp.StatusCode, jsonReply)
	}

	expectedEchoReply := fmt.Sprintf("%v/%v/%v", self.Param1, self.Param2, conf.Task.Id)
	if jsonReply["echo"] != expectedEchoReply {
		return fmt.Errorf("Wrong echo reply! Wanted %v, got %v", expectedEchoReply, jsonReply["echo"])
	}
	return nil
}

func TestRegistry(t *testing.T) {
	Convey("With a SimpleRegistry", t, func() {
		Convey("Registering a plugin twice should return err", func() {
			registry := plugin.NewSimpleRegistry()
			err := registry.Register(&MockPlugin{})
			util.HandleTestingErr(err, t, "Couldn't register plugin")
			err = registry.Register(&shell.ShellPlugin{})
			util.HandleTestingErr(err, t, "Couldn't register plugin")
			err = registry.Register(&expansions.ExpansionsPlugin{})
			util.HandleTestingErr(err, t, "Couldn't register plugin")
		})
		Convey("with a project file containing references to a valid plugin", func() {
			registry := plugin.NewSimpleRegistry()
			registry.Register(&MockPlugin{})
			registry.Register(&shell.ShellPlugin{})
			registry.Register(&expansions.ExpansionsPlugin{})
			data, err := ioutil.ReadFile("testdata/plugin_project.yml")
			util.HandleTestingErr(err, t, "failed to load test yaml file")
			project := &model.Project{}
			err = yaml.Unmarshal(data, project)
			Convey("all commands in project file should load parse successfully", func() {
				for _, task := range project.Tasks {
					for _, command := range task.Commands {
						pluginCmds, err := registry.GetCommands(command, project.Functions)
						util.HandleTestingErr(err, t, "Got error getting plugin commands: %v")
						So(pluginCmds, ShouldNotBeNil)
						So(err, ShouldBeNil)
					}
				}
			})
		})
	})
}

func TestPluginFunctions(t *testing.T) {
	Convey("With a SimpleRegistry", t, func() {
		Convey("with a project file containing functions", func() {
			registry := plugin.NewSimpleRegistry()
			err := registry.Register(&shell.ShellPlugin{})
			util.HandleTestingErr(err, t, "Couldn't register plugin")
			err = registry.Register(&expansions.ExpansionsPlugin{})
			util.HandleTestingErr(err, t, "Couldn't register plugin")

			testServer, err := apiserver.CreateTestServer(mci.TestConfig(), nil, plugin.Published, false)
			util.HandleTestingErr(err, t, "Couldn't set up testing server")

			taskConfig, err := createTestConfig("testdata/plugin_project_functions.yml", t)
			util.HandleTestingErr(err, t, "failed to create test config: %v", err)

			Convey("all commands in project file should parse successfully", func() {
				for _, task := range taskConfig.Project.Tasks {
					for _, command := range task.Commands {
						pluginCmd, err := registry.GetCommands(command, taskConfig.Project.Functions)
						util.HandleTestingErr(err, t, "Got error getting plugin command: %v")
						So(pluginCmd, ShouldNotBeNil)
						So(err, ShouldBeNil)
					}
				}
			})

			httpCom, err := agent.NewHTTPAgentCommunicator(testServer.URL, "mocktaskid", "mocktasksecret", "")
			So(err, ShouldBeNil)
			So(httpCom, ShouldNotBeNil)

			Convey("all commands in test project should execute successfully", func() {
				sliceAppender := &mci.SliceAppender{[]*slogger.Log{}}
				logger := agent.NewTestAgentLogger(sliceAppender)
				for _, task := range taskConfig.Project.Tasks {
					So(len(task.Commands), ShouldNotEqual, 0)
					for _, command := range task.Commands {
						pluginCmds, err := registry.GetCommands(command, taskConfig.Project.Functions)
						util.HandleTestingErr(err, t, "Couldn't get plugin command: %v")
						So(pluginCmds, ShouldNotBeNil)
						So(err, ShouldBeNil)
						So(len(pluginCmds), ShouldEqual, 1)
						cmd := pluginCmds[0]
						pluginCom := &agent.TaskJSONCommunicator{cmd.Plugin(), httpCom}
						err = cmd.Execute(logger, pluginCom, taskConfig, make(chan bool))
						So(err, ShouldBeNil)
					}
				}
			})
		})
	})
}

func TestPluginExecution(t *testing.T) {
	Convey("With a SimpleRegistry and test project file", t, func() {
		registry := plugin.NewSimpleRegistry()

		plugins := []plugin.Plugin{&MockPlugin{}, &expansions.ExpansionsPlugin{}, &shell.ShellPlugin{}}
		for _, p := range plugins {
			err := registry.Register(p)
			util.HandleTestingErr(err, t, "failed to register plugin")
		}

		testServer, err := apiserver.CreateTestServer(mci.TestConfig(), nil, plugins, false)
		util.HandleTestingErr(err, t, "Couldn't set up testing server")

		httpCom, err := agent.NewHTTPAgentCommunicator(testServer.URL, "mocktaskid", "mocktasksecret", "")
		So(err, ShouldBeNil)
		So(httpCom, ShouldNotBeNil)

		taskConfig, err := createTestConfig("testdata/plugin_project.yml", t)
		util.HandleTestingErr(err, t, "failed to create test config: %v", err)
		sliceAppender := &mci.SliceAppender{[]*slogger.Log{}}
		logger := agent.NewTestAgentLogger(sliceAppender)

		Convey("all commands in test project should execute successfully", func() {
			for _, task := range taskConfig.Project.Tasks {
				So(len(task.Commands), ShouldNotEqual, 0)
				for _, command := range task.Commands {
					pluginCmds, err := registry.GetCommands(command, taskConfig.Project.Functions)
					util.HandleTestingErr(err, t, "Couldn't get plugin command: %v")
					So(pluginCmds, ShouldNotBeNil)
					So(err, ShouldBeNil)
					for _, c := range pluginCmds {
						pluginCom := &agent.TaskJSONCommunicator{c.Plugin(), httpCom}
						err = c.Execute(logger, pluginCom, taskConfig, make(chan bool))
						So(err, ShouldBeNil)
					}
				}
			}
		})
	})
}

func createTestConfig(filename string, t *testing.T) (*model.TaskConfig, error) {
	clearDataMsg := "Failed to clear test data collection"
	util.HandleTestingErr(
		db.ClearCollections(
			model.TasksCollection, model.ProjectVarsCollection),
		t, clearDataMsg)

	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	project := &model.Project{}
	err = yaml.Unmarshal(data, project)
	if err != nil {
		return nil, err
	}

	workDir, err := ioutil.TempDir("", "plugintest_")
	if err != nil {
		return nil, err
	}

	testTask := &model.Task{
		Id:           "mocktaskid",
		BuildId:      "testBuildId",
		BuildVariant: "linux-64",
		Project:      "mongodb-mongo-master",
		DisplayName:  "test",
		HostId:       "testHost",
		Secret:       "mocktasksecret",
		Status:       mci.TaskDispatched,
		Revision:     "d0c52298b222f4973c48e9834a57966c448547de",
		Requester:    mci.RepotrackerVersionRequester,
	}
	util.HandleTestingErr(testTask.Insert(), t, "failed to insert task")

	projectVars := &model.ProjectVars{
		Id: "mongodb-mongo-master",
		Vars: map[string]string{
			"abc": "xyz",
			"123": "456",
		},
	}
	_, err = projectVars.Upsert()
	util.HandleTestingErr(err, t, "failed to upsert project vars")
	return model.NewTaskConfig(&distro.Distro{Name: "linux-64"}, project,
		testTask, workDir)
}

func setupAPITestData(taskDisplayName string, patch bool, t *testing.T) (*model.Task, *model.Build, error) {
	//ignore errs here because the ns might just not exist.
	clearDataMsg := "Failed to clear test data collection"

	util.HandleTestingErr(
		db.ClearCollections(
			model.TasksCollection, model.BuildsCollection, host.Collection,
			version.Collection, model.PatchCollection),
		t, clearDataMsg)

	testHost := &host.Host{
		Id:          "testHost",
		Host:        "testHost",
		RunningTask: "testTaskId",
		StartedBy:   mci.MCIUser,
	}
	util.HandleTestingErr(testHost.Insert(), t, "failed to insert host")

	task := &model.Task{
		Id:           "testTaskId",
		BuildId:      "testBuildId",
		DistroId:     "rhel55",
		BuildVariant: "linux-64",
		Project:      "mongodb-mongo-master",
		DisplayName:  taskDisplayName,
		HostId:       "testHost",
		Secret:       "testTaskSecret",
		Status:       mci.TaskDispatched,
		Requester:    mci.RepotrackerVersionRequester,
	}

	if patch {
		task.Requester = mci.PatchVersionRequester
	}

	util.HandleTestingErr(task.Insert(), t, "failed to insert task")

	v := &version.Version{
		Id:       "testVersionId",
		BuildIds: []string{task.BuildId},
	}
	util.HandleTestingErr(v.Insert(), t, "failed to insert version %v")
	if patch {
		mainPatchContent, err := ioutil.ReadFile("testdata/test.patch")
		util.HandleTestingErr(err, t, "failed to read test patch file %v")
		modulePatchContent, err := ioutil.ReadFile("testdata/testmodule.patch")
		util.HandleTestingErr(err, t, "failed to read test module patch file %v")

		patch := &model.Patch{
			Status:  mci.PatchCreated,
			Version: v.Id,
			Patches: []model.ModulePatch{
				model.ModulePatch{
					ModuleName: "",
					Githash:    "d0c52298b222f4973c48e9834a57966c448547de",
					PatchSet:   model.PatchSet{Patch: string(mainPatchContent)},
				},
				model.ModulePatch{
					ModuleName: "enterprise",
					Githash:    "c2d7ce942a96d7dacd27c55b257e3f2774e04abf",
					PatchSet:   model.PatchSet{Patch: string(modulePatchContent)},
				},
			},
		}

		util.HandleTestingErr(patch.Insert(), t, "failed to insert version %v")

	}

	session, _, err := db.GetGlobalSessionFactory().GetSession()
	util.HandleTestingErr(err, t, "couldn't get db session!")

	//Remove any logs for our test task from previous runs.
	_, err = session.DB(model.TaskLogDB).C(model.TaskLogCollection).RemoveAll(bson.M{"t_id": task.Id})
	util.HandleTestingErr(err, t, "failed to remove logs")

	build := &model.Build{
		Id: "testBuildId",
		Tasks: []model.TaskCache{
			model.NewTaskCache(task.Id, task.DisplayName, true),
		},
		Version: "testVersionId",
	}

	util.HandleTestingErr(build.Insert(), t, "failed to insert build %v")
	return task, build, nil
}

func TestPluginSelfRegistration(t *testing.T) {
	Convey("Assuming the plugin collection has run its init functions", t, func() {
		So(len(plugin.Published), ShouldBeGreaterThan, 0)
		nameMap := map[string]uint{}
		// count all occurances of a plugin name
		for _, plugin := range plugin.Published {
			nameMap[plugin.Name()] = nameMap[plugin.Name()] + 1
		}

		Convey("no plugin should be present in Published more than once", func() {
			for _, count := range nameMap {
				So(count, ShouldEqual, 1)
			}
		})

		Convey("some known default plugins should be present in the list", func() {
			// These use strings instead of consts from the plugin
			// packages, so we can avoid importing those packages
			// and make sure the registration from plguin/config
			// is actually happening
			So(nameMap["attach"], ShouldEqual, 1)
			So(nameMap["s3"], ShouldEqual, 1)
			So(nameMap["s3Copy"], ShouldEqual, 1)
			So(nameMap["archive"], ShouldEqual, 1)
			So(nameMap["expansions"], ShouldEqual, 1)
			So(nameMap["git"], ShouldEqual, 1)
			So(nameMap["shell"], ShouldEqual, 1)
		})
	})
}
