package expansions_test

import (
	"testing"

	"github.com/evergreen-ci/evergreen/agent/comm"
	"github.com/evergreen-ci/evergreen/command"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	modelutil "github.com/evergreen-ci/evergreen/model/testutil"
	. "github.com/evergreen-ci/evergreen/plugin/builtin/expansions"
	"github.com/evergreen-ci/evergreen/plugin/plugintest"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

func TestExpansionsPlugin(t *testing.T) {
	Convey("Should be able to update expansions", t, func() {
		updateCommand := UpdateCommand{
			Updates: []PutCommandParams{
				{
					Key:   "base",
					Value: "eggs",
				},
				{
					Key:    "topping",
					Concat: ",sausage",
				},
			},
		}

		expansions := command.Expansions{}
		expansions.Put("base", "not eggs")
		expansions.Put("topping", "bacon")

		taskConfig := model.TaskConfig{
			Expansions: &expansions,
		}

		So(updateCommand.ExecuteUpdates(&taskConfig), ShouldBeNil)

		So(expansions.Get("base"), ShouldEqual, "eggs")
		So(expansions.Get("topping"), ShouldEqual, "bacon,sausage")
	})

}

func TestExpansionsPluginWExecution(t *testing.T) {
	stopper := make(chan bool)
	defer close(stopper)

	testConfig := testutil.TestConfig()
	server, err := service.CreateTestServer(testConfig, nil)
	testutil.HandleTestingErr(err, t, "Couldn't set up testing server")
	defer server.Close()

	httpCom := plugintest.TestAgentCommunicator(&modelutil.TestModelData{}, server.URL)
	jsonCom := &comm.TaskJSONCommunicator{"shell", httpCom}

	conf := &model.TaskConfig{Expansions: &command.Expansions{}, Task: &task.Task{}, Project: &model.Project{}}

	Convey("When running Update commands", t, func() {
		Convey("if there is no expansion, the file name is not changed", func() {
			So(conf.Expansions, ShouldResemble, &command.Expansions{})
			cmd := &UpdateCommand{YamlFile: "foo"}
			So(cmd.Execute(&plugintest.MockLogger{}, jsonCom, conf, stopper), ShouldNotBeNil)
			So(cmd.YamlFile, ShouldEqual, "foo")
		})

		Convey("With an Expansion, the file name is expanded", func() {
			conf.Expansions = command.NewExpansions(map[string]string{"foo": "bar"})
			cmd := &UpdateCommand{YamlFile: "${foo}"}
			So(cmd.Execute(&plugintest.MockLogger{}, jsonCom, conf, stopper), ShouldNotBeNil)
			So(cmd.YamlFile, ShouldEqual, "bar")
		})
	})
}
