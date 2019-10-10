package client

import (
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/task"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

func TestTimeoutSender(t *testing.T) {
	assert := assert.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	comm := NewMock("url")
	td := TaskData{ID: "task", Secret: "secret"}
	sender := newEvergreenLogSender(ctx, comm, "testStream", td, defaultLogBufferSize, defaultLogBufferTime)
	s, ok := sender.(*evergreenLogSender)
	assert.True(ok)
	s.setBufferTime(10 * time.Millisecond)
	sender = makeTimeoutLogSender(s, comm)

	// If no messages are sent, the last message time *should not* update
	last1 := comm.LastMessageAt()
	time.Sleep(20 * time.Millisecond)
	last2 := comm.LastMessageAt()
	assert.Equal(last1, last2)

	// If a message is sent, the last message time *should* upate
	sender.Send(message.NewDefaultMessage(level.Error, "hello world!!"))
	time.Sleep(20 * time.Millisecond)
	assert.NoError(s.Close())
	last3 := comm.LastMessageAt()
	assert.NotEqual(last2, last3)
}

type logSenderSuite struct {
	suite.Suite
	restClient  *communicatorImpl
	tempDir     string
	numMessages int
	maxSleep    time.Duration
}

func TestLogSenders(t *testing.T) {
	s := logSenderSuite{}
	suite.Run(t, &s)
}

func (s *logSenderSuite) SetupSuite() {
	s.restClient = NewCommunicator("foo").(*communicatorImpl)
	tempDir, err := ioutil.TempDir("", "logSenderSuite")
	s.Require().NoError(err)
	s.tempDir = tempDir
	s.numMessages = 1000
	s.maxSleep = 10 * time.Millisecond
	rand.Seed(time.Now().UnixNano())
}

func (s *logSenderSuite) TearDownSuite() {
	s.Require().NoError(os.RemoveAll(s.tempDir))
}

func (s *logSenderSuite) randomSleep() {
	r := rand.Float64()
	sleep := r * float64(s.maxSleep)
	time.Sleep(time.Duration(sleep))
}

func (s *logSenderSuite) TestFileLogger() {
	logFileName := fmt.Sprintf("%s/log", s.tempDir)
	fileSender, err := s.restClient.makeSender(context.Background(), &task.Task{}, []LogOpts{{Sender: model.FileLogSender, Filepath: logFileName}}, "")
	s.NoError(err)
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
	logs, err := ioutil.ReadAll(f)
	s.NoError(err)
	logStr := string(logs)
	for i := 0; i < s.numMessages; i++ {
		s.Contains(logStr, fmt.Sprintf("%d\n", i))
	}
	s.Contains(logStr, "p=debug")

	// no file logger for system logs
	path := filepath.Join(s.tempDir, "nothere")
	defaultSender, err := s.restClient.makeSender(context.Background(), &task.Task{}, []LogOpts{{Sender: model.FileLogSender, Filepath: path}}, apimodels.SystemLogPrefix)
	s.NoError(err)
	s.NotNil(defaultSender)
	logger = logging.MakeGrip(defaultSender)
	logger.Debug("foo")
	s.NoError(defaultSender.Close())
	_, err = os.Stat(path)
	s.True(os.IsNotExist(err))
}

func (s *logSenderSuite) TestEvergreenLogger() {
	ctx := context.Background()
	comm := NewMock("url")
	td := TaskData{ID: "task", Secret: "secret"}
	sender := newEvergreenLogSender(ctx, comm, "testStream", td, defaultLogBufferSize, defaultLogBufferTime)
	e, ok := sender.(*evergreenLogSender)
	s.True(ok)
	e.setBufferTime(1 * time.Second)
	sender = makeTimeoutLogSender(e, comm)
	logger := logging.MakeGrip(sender)

	for i := 0; i < s.numMessages; i++ {
		logger.Debug(i)
		s.randomSleep()
	}
	s.NoError(sender.Close())

	msgs := comm.GetMockMessages()[td.ID]
	for i := 0; i < s.numMessages; i++ {
		s.Equal(strconv.Itoa(i), msgs[i].Message)
	}
}

func (s *logSenderSuite) TestMisconfiguredLogkeeper() {
	sender, err := s.restClient.makeSender(context.Background(), &task.Task{}, []LogOpts{{Sender: model.LogkeeperLogSender}}, "")
	s.Error(err)
	s.Nil(sender)
}
