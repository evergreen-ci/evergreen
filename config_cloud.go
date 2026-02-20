package evergreen

import (
	"context"

	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

var (
	cloudProvidersAWSKey    = bsonutil.MustHaveTag(CloudProviders{}, "AWS")
	cloudProvidersDockerKey = bsonutil.MustHaveTag(CloudProviders{}, "Docker")
)

// CloudProviders stores configuration settings for the supported cloud host providers.
type CloudProviders struct {
	AWS    AWSConfig    `bson:"aws" json:"aws" yaml:"aws"`
	Docker DockerConfig `bson:"docker" json:"docker" yaml:"docker"`
}

func (c *CloudProviders) SectionId() string { return "providers" }

func (c *CloudProviders) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

func (c *CloudProviders) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			cloudProvidersAWSKey:    c.AWS,
			cloudProvidersDockerKey: c.Docker,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *CloudProviders) ValidateAndDefault() error {
	catcher := grip.NewBasicCatcher()
	for i, m := range c.AWS.AccountRoles {
		catcher.Wrapf(m.Validate(), "invalid account role mapping at index %d", i)
	}
	return catcher.Resolve()
}

// EC2Key links a region with a corresponding key and secret
type EC2Key struct {
	Name   string `bson:"name" json:"name" yaml:"name"`
	Key    string `bson:"key" json:"key" yaml:"key" secret:"true"`
	Secret string `bson:"secret" json:"secret" yaml:"secret" secret:"true"`
}

type Subnet struct {
	AZ       string `bson:"az" json:"az" yaml:"az"`
	SubnetID string `bson:"subnet_id" json:"subnet_id" yaml:"subnet_id"`
}

// AWSConfig stores auth info for Amazon Web Services.
type AWSConfig struct {
	// EC2Keys stored as a list to allow for possible multiple accounts in the future.
	EC2Keys []EC2Key `bson:"ec2_keys" json:"ec2_keys" yaml:"ec2_keys"`
	Subnets []Subnet `bson:"subnets" json:"subnets" yaml:"subnets"`

	// ParserProject is configuration for storing and accessing parser projects
	// in S3.
	ParserProject ParserProjectS3Config `bson:"parser_project" json:"parser_project" yaml:"parser_project"`

	// PersistentDNS is AWS configuration for maintaining persistent DNS names for hosts.
	PersistentDNS PersistentDNSConfig `bson:"persistent_dns" json:"persistent_dns" yaml:"persistent_dns"`

	DefaultSecurityGroup string `bson:"default_security_group" json:"default_security_group" yaml:"default_security_group"`

	AllowedRegions []string `bson:"allowed_regions" json:"allowed_regions" yaml:"allowed_regions"`
	// EC2 instance types for spawn hosts
	AllowedInstanceTypes []string `bson:"allowed_instance_types" json:"allowed_instance_types" yaml:"allowed_instance_types"`
	// EC2 instance types that should trigger alerts when used by spawn hosts
	AlertableInstanceTypes []string `bson:"alertable_instance_types" json:"alertable_instance_types" yaml:"alertable_instance_types"`
	MaxVolumeSizePerUser   int      `bson:"max_volume_size" json:"max_volume_size" yaml:"max_volume_size"`

	AccountRoles []AWSAccountRoleMapping `bson:"account_roles" json:"account_roles" yaml:"account_roles"`

	// IPAMPoolID is the ID for the IP address management (IPAM) pool in AWS.
	IPAMPoolID string `bson:"ipam_pool_id" json:"ipam_pool_id" yaml:"ipam_pool_id"`
	// ElasticIPUsageRate is the probability (out of 1) of a host that has
	// elastic IPs enabled being assigned an elastic IP address.
	ElasticIPUsageRate float64 `bson:"elastic_ip_usage_rate" json:"elastic_ip_usage_rate" yaml:"elastic_ip_usage_rate"`
}

// AccountRoleMapping is a mapping of an AWS account to the role that needs to
// be assumed to make authorized API calls in that account.
type AWSAccountRoleMapping struct {
	// Account is the identifier for the AWS account.
	Account string `bson:"account" json:"account" yaml:"account"`
	// Role is the the role to assume to make authorized API calls.
	Role string `bson:"role" json:"role" yaml:"role"`
}

func (m *AWSAccountRoleMapping) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(m.Account == "", "account must not be empty")
	catcher.NewWhen(m.Role == "", "role must not be empty")
	return catcher.Resolve()
}

type S3Credentials struct {
	Key    string `bson:"key" json:"key" yaml:"key" secret:"true"`
	Secret string `bson:"secret" json:"secret" yaml:"secret" secret:"true"`
	Bucket string `bson:"bucket" json:"bucket" yaml:"bucket"`
}

func (c *S3Credentials) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(c.Key == "", "key must not be empty")
	catcher.NewWhen(c.Secret == "", "secret must not be empty")
	catcher.NewWhen(c.Bucket == "", "bucket must not be empty")
	return catcher.Resolve()
}

// ParserProjectS3Config is the configuration options for storing and accessing
// parser projects in S3.
type ParserProjectS3Config struct {
	S3Credentials `bson:",inline" yaml:",inline"`
	Prefix        string `bson:"prefix" json:"prefix" yaml:"prefix"`
	// GeneratedJSONPrefix is the prefix to use for storing intermediate
	// JSON configuration for generate.tasks, which will update the parser
	// project.
	GeneratedJSONPrefix string `bson:"generated_json_prefix" json:"generated_json_prefix" yaml:"generated_json_prefix"`
}

func (c *ParserProjectS3Config) Validate() error { return nil }

// PersistentDNSConfig is the configuration options to support persistent DNS
// names for hosts.
type PersistentDNSConfig struct {
	// HostedZoneID is the ID of the hosted zone in Route 53 where DNS names are
	// managed.
	HostedZoneID string `bson:"hosted_zone_id" json:"hosted_zone_id" yaml:"hosted_zone_id"`
	// Domain is the domain name of persistent DNS names.
	Domain string `bson:"domain" json:"domain" yaml:"domain"`
}

// AWSVPCConfig represents configuration when using AWSVPC networking in ECS.
type AWSVPCConfig struct {
	Subnets        []string `bson:"subnets" json:"subnets" yaml:"subnets"`
	SecurityGroups []string `bson:"security_groups" json:"security_groups" yaml:"security_groups"`
}

// AWSClientType represents the different types of AWS client implementations
// that can be used.
type AWSClientType string

const (

	// AWSClientTypeMock is the mock implementation of an AWS client for testing
	// purposes only. This should never be used in production.
	AWSClientTypeMock AWSClientType = "mock"
)

// DockerConfig stores auth info for Docker.
type DockerConfig struct {
	APIVersion string `bson:"api_version" json:"api_version" yaml:"api_version"`
}
