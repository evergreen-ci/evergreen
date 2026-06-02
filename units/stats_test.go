package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type StatUnitsSuite struct {
	sender *send.InternalSender
	env    *mock.Environment
	cancel context.CancelFunc
	suite.Suite
}

func TestStatUnitsSuite(t *testing.T) {
	suite.Run(t, new(StatUnitsSuite))
}

func (s *StatUnitsSuite) SetupTest() {
	s.sender = send.MakeInternalLogger()
	s.env = &mock.Environment{}

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.Require().NoError(s.env.Configure(ctx))
}

func (s *StatUnitsSuite) TestAmboyStatsCollector() {
	factory, err := registry.GetJobFactory(amboyStatsCollectorJobName)
	s.NoError(err)
	s.NotNil(factory)
	s.NotNil(factory())
	s.Equal(amboyStatsCollectorJobName, factory().Type().Name)

	// if the env is set, but the queues aren't logged it should
	// run and complete but report an error.
	j := makeAmboyStatsCollector()
	s.False(j.Status().Completed)
	j.env = s.env
	j.SetID(amboyStatsCollectorJobName + "-")
	j.logger = logging.MakeGrip(s.sender)
	s.False(s.sender.HasMessage())

	s.False(j.ExcludeLocal)
	s.False(j.ExcludeRemote)
	s.True(j.env.LocalQueue().Info().Started)
	s.True(j.env.RemoteQueue().Info().Started)

	j.Run(context.Background())
	s.True(s.sender.HasMessage())
	s.True(j.Status().Completed)
	s.False(j.HasErrors())

	j = makeAmboyStatsCollector()
	s.False(j.Status().Completed)
	j.env = s.env
	j.logger = logging.MakeGrip(s.sender)
	orig := s.sender.Len()

	j.Run(context.Background())
	s.Less(orig, s.sender.Len())
	s.True(j.Status().Completed)
	s.False(j.HasErrors())

	m1, ok1 := s.sender.GetMessageSafe()
	if s.True(ok1) {
		s.True(m1.Logged)
		s.Contains(m1.Message.String(), "local queue stats", m1.Message.String())
	}

	m2, ok2 := s.sender.GetMessageSafe()
	if s.True(ok2) {
		s.True(m2.Logged)
		s.Contains(m2.Message.String(), "remote queue stats", m2.Message.String())
	}
}

func (s *StatUnitsSuite) TestHostStatsCollector() {
	factory, err := registry.GetJobFactory(hostStatsCollectorJobName)
	s.NoError(err)
	s.NotNil(factory)
	s.NotNil(factory())
	s.Equal(hostStatsCollectorJobName, factory().Type().Name)

	j, ok := NewHostStatsCollector("id").(*hostStatsCollector)
	j.logger = logging.MakeGrip(s.sender)
	s.True(ok)
	s.False(s.sender.HasMessage())
	j.Run(context.Background())
	s.False(j.HasErrors())
	s.True(s.sender.HasMessage())

	m1, ok1 := s.sender.GetMessageSafe()
	if s.True(ok1) {
		s.True(m1.Logged)
		s.Contains(m1.Message.String(), "host stats by distro", m1.Message)
	}

	// NOTE: you can't trigger the error case given that the db
	// method that the job calls use the global session factory
}

func (s *StatUnitsSuite) TestTaskStatsCollector() {
	factory, err := registry.GetJobFactory(taskStatsCollectorJobName)
	s.NoError(err)
	s.NotNil(factory)
	s.NotNil(factory())
	s.Equal(taskStatsCollectorJobName, factory().Type().Name)

	j, ok := NewTaskStatsCollector("id").(*taskStatsCollector)
	j.logger = logging.MakeGrip(s.sender)
	s.True(ok)
	s.False(s.sender.HasMessage())
	s.False(j.Status().Completed)
	j.Run(context.Background())
	s.True(j.Status().Completed)
	s.False(j.HasErrors())
	s.True(s.sender.HasMessage())

	m1, ok1 := s.sender.GetMessageSafe()
	if s.True(ok1) {
		s.False(m1.Logged, "%+v", m1)
		s.Equal(0, m1.Message.(*task.ResultCounts).Total)
	}

	// NOTE: you can't trigger the error case given that the db
	// method that the job calls use the global session factory
}

func (s *StatUnitsSuite) TestSysInfoCollector() {
	factory, err := registry.GetJobFactory(sysInfoStatsCollectorJobName)
	s.NoError(err)
	s.NotNil(factory)
	s.NotNil(factory())
	s.Equal(sysInfoStatsCollectorJobName, factory().Type().Name)

	j, ok := NewSysInfoStatsCollector("id").(*sysInfoStatsCollector)
	s.True(ok)
	j.logger = logging.MakeGrip(s.sender)

	s.False(s.sender.HasMessage())
	s.False(j.Status().Completed)
	j.Run(context.Background())
	s.True(j.Status().Completed)
	s.False(j.HasErrors())
	s.True(s.sender.HasMessage())

	m, ok := s.sender.GetMessageSafe()
	if s.True(ok) {
		s.True(m.Logged)
		s.Contains(m.Message.String(), "cgo.calls", m.Message.String())
	}
}

func TestGetWaitTimeMessagesOverheadRatio(t *testing.T) {
	now := time.Now()
	tasks := []task.Task{
		{
			ScheduledTime: now.Add(-10 * time.Minute),
			StartTime:     now.Add(-5 * time.Minute),
			FinishTime:    now,
			DistroId:      "rhel80",
			Requester:     "patch",
		},
	}

	msgs := getWaitTimeMessages(tasks)
	require.NotEmpty(t, msgs)

	global := findMessage(t, msgs, "", "")
	require.NotNil(t, global)
	assert.Equal(t, 0.5, (*global)["p50_overhead_ratio"])
	assert.Equal(t, 0.5, (*global)["p90_overhead_ratio"])
}

func TestGetWaitTimeMessagesGroupsByDistro(t *testing.T) {
	now := time.Now()

	var tasks []task.Task
	for i := range minDistroTasksForStats {
		tasks = append(tasks, task.Task{
			ScheduledTime: now.Add(-10 * time.Minute),
			StartTime:     now.Add(-time.Duration(5+i) * time.Minute),
			FinishTime:    now,
			DistroId:      "rhel80",
			Requester:     "patch",
		})
	}

	tasks = append(tasks, task.Task{
		ScheduledTime: now.Add(-10 * time.Minute),
		StartTime:     now.Add(-3 * time.Minute),
		FinishTime:    now,
		DistroId:      "ubuntu2204",
		Requester:     "patch",
	})

	msgs := getWaitTimeMessages(tasks)

	var distroMsg *message.Fields
	for i := range msgs {
		if msgs[i]["stats"] == "queue-wait-by-distro" {
			distroMsg = &msgs[i]
			break
		}
	}
	require.NotNil(t, distroMsg)

	distros, ok := (*distroMsg)["distros"].(map[string]message.Fields)
	require.True(t, ok)
	assert.Contains(t, distros, "rhel80")
	assert.NotContains(t, distros, "ubuntu2204")
	assert.Equal(t, minDistroTasksForStats, distros["rhel80"]["sample_size"])
}

func TestGetWaitTimeMessagesGroupsByRequester(t *testing.T) {
	now := time.Now()
	tasks := []task.Task{
		{
			ScheduledTime: now.Add(-10 * time.Minute),
			StartTime:     now.Add(-5 * time.Minute),
			FinishTime:    now,
			DistroId:      "rhel80",
			Requester:     "patch",
		},
		{
			ScheduledTime: now.Add(-10 * time.Minute),
			StartTime:     now.Add(-7 * time.Minute),
			FinishTime:    now,
			DistroId:      "rhel80",
			Requester:     "gitter_request",
		},
	}

	msgs := getWaitTimeMessages(tasks)

	patch := findMessage(t, msgs, "requester", "patch")
	require.NotNil(t, patch)
	assert.Equal(t, 1, (*patch)["sample_size"])

	gitter := findMessage(t, msgs, "requester", "gitter_request")
	require.NotNil(t, gitter)
	assert.Equal(t, 1, (*gitter)["sample_size"])
	assert.Equal(t, 180.0, (*gitter)["p50_wait_seconds"])
}

func findMessage(t *testing.T, msgs []message.Fields, groupKey, groupValue string) *message.Fields {
	t.Helper()
	for i := range msgs {
		if groupKey == "" {
			if _, has := msgs[i]["distros"]; has {
				continue
			}
			if _, has := msgs[i]["requester"]; has {
				continue
			}
			return &msgs[i]
		}
		if msgs[i][groupKey] == groupValue {
			return &msgs[i]
		}
	}
	return nil
}

func TestTopNProjects(t *testing.T) {
	t.Run("ReturnsTopN", func(t *testing.T) {
		counts := map[string]int{
			"project-a": 100,
			"project-b": 50,
			"project-c": 30,
			"project-d": 20,
			"project-e": 10,
		}
		result := topNProjects(counts, 3)
		assert.Len(t, result, 3)
		assert.Equal(t, 100, result["project-a"])
		assert.Equal(t, 50, result["project-b"])
		assert.Equal(t, 30, result["project-c"])
		assert.NotContains(t, result, "project-d")
		assert.NotContains(t, result, "project-e")
	})
}
