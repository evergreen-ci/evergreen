package evergreen

import (
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
		return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
	}

	if err := res.Decode(c); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = CloudProviders{}
			return nil
		}

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

func (c *CloudProviders) ValidateAndDefault() error { return nil }

// EC2Key links a region with a corresponding key and secret
type EC2Key struct {
	Region string `bson:"region" json:"region" yaml:"region"`
	Key    string `bson:"key" json:"key" yaml:"key"`
	Secret string `bson:"secret" json:"secret" yaml:"secret"`
}

// AWSConfig stores auth info for Amazon Web Services.
type AWSConfig struct {
	EC2Keys []EC2Key `bson:"ec2_keys" json:"ec2_keys" yaml:"ec2_keys"`

	// Legacy fields (to be removed when EC2Keys struct is set)
	EC2Key    string `bson:"aws_id" json:"aws_id" yaml:"aws_id"`
	EC2Secret string `bson:"aws_secret" json:"aws_secret" yaml:"aws_secret"`

	S3Key     string `bson:"s3_key" json:"s3_key" yaml:"s3_key"`
	S3Secret  string `bson:"s3_secret" json:"s3_secret" yaml:"s3_secret"`
	Bucket    string `bson:"bucket" json:"bucket" yaml:"bucket"`
	S3BaseURL string `bson:"s3_base_url" json:"s3_base_url" yaml:"s3_base_url"`
}

// DockerConfig stores auth info for Docker.
type DockerConfig struct {
	APIVersion string `bson:"api_version" json:"api_version" yaml:"api_version"`
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
