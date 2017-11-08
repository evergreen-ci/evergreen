package perfdash

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model"
	. "github.com/smartystreets/goconvey/convey"
)

func TestGetVariantsWithCommand(t *testing.T) {
	Convey("Given a set of project tasks and commands", t, func() {

		jsonSendCommand := model.PluginCommandConf{
			Command:  "json.send",
			Params:   map[string]interface{}{"name": "dashboard"},
			Variants: []string{"exampleVar", "anotherOne"},
		}
		notJSONCommand := model.PluginCommandConf{
			Command: "something.something",
			Params:  map[string]interface{}{"name": "dashboard"},
		}
		notDashboardCommand := model.PluginCommandConf{
			Command: "json.send",
			Params:  map[string]interface{}{"name": "perf"},
		}

		jsonSendTask := model.ProjectTask{
			Name:     "test task",
			Commands: []model.PluginCommandConf{jsonSendCommand},
		}
		noJSONSendTask := model.ProjectTask{
			Name:     "nope",
			Commands: []model.PluginCommandConf{notJSONCommand},
		}

		notDashboardTask := model.ProjectTask{
			Name:     "shouldnt exist",
			Commands: []model.PluginCommandConf{notDashboardCommand},
		}

		singleCommand := model.YAMLCommandSet{SingleCommand: &jsonSendCommand}
		multiCommands := model.YAMLCommandSet{MultiCommand: []model.PluginCommandConf{jsonSendCommand, notDashboardCommand}}
		functions := map[string]*model.YAMLCommandSet{"single": &singleCommand, "multi": &multiCommands}
		functionCommandSingle := model.PluginCommandConf{Function: "single"}
		functionCommandMulti := model.PluginCommandConf{Function: "multi"}
		anotherTask := model.ProjectTask{
			Name:     "functionsTask",
			Commands: []model.PluginCommandConf{functionCommandSingle, functionCommandMulti},
		}

		// build variants
		bv1 := model.BuildVariant{
			Name: "bv1",
			Tasks: []model.BuildVariantTask{
				{Name: "test task"},
				{Name: "nope"},
			},
		}
		bv2 := model.BuildVariant{
			Name: "bv2",
			Tasks: []model.BuildVariantTask{
				{Name: "nope"},
			},
		}

		bv3 := model.BuildVariant{
			Name: "bv3",
			Tasks: []model.BuildVariantTask{
				{Name: "test task"},
			},
		}

		Convey("with a project that has two tasks and three build variants", func() {
			proj := model.Project{
				Identifier:    "sampleProject",
				Tasks:         []model.ProjectTask{jsonSendTask, noJSONSendTask, notDashboardTask},
				BuildVariants: []model.BuildVariant{bv1, bv2, bv3},
			}
			Convey("task cache should be created with one task in it", func() {
				taskCache := createTaskCacheForCommand("json.send", &proj)
				So(taskCache, ShouldNotBeEmpty)
				So(len(taskCache), ShouldEqual, 1)
				Convey("build variant map should be created properly", func() {
					vars := getVariantsWithCommand("json.send", &proj)
					So(vars, ShouldNotBeEmpty)
					So(vars["test task"], ShouldNotBeEmpty)
					So(len(vars["test task"]), ShouldEqual, 2)
				})
			})

		})
		Convey("with a project with functions in the tasks,", func() {
			// build variants
			bv := model.BuildVariant{
				Name: "bv1",
				Tasks: []model.BuildVariantTask{
					{Name: "functionsTask"},
					{Name: "nope"},
				},
			}
			proj := model.Project{
				Identifier:    "sampleProject",
				Tasks:         []model.ProjectTask{anotherTask},
				BuildVariants: []model.BuildVariant{bv},
				Functions:     functions,
			}
			Convey("task cache should be created with one task in it", func() {
				taskCache := createTaskCacheForCommand("json.send", &proj)
				So(taskCache, ShouldNotBeEmpty)
				So(len(taskCache), ShouldEqual, 1)
				_, ok := taskCache["functionsTask"]
				So(ok, ShouldBeTrue)
				Convey("build variant map should be created properly", func() {
					vars := getVariantsWithCommand("json.send", &proj)
					So(vars, ShouldNotBeEmpty)
					So(len(vars["functionsTask"]), ShouldNotBeEmpty)
				})
			})
		})
	})
}
