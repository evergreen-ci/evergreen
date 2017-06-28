package agent

import (
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/agent/testutil"
	"github.com/evergreen-ci/evergreen/command"
	"github.com/evergreen-ci/evergreen/service"
	tu "github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	. "github.com/smartystreets/goconvey/convey"
)

func TestLocalJob(t *testing.T) {
	testServer, err := service.CreateTestServer(tu.TestConfig(), nil)
	tu.HandleTestingErr(err, t, "failed to start server")
	defer testServer.Close()

	Convey("With an agent command", t, func() {
		Convey("command's stdout/stderr should be captured by logger", func() {
			sender := send.MakeInternalLogger()
			killChan := make(chan bool)
			testCmd := &AgentCommand{
				ScriptLine:   "echo 'hi stdout!'; echo 'hi stderr!' >&2;",
				StreamLogger: testutil.NewTestLogger(sender),
				KillChan:     killChan,
				Expansions:   command.NewExpansions(map[string]string{}),
			}
			err := testCmd.Run("")
			So(err, ShouldBeNil)
			testCmd.FlushAndWait()

			var levelToString = map[level.Priority]string{
				level.Error: "hi stderr!",
				level.Info:  "hi stdout!",
			}
			So(sender.Len(), ShouldEqual, 4)
			// 2 lines from the command, plus 2 lines from the Run() func itself
			_ = sender.GetMessage()
			_ = sender.GetMessage()
			firstMsg := sender.GetMessage()
			secondMsg := sender.GetMessage()
			So(firstMsg.Rendered, ShouldEndWith, levelToString[firstMsg.Priority])
			So(secondMsg.Rendered, ShouldEndWith, levelToString[secondMsg.Priority])
		})
	})

	Convey("With a long-running agent command", t, func() {
		sender := send.MakeInternalLogger()
		killChan := make(chan bool)
		testCmd := &AgentCommand{
			ScriptLine:   "echo 'hi'; sleep 5; echo 'i should not get run'",
			StreamLogger: testutil.NewTestLogger(sender),
			KillChan:     killChan,
			Expansions:   command.NewExpansions(map[string]string{}),
		}

		Convey("using kill channel should abort command right away", func() {
			commandChan := make(chan error)
			go func() {
				err := testCmd.Run("")
				commandChan <- err
			}()

			go func() {
				// after a delay, signal the command to stop
				time.Sleep(time.Second)
				killChan <- true
				close(killChan)
			}()

			err := <-commandChan
			So(err, ShouldEqual, InterruptedCmdError)

			testCmd.Flush()

			// the first two messages are from the agent
			_ = sender.GetMessage()
			_ = sender.GetMessage()
			firstMessage := sender.GetMessage().Rendered
			secondMessage := sender.GetMessage().Rendered
			fmt.Println("first", firstMessage)
			fmt.Println("second", secondMessage)
			So(firstMessage, ShouldEndWith, "hi")
			So(secondMessage, ShouldContainSubstring, "Got kill signal")
		})

	})

}
