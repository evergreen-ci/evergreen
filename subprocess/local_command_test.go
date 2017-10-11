package subprocess

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
)

func TestLocalCommands(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell command test doesn't make sense on windows")
	}
	ctx := context.Background()

	Convey("When running local commands", t, func() {

		Convey("the preparation step should replace expansions and forward"+
			" slashes in the command string", func() {

			command := &LocalCommand{
				CmdString: "one ${two} \\three${four|five}",
			}

			expansions := util.NewExpansions(map[string]string{
				"two": "TWO",
				"six": "SIX",
			})

			// run the preparation stage, and make sure the replacements are
			// correctly made
			So(command.PrepToRun(expansions), ShouldBeNil)
			So(command.CmdString, ShouldEqual, "one TWO \\threefive")

		})

		Convey("the preparation step should replace expansion in the working directory", func() {
			command := &LocalCommand{
				WorkingDirectory: "one ${two} ${other|three} four five ${six}",
			}

			expansions := util.NewExpansions(map[string]string{
				"two": "TWO",
				"six": "SIX",
			})

			So(command.PrepToRun(expansions), ShouldBeNil)
			So(command.WorkingDirectory, ShouldEqual, "one TWO three four five SIX")
		})

		Convey("the perpetration step should return an error if invalid expansions", func() {
			expansions := util.NewExpansions(map[string]string{"foo": "bar"})

			for _, cmd := range []*LocalCommand{
				{WorkingDirectory: "${foo|${bar}}"},
				{CmdString: "${foo${bar}}"},
			} {
				So(cmd.PrepToRun(expansions), ShouldNotBeNil)
			}

		})

		Convey("the preparation step should not replace strings without expansions", func() {
			var cmd *LocalCommand

			expansions := util.NewExpansions(map[string]string{"foo": "bar"})

			for _, input := range []string{"", "nothing", "this is empty", "foo"} {
				cmd = &LocalCommand{WorkingDirectory: input}
				So(cmd.PrepToRun(expansions), ShouldBeNil)
				So(cmd.WorkingDirectory, ShouldEqual, input)

				cmd = &LocalCommand{CmdString: input}
				So(cmd.PrepToRun(expansions), ShouldBeNil)
				So(cmd.CmdString, ShouldEqual, input)
			}
		})

		Convey("the specified environment should be used", func() {
			stdout := &CacheLastWritten{}

			command := &LocalCommand{
				CmdString: "echo $local_command_test",
				Stdout:    stdout,
				Stderr:    ioutil.Discard,
			}

			// get the current env
			command.Environment = os.Environ()

			// run the command - the environment variable should be empty
			So(command.Run(ctx), ShouldBeNil)
			So(string(stdout.LastWritten), ShouldEqual, "\n")

			// add the environment variable to the env
			command.Environment = append(command.Environment,
				"local_command_test=hello")

			// run the command again - the environment variable should be set
			// correctly
			So(command.Run(ctx), ShouldBeNil)
			So(string(stdout.LastWritten), ShouldEqual, "hello\n")

		})

		Convey("the specified working directory should be used", func() {

			stdout := &CacheLastWritten{}

			workingDir, err := filepath.Abs(evergreen.FindEvergreenHome())
			So(err, ShouldBeNil)

			command := &LocalCommand{
				CmdString:        "pwd",
				Stdout:           stdout,
				Stderr:           ioutil.Discard,
				WorkingDirectory: workingDir,
			}
			// run the command - the working directory should be as specified
			So(command.Run(ctx), ShouldBeNil)

			reportedPwd := string(stdout.LastWritten)
			reportedPwd = reportedPwd[:len(reportedPwd)-1]
			reportedPwd, err = filepath.EvalSymlinks(reportedPwd)
			So(err, ShouldBeNil)

			So(reportedPwd, ShouldEqual, workingDir)

		})

		Convey("the specified shell should be used", func() {
			for _, sh := range []string{"bash", "sh", "/bin/bash", "/bin/sh"} {
				stdout := &CacheLastWritten{}
				command := &LocalCommand{
					Shell:     sh,
					CmdString: "echo $0",
					Stdout:    stdout,
					Stderr:    ioutil.Discard,
				}

				So(command.Run(ctx), ShouldBeNil)
				So(string(stdout.LastWritten), ShouldEqual, sh+"\n")
			}
		})

		Convey("if not specified, sh should be used", func() {
			stdout := &CacheLastWritten{}
			command := &LocalCommand{
				CmdString: "echo $0",
				Stdout:    stdout,
				Stderr:    ioutil.Discard,
			}

			So(command.Run(ctx), ShouldBeNil)
			So(string(stdout.LastWritten), ShouldEqual, "sh\n")
		})

		Convey("when specified, local command can also use python", func() {
			stdout := &CacheLastWritten{}
			command := &LocalCommand{
				Shell:     "python",
				CmdString: "print('hello world')",
				Stdout:    stdout,
				Stderr:    ioutil.Discard,
			}

			So(command.Run(ctx), ShouldBeNil)
			So(string(stdout.LastWritten), ShouldEqual, "hello world\n")
		})

	})
}

func TestLocalScript(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell command test doesn't make sense on windows")
	}

	ctx := context.TODO()
	Convey("When running local commands in script mode", t, func() {

		Convey("A multi-line script should run all lines", func() {

			stdout := &CacheLastWritten{}

			workingDir, err := filepath.Abs(evergreen.FindEvergreenHome())
			So(err, ShouldBeNil)

			command := &LocalCommand{
				CmdString:        "set -v\necho 'hi'\necho 'foo'\necho `pwd`",
				ScriptMode:       true,
				Stdout:           stdout,
				Stderr:           ioutil.Discard,
				WorkingDirectory: workingDir,
			}

			// run the command - the working directory should be as specified
			So(command.Run(ctx), ShouldBeNil)

			reportedPwd := string(stdout.LastWritten)
			reportedPwd = reportedPwd[:len(reportedPwd)-1]
			reportedPwd, err = filepath.EvalSymlinks(reportedPwd)
			So(err, ShouldBeNil)
			So(reportedPwd, ShouldEqual, workingDir)
		})

	})
}
