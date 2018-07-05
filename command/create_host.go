package command

import (
	"context"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type createHost struct {
	CreateHost *apimodels.CreateHost
	base
}

const (
	ProviderEC2                = "ec2"
	ScopeTask                  = "task"
	ScopeBuild                 = "build"
	DefaultSetupTimeoutSecs    = 600
	DefaultTeardownTimeoutSecs = 21600
)

func createHostFactory() Command { return &createHost{} }

func (c *createHost) Name() string { return "host.create" }

func (c *createHost) ParseParams(params map[string]interface{}) error {
	c.CreateHost = &apimodels.CreateHost{}
	if err := mapstructure.Decode(params, c.CreateHost); err != nil {
		return errors.Wrapf(err, "error parsing '%s' params", c.Name())
	}

	catcher := grip.NewBasicCatcher()
	if (c.CreateHost.AMI != "" && c.CreateHost.Distro != "") || (c.CreateHost.AMI == "" && c.CreateHost.Distro == "") {
		catcher.Add(errors.New("must set exactly one of ami or distro"))
	}
	if c.CreateHost.AMI != "" {
		if c.CreateHost.InstanceType == "" {
			catcher.Add(errors.New("instance_type must be set if ami is set"))
		}
		if len(c.CreateHost.SecurityGroups) == 0 {
			catcher.Add(errors.New("must specify security_group_ids if ami is set"))
		}
		if c.CreateHost.Subnet == "" {
			catcher.Add(errors.New("subnet_id must be set if ami is set"))
		}
		if c.CreateHost.VPC == "" {
			catcher.Add(errors.New("vpc_id must be set if ami is set"))
		}
	}

	if !(c.CreateHost.AWSKeyID == "" && c.CreateHost.AWSSecret == "" && c.CreateHost.KeyName == "") &&
		!(c.CreateHost.AWSKeyID != "" && c.CreateHost.AWSSecret != "" && c.CreateHost.KeyName != "") {
		catcher.Add(errors.New("aws_access_key_id, aws_secret_access_key, key_name must all be set or unset"))
	}

	if c.CreateHost.NumHosts > 10 || c.CreateHost.NumHosts < 0 {
		catcher.Add(errors.New("num_hosts must be between 1 and 10"))
	} else if c.CreateHost.NumHosts == 0 {
		c.CreateHost.NumHosts = 1
	}
	if c.CreateHost.CloudProvider == "" {
		c.CreateHost.CloudProvider = ProviderEC2
	}
	if c.CreateHost.CloudProvider != ProviderEC2 {
		catcher.Add(errors.New("only 'ec2' is supported for providers"))
	}
	if c.CreateHost.Scope != ScopeTask && c.CreateHost.Scope != ScopeBuild {
		catcher.Add(errors.New("scope must be build or task"))
	}
	if c.CreateHost.SetupTimeoutSecs == 0 {
		c.CreateHost.SetupTimeoutSecs = DefaultSetupTimeoutSecs
	}
	if c.CreateHost.SetupTimeoutSecs < 60 || c.CreateHost.SetupTimeoutSecs > 3600 {
		catcher.Add(errors.New("timeout_setup_secs must be between 60 and 3600"))
	}
	if c.CreateHost.TeardownTimeoutSecs == 0 {
		c.CreateHost.TeardownTimeoutSecs = DefaultTeardownTimeoutSecs
	}
	if c.CreateHost.TeardownTimeoutSecs < 60 || c.CreateHost.TeardownTimeoutSecs > 604800 {
		catcher.Add(errors.New("timeout_teardown_secs must be between 60 and 604800"))
	}
	return catcher.Resolve()
}

func (c *createHost) Execute(ctx context.Context, client client.Communicator,
	logger client.LoggerProducer, conf *model.TaskConfig) error {

	return errors.New("createHost is not yet implemented")
}
