package command

import (
	"context"
	"os"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

		taskConfig := internal.TaskConfig{
			Expansions: expansions,
		}

		So(updateCommand.ExecuteUpdates(ctx, &taskConfig), ShouldBeNil)
		So(taskConfig.DynamicExpansions, ShouldResemble, util.Expansions{"base": "eggs", "topping": "bacon,sausage"})
		So(expansions.Get("base"), ShouldEqual, "eggs")
		So(expansions.Get("topping"), ShouldEqual, "bacon,sausage")
	})

}

func TestExpansionsPluginWExecution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	comm := client.NewMock("http://localhost.com")
	conf := &internal.TaskConfig{Expansions: util.Expansions{}, Task: task.Task{}, Project: model.Project{}}
	logger, _ := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}, nil)

	Convey("When running Update commands", t, func() {
		Convey("if there is no expansion, the file name is not changed", func() {
			So(conf.Expansions, ShouldResemble, util.Expansions{})
			cmd := &update{YamlFile: "foo"}
			So(cmd.Execute(ctx, comm, logger, conf), ShouldNotBeNil)
			So(cmd.YamlFile, ShouldEqual, "foo")
		})

		Convey("With an Expansion, the file name is expanded", func() {
			conf.Expansions = *util.NewExpansions(map[string]string{"foo": "bar"})
			cmd := &update{YamlFile: "${foo}"}
			So(cmd.Execute(ctx, comm, logger, conf), ShouldNotBeNil)
			So(cmd.YamlFile, ShouldEqual, "bar")
		})
	})
}

func TestExpansionWriter(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	comm := client.NewMock("http://localhost.com")
	logger, err := comm.GetLoggerProducer(ctx, client.TaskData{ID: "id", Secret: "secret"}, nil)
	assert.NoError(err)
	tc := &internal.TaskConfig{
		Expansions: util.Expansions{
			"foo":                                "bar",
			"baz":                                "qux",
			"password":                           "hunter2",
			evergreen.GlobalGitHubTokenExpansion: "sample_token",
		},
		Redacted: map[string]bool{
			"password": true,
		},
	}
	f, err := os.CreateTemp("", t.Name())
	require.NoError(err)
	defer os.Remove(f.Name())

	writer := &expansionsWriter{File: f.Name()}
	err = writer.Execute(ctx, comm, logger, tc)
	assert.NoError(err)
	out, err := os.ReadFile(f.Name())
	assert.NoError(err)
	assert.Equal("baz: qux\nfoo: bar\n", string(out))

	writer = &expansionsWriter{File: f.Name(), Redacted: true}
	err = writer.Execute(ctx, comm, logger, tc)
	assert.NoError(err)
	out, err = os.ReadFile(f.Name())
	assert.NoError(err)
	assert.Equal("baz: qux\nfoo: bar\npassword: hunter2\n", string(out))
}
