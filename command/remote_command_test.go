package command

import (
	"fmt"
	"io/ioutil"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRemoteCommand(t *testing.T) {
	t.Skip("skipping because local testing ssh configuration is not implemented")

	Convey("With a remote command", t, func() {

		Convey("failure and success should be detected", func() {
			failCmd := &RemoteCommand{
				CmdString:      "false",
				Stdout:         ioutil.Discard,
				Stderr:         ioutil.Discard,
				RemoteHostName: TestRemote,
				User:           TestRemoteUser,
				Options:        []string{"-i", TestRemoteKey},
			}
			So(failCmd.Run(), ShouldNotBeNil)

			trueCmd := &RemoteCommand{
				CmdString:      "true",
				Stdout:         ioutil.Discard,
				Stderr:         ioutil.Discard,
				RemoteHostName: TestRemote,
				User:           TestRemoteUser,
				Options:        []string{"-i", TestRemoteKey},
			}
			So(trueCmd.Run(), ShouldBeNil)
		})

		Convey("output should be passed appropriately to the stdout and stderr"+
			" writers", func() {

			stdout := &CacheLastWritten{}
			stderr := &CacheLastWritten{}

			command := &RemoteCommand{
				CmdString:      "echo 'hi stdout'; echo 'hi stderr'>&2",
				Stdout:         stdout,
				Stderr:         stderr,
				RemoteHostName: TestRemote,
				User:           TestRemoteUser,
				Options:        []string{"-i", TestRemoteKey},
			}

			// run the command, make sure the output was given to stdout
			So(command.Run(), ShouldBeNil)
			So(stdout.LastWritten, ShouldResemble, []byte("hi stdout\n"))
			So(stderr.LastWritten, ShouldResemble, []byte("hi stderr\n"))

		})

		Convey("if the background option is set to true, the ssh connection"+
			" should not wait for the command to finish", func() {

			// this command would sleep for 30 years if it were waited for
			sleepCmd := "sleep 1000000000"
			command := &RemoteCommand{
				CmdString:      sleepCmd,
				Stdout:         ioutil.Discard,
				Stderr:         ioutil.Discard,
				RemoteHostName: TestRemote,
				User:           TestRemoteUser,
				Options:        []string{"-i", TestRemoteKey},
				Background:     true,
			}

			// run the command, it should finish rather than sleeping forever
			So(command.Run(), ShouldBeNil)

			// clean up the sleeping process
			cleanupCmd := &RemoteCommand{
				CmdString:      fmt.Sprintf("pkill -xf '%v'", sleepCmd),
				Stdout:         ioutil.Discard,
				Stderr:         ioutil.Discard,
				RemoteHostName: TestRemote,
				User:           TestRemoteUser,
				Options:        []string{"-i", TestRemoteKey},
			}
			So(cleanupCmd.Run(), ShouldBeNil)

		})

	})

}
