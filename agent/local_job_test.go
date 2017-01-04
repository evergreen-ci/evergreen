package agent

import (
	"fmt"
	"testing"
	"time"

	slogger "github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen/agent/testutil"
	"github.com/evergreen-ci/evergreen/command"
	evgtestutil "github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

func TestLocalJob(t *testing.T) {
	Convey("With an agent command", t, func() {
		Convey("command's stdout/stderr should be captured by logger", func() {
			appender := &evgtestutil.SliceAppender{}
			killChan := make(chan bool)
			testCmd := &AgentCommand{
				ScriptLine:   "echo 'hi stdout!'; echo 'hi stderr!' >&2;",
				StreamLogger: testutil.NewTestLogger(appender),
				KillChan:     killChan,
				Expansions:   command.NewExpansions(map[string]string{}),
			}
			err := testCmd.Run("")
			So(err, ShouldBeNil)
			testCmd.FlushAndWait()
			// 2 lines from the command, plus 2 lines from the Run() func itself
			messages := appender.Messages()
			for _, v := range messages {
				fmt.Println(v.Message())
			}

			var levelToString = map[slogger.Level]string{
				slogger.ERROR: "hi stderr!",
				slogger.INFO:  "hi stdout!",
			}
			So(len(messages), ShouldEqual, 4)
			MsgA := messages[len(messages)-2]
			MsgB := messages[len(messages)-1]
			So(MsgA.Message(), ShouldEndWith, levelToString[MsgA.Level])
			So(MsgB.Message(), ShouldEndWith, levelToString[MsgB.Level])
		})

		Convey("command's stdout/stderr should only print newlines with \\n", func() {
			appender := &evgtestutil.SliceAppender{}
			killChan := make(chan bool)
			newlineTestCmd := &AgentCommand{
				ScriptLine:   "printf 'this is not a newline...'; printf 'this is a newline \n';",
				StreamLogger: testutil.NewTestLogger(appender),
				KillChan:     killChan,
				Expansions:   command.NewExpansions(map[string]string{}),
			}

			err := newlineTestCmd.Run("")
			So(err, ShouldBeNil)
			newlineTestCmd.FlushAndWait()

			messages := appender.Messages()
			So(len(messages), ShouldEqual, 3)

			// 2 lines from the command, plus 1 lines from the Run() func itself
			for _, v := range messages {
				fmt.Println(v.Message())
			}
			NewLineMessage := messages[len(messages)-1]
			So(NewLineMessage.Message(), ShouldEqual, "this is not a newline...this is a newline ")
		})

	})

	Convey("With a long-running agent command", t, func() {
		appender := &evgtestutil.SliceAppender{}
		killChan := make(chan bool)
		testCmd := &AgentCommand{
			ScriptLine:   "echo 'hi'; sleep 4; echo 'i should not get run'",
			StreamLogger: testutil.NewTestLogger(appender),
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
				time.Sleep(1 * time.Second)
				close(killChan)
			}()

			err := <-commandChan
			So(err, ShouldEqual, InterruptedCmdError)

			testCmd.Flush()
			messages := appender.Messages()

			So(len(messages), ShouldEqual, 6)
			lastMessage := messages[len(messages)-3]
			nextLastMessage := messages[len(messages)-4]
			So(lastMessage.Message(), ShouldStartWith, "Got kill signal")
			So(nextLastMessage.Message(), ShouldEqual, "hi")
		})

	})

}
