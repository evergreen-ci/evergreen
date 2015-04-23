package expansions_test

import (
	"10gen.com/mci/command"
	"10gen.com/mci/model"
	. "10gen.com/mci/plugin/builtin/expansions"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestExpansionsPlugin(t *testing.T) {
	Convey("Should be able to update expansions", t, func() {
		updateCommand := UpdateCommand{
			Updates: []PutCommandParams{
				PutCommandParams{
					Key:   "base",
					Value: "eggs",
				},
				PutCommandParams{
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

		updateCommand.ExecuteUpdates(&taskConfig)

		So(expansions.Get("base"), ShouldEqual, "eggs")
		So(expansions.Get("topping"), ShouldEqual, "bacon,sausage")
	})
}
