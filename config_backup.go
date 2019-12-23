package evergreen

import (
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type BackupConfig struct {
	BucketName string `bson:"bucket_name" json:"bucket_name" yaml:"bucket_name"`
	Key        string `bson:"key" json:"key" yaml:"key"`
	Secret     string `bson:"secret" json:"secret" yaml:"secret"`
	Prefix     string `bson:"prefix" json:"prefix" yaml:"prefix"`
	Compress   bool   `bson:"compress" json:"compress" yaml:"compress"`
}

func (c *BackupConfig) SectionId() string         { return "backup" }
func (c *BackupConfig) ValidateAndDefault() error { return nil }

func (c *BackupConfig) Set() error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	_, err := coll.UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"bucket_name": c.BucketName,
			"key":         c.Key,
			"secret":      c.Secret,
			"compress":    c.Compress,
			"prefix":      c.Prefix,
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

func (c *BackupConfig) Get(env Environment) error {
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	res := coll.FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
	}

	if err := res.Decode(c); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = BackupConfig{}
			return nil
		}

		return errors.Wrap(err, "problem decoding result")
	}

	return nil
}
