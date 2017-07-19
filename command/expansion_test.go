package command

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestExpansionsPlugin(t *testing.T) {
	ctx := context.Background()

	Convey("Should be able to update expansions", t, func() {
		updateCommand := update{
			Updates: []updateParams{
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

		expansions := util.Expansions{}
		expansions.Put("base", "not eggs")
		expansions.Put("topping", "bacon")

		taskConfig := model.TaskConfig{
			Expansions: &expansions,
		}

		So(updateCommand.ExecuteUpdates(ctx, &taskConfig), ShouldBeNil)

		So(expansions.Get("base"), ShouldEqual, "eggs")
		So(expansions.Get("topping"), ShouldEqual, "bacon,sausage")
	})

}

func TestExpansionsPluginWExecution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	comm := client.NewMock("http://localhost.com")
	conf := &model.TaskConfig{Expansions: &util.Expansions{}, Task: &task.Task{}, Project: &model.Project{}}
	logger := comm.GetLoggerProducer(client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret})

	Convey("When running Update commands", t, func() {
		Convey("if there is no expansion, the file name is not changed", func() {
			So(conf.Expansions, ShouldResemble, &util.Expansions{})
			cmd := &update{YamlFile: "foo"}
			So(cmd.Execute(ctx, comm, logger, conf), ShouldNotBeNil)
			So(cmd.YamlFile, ShouldEqual, "foo")
		})

		Convey("With an Expansion, the file name is expanded", func() {
			conf.Expansions = util.NewExpansions(map[string]string{"foo": "bar"})
			cmd := &update{YamlFile: "${foo}"}
			So(cmd.Execute(ctx, comm, logger, conf), ShouldNotBeNil)
			So(cmd.YamlFile, ShouldEqual, "bar")
		})
	})
}
