// +build !windows

package subprocess

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/logging"
	. "github.com/smartystreets/goconvey/convey"
)

func TestSubtreeCleanup(t *testing.T) {
	Convey("With a tracked long-running shell command", t, func() {
		id := "testID"
		buf := &bytes.Buffer{}
		env := os.Environ()
		env = append(env, "EVR_TASK_ID=bogus")
		env = append(env, "EVR_AGENT_PID=12345")
		env = append(env, fmt.Sprintf("EVR_TASK_ID=%v", id))
		env = append(env, fmt.Sprintf("EVR_AGENT_PID=%v", os.Getpid()))
		localCmd := &LocalCommand{
			CmdString:   "while true; do sleep 1; done; echo 'finish'",
			Stdout:      buf,
			Stderr:      buf,
			ScriptMode:  true,
			Environment: env,
		}
		So(localCmd.Start(), ShouldBeNil)
		TrackProcess(id, localCmd.Cmd.Process.Pid, logging.MakeGrip(grip.GetSender()))

		Convey("running KillSpawnedProcs should kill the process before it finishes", func() {
			So(KillSpawnedProcs(id, logging.MakeGrip(grip.GetSender())), ShouldBeNil)
			So(localCmd.Cmd.Wait(), ShouldNotBeNil)
			So(buf.String(), ShouldNotContainSubstring, "finish")
		})
	})
}
