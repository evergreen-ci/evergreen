package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"gopkg.in/mgo.v2/bson"
)

var taskLogTestConfig = testutil.TestConfig()

func init() {
	db.SetGlobalSessionProvider(taskLogTestConfig.SessionFactory())
}

func cleanUpLogDB() error {
	session, _, err := db.GetGlobalSessionFactory().GetSession()
	if err != nil {
		return err
	}
	defer session.Close()
	_, err = session.DB(TaskLogDB).C(TaskLogCollection).RemoveAll(bson.M{})
	return err
}

func TestFindMostRecentTaskLogs(t *testing.T) {

	Convey("When finding the most recent task logs", t, func() {

		testutil.HandleTestingErr(cleanUpLogDB(), t, "Error cleaning up task log"+
			" database")

		Convey("the ones with the most recent timestamp should be retrieved,"+
			" in backwards time order", func() {

			// insert 10 logs
			startTime := time.Now()
			for i := 0; i < 10; i++ {
				taskLog := &TaskLog{}
				taskLog.TaskId = "task_id"
				taskLog.MessageCount = i + 1
				taskLog.Messages = []apimodels.LogMessage{}
				taskLog.Timestamp = startTime.Add(
					time.Second * time.Duration(i))
				So(taskLog.Insert(), ShouldBeNil)
			}

			// get the last 5 logs from the database
			fromDB, err := FindMostRecentTaskLogs("task_id", 0, 5)
			So(err, ShouldBeNil)
			So(len(fromDB), ShouldEqual, 5)

			// this is kinda hacky, but it works
			for i := 0; i < 5; i++ {
				So(fromDB[i].MessageCount, ShouldEqual, 10-i)
			}

		})

	})

}

func TestFindTaskLogsBeforeTime(t *testing.T) {

	Convey("When finding task logs before a specified time", t, func() {

		testutil.HandleTestingErr(cleanUpLogDB(), t, "Error cleaning up task log"+
			" database")

		Convey("the specified number of task logs should be returned, in"+
			" backwards time order, all from before the specified"+
			" time", func() {

			startTime := time.Now()
			// insert 5 logs, 4 before the specified time
			for i := 0; i < 5; i++ {
				taskLog := &TaskLog{}
				taskLog.TaskId = "task_id"
				taskLog.MessageCount = 1 // to differentiate these
				taskLog.Messages = []apimodels.LogMessage{}
				taskLog.Timestamp = startTime.Add(
					time.Second * time.Duration(-i))
				So(taskLog.Insert(), ShouldBeNil)
			}
			// insert 4 logs, 3 after the specified time
			for i := 0; i < 4; i++ {
				taskLog := &TaskLog{}
				taskLog.TaskId = "task_id"
				taskLog.MessageCount = 0
				taskLog.Messages = []apimodels.LogMessage{}
				taskLog.Timestamp = startTime.Add(
					time.Second * time.Duration(i))
				So(taskLog.Insert(), ShouldBeNil)
			}

			fromDB, err := FindTaskLogsBeforeTime("task_id", 0, startTime, 10)
			So(err, ShouldBeNil)
			So(len(fromDB), ShouldEqual, 4)
			for _, log := range fromDB {
				So(log.MessageCount, ShouldEqual, 1)
			}

		})

	})

}

func TestAddLogMessage(t *testing.T) {

	Convey("When adding a log message to a task log", t, func() {

		testutil.HandleTestingErr(cleanUpLogDB(), t, "Error cleaning up task log"+
			" database")

		Convey("both the in-memory and database copies of the task log should"+
			" be updated", func() {

			taskLog := &TaskLog{}
			taskLog.TaskId = "task_id"
			taskLog.MessageCount = 0

			taskLog.Messages = []apimodels.LogMessage{}
			taskLog.Timestamp = time.Now()

			So(taskLog.Insert(), ShouldBeNil)

			// so that we get the _id
			taskLogs, err := FindTaskLogsBeforeTime(taskLog.TaskId, 0,
				time.Now().Add(time.Second), 1)
			So(err, ShouldBeNil)
			taskLog = &(taskLogs[0])

			for i := 0; i < 5; i++ {
				logMsg := &apimodels.LogMessage{}
				logMsg.Message = "Hello"
				logMsg.Severity = apimodels.LogDebugPrefix
				logMsg.Timestamp = time.Now()
				logMsg.Type = apimodels.SystemLogPrefix
				So(taskLog.AddLogMessage(*logMsg), ShouldBeNil)
			}
			So(taskLog.MessageCount, ShouldEqual, 5)
			So(len(taskLog.Messages), ShouldEqual, 5)

			taskLogsFromDB, err := FindTaskLogsBeforeTime(taskLog.TaskId, 0,
				time.Now().Add(time.Second), 1)
			So(err, ShouldBeNil)
			taskLogFromDB := &(taskLogsFromDB[0])
			So(err, ShouldBeNil)
			So(taskLogFromDB.MessageCount, ShouldEqual, 5)
			So(len(taskLogFromDB.Messages), ShouldEqual, 5)

		})

	})

}

func TestInsertLogMessage(t *testing.T) {

	Convey("When inserting a log message", t, func() {

		testutil.HandleTestingErr(cleanUpLogDB(), t, "Error cleaning up task log"+
			" database")

		Convey("the log message should be added to the most recent task log"+
			" document for the task", func() {

			startTime := time.Now()
			taskLog := &TaskLog{
				TaskId:    "task_id",
				Execution: 0,
			}
			for i := 0; i < MessagesPerLog*2; i++ {
				logMsg := apimodels.LogMessage{}
				logMsg.Severity = apimodels.LogDebugPrefix
				logMsg.Timestamp = startTime.Add(time.Second * time.Duration(i))
				logMsg.Type = apimodels.SystemLogPrefix

				taskLog.MessageCount++
				taskLog.Messages = append(taskLog.Messages, logMsg)
			}

			So(taskLog.Insert(), ShouldBeNil)

			fromDB, err := FindMostRecentTaskLogs("task_id", 0, 2)
			So(err, ShouldBeNil)
			So(len(fromDB), ShouldEqual, 1)

			// since log saving happens as fire-and-forget, we're not entirely
			// guaranteed that there will be the full number of messages in
			// each task log document. however, there have to be enough in the
			// first one (but there could be more)
			So(fromDB[0].MessageCount, ShouldBeGreaterThanOrEqualTo, MessagesPerLog)
			So(len(fromDB[0].Messages), ShouldEqual, fromDB[0].MessageCount)

		})

	})

}

func TestFindMostRecentLogMessages(t *testing.T) {

	Convey("When finding the most recent log messages", t, func() {

		testutil.HandleTestingErr(cleanUpLogDB(), t, "Error cleaning up task log"+
			" database")

		Convey("the specified number of log messages should be retrieved,"+
			" using the specified severity and log type filters", func() {

			getRandomSeverity := func(i int) string {
				switch i % 3 {
				case 0:
					return apimodels.LogDebugPrefix
				case 1:
					return apimodels.LogInfoPrefix
				case 2:
					return apimodels.LogWarnPrefix
				}
				return apimodels.LogErrorPrefix
			}

			getRandomMsgType := func(i int) string {
				switch i % 2 {
				case 0:
					return apimodels.SystemLogPrefix
				case 1:
					return apimodels.TaskLogPrefix
				}
				return apimodels.LogErrorPrefix
			}

			startTime := time.Now().Add(time.Second * time.Duration(-1000))
			// insert a lot of log messages
			taskLog := &TaskLog{
				TaskId:    "task_id",
				Execution: 0,
			}
			for i := 0; i < 150; i++ {
				logMsg := apimodels.LogMessage{}
				logMsg.Severity = getRandomSeverity(i)
				logMsg.Type = getRandomMsgType(i)
				logMsg.Message = "Hello"
				logMsg.Timestamp = startTime.Add(time.Second * time.Duration(i))

				taskLog.MessageCount++
				taskLog.Messages = append(taskLog.Messages, logMsg)
			}

			So(taskLog.Insert(), ShouldBeNil)

			// now get the last 10 messages, unfiltered
			fromDB, err := FindMostRecentLogMessages("task_id", 0, 10, []string{},
				[]string{})
			So(err, ShouldBeNil)
			So(len(fromDB), ShouldEqual, 10)

			// filter by severity and log type
			fromDB, err = FindMostRecentLogMessages("task_id", 0, 10,
				[]string{apimodels.LogDebugPrefix},
				[]string{apimodels.SystemLogPrefix})
			So(err, ShouldBeNil)
			So(len(fromDB), ShouldEqual, 10)
			for _, msgFromDB := range fromDB {
				So(msgFromDB.Severity, ShouldEqual, apimodels.LogDebugPrefix)
				So(msgFromDB.Type, ShouldEqual, apimodels.SystemLogPrefix)
			}

			// filter on multiple severities
			fromDB, err = FindMostRecentLogMessages("task_id", 0, 10,
				[]string{apimodels.LogDebugPrefix, apimodels.LogInfoPrefix}, []string{})
			So(err, ShouldBeNil)
			So(len(fromDB), ShouldEqual, 10)
			for _, logMsg := range fromDB {
				So(logMsg.Severity != apimodels.LogDebugPrefix || logMsg.Severity != apimodels.LogInfoPrefix, // nolint
					ShouldBeTrue)
			}

			// filter on a non-existent severity
			fromDB, err = FindMostRecentLogMessages("task_id", 0, 10, []string{""},
				[]string{})
			So(err, ShouldBeNil)
			So(len(fromDB), ShouldEqual, 0)

		})

	})

}
