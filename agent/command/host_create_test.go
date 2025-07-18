package command

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/suite"
)

type createHostSuite struct {
	params map[string]any
	cmd    createHost
	conf   *internal.TaskConfig
	comm   client.Communicator
	logger client.LoggerProducer

	suite.Suite
}

func TestCreateHostSuite(t *testing.T) {
	suite.Run(t, &createHostSuite{})
}

func (s *createHostSuite) SetupSuite() {
	var err error
	s.comm = client.NewMock("http://localhost.com")
	s.conf = &internal.TaskConfig{
		Expansions: util.Expansions{
			"subnet_id": "subnet-123456",
			"tenancy":   string(evergreen.EC2TenancyDedicated),
		},
		Task:    task.Task{Id: "mock_id", Secret: "mock_secret"},
		Project: model.Project{}}
	s.logger, err = s.comm.GetLoggerProducer(context.Background(), &s.conf.Task, nil)
	s.Require().NoError(err)
}

func (s *createHostSuite) SetupTest() {
	s.params = map[string]any{
		"distro":    "myDistro",
		"scope":     "task",
		"subnet_id": "${subnet_id}",
		"tenancy":   "${tenancy}",
	}
	s.cmd = createHost{}
}

func (s *createHostSuite) TestParamDefaults() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.NoError(s.cmd.ParseParams(s.params))
	s.NoError(s.cmd.expandAndValidate(ctx, s.conf))
	s.Equal(apimodels.DefaultSetupTimeoutSecs, s.cmd.CreateHost.SetupTimeoutSecs)
	s.Equal(apimodels.DefaultTeardownTimeoutSecs, s.cmd.CreateHost.TeardownTimeoutSecs)

	s.params["image"] = "my-image"
	s.params["command"] = "echo hi"
	s.NoError(s.cmd.ParseParams(s.params))
	s.NoError(s.cmd.expandAndValidate(ctx, s.conf))
}

func (s *createHostSuite) TestParseFromFile() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	//file for testing parsing from a json file
	tmpdir := s.T().TempDir()
	ebsDevice := []map[string]any{
		{
			"device_name": "myDevice",
			"ebs_size":    1,
		},
	}
	path := filepath.Join(tmpdir, "example.json")
	fileContent := map[string]any{
		"distro":           "myDistro",
		"scope":            "task",
		"subnet_id":        "${subnet_id}",
		"ebs_block_device": ebsDevice,
		"tenancy":          "${tenancy}",
	}
	//parse from JSON file
	s.NoError(utility.WriteJSONFile(path, fileContent))
	_, err := os.Stat(path)
	s.Require().False(os.IsNotExist(err))
	s.params = map[string]any{
		"file": path,
	}

	s.NoError(s.cmd.ParseParams(s.params))
	s.NoError(s.cmd.expandAndValidate(ctx, s.conf))
	s.Equal("myDistro", s.cmd.CreateHost.Distro)
	s.Equal("task", s.cmd.CreateHost.Scope)
	s.Equal("subnet-123456", s.cmd.CreateHost.Subnet)
	s.Equal(evergreen.EC2TenancyDedicated, s.cmd.CreateHost.Tenancy)
	s.Equal("myDevice", s.cmd.CreateHost.EBSDevices[0].DeviceName)

	//parse from YAML file
	path = filepath.Join(tmpdir, "example.yml")
	s.NoError(utility.WriteYAMLFile(path, fileContent))
	_, err = os.Stat(path)
	s.Require().False(os.IsNotExist(err))
	s.params = map[string]any{
		"file": path,
	}

	s.NoError(s.cmd.ParseParams(s.params))
	s.NoError(s.cmd.expandAndValidate(ctx, s.conf))
	s.Equal("myDistro", s.cmd.CreateHost.Distro)
	s.Equal("task", s.cmd.CreateHost.Scope)
	s.Equal("subnet-123456", s.cmd.CreateHost.Subnet)
	s.Equal(evergreen.EC2TenancyDedicated, s.cmd.CreateHost.Tenancy)
	s.Equal("myDevice", s.cmd.CreateHost.EBSDevices[0].DeviceName)

	//test with both file and other params
	s.params = map[string]any{
		"file":   path,
		"distro": "myDistro",
	}

	err = s.cmd.ParseParams(s.params)
	s.Require().NotNil(err)
	s.Contains(err.Error(), "when using a file to parse params, no additional params other than file name should be defined")

}

func (s *createHostSuite) TestParamValidation() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// having no ami or distro is an error
	s.params["distro"] = ""
	s.NoError(s.cmd.ParseParams(s.params))
	s.Contains(s.cmd.expandAndValidate(ctx, s.conf).Error(), "must set exactly one of AMI or distro")

	// verify errors if missing required info for ami
	s.params["ami"] = "ami"
	delete(s.params, "security_group_ids")
	delete(s.params, "subnet_id")
	delete(s.params, "instance_type")
	s.NoError(s.cmd.ParseParams(s.params))
	err := s.cmd.expandAndValidate(ctx, s.conf)
	s.Require().Error(err)
	s.Contains(err.Error(), "must specify security group IDs if AMI is set")
	s.Contains(err.Error(), "subnet ID must be set if AMI is set")
	s.Contains(err.Error(), "instance type must be set if AMI is set")

	// valid AMI info
	s.params["instance_type"] = "instance"
	s.params["security_group_ids"] = []string{"foo"}
	s.params["subnet_id"] = "subnet"
	s.NoError(s.cmd.ParseParams(s.params))
	s.NoError(s.cmd.expandAndValidate(ctx, s.conf))

	// verify errors for things controlled by the agent
	s.params["num_hosts"] = "11"
	s.NoError(s.cmd.ParseParams(s.params))
	err = s.cmd.expandAndValidate(ctx, s.conf)
	s.Require().Error(err)
	s.Contains(err.Error(), "num hosts must be between 1 and 10")

	s.params["scope"] = "idk"
	s.NoError(s.cmd.ParseParams(s.params))
	err = s.cmd.expandAndValidate(ctx, s.conf)
	s.Require().Error(err)
	s.Contains(err.Error(), "scope must be build or task")

	s.params["timeout_teardown_secs"] = 55
	s.NoError(s.cmd.ParseParams(s.params))
	err = s.cmd.expandAndValidate(ctx, s.conf)
	s.Require().Error(err)
	s.Contains(err.Error(), "timeout teardown (seconds) must be between 60 and 604800")

	// Validate num_hosts can be an int
	s.params["timeout_teardown_secs"] = 60
	s.params["scope"] = "task"
	s.params["num_hosts"] = 2
	s.NoError(s.cmd.ParseParams(s.params))
	s.NoError(s.cmd.expandAndValidate(ctx, s.conf))
}

func (s *createHostSuite) TestPopulateUserdata() {
	const userdataFileName = "TestPopulateUserdata.sh"
	defer os.RemoveAll(userdataFileName)
	userdataFile := []byte("#!/bin/bash\nsome commands")
	s.NoError(os.WriteFile(userdataFileName, userdataFile, 0644))
	s.cmd.CreateHost = &apimodels.CreateHost{UserdataFile: userdataFileName}
	s.NoError(s.cmd.populateUserdata())
	s.Equal("#!/bin/bash\nsome commands", s.cmd.CreateHost.UserdataCommand)
}

func (s *createHostSuite) TestExecuteCommand() {
	s.NoError(s.cmd.ParseParams(s.params))
	s.NoError(s.cmd.Execute(context.Background(), s.comm, s.logger, s.conf))
	s.Equal("subnet-123456", s.comm.(*client.Mock).CreatedHost.Subnet)
}
