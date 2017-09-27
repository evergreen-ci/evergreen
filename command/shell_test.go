package command

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestShellExecuteCommand(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	comm := client.NewMock("http://localhost.com")
	conf := &model.TaskConfig{Expansions: &util.Expansions{}, Task: &task.Task{}, Project: &model.Project{}}
	logger := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret})

	Convey("With a shell command", t, func() {

		Convey("if unset, default is determined by local command", func() {
			cmd := &shellExec{WorkingDir: testutil.GetDirectoryOfFile()}
			So(cmd.Execute(ctx, comm, logger, conf), ShouldBeNil)
			So(cmd.Shell, ShouldEqual, "")
		})

		shells := []string{"bash", "python", "sh"}

		if runtime.GOOS != "windows" {
			shells = append(shells, "/bin/sh", "/bin/bash", "/usr/bin/python")
		}

		for _, sh := range shells {
			Convey(fmt.Sprintf("when set, %s is not overwritten during execution", sh), func() {
				cmd := &shellExec{Shell: sh, WorkingDir: testutil.GetDirectoryOfFile()}
				So(cmd.Execute(ctx, comm, logger, conf), ShouldBeNil)
				So(cmd.Shell, ShouldEqual, sh)
			})
		}

		Convey("command should error if working directory is unset", func() {
			cmd := &shellExec{}
			So(cmd.Execute(ctx, comm, logger, conf), ShouldNotBeNil)
		})

		Convey("command should error if working directory does not exist", func() {
			path := "foo/bar/baz"
			_, err := os.Stat(path)
			So(os.IsNotExist(err), ShouldBeTrue)
			_, err = os.Stat(filepath.Join(conf.WorkDir))
			So(os.IsNotExist(err), ShouldBeTrue)

			cmd := &shellExec{WorkingDir: path}
			So(cmd.Execute(ctx, comm, logger, conf), ShouldNotBeNil)

		})

	})
}
