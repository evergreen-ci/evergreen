package evergreen

import (
	"context"

	"github.com/mongodb/grip"
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

func (c *LoggerConfig) Get(ctx context.Context) error {
	res := GetEnvironment().DB().Collection(ConfigCollection).FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = LoggerConfig{}
			return nil
		}
		return errors.Wrapf(err, "getting config section '%s'", c.SectionId())
	}

	if err := res.Decode(c); err != nil {
		return errors.Wrapf(err, "decoding config section '%s'", c.SectionId())
	}

	return nil
}

func (c *LoggerConfig) Set(ctx context.Context) error {
	_, err := GetEnvironment().DB().Collection(ConfigCollection).UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"buffer":          c.Buffer,
			"default_level":   c.DefaultLevel,
			"threshold_level": c.ThresholdLevel,
			"logkeeper_url":   c.LogkeeperURL,
			"default_logger":  c.DefaultLogger,
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "updating config section '%s'", c.SectionId())
}

func (c *LoggerConfig) ValidateAndDefault() error {
	if c.DefaultLevel == "" {
		c.DefaultLevel = "info"
	}

	if c.ThresholdLevel == "" {
		c.ThresholdLevel = "debug"
	}

	catcher := grip.NewBasicCatcher()
	catcher.Add(c.Buffer.validateAndDefault())

	info := c.Info()
	if !info.Valid() {
		catcher.Errorf("logging level configuration is not valid [%+v]", info)
	}

	return catcher.Resolve()
}

type LogBuffering struct {
	UseAsync             bool `bson:"use_async" json:"use_async" yaml:"use_async"`
	DurationSeconds      int  `bson:"duration_seconds" json:"duration_seconds" yaml:"duration_seconds"`
	Count                int  `bson:"count" json:"count" yaml:"count"`
	IncomingBufferFactor int  `bson:"incoming_buffer_factor" json:"incoming_buffer_factor" yaml:"incoming_buffer_factor"`
}

func (b *LogBuffering) validateAndDefault() error {
	catcher := grip.NewBasicCatcher()

	if b.DurationSeconds < 0 {
		catcher.New("buffering duration seconds can not be negative")
	} else if b.DurationSeconds == 0 {
		b.DurationSeconds = defaultLogBufferingDuration
	}

	if b.Count < 0 {
		catcher.New("buffering count can not be negative")
	} else if b.Count == 0 {
		b.Count = defaultLogBufferingCount
	}

	if b.UseAsync {
		if b.IncomingBufferFactor < 0 {
			catcher.New("incoming buffer factor can not be negative")
		} else if b.IncomingBufferFactor == 0 {
			b.IncomingBufferFactor = defaultLogBufferingIncomingFactor
		}
	}

	return catcher.Resolve()
}
