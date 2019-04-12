package evergreen

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

type LoggerConfig struct {
	Buffer         LogBuffering `bson:"buffer" json:"buffer" yaml:"buffer"`
	DefaultLevel   string       `bson:"default_level" json:"default_level" yaml:"default_level"`
	ThresholdLevel string       `bson:"threshold_level" json:"threshold_level" yaml:"threshold_level"`
	LogkeeperURL   string       `bson:"logkeeper_url" json:"logkeeper_url" yaml:"logkeeper_url"`
}

func (c LoggerConfig) Info() send.LevelInfo {
	return send.LevelInfo{
		Default:   level.FromString(c.DefaultLevel),
		Threshold: level.FromString(c.ThresholdLevel),
	}
}

func (c *LoggerConfig) SectionId() string { return "logger_config" }

func (c *LoggerConfig) Get() error {
	err := db.FindOneQ(ConfigCollection, db.Query(byId(c.SectionId())), c)
	if err != nil && err.Error() == errNotFound {
		*c = LoggerConfig{}
		return nil
	}
	return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
}

func (c *LoggerConfig) Set() error {
	_, err := db.Upsert(ConfigCollection, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"buffer":          c.Buffer,
			"default_level":   c.DefaultLevel,
			"threshold_level": c.ThresholdLevel,
			"logkeeper_url":   c.LogkeeperURL,
		},
	})
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
