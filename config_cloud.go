package evergreen

import (
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// CloudProviders stores configuration settings for the supported cloud host providers.
type CloudProviders struct {
	AWS       AWSConfig       `bson:"aws" json:"aws" yaml:"aws"`
	Docker    DockerConfig    `bson:"docker" json:"docker" yaml:"docker"`
	GCE       GCEConfig       `bson:"gce" json:"gce" yaml:"gce"`
	OpenStack OpenStackConfig `bson:"openstack" json:"openstack" yaml:"openstack"`
	VSphere   VSphereConfig   `bson:"vsphere" json:"vsphere" yaml:"vsphere"`
}

func (c *CloudProviders) SectionId() string { return "providers" }

func (c *CloudProviders) Get(env Environment) error {
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	res := coll.FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = CloudProviders{}
			return nil
		}
		return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
	}
	if err := res.Decode(c); err != nil {
		return errors.Wrap(err, "problem decoding result")
	}

	return nil
}

func (c *CloudProviders) Set() error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	_, err := coll.UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"aws":       c.AWS,
			"docker":    c.Docker,
			"gce":       c.GCE,
			"openstack": c.OpenStack,
			"vsphere":   c.VSphere,
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

func (c *CloudProviders) ValidateAndDefault() error {
	catcher := grip.NewBasicCatcher()
	catcher.Wrap(c.AWS.Pod.Validate(), "invalid ECS config")
	return catcher.Resolve()
}

// EC2Key links a region with a corresponding key and secret
type EC2Key struct {
	Name   string `bson:"name" json:"name" yaml:"name"`
	Region string `bson:"region" json:"region" yaml:"region"` // this can be removed after EVG-8284 is merged
	Key    string `bson:"key" json:"key" yaml:"key"`
	Secret string `bson:"secret" json:"secret" yaml:"secret"`
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

	S3 S3Credentials `bson:"s3_credentials"`
	// TaskSync stores credentials for storing task data in S3.
	TaskSync S3Credentials `bson:"task_sync" json:"task_sync" yaml:"task_sync"`
	// TaskSyncRead stores credentials for reading task data in S3.
	TaskSyncRead S3Credentials `bson:"task_sync_read" json:"task_sync_read" yaml:"task_sync_read"`

	DefaultSecurityGroup string `bson:"default_security_group" json:"default_security_group" yaml:"default_security_group"`

	AllowedRegions []string `bson:"allowed_regions" json:"allowed_regions" yaml:"allowed_regions"`
	// EC2 instance types for spawn hosts
	AllowedInstanceTypes []string `bson:"allowed_instance_types" json:"allowed_instance_types" yaml:"allowed_instance_types"`
	MaxVolumeSizePerUser int      `bson:"max_volume_size" json:"max_volume_size" yaml:"max_volume_size"`

	// Pod represents configuration for using pods in AWS.
	Pod AWSPodConfig
}

type S3Credentials struct {
	Key    string `bson:"key" json:"key" yaml:"key"`
	Secret string `bson:"secret" json:"secret" yaml:"secret"`
	Bucket string `bson:"bucket" json:"bucket" yaml:"bucket"`
}

func (c *S3Credentials) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(c.Key == "", "key must not be empty")
	catcher.NewWhen(c.Secret == "", "secret must not be empty")
	catcher.NewWhen(c.Bucket == "", "bucket must not be empty")
	return catcher.Resolve()
}

// AWSPodConfig represents configuration for using pods backed by AWS.
type AWSPodConfig struct {
	// Role is the role to assume to make API calls that manage pods.
	Role string `bson:"role,omitempty" json:"role,omitempty" yaml:"role,omitempty"`
	// Region is the region where the pods are managed.
	Region string `bson:"region,omitempty" json:"region,omitempty" yaml:"region,omitempty"`
	// ECS represents configuration for using AWS ECS to manage pods.
	ECS ECSConfig `bson:"ecs" json:"ecs" yaml:"ecs"`
	// SecretsManager represents configuration for using AWS Secrets Manager
	// with AWS ECS for pods.
	SecretsManager SecretsManagerConfig `bson:"secrets_manager" json:"secrets_manager" yaml:"secrets_manager"`
}

// Validate checks that the ECS configuration is valid.
func (c *AWSPodConfig) Validate() error {
	return c.ECS.Validate()
}

// ECSConfig represents configuration for AWS ECS.
type ECSConfig struct {
	// TaskDefinitionPrefix is the prefix for the task definition families.
	TaskDefinitionPrefix string `bson:"task_definition_prefix,omitempty" json:"task_definition_prefix,omitempty" yaml:"task_definition_prefix,omitempty"`
	// TaskRole is the IAM role that ECS tasks can assume to make AWS requests.
	TaskRole string `bson:"task_role,omitempty" json:"task_role,omitempty" yaml:"task_role,omitempty"`
	// ExecutionRole is the IAM role that ECS container instances can assume to
	// make AWS requests.
	ExecutionRole string `bson:"execution_role,omitempty" json:"execution_role,omitempty" yaml:"execution_role,omitempty"`
	// AWSVPC specifies configuration when ECS tasks use AWSVPC networking.
	AWSVPC AWSVPCConfig `bson:"awsvpc,omitempty" json:"awsvpc,omitempty" yaml:"awsvpc,omitempty"`
	// Clusters specify the configuration of each particular ECS cluster.
	Clusters []ECSClusterConfig `bson:"clusters,omitempty" json:"clusters,omitempty" yaml:"clusters,omitempty"`
}

// AWSVPCConfig represents configuration when using AWSVPC networking in ECS.
type AWSVPCConfig struct {
	Subnets        []string `bson:"subnets,omitempty" json:"subnets,omitempty" yaml:"subnets,omitempty"`
	SecurityGroups []string `bson:"security_groups,omitempty" json:"security_groups,omitempty" yaml:"security_groups,omitempty"`
}

// Validate checks that the required ECS configuration options are given.
func (c *ECSConfig) Validate() error {
	catcher := grip.NewBasicCatcher()
	for _, clusterConf := range c.Clusters {
		catcher.Add(clusterConf.Validate())
	}
	return catcher.Resolve()
}

// ECSClusterConfig represents configuration specific to a particular ECS
// cluster.
type ECSClusterConfig struct {
	// Name is the ECS cluster name.
	Name string `bson:"name,omitempty" json:"name,omitempty" yaml:"name,omitempty"`
	// Platform is the platform supported by the cluster.
	Platform ECSClusterPlatform `bson:"platform,omitempty" json:"platform,omitempty" yaml:"platform,omitempty"`
}

// Validate checks that the ECS cluster configuration has the required fields
// and all fields are valid values.
func (c *ECSClusterConfig) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(c.Name == "", "must specify a cluster name")
	catcher.Wrap(c.Platform.Validate(), "invalid platform")
	return catcher.Resolve()
}

// SecretsManagerConfig represents configuration for AWS Secrets Manager.
type SecretsManagerConfig struct {
	// SecretPrefix is the prefix for secret names.
	SecretPrefix string `bson:"secret_prefix,omitempty" json:"secret_prefix,omitempty" yaml:"secret_prefix,omitempty"`
}

// DockerConfig stores auth info for Docker.
type DockerConfig struct {
	APIVersion    string `bson:"api_version" json:"api_version" yaml:"api_version"`
	DefaultDistro string `bson:"default_distro" json:"default_distro" yaml:"default_distro"`
}

// OpenStackConfig stores auth info for Linaro using Identity V3. All fields required.
//
// The config is NOT compatible with Identity V2.
type OpenStackConfig struct {
	IdentityEndpoint string `bson:"identity_endpoint" json:"identity_endpoint" yaml:"identity_endpoint"`

	Username   string `bson:"username" json:"username" yaml:"username"`
	Password   string `bson:"password" json:"password" yaml:"password"`
	DomainName string `bson:"domain_name" json:"domain_name" yaml:"domain_name"`

	ProjectName string `bson:"project_name" json:"project_name" yaml:"project_name"`
	ProjectID   string `bson:"project_id" json:"project_id" yaml:"project_id"`

	Region string `bson:"region" json:"region" yaml:"region"`
}

// GCEConfig stores auth info for Google Compute Engine. Can be retrieved from:
// https://developers.google.com/identity/protocols/application-default-credentials
type GCEConfig struct {
	ClientEmail  string `bson:"client_email" json:"client_email" yaml:"client_email"`
	PrivateKey   string `bson:"private_key" json:"private_key" yaml:"private_key"`
	PrivateKeyID string `bson:"private_key_id" json:"private_key_id" yaml:"private_key_id"`
	TokenURI     string `bson:"token_uri" json:"token_uri" yaml:"token_uri"`
}

// VSphereConfig stores auth info for VMware vSphere. The config fields refer
// to your vCenter server, a centralized management tool for the vSphere suite.
type VSphereConfig struct {
	Host     string `bson:"host" json:"host" yaml:"host"`
	Username string `bson:"username" json:"username" yaml:"username"`
	Password string `bson:"password" json:"password" yaml:"password"`
}
