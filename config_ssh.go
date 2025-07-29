package evergreen

import (
	"context"

	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

type SSHConfig struct {
	// TaskHostKey is the key pair used for SSHing onto task hosts.
	TaskHostKey SSHKeyPair `bson:"task_host_key" json:"task_host_key" yaml:"task_host_key"`
	// SpawnHostKey is the key pair used for SSHing onto spawn hosts.
	SpawnHostKey SSHKeyPair `bson:"spawn_host_key" json:"spawn_host_key" yaml:"spawn_host_key"`
}

type SSHKeyPair struct {
	// Name corresponds to the name of the public key for this key pair.
	// Public keys in EC2 are referred to by names: https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-key-pairs.html
	Name string `yaml:"name" bson:"name" json:"name"`
	// SecretARN corresponds to a secret in AWS Secrets Manager that contains the private key for this key pair.
	// This variable does not need to be a secret because it only contains the ARN that points to the secret.
	SecretARN string `yaml:"secret_arn" bson:"secret_arn" json:"secret_arn"`
}

func (c *SSHConfig) SectionId() string { return "ssh" }

func (c *SSHConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

var (
	taskHostKeyKey  = bsonutil.MustHaveTag(SSHConfig{}, "TaskHostKey")
	spawnHostKeyKey = bsonutil.MustHaveTag(SSHConfig{}, "SpawnHostKey")
)

func (c *SSHConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			taskHostKeyKey:  c.TaskHostKey,
			spawnHostKeyKey: c.SpawnHostKey,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *SSHConfig) ValidateAndDefault() error {
	catcher := grip.NewBasicCatcher()
	catcher.Wrap(c.TaskHostKey.validate(), "validating task host key")
	catcher.Wrap(c.SpawnHostKey.validate(), "validating spawn host key")
	return catcher.Resolve()
}

func (c *SSHKeyPair) validate() error {
	if c.Name != "" && c.SecretARN == "" {
		return errors.New("secret ARN must be set")
	}
	if c.Name == "" && c.SecretARN != "" {
		return errors.New("key name must be set")
	}

	return nil
}
