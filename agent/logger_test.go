package agent

import (
	"testing"
	"time"

	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen/model"
	. "github.com/smartystreets/goconvey/convey"
)

func TestLogging(t *testing.T) {
	var apiLogger *APILogger
	var taskCommunicator *MockCommunicator
	var testLogger slogger.Logger
	Convey("With a remote logging appender", t, func() {
		taskCommunicator = &MockCommunicator{
			logChan: make(chan []model.LogMessage, 100),
		}
		apiLogger = NewAPILogger(taskCommunicator)

		testLogger = slogger.Logger{
			Prefix:    "",
			Appenders: []slogger.Appender{apiLogger},
		}

		Convey("Logging fewer msgs than threshold should not flush", func() {
			for i := 0; i < apiLogger.SendAfterLines-1; i++ {
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
				So(len(receivedMsgs), ShouldEqual, apiLogger.SendAfterLines)
				So(len(apiLogger.messages), ShouldEqual, 0)
			})
		})

		Convey("Calling flush() directly should trigger a flush", func() {
			testLogger.Logf(slogger.INFO, "test %v", 11)
			time.Sleep(10 * time.Millisecond)
			apiLogger.Flush()

			receivedMsgs, ok := <-taskCommunicator.logChan
			So(ok, ShouldBeTrue)
			So(len(receivedMsgs), ShouldEqual, 1)
		})

		Convey("Calling flush() when empty should not send anything", func() {
			apiLogger.Flush()
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

func TestCommandLogger(t *testing.T) {

	Convey("With an CommandLogger", t, func() {

		var logger *StreamLogger
		var commandLogger *CommandLogger

		Convey("logging via the CommandLogger should add the command"+
			" name to the front of the message", func() {

			appender := &sliceAppender{}
			logger = &StreamLogger{
				Local: &slogger.Logger{
					Prefix:    "test",
					Appenders: []slogger.Appender{appender},
				},
			}

			commandLogger = &CommandLogger{
				commandName: "test",
				logger:      logger,
			}

			commandLogger.LogLocal(slogger.INFO, "Test %v", 1)
			commandLogger.LogLocal(slogger.INFO, "Test %v", "2")
			So(len(appender.messages), ShouldEqual, 2)
			So(appender.messages[0], ShouldEndWith, "[test] Test 1\n")
			So(appender.messages[1], ShouldEndWith, "[test] Test 2\n")

		})

	})
}
