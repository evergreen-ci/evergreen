package agent

import (
	"10gen.com/mci/model"
	"github.com/10gen-labs/slogger/v1"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
	"time"
)

func TestLogging(t *testing.T) {
	var remoteAppender *TaskCommunicatorAppender
	var taskCommunicator *MockCommunicator
	var signalChan chan AgentSignal
	var testLogger slogger.Logger
	Convey("With a remote logging appender", t, func() {
		taskCommunicator = &MockCommunicator{
			logChan: make(chan []model.LogMessage, 100),
		}
		signalChan = make(chan AgentSignal)
		remoteAppender = NewTaskCommunicatorAppender(taskCommunicator, signalChan)

		testLogger = slogger.Logger{
			Prefix:    "",
			Appenders: []slogger.Appender{remoteAppender},
		}

		Convey("Logging fewer msgs than threshold should not flush", func() {
			for i := 0; i < remoteAppender.SendAfterLines-1; i++ {
				testLogger.Logf(slogger.INFO, "test %v", i)
			}

			select {
			case _, ok := <-taskCommunicator.logChan:
				So(ok, ShouldBeFalse)
			default:
				So(true, ShouldBeTrue)
			}
			Convey("Logging beyond the threshold should trigger a flush", func() {
				testLogger.Logf(slogger.INFO, "test %v", 10)
				time.Sleep(10 * time.Millisecond)
				receivedMsgs, ok := <-taskCommunicator.logChan
				So(ok, ShouldBeTrue)
				So(len(receivedMsgs), ShouldEqual, remoteAppender.SendAfterLines)
				So(len(remoteAppender.messages), ShouldEqual, 0)
			})
		})

		Convey("Calling flush() directly should trigger a flush", func() {
			testLogger.Logf(slogger.INFO, "test %v", 11)
			time.Sleep(10 * time.Millisecond)
			remoteAppender.Flush()

			receivedMsgs, ok := <-taskCommunicator.logChan
			So(ok, ShouldBeTrue)
			So(len(receivedMsgs), ShouldEqual, 1)
		})

		Convey("Calling flush() when empty should not send anything", func() {
			remoteAppender.Flush()
			time.Sleep(10 * time.Millisecond)
			select {
			case _, ok := <-taskCommunicator.logChan:
				So(ok, ShouldBeFalse)
			default:
				So(true, ShouldBeTrue)
			}
		})

	})
}

// mock appender to just store into a slice
type sliceAppender struct {
	messages []string
}

func (self *sliceAppender) Append(log *slogger.Log) error {
	self.messages = append(self.messages, slogger.FormatLog(log))
	return nil
}

func TestAgentCommandLogger(t *testing.T) {

	Convey("With an AgentCommandLogger", t, func() {

		var agentLogger *AgentLogger
		var agentCommandLogger *AgentCommandLogger

		Convey("logging via the AgentCommandLogger should add the command"+
			" name to the front of the message", func() {

			appender := &sliceAppender{}
			agentLogger = &AgentLogger{
				LocalLogger: &slogger.Logger{
					Prefix:    "test",
					Appenders: []slogger.Appender{appender},
				},
			}

			agentCommandLogger = &AgentCommandLogger{
				commandName: "test",
				agentLogger: agentLogger,
			}

			agentCommandLogger.LogLocal(slogger.INFO, "Test %v", 1)
			agentCommandLogger.LogLocal(slogger.INFO, "Test %v", "2")
			So(len(appender.messages), ShouldEqual, 2)
			So(appender.messages[0], ShouldEndWith, "[test] Test 1\n")
			So(appender.messages[1], ShouldEndWith, "[test] Test 2\n")

		})

	})
}
