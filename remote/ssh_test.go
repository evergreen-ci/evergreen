package remote

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/command"
	. "github.com/smartystreets/goconvey/convey"
)

func TestSSHCommand(t *testing.T) {
	t.Skipf("the fixtures for remote ssh are not properly configured/conceived.")

	Convey("When sshing", t, func() {

		Convey("a simple command should execute successfully and provide the"+
			" correct output", func() {

			cmd := &SSHCommand{
				Command: "echo 'hi stdout'; echo 'hi stderr' 1>&2",
				Host:    command.TestRemote + ":22",
				User:    command.TestRemoteUser,
				Keyfile: command.TestRemoteKey,
				Timeout: 1 * time.Second,
			}

			output, err := cmd.Run()
			So(err, ShouldBeNil)
			So(output, ShouldResemble, []byte("hi stdout\r\nhi stderr\r\n"))

		})

		Convey("if a command times out, it should be killed appropriately", func() {

			cmd := &SSHCommand{
				Command: "sleep 10",
				Host:    command.TestRemote + ":22",
				User:    command.TestRemoteUser,
				Keyfile: command.TestRemoteKey,
				Timeout: 500 * time.Millisecond,
			}

			output, err := cmd.Run()
			So(err, ShouldEqual, ErrCmdTimedOut)
			So(output, ShouldEqual, nil)

		})

	})

}
