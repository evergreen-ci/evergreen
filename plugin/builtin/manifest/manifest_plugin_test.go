package manifest

import (
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/comm"
	agentutil "github.com/evergreen-ci/evergreen/agent/testutil"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/plugin/builtin/git"
	"github.com/evergreen-ci/evergreen/plugin/plugintest"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip/slogger"
	. "github.com/smartystreets/goconvey/convey"
)

func reset(t *testing.T) {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testutil.TestConfig()))
	testutil.HandleTestingErr(
		db.ClearCollections(manifest.Collection), t,
		"error clearing test collections")
}

func TestManifest(t *testing.T) {
	reset(t)
	Convey("With a pre-existing manifest for a revision existing", t, func() {
		m := manifest.Manifest{
			Id:          "abc123",
			ProjectName: "mci_test",
			Modules:     map[string]*manifest.Module{},
		}
		m.Modules["sample"] = &manifest.Module{
			Branch:   "master",
			Revision: "xyz345",
			Repo:     "repo",
			Owner:    "sr527",
			URL:      "randomurl.com",
		}

		dup, err := m.TryInsert()
		So(dup, ShouldBeFalse)
		So(err, ShouldBeNil)

		Convey("insertion of another manifest should give a duplicate error", func() {
			badManifest := manifest.Manifest{
				Id:          "abc123",
				ProjectName: "this_shouldn't_insert",
				Modules:     map[string]*manifest.Module{},
			}
			dup, err = badManifest.TryInsert()
			So(dup, ShouldBeTrue)
			So(err, ShouldBeNil)
		})

	})

}

// ManifestFetchCmd integration tests

func TestManifestLoad(t *testing.T) {
	reset(t)
	testConfig := testutil.TestConfig()

	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))
	testutil.ConfigureIntegrationTest(t, testConfig, "TestManifestFetch")
	Convey("With a SimpleRegistry and test project file", t, func() {

		registry := plugin.NewSimpleRegistry()
		manifestPlugin := &ManifestPlugin{}
		testutil.HandleTestingErr(registry.Register(manifestPlugin), t, "failed to register manifest plugin")

		gitPlugin := &git.GitPlugin{}
		testutil.HandleTestingErr(registry.Register(gitPlugin), t, "failed to register git plugin")

		server, err := service.CreateTestServer(testConfig, nil, plugin.APIPlugins, false)
		testutil.HandleTestingErr(err, t, "Couldn't set up testing server")
		defer server.Close()

		taskConfig, err := plugintest.CreateTestConfig(filepath.Join(testutil.GetDirectoryOfFile(),
			"testdata", "mongodb-mongo-master.yml"), t)
		testutil.HandleTestingErr(err, t, "Couldnt get task config from config file")

		logger := agentutil.NewTestLogger(slogger.StdOutAppender())

		httpCom := plugintest.TestAgentCommunicator("mocktaskid", "mocktasksecret", server.URL)

		Convey("the manifest load command should execute successfully", func() {
			for _, task := range taskConfig.Project.Tasks {
				So(len(task.Commands), ShouldNotEqual, 0)
				for _, command := range task.Commands {
					pluginCmds, err := registry.GetCommands(command, taskConfig.Project.Functions)
					testutil.HandleTestingErr(err, t, "Couldn't get plugin command: %v")
					So(pluginCmds, ShouldNotBeNil)
					So(err, ShouldBeNil)
					pluginCom := &comm.TaskJSONCommunicator{manifestPlugin.Name(),
						httpCom}
					err = pluginCmds[0].Execute(logger, pluginCom, taskConfig,
						make(chan bool))
					So(err, ShouldBeNil)
				}

			}
			Convey("the manifest should be inserted properly into the database", func() {
				currentManifest, err := manifest.FindOne(manifest.ById(taskConfig.Task.Version))
				So(err, ShouldBeNil)
				So(currentManifest, ShouldNotBeEmpty)
				So(currentManifest.ProjectName, ShouldEqual, taskConfig.ProjectRef.Identifier)
				So(currentManifest.Modules, ShouldNotBeNil)
				So(len(currentManifest.Modules), ShouldEqual, 1)
				for key := range currentManifest.Modules {
					So(key, ShouldEqual, "sample")
				}
				So(taskConfig.Expansions.Get("sample_rev"), ShouldEqual, "3c7bfeb82d492dc453e7431be664539c35b5db4b")
			})
			Convey("with a manifest already in the database the manifest should not create a new manifest", func() {
				for _, task := range taskConfig.Project.Tasks {
					So(len(task.Commands), ShouldNotEqual, 0)
					for _, command := range task.Commands {
						pluginCmds, err := registry.GetCommands(command, taskConfig.Project.Functions)
						testutil.HandleTestingErr(err, t, "Couldn't get plugin command: %v")
						So(pluginCmds, ShouldNotBeNil)
						So(err, ShouldBeNil)
						pluginCom := &comm.TaskJSONCommunicator{manifestPlugin.Name(),
							httpCom}
						err = pluginCmds[0].Execute(logger, pluginCom, taskConfig,
							make(chan bool))
						So(err, ShouldBeNil)
					}

				}
				Convey("the manifest should be inserted properly into the database", func() {
					currentManifest, err := manifest.FindOne(manifest.ById(taskConfig.Task.Version))
					So(err, ShouldBeNil)
					So(currentManifest, ShouldNotBeEmpty)
					So(currentManifest.ProjectName, ShouldEqual, taskConfig.ProjectRef.Identifier)
					So(currentManifest.Modules, ShouldNotBeNil)
					So(len(currentManifest.Modules), ShouldEqual, 1)
					for key := range currentManifest.Modules {
						So(key, ShouldEqual, "sample")
					}
					So(currentManifest.Modules["sample"].Repo, ShouldEqual, "sample")
					So(taskConfig.Expansions.Get("sample_rev"), ShouldEqual, "3c7bfeb82d492dc453e7431be664539c35b5db4b")
					So(currentManifest.Id, ShouldEqual, taskConfig.Task.Version)
				})
			})

		})
	})
}
