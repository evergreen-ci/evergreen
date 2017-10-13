package agent

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	dbutil "github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/host"
	modelutil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

func createAgent(testServer *service.TestServer, testHost *host.Host) *Agent {
	initialOptions := Options{
		HostID:            testHost.Id,
		HostSecret:        testHost.Secret,
		StatusPort:        2285,
		LogPrefix:         evergreen.LocalLoggingOverride,
		HeartbeatInterval: 5 * time.Second,
	}

	return New(initialOptions, client.NewCommunicator(testServer.URL))
}

type AgentIntegrationSuite struct {
	suite.Suite
	a             *Agent
	tc            *taskContext
	testDirectory string
	modelData     *modelutil.TestModelData
	testConfig    *evergreen.Settings
	testServer    *service.TestServer
}

func TestAgentIntegrationSuite(t *testing.T) {
	suite.Run(t, new(AgentIntegrationSuite))
}

func (s *AgentIntegrationSuite) SetupSuite() {
	s.testDirectory = testutil.GetDirectoryOfFile()
	s.testConfig = testutil.TestConfig()
	testutil.ConfigureIntegrationTest(s.T(), s.testConfig, "AgentIntegrationSuite")
	dbutil.SetGlobalSessionProvider(dbutil.SessionFactoryFromConfig(s.testConfig))
}

func (s *AgentIntegrationSuite) SetupTest() {
	var err error

	s.modelData, err = modelutil.SetupAPITestData(s.testConfig, "print_dir_task", "linux-64", filepath.Join(s.testDirectory, "testdata/agent-integration.yml"), modelutil.NoPatch)
	s.Require().NoError(err)

	s.testServer, err = service.CreateTestServer(s.testConfig, nil)
	s.Require().NoError(err)

	s.a = createAgent(s.testServer, s.modelData.Host)
	s.tc = &taskContext{
		task: client.TaskData{
			ID:     s.modelData.Task.Id,
			Secret: s.modelData.Task.Secret,
		},
	}
}

func (s *AgentIntegrationSuite) TearDownTest() {
	if s.testServer != nil {
		s.testServer.Close()
	}
	s.NoError(modelutil.CleanupAPITestData())
}

func (s *AgentIntegrationSuite) TestRunTask() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.NoError(s.a.resetLogging(ctx, s.tc))
	s.NoError(s.a.runTask(ctx, s.tc))
}

func (s *AgentIntegrationSuite) TestAbortTask() {
	errChan := make(chan error)

	ctx := context.Background()
	tskCtx, cancel := context.WithCancel(ctx)
	lgrCtx, lgrCancel := context.WithCancel(ctx)
	defer lgrCancel()
	go func() {
		err := s.a.resetLogging(lgrCtx, s.tc)
		if err != nil {
			errChan <- err
			return
		}
		err = s.a.runTask(tskCtx, s.tc)
		errChan <- err
	}()
	cancel()
	s.Error(<-errChan)
}
