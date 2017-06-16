package event

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/suite"
)

type TaskResourceSuite struct {
	taskId string
	suite.Suite
}

func TestTaskResourceSuite(t *testing.T) {
	suite.Run(t, new(TaskResourceSuite))
}

func (s *TaskResourceSuite) SetupSuite() {
	s.taskId = "taskId"
}

func (s *TaskResourceSuite) SetupTest() {
	s.Require().NoError(db.Clear(TaskLogCollection))
}

func (s *TaskResourceSuite) TestNoSystemInfoResultsBeforeLoggingResults() {
	results, err := Find(TaskLogCollection, TaskSystemInfoEvents(s.taskId, time.Now(), 100, -1))
	s.NoError(err)
	s.Len(results, 0)
}

func (s *TaskResourceSuite) TestLoggedSystemInfoEventIsRetreivable() {
	sysInfo, ok := message.CollectSystemInfo().(*message.SystemInfo)
	s.True(ok)

	LogTaskSystemData(s.taskId, sysInfo)
	results, err := Find(TaskLogCollection, TaskSystemInfoEvents(s.taskId, time.Now(), 100, -1))
	s.NoError(err)
	s.Len(results, 1)
}

func (s *TaskResourceSuite) TestLoggingManySystemInfoEvents() {
	for i := 0; i < 10; i++ {
		info, ok := message.CollectSystemInfo().(*message.SystemInfo)
		s.True(ok)
		LogTaskSystemData(s.taskId, info)
	}

	results, err := Find(TaskLogCollection, TaskSystemInfoEvents(s.taskId, time.Now(), 100, -1))
	s.NoError(err)
	s.Len(results, 10)
}

func (s *TaskResourceSuite) TestNoProcessEventsBeforeLoggingResults() {
	results, err := Find(TaskLogCollection, TaskProcessInfoEvents(s.taskId, time.Now(), 100, -1))
	s.NoError(err)
	s.Len(results, 0)
}

func (s *TaskResourceSuite) TestLogSingleProcessEvent() {
	pm, ok := message.CollectProcessInfoSelf().(*message.ProcessInfo)
	s.True(ok)

	LogTaskProcessData(s.taskId, []*message.ProcessInfo{pm})

	results, err := Find(TaskLogCollection, TaskProcessInfoEvents(s.taskId, time.Now(), 100, -1))
	s.NoError(err)
	s.Len(results, 1)
}

func (s *TaskResourceSuite) TestLogManyProcessEvents() {
	var count int
	s.taskId += "batch"

	infos := []*message.ProcessInfo{}
	msgs := message.CollectProcessInfoSelfWithChildren()

	for _, m := range msgs {
		count++

		info, ok := m.(*message.ProcessInfo)
		s.True(ok)
		infos = append(infos, info)
	}
	s.Equal(len(infos), len(msgs))
	LogTaskProcessData(s.taskId, infos)

	s.Equal(count, len(infos))
	results, err := Find(TaskLogCollection, TaskProcessInfoEvents(s.taskId, time.Now(), 100, -1))
	s.NoError(err)
	s.Len(results, count)
}

func (s *TaskResourceSuite) TestLoggedSystemEventsWithoutTimestampsGetCurrentTimeByDefault() {
	startTime := time.Now().Add(-100 * time.Millisecond).Round(time.Millisecond)

	sys := new(message.SystemInfo)
	s.True(sys.Base.Time.IsZero())
	s.True(startTime.After(sys.Base.Time))

	LogTaskSystemData(s.taskId, sys)
	results, err := Find(TaskLogCollection, TaskSystemInfoEvents(s.taskId, time.Now(), 100, -1))
	s.NoError(err)
	s.Len(results, 1)
	event := results[0]
	info := event.Data.Data.(*TaskSystemResourceData).SystemInfo
	s.False(event.Timestamp.IsZero())
	s.False(info.Base.Time.IsZero())
	s.Equal(event.Timestamp, info.Base.Time)

	s.True(event.Timestamp.After(startTime),
		"started at %s but was %s", startTime, event.Timestamp)
}

func (s *TaskResourceSuite) TestLoggedProcessEventsWithoutTimestampsGetCurrentTimeByDefault() {
	startTime := time.Now().Add(-100 * time.Millisecond).Round(time.Millisecond)
	info := &message.ProcessInfo{}

	s.True(info.Base.Time.IsZero())
	s.True(startTime.After(info.Base.Time))

	LogTaskProcessData(s.taskId, []*message.ProcessInfo{info})
	results, err := Find(TaskLogCollection, TaskProcessInfoEvents(s.taskId, time.Now(), 100, -1))
	s.NoError(err)
	s.Len(results, 1)
	event := results[0]
	s.Len(event.Data.Data.(*TaskProcessResourceData).Processes, 1)

	s.False(event.Timestamp.IsZero())

	s.True(event.Timestamp.After(startTime),
		"started at %s but was %s", startTime, event.Timestamp)
}

func (s *TaskResourceSuite) TestProcessInfosWithTimestampsPersist() {
	startTime := time.Now().Add(-100 * time.Millisecond)
	info := &message.ProcessInfo{}
	s.NoError(info.Base.Collect())
	s.False(info.Base.Time.IsZero())
	s.True(startTime.Before(info.Base.Time))

	LogTaskProcessData(s.taskId, []*message.ProcessInfo{info})
	results, err := Find(TaskLogCollection, TaskProcessInfoEvents(s.taskId, time.Now(), 100, -1))
	s.NoError(err)
	s.Len(results, 1)
	event := results[0]
	s.WithinDuration(info.Base.Time, event.Timestamp, 10*time.Millisecond)
	s.Len(event.Data.Data.(*TaskProcessResourceData).Processes, 1)
	proc := event.Data.Data.(*TaskProcessResourceData).Processes[0]
	s.Equal(proc.Base.Time, event.Timestamp)
	s.True(startTime.Before(event.Timestamp))
}

func (s *TaskResourceSuite) TestSystemInfosWithTimestampsPersist() {
	startTime := time.Now().Add(-100 * time.Millisecond)
	info := &message.SystemInfo{}
	s.NoError(info.Base.Collect())
	s.False(info.Base.Time.IsZero())
	s.True(startTime.Before(info.Base.Time))

	LogTaskSystemData(s.taskId, info)
	results, err := Find(TaskLogCollection, TaskSystemInfoEvents(s.taskId, time.Now(), 100, -1))
	s.NoError(err)
	s.Len(results, 1)
	event := results[0]
	s.WithinDuration(info.Base.Time, event.Timestamp, 10*time.Millisecond)
	sys := event.Data.Data.(*TaskSystemResourceData).SystemInfo
	s.Equal(sys.Base.Time, event.Timestamp)
	s.True(startTime.Before(event.Timestamp))
}

func (s *TaskResourceSuite) TestFinderMethodsFilterEventsByTypeAndTaskName() {
	const baseNum = 5
	altName := s.taskId + ".alt"

	// generate and add data

	sys := message.CollectSystemInfo().(*message.SystemInfo)
	procs := []*message.ProcessInfo{}
	for _, p := range message.CollectProcessInfoSelfWithChildren() {
		procs = append(procs, p.(*message.ProcessInfo))
	}

	for i := 0; i < baseNum; i++ {
		LogTaskSystemData(s.taskId, sys)
		LogTaskProcessData(s.taskId, procs)
		LogTaskSystemData(altName, sys)
		LogTaskProcessData(altName, procs)
	}

	// run counts on the collection

	num, err := CountProcessEvents("")
	s.NoError(err)
	s.Equal(num, baseNum*2)
	num, err = CountSystemEvents("")
	s.NoError(err)
	s.Equal(num, baseNum*2)

	num, err = CountProcessEvents(s.taskId)
	s.NoError(err)
	s.Equal(num, baseNum)
	num, err = CountSystemEvents(s.taskId)
	s.NoError(err)
	s.Equal(num, baseNum)

	num, err = CountProcessEvents(altName)
	s.NoError(err)
	s.Equal(num, baseNum)
	num, err = CountSystemEvents(altName)
	s.NoError(err)
	s.Equal(num, baseNum)

	// Actually check the results methods

	sysresults, err := Find(TaskLogCollection, TaskSystemInfoEvents(s.taskId, time.Now(), 100, -1))
	s.NoError(err)
	s.Len(sysresults, baseNum)
	procresults, err := Find(TaskLogCollection, TaskProcessInfoEvents(s.taskId, time.Now(), 100, -1))
	s.NoError(err)
	s.Len(sysresults, baseNum)

	altsysresults, err := Find(TaskLogCollection, TaskSystemInfoEvents(altName, time.Now(), 100, -1))
	s.NoError(err)
	s.Len(sysresults, baseNum)
	altprocresults, err := Find(TaskLogCollection, TaskProcessInfoEvents(altName, time.Now(), 100, -1))
	s.NoError(err)
	s.Len(sysresults, baseNum)

	s.NotEqual(sysresults, altsysresults)
	s.NotEqual(procresults, altprocresults)
}
