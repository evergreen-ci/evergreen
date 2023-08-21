package evergreen

import (
	"context"

	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// HostJasperConfig represents the configuration of the Jasper service running
// on non-legacy hosts.
type HostJasperConfig struct {
	BinaryName       string `yaml:"binary_name" bson:"binary_name" json:"binary_name"`
	DownloadFileName string `yaml:"download_file_name" bson:"download_file_name" json:"download_file_name"`
	Port             int    `yaml:"port" bson:"port" json:"port"`
	URL              string `yaml:"url" bson:"url" json:"url"`
	Version          string `yaml:"version" bson:"version" json:"version"`
}

var (
	hostJasperBinaryNameKey       = bsonutil.MustHaveTag(HostJasperConfig{}, "BinaryName")
	hostJasperDownloadFileNameKey = bsonutil.MustHaveTag(HostJasperConfig{}, "DownloadFileName")
	hostJasperPortKey             = bsonutil.MustHaveTag(HostJasperConfig{}, "Port")
	hostJasperURLKey              = bsonutil.MustHaveTag(HostJasperConfig{}, "URL")
	hostJasperVersionKey          = bsonutil.MustHaveTag(HostJasperConfig{}, "Version")
)

func (c *HostJasperConfig) SectionId() string { return "host_jasper" }

func (c *HostJasperConfig) Get(ctx context.Context) error {
	res := GetEnvironment().DB().Collection(ConfigCollection).FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = HostJasperConfig{}
			return nil
		}
		return errors.Wrapf(err, "getting config section '%s'", c.SectionId())
	}

	if err := res.Decode(&c); err != nil {
		return errors.Wrapf(err, "decoding config section '%s'", c.SectionId())
	}

	return nil
}

func (c *HostJasperConfig) Set(ctx context.Context) error {
	_, err := GetEnvironment().DB().Collection(ConfigCollection).UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			hostJasperBinaryNameKey:       c.BinaryName,
			hostJasperDownloadFileNameKey: c.DownloadFileName,
			hostJasperPortKey:             c.Port,
			hostJasperURLKey:              c.URL,
			hostJasperVersionKey:          c.Version,
		},
	}, options.Update().SetUpsert(true))
	return errors.Wrapf(err, "updating config section '%s'", c.SectionId())
}

func (c *HostJasperConfig) ValidateAndDefault() error {
	if c.Port == 0 {
		c.Port = DefaultJasperPort
	} else if c.Port < 0 {
		return errors.Errorf("Jasper port must be a positive integer")
	}
	return nil
}
