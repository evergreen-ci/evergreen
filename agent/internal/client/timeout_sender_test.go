package client

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/log"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestTimeoutSender(t *testing.T) {
	comm := NewMock("url")
	td := TaskData{ID: "task", Secret: "secret"}
	ms := newMockSender("test_timeout_sender", func(line log.LogLine) error {
		return comm.sendTaskLogLine(td, line)
	})
	sender := makeTimeoutLogSender(ms, comm)

	// If no messages are sent, the last message time *should not* update.
	last1 := comm.LastMessageAt()
	time.Sleep(20 * time.Millisecond)
	last2 := comm.LastMessageAt()
	assert.Equal(t, last1, last2)

	// If a message is sent, the last message time *should* update.
	sender.Send(message.NewDefaultMessage(level.Error, "hello world!!"))
	time.Sleep(20 * time.Millisecond)
	require.NoError(t, sender.Close())
	last3 := comm.LastMessageAt()
	assert.NotEqual(t, last2, last3)
}

type logSenderSuite struct {
	suite.Suite
	restClient        *hostCommunicator
	tempDir           string
	numMessages       int
	maxSleep          time.Duration
	underlyingSenders []send.Sender
}

func TestLogSenders(t *testing.T) {
	s := logSenderSuite{}
	suite.Run(t, &s)
}

func (s *logSenderSuite) SetupTest() {
	s.underlyingSenders = []send.Sender{}
}

func (s *logSenderSuite) SetupSuite() {
	s.restClient = NewHostCommunicator("foo", "hostID", "hostSecret").(*hostCommunicator)
	s.tempDir = s.T().TempDir()
	s.numMessages = 1000
	s.maxSleep = 10 * time.Millisecond
	rand.Seed(time.Now().UnixNano())
}

func (s *logSenderSuite) TearDownTest() {
	for _, sender := range s.underlyingSenders {
		s.Require().NoError(sender.Close())
	}
}

func (s *logSenderSuite) randomSleep() {
	r := rand.Float64()
	sleep := r * float64(s.maxSleep)
	time.Sleep(time.Duration(sleep))
}

func (s *logSenderSuite) TestFileLogger() {
	logFileName := fmt.Sprintf("%s/log", s.tempDir)
	fileSender, toClose, err := s.restClient.makeSender(context.Background(), TaskData{}, []LogOpts{{Sender: model.FileLogSender, Filepath: logFileName}}, false, "", "")
	s.NoError(err)
	s.underlyingSenders = append(s.underlyingSenders, toClose...)
	s.NotNil(fileSender)
	logger := logging.MakeGrip(fileSender)

	for i := 0; i < s.numMessages; i++ {
		logger.Debug(i)
		s.randomSleep()
	}
	s.NoError(fileSender.Close())

	f, err := os.Open(logFileName)
	s.Require().NoError(err)
	defer f.Close()
	logs, err := io.ReadAll(f)
	s.NoError(err)
	logStr := string(logs)
	for i := 0; i < s.numMessages; i++ {
		s.Contains(logStr, fmt.Sprintf("%d\n", i))
	}
	s.Contains(logStr, "p=debug")

	// No file logger for system logs.
	path := filepath.Join(s.tempDir, "nothere")
	_, _, err = s.restClient.makeSender(context.Background(), TaskData{}, []LogOpts{{Sender: model.FileLogSender, Filepath: path}}, false, apimodels.SystemLogPrefix, "")
	s.Error(err)
}

func (s *logSenderSuite) TestMisconfiguredSender() {
	sender, toClose, err := s.restClient.makeSender(context.Background(), TaskData{}, []LogOpts{{Sender: model.BuildloggerLogSender}}, false, "", "")
	s.underlyingSenders = append(s.underlyingSenders, toClose...)
	s.Error(err)
	s.Nil(sender)
}
