package evergreen

import (
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type LoggerConfig struct {
	Buffer         LogBuffering `bson:"buffer" json:"buffer" yaml:"buffer"`
	DefaultLevel   string       `bson:"default_level" json:"default_level" yaml:"default_level"`
	ThresholdLevel string       `bson:"threshold_level" json:"threshold_level" yaml:"threshold_level"`
	LogkeeperURL   string       `bson:"logkeeper_url" json:"logkeeper_url" yaml:"logkeeper_url"`
	DefaultLogger  string       `bson:"default_logger" json:"default_logger" yaml:"default_logger"`
}

func (c LoggerConfig) Info() send.LevelInfo {
	return send.LevelInfo{
		Default:   level.FromString(c.DefaultLevel),
		Threshold: level.FromString(c.ThresholdLevel),
	}
}

func (c *LoggerConfig) SectionId() string { return "logger_config" }

func (c *LoggerConfig) Get(env Environment) error {
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	res := coll.FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = LoggerConfig{}
			return nil
		}
		return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
	}

	if err := res.Decode(c); err != nil {
		return errors.Wrap(err, "problem decoding result")
	}

	return nil
}

func (c *LoggerConfig) Set() error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	_, err := coll.UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"buffer":          c.Buffer,
			"default_level":   c.DefaultLevel,
			"threshold_level": c.ThresholdLevel,
			"logkeeper_url":   c.LogkeeperURL,
			"default_logger":  c.DefaultLogger,
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

func (c *LoggerConfig) ValidateAndDefault() error {
	if c.Buffer.DurationSeconds == 0 {
		c.Buffer.DurationSeconds = defaultLogBufferingDuration
	}

	if c.DefaultLevel == "" {
		c.DefaultLevel = "info"
	}

	if c.ThresholdLevel == "" {
		c.ThresholdLevel = "debug"
	}

	info := c.Info()
	if !info.Valid() {
		return errors.Errorf("logging level configuration is not valid [%+v]", info)
	}

	return nil
}

type LogBuffering struct {
	DurationSeconds int `bson:"duration_seconds" json:"duration_seconds" yaml:"duration_seconds"`
	Count           int `bson:"count" json:"count" yaml:"count"`
}
