package attach_test

import (
	"10gen.com/mci"
	"10gen.com/mci/agent"
	"10gen.com/mci/apiserver"
	"10gen.com/mci/db"
	"10gen.com/mci/model"
	"10gen.com/mci/plugin"
	. "10gen.com/mci/plugin/builtin/attach"
	"10gen.com/mci/plugin/testutil"
	"10gen.com/mci/util"
	"github.com/10gen-labs/slogger/v1"
	. "github.com/smartystreets/goconvey/convey"
	"net/http"
	"testing"
)

func reset(t *testing.T) {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(mci.TestConfig()))
	util.HandleTestingErr(
		db.ClearCollections(model.TasksCollection, model.ArtifactFilesCollection), t,
		"error clearing test collections")
}

func TestAttachFilesApi(t *testing.T) {
	Convey("With a running api server and installed api hook", t, func() {
		reset(t)
		taskConfig, _ := testutil.CreateTestConfig("testdata/plugin_attach_files.yml", t)
		registry := plugin.NewSimplePluginRegistry()
		attachPlugin := &AttachPlugin{}
		err := registry.Register(attachPlugin)
		util.HandleTestingErr(err, t, "Couldn't register patch plugin")
		url, server, err := apiserver.CreateTestServer(mci.TestConfig(), nil, false)
		util.HandleTestingErr(err, t, "Couldn't set up testing server")
		server.InstallPlugin(attachPlugin)
		sliceAppender := &mci.SliceAppender{[]*slogger.Log{}}
		logger := agent.NewTestAgentLogger(sliceAppender)

		testTask := model.Task{Id: "test1", DisplayName: "TASK!!!", BuildId: "build1"}
		util.HandleTestingErr(testTask.Insert(), t, "couldn't insert test task")
		taskConfig.Task = &testTask

		httpCom := testutil.TestAgentCommunicator(testTask.Id, testTask.Secret, url)
		pluginCom := &agent.TaskJSONCommunicator{attachPlugin.Name(), httpCom}

		Convey("using a well-formed api call", func() {
			testCommand := AttachTaskFilesCommand{
				model.ArtifactFileParams{
					"upload":   "gopher://mci.equipment",
					"coverage": "http://www.blankets.com",
				},
			}
			err := testCommand.SendTaskFiles(taskConfig, logger, pluginCom)
			So(err, ShouldBeNil)

			Convey("the given values should be written to the db", func() {
				entry, err := model.FindOneArtifactFileEntryByTask(testTask.Id)
				So(err, ShouldBeNil)
				So(entry, ShouldNotBeNil)
				So(entry.TaskId, ShouldEqual, testTask.Id)
				So(entry.TaskDisplayName, ShouldEqual, testTask.DisplayName)
				So(entry.BuildId, ShouldEqual, testTask.BuildId)
				So(len(entry.Files), ShouldEqual, 2)
			})

			Convey("with a second api call", func() {
				testCommand := AttachTaskFilesCommand{
					model.ArtifactFileParams{
						"3x5":      "15",
						"$b.o.o.l": "{\"json\":false}",
						"coverage": "http://tumblr.com/tagged/tarp",
					},
				}
				err := testCommand.SendTaskFiles(taskConfig, logger, pluginCom)
				So(err, ShouldBeNil)
				entry, err := model.FindOneArtifactFileEntryByTask(testTask.Id)
				So(err, ShouldBeNil)
				So(entry, ShouldNotBeNil)

				Convey("new values should be added", func() {
					Convey("and old values should still remain", func() {
						So(len(entry.Files), ShouldEqual, 5)
					})
				})
			})
		})

		Convey("but the following malformed calls should fail:", func() {
			Convey("- calls with garbage content", func() {
				resp, err := pluginCom.TaskPostJSON(
					AttachTaskFilesAPIEndpoint,
					"I am not a proper post request for this endpoint",
				)
				So(err, ShouldBeNil)
				So(resp, ShouldNotBeNil)
				So(resp.StatusCode, ShouldEqual, http.StatusBadRequest)
			})

			Convey("- calls with nested subdocs", func() {
				resp, err := pluginCom.TaskPostJSON(
					AttachTaskFilesAPIEndpoint,
					map[string]interface{}{
						"cool": map[string]interface{}{
							"this_is": "a",
							"broken":  "test",
						},
					})
				So(err, ShouldBeNil)
				So(resp, ShouldNotBeNil)
				So(resp.StatusCode, ShouldEqual, http.StatusBadRequest)
			})
		})
	})
}

func TestAttachTaskFilesPlugin(t *testing.T) {
	Convey("With attach plugin installed into plugin registry", t, func() {
		registry := plugin.NewSimplePluginRegistry()
		attachPlugin := &AttachPlugin{}
		err := registry.Register(attachPlugin)
		util.HandleTestingErr(err, t, "Couldn't register plugin %v")

		url, server, err := apiserver.CreateTestServer(mci.TestConfig(), nil, false)
		util.HandleTestingErr(err, t, "Couldn't set up testing server")
		server.InstallPlugin(attachPlugin)
		httpCom := testutil.TestAgentCommunicator("testTaskId", "testTaskSecret", url)

		sliceAppender := &mci.SliceAppender{[]*slogger.Log{}}
		logger := agent.NewTestAgentLogger(sliceAppender)

		Convey("all commands in test project should execute successfully", func() {
			taskConfig, _ := testutil.CreateTestConfig("testdata/plugin_attach_files.yml", t)
			_, _, err = testutil.SetupAPITestData("testTask", true, t)
			util.HandleTestingErr(err, t, "Couldn't set up test documents")

			for _, task := range taskConfig.Project.Tasks {
				So(len(task.Commands), ShouldNotEqual, 0)
				for _, command := range task.Commands {
					pluginCmd, plugin, err := registry.GetCommand(command,
						taskConfig.Project.Functions)
					util.HandleTestingErr(err, t, "Couldn't get plugin command: %v")
					So(plugin, ShouldNotBeNil)
					So(pluginCmd, ShouldNotBeNil)
					So(err, ShouldBeNil)
					pluginCom := &agent.TaskJSONCommunicator{plugin.Name(), httpCom}
					err = pluginCmd.Execute(logger, pluginCom, taskConfig, make(chan bool))
					So(err, ShouldBeNil)
				}
			}

			Convey("and these file entry fields should exist in the db:", func() {
				entry, err := model.FindOneArtifactFileEntryByTask("testTaskId")
				So(err, ShouldBeNil)
				So(entry, ShouldNotBeNil)
				So(entry.TaskDisplayName, ShouldEqual, "testTask")
				So(len(entry.Files), ShouldEqual, 5)

				var regular model.ArtifactFile
				var expansion model.ArtifactFile
				var overwritten model.ArtifactFile

				for _, file := range entry.Files {
					switch file.Name {
					case "file1":
						expansion = file
					case "file2":
						overwritten = file
					case "file3":
						regular = file
					}
				}
				Convey("- regular link", func() {
					So(regular, ShouldResemble,
						model.ArtifactFile{"file3", "http://kyle.diamonds"})
				})

				Convey("- link with expansion", func() {
					So(expansion, ShouldResemble,
						model.ArtifactFile{"file1", "i am a FILE!"})
				})

				Convey("- link that is overwritten", func() {
					So(overwritten, ShouldResemble,
						model.ArtifactFile{"file2", "replaced!"})
				})
			})
		})
	})
}
