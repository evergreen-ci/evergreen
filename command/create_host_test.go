package command

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type createHostSuite struct {
	params map[string]interface{}
	cmd    createHost

	suite.Suite
}

func TestCreateHostSuite(t *testing.T) {
	suite.Run(t, &createHostSuite{})
}

func (s *createHostSuite) SetupTest() {
	s.params = map[string]interface{}{
		"distro": "myDistro",
		"scope":  "task",
	}
	s.cmd = createHost{}
}

func (s *createHostSuite) TestParamDefaults() {
	s.NoError(s.cmd.ParseParams(s.params))
	s.Equal(ProviderEC2, s.cmd.CloudProvider)
	s.Equal(DefaultSetupTimeoutSecs, s.cmd.SetupTimeoutSecs)
	s.Equal(DefaultTeardownTimeoutSecs, s.cmd.TeardownTimeoutSecs)
}

func (s *createHostSuite) TestParamValidation() {
	// having no ami or distro is an error
	s.params["distro"] = ""
	s.Contains(s.cmd.ParseParams(s.params).Error(), "must set exactly one of ami or distro")

	// verify errors if missing required info for ami
	s.params["ami"] = "ami"
	err := s.cmd.ParseParams(s.params)
	s.Contains(err.Error(), "instance_type must be set if ami is set")
	s.Contains(err.Error(), "must specify security_group_ids if ami is set")
	s.Contains(err.Error(), "instance_type must be set if ami is set")
	s.Contains(err.Error(), "vpc_id must be set if ami is set")

	// valid AMI info
	s.params["instance_type"] = "instance"
	s.params["security_group_ids"] = []string{"foo"}
	s.params["subnet_id"] = "subnet"
	s.params["vpc_id"] = "vpc"
	s.NoError(s.cmd.ParseParams(s.params))

	// having a key id but nothing else is an error
	s.params["aws_access_key_id"] = "keyid"
	s.Contains(s.cmd.ParseParams(s.params).Error(), "aws_access_key_id, aws_secret_access_key, key_name must all be set or unset")
	s.params["aws_secret_access_key"] = "secret"
	s.params["key_name"] = "key"
	s.NoError(s.cmd.ParseParams(s.params))

	// verify errors for things controlled by the agent
	s.params["num_hosts"] = 11
	s.Contains(s.cmd.ParseParams(s.params).Error(), "num_hosts must be between 1 and 10")
	s.params["scope"] = "idk"
	s.Contains(s.cmd.ParseParams(s.params).Error(), "scope must be build or task")
	s.params["timeout_teardown_secs"] = 55
	s.Contains(s.cmd.ParseParams(s.params).Error(), "timeout_teardown_secs must be between 60 and 604800")
}
