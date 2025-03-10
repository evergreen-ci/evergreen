package evergreen

import (
	"context"

	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type LoggerConfig struct {
	Buffer         LogBuffering `bson:"buffer" json:"buffer" yaml:"buffer"`
	DefaultLevel   string       `bson:"default_level" json:"default_level" yaml:"default_level"`
	ThresholdLevel string       `bson:"threshold_level" json:"threshold_level" yaml:"threshold_level"`
	LogkeeperURL   string       `bson:"logkeeper_url" json:"logkeeper_url" yaml:"logkeeper_url"`
	RedactKeys     []string     `bson:"redact_keys" json:"redact_keys" yaml:"redact_keys"`
}

var (
	bufferKey         = bsonutil.MustHaveTag(LoggerConfig{}, "Buffer")
	defaultLevelKey   = bsonutil.MustHaveTag(LoggerConfig{}, "DefaultLevel")
	thresholdLevelKey = bsonutil.MustHaveTag(LoggerConfig{}, "ThresholdLevel")
	logkeeperURLKey   = bsonutil.MustHaveTag(LoggerConfig{}, "LogkeeperURL")
	redactKeysKey     = bsonutil.MustHaveTag(LoggerConfig{}, "RedactKeys")
)

func (c LoggerConfig) Info() send.LevelInfo {
	return send.LevelInfo{
		Default:   level.FromString(c.DefaultLevel),
		Threshold: level.FromString(c.ThresholdLevel),
	}
}

func (c *LoggerConfig) SectionId() string { return "logger_config" }

func (c *LoggerConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

func (c *LoggerConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			bufferKey:         c.Buffer,
			defaultLevelKey:   c.DefaultLevel,
			thresholdLevelKey: c.ThresholdLevel,
			logkeeperURLKey:   c.LogkeeperURL,
			redactKeysKey:     c.RedactKeys,
		}}), "updating config section '%s'", c.SectionId(),
	)
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
