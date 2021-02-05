package command

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
)

func TestDownstreamExpansions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	comm := client.NewMock("http://localhost.com")
	conf := &internal.TaskConfig{Expansions: &util.Expansions{}, Task: &task.Task{}, Project: &model.Project{}}
	logger, _ := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}, nil)
	cwd := testutil.GetDirectoryOfFile()

	Convey("When running setDownstream commands", t, func() {

		Convey("With an Expansion, the file name is expanded", func() {
			path := filepath.Join(cwd, "testdata", "git", "test_expansions.yml")
			conf.Expansions = util.NewExpansions(map[string]string{"foo": path})
			cmd := &setDownstream{YamlFile: "${foo}"}
			cmdExecute := cmd.Execute(ctx, comm, logger, conf)
			So(cmdExecute, ShouldBeNil)
			So(cmd.YamlFile, ShouldEqual, path)
		})

		Convey("The contents of the file are stored in the struct", func() {
			path := filepath.Join(cwd, "testdata", "git", "test_expansions.yml")
			cmd := &setDownstream{YamlFile: path}
			So(cmd.Execute(ctx, comm, logger, conf), ShouldBeNil)
			So(cmd.DownstreamParams[0].Key, ShouldEqual, "key_1")
			So(cmd.DownstreamParams[0].Value, ShouldEqual, "value_1")
			So(cmd.DownstreamParams[1].Key, ShouldEqual, "my_docker_image")
			So(cmd.DownstreamParams[1].Value, ShouldEqual, "my_image")

		})

	})
}
