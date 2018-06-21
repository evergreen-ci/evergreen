package command

import (
	"context"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type createHost struct {
	// EC2-related settings
	AMI            string    `mapstructure:"ami"`
	Distro         string    `mapstructure:"distro"`
	EBSDevice      ebsDevice `mapstructure:"ebs_block_device"`
	InstanceType   string    `mapstructure:"instance_type"`
	Region         string    `mapstructure:"region"`
	SecurityGroups []string  `mapstructure:"security_group_ids"`
	Spot           bool      `mapstructure:"spot"`
	Subnet         string    `mapstructure:"subnet_id"`
	UserdataFile   string    `mapstructure:"userdata_file"`
	VPC            string    `mapstructure:"vpc_id"`

	// authentication settings
	AWSKeyID  string `mapstructure:"aws_access_key_id"`
	AWSSecret string `mapstructure:"aws_secret_access_key"`
	KeyName   string `mapstructure:"key_name"`

	// agent-controlled settings
	CloudProvider       string `mapstructure:"provider"`
	NumHosts            int    `mapstructure:"num_hosts"`
	Scope               string `mapstructure:"scope"`
	SetupTimeoutSecs    int    `mapstructure:"timeout_setup_secs"`
	TeardownTimeoutSecs int    `mapstructure:"timeout_teardown_secs"`

	base
}

const (
	ProviderEC2                = "ec2"
	ScopeTask                  = "task"
	ScopeBuild                 = "build"
	DefaultSetupTimeoutSecs    = 600
	DefaultTeardownTimeoutSecs = 21600
)

type ebsDevice struct {
	IOPS       int    `mapstructure:"ebs_iops"`
	SizeGB     int    `mapstructure:"ebs_size"`
	SnapshotID string `mapstructure:"ebs_snapshot_id"`
}

func createHostFactory() Command { return &createHost{} }

func (c *createHost) Name() string { return "create.host" }

func (c *createHost) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, c); err != nil {
		return errors.Wrapf(err, "error parsing '%s' params", c.Name())
	}

	catcher := grip.NewBasicCatcher()
	if (c.AMI != "" && c.Distro != "") || (c.AMI == "" && c.Distro == "") {
		catcher.Add(errors.New("must set exactly one of ami or distro"))
	}
	if c.AMI != "" {
		if c.InstanceType == "" {
			catcher.Add(errors.New("instance_type must be set if ami is set"))
		}
		if len(c.SecurityGroups) == 0 {
			catcher.Add(errors.New("must specify security_group_ids if ami is set"))
		}
		if c.Subnet == "" {
			catcher.Add(errors.New("subnet_id must be set if ami is set"))
		}
		if c.VPC == "" {
			catcher.Add(errors.New("vpc_id must be set if ami is set"))
		}
	}

	if (c.AWSKeyID != "" && c.AWSSecret == "") || (c.AWSKeyID == "" && c.AWSSecret != "") {
		catcher.Add(errors.New("aws_access_key_id and aws_secret_access_key must both be set or unset"))
	}
	if c.KeyName != "" && (c.AWSKeyID != "" || c.AWSSecret != "") {
		catcher.Add(errors.New("key_name cannot be set if aws_access_key_id is set"))
	}
	if c.KeyName == "" && c.AWSKeyID == "" {
		catcher.Add(errors.New("must set either key_name or aws_access_key_id"))
	}

	if c.NumHosts > 10 || c.NumHosts < 0 {
		catcher.Add(errors.New("num_hosts must be between 1 and 10"))
	} else if c.NumHosts == 0 {
		c.NumHosts = 1
	}
	if c.CloudProvider == "" {
		c.CloudProvider = ProviderEC2
	}
	if c.CloudProvider != ProviderEC2 {
		catcher.Add(errors.New("only 'ec2' is supported for providers"))
	}
	if c.Scope != ScopeTask && c.Scope != ScopeBuild {
		catcher.Add(errors.New("scope must be build or task"))
	}
	if c.SetupTimeoutSecs == 0 {
		c.SetupTimeoutSecs = DefaultSetupTimeoutSecs
	}
	if c.SetupTimeoutSecs < 60 || c.SetupTimeoutSecs > 3600 {
		catcher.Add(errors.New("timeout_setup_secs must be between 60 and 3600"))
	}
	if c.TeardownTimeoutSecs == 0 {
		c.TeardownTimeoutSecs = DefaultTeardownTimeoutSecs
	}
	if c.TeardownTimeoutSecs < 60 || c.TeardownTimeoutSecs > 604800 {
		catcher.Add(errors.New("timeout_teardown_secs must be between 60 and 604800"))
	}
	return catcher.Resolve()
}

func (c *createHost) Execute(ctx context.Context, client client.Communicator,
	logger client.LoggerProducer, conf *model.TaskConfig) error {

	return errors.New("createHost is not yet implemented")
}
