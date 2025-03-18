package evergreen

import (
	"context"

	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

type SSHConfig struct {
	TaskHostKey  SSHKeyPair `bson:"task_host_key" json:"task_host_key" yaml:"task_host_key"`
	SpawnHostKey SSHKeyPair `bson:"spawn_host_key" json:"spawn_host_key" yaml:"spawn_host_key"`
}

type SSHKeyPair struct {
	Name      string `yaml:"name" bson:"name" json:"name"`
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
