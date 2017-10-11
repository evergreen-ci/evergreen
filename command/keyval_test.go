package command

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	modelutil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testutil.TestConfig()))
}

func TestIncKey(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("With keyval plugin installed", t, func() {
		err := db.Clear(model.KeyValCollection)
		testutil.HandleTestingErr(err, t, "Couldn't clear test collection: %s", model.KeyValCollection)
		testutil.HandleTestingErr(err, t, "Couldn't register keyval plugin")

		testConfig := testutil.TestConfig()
		configPath := filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "plugin_keyval.yml")

		comm := client.NewMock("http://localhost.com")

		modelData, err := modelutil.SetupAPITestData(testConfig, "test", "rhel55", configPath, modelutil.NoPatch)
		testutil.HandleTestingErr(err, t, "couldn't create test task")

		Convey("Inc command should increment a key successfully", func() {
			conf := modelData.TaskConfig
			logger := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret})
			for _, task := range conf.Project.Tasks {
				So(len(task.Commands), ShouldNotEqual, 0)
				for _, command := range task.Commands {
					pluginCmds, err := Render(command, nil)
					testutil.HandleTestingErr(err, t, "Couldn't get plugin command: %v")
					So(pluginCmds, ShouldNotBeNil)
					So(err, ShouldBeNil)
					for _, cmd := range pluginCmds {
						err = cmd.Execute(ctx, comm, logger, conf)
						So(err, ShouldBeNil)
					}
				}
				So(conf.Expansions.Get("testkey"), ShouldEqual, "2")
				So(conf.Expansions.Get("testkey_x"), ShouldEqual, "1")
			}
		})
	})
}
