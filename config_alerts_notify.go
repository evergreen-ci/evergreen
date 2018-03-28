package evergreen

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

// NotifyConfig hold logging and email settings for the notify package.
type NotifyConfig struct {
	SMTP *SMTPConfig `bson:"smtp" json:"smtp" yaml:"smtp"`
}

func (c *NotifyConfig) SectionId() string { return "notify" }

func (c *NotifyConfig) Get() error {
	err := db.FindOneQ(ConfigCollection, db.Query(byId(c.SectionId())), c)
	if err != nil && err.Error() == errNotFound {
		*c = NotifyConfig{}
		return nil
	}
	return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
}

func (c *NotifyConfig) Set() error {
	_, err := db.Upsert(ConfigCollection, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"smtp": c.SMTP,
		},
	})
	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

func (c *NotifyConfig) ValidateAndDefault() error {
	notifyConfig := c.SMTP

	if notifyConfig == nil {
		return nil
	}

	if notifyConfig.Server == "" || notifyConfig.Port == 0 {
		return errors.New("You must specify a SMTP server and port")
	}

	if len(notifyConfig.AdminEmail) == 0 {
		return errors.New("You must specify at least one admin_email")
	}

	if notifyConfig.From == "" {
		return errors.New("You must specify a from address")
	}

	return nil
}

type AlertsConfig struct {
	SMTP *SMTPConfig `bson:"smtp" json:"smtp" yaml:"smtp"`
}

func (c *AlertsConfig) SectionId() string { return "alerts" }

func (c *AlertsConfig) Get() error {
	err := db.FindOneQ(ConfigCollection, db.Query(byId(c.SectionId())), c)
	if err != nil && err.Error() == errNotFound {
		*c = AlertsConfig{}
		return nil
	}
	return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
}

func (c *AlertsConfig) Set() error {
	_, err := db.Upsert(ConfigCollection, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"smtp": c.SMTP,
		},
	})
	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

func (c *AlertsConfig) ValidateAndDefault() error { return nil }

// SMTPConfig holds SMTP email settings.
type SMTPConfig struct {
	Server     string   `bson:"server" json:"server" yaml:"server"`
	Port       int      `bson:"port" json:"port" yaml:"port"`
	UseSSL     bool     `bson:"use_ssl" json:"use_ssl" yaml:"use_ssl"`
	Username   string   `bson:"username" json:"username" yaml:"username"`
	Password   string   `bson:"password" json:"password" yaml:"password"`
	From       string   `bson:"from" json:"from" yaml:"from"`
	AdminEmail []string `bson:"admin_email" json:"admin_email" yaml:"admin_email"`
}
