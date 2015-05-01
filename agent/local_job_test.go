package agent

import (
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/command"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
	"time"
)

func TestLocalJob(t *testing.T) {
	Convey("With an agent command", t, func() {
		Convey("command's stdout/stderr should be captured by logger", func() {
			appender := &evergreen.SliceAppender{[]*slogger.Log{}}
			killChan := make(chan bool)
			testCmd := &AgentCommand{
				ScriptLine:  "echo 'hi stdout!'; echo 'hi stderr!' >&2;",
				AgentLogger: NewTestAgentLogger(appender),
				KillChannel: killChan,
				Expansions:  command.NewExpansions(map[string]string{}),
			}
			err := testCmd.Run("")
			So(err, ShouldBeNil)
			testCmd.FlushAndWait()
			//2 lines from the command, plus 2 lines from the Run() func itself
			for _, v := range appender.Messages {
				fmt.Println(v.Message())
			}

			var levelToString = map[slogger.Level]string{
				slogger.ERROR: "hi stderr!",
				slogger.INFO:  "hi stdout!",
			}
			So(len(appender.Messages), ShouldEqual, 4)
			MsgA := appender.Messages[len(appender.Messages)-2]
			MsgB := appender.Messages[len(appender.Messages)-1]
			So(MsgA.Message(), ShouldEndWith, levelToString[MsgA.Level])
			So(MsgB.Message(), ShouldEndWith, levelToString[MsgB.Level])
		})

		Convey("command's stdout/stderr should only print newlines with \\n", func() {
			appender := &evergreen.SliceAppender{[]*slogger.Log{}}
			killChan := make(chan bool)
			newlineTestCmd := &AgentCommand{
				ScriptLine:  "printf 'this is not a newline...'; printf 'this is a newline \n';",
				AgentLogger: NewTestAgentLogger(appender),
				KillChannel: killChan,
				Expansions:  command.NewExpansions(map[string]string{}),
			}

			err := newlineTestCmd.Run("")
			So(err, ShouldBeNil)
			newlineTestCmd.FlushAndWait()

			//2 lines from the command, plus 1 lines from the Run() func itself
			for _, v := range appender.Messages {
				fmt.Println(v.Message())
			}
			So(len(appender.Messages), ShouldEqual, 3)
			NewLineMessage := appender.Messages[len(appender.Messages)-1]
			So(NewLineMessage.Message(), ShouldEqual, "this is not a newline...this is a newline ")
		})

	})

	Convey("With a long-running agent command", t, func() {
		appender := &evergreen.SliceAppender{[]*slogger.Log{}}
		killChan := make(chan bool)
		testCmd := &AgentCommand{
			ScriptLine:  "echo 'hi'; sleep 4; echo 'i should not get run'",
			AgentLogger: NewTestAgentLogger(appender),
			KillChannel: killChan,
			Expansions:  command.NewExpansions(map[string]string{}),
		}

		Convey("using kill channel should abort command right away", func() {

			commandChan := make(chan error)
			go func() {
				err := testCmd.Run("")
				commandChan <- err
			}()

			go func() {
				//after a delay, signal the command to stop
				time.Sleep(1 * time.Second)
				close(killChan)
			}()

			err := <-commandChan
			So(err, ShouldEqual, InterruptedCmdError)
			testCmd.Flush()
			lastMessage := appender.Messages[len(appender.Messages)-1]
			nextLastMessage := appender.Messages[len(appender.Messages)-2]
			So(lastMessage.Message(), ShouldStartWith, "Got kill signal")
			So(nextLastMessage.Message(), ShouldEqual, "hi")
		})

	})

}
