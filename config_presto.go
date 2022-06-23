package evergreen

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"time"

	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"github.com/trinodb/trino-go-client/trino"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// PrestoConfig represents configuration information for the application level
// Presto DB connection.
type PrestoConfig struct {
	BaseURI           string            `bson:"base_uri" json:"base_uri" yaml:"base_uri"`
	Port              int               `bson:"port" json:"port" yaml:"port"`
	TLS               bool              `bson:"tls" json:"tls" yaml:"tls"`
	Username          string            `bson:"username" json:"username" yaml:"username"`
	Password          string            `bson:"password" json:"password" yaml:"password"`
	Source            string            `bson:"source" json:"source" yaml:"source"`
	Catalog           string            `bson:"catalog" json:"catalog" yaml:"catalog"`
	Schema            string            `bson:"schema" json:"schema" yaml:"schema"`
	SessionProperties map[string]string `bson:"session_properties" json:"session_properties" yaml:"session_properties"`

	db *sql.DB
}

var (
	PrestoConfigBaseURIKey           = bsonutil.MustHaveTag(PrestoConfig{}, "BaseURI")
	PrestoConfigPortKey              = bsonutil.MustHaveTag(PrestoConfig{}, "Port")
	PrestoConfigTLSKey               = bsonutil.MustHaveTag(PrestoConfig{}, "TLS")
	PrestoConfigUsernameKey          = bsonutil.MustHaveTag(PrestoConfig{}, "Username")
	PrestoConfigPasswordKey          = bsonutil.MustHaveTag(PrestoConfig{}, "Password")
	PrestoConfigSourceKey            = bsonutil.MustHaveTag(PrestoConfig{}, "Source")
	PrestoConfigCatalogKey           = bsonutil.MustHaveTag(PrestoConfig{}, "Catalog")
	PrestoConfigSchemaKey            = bsonutil.MustHaveTag(PrestoConfig{}, "Schema")
	PrestoConfigSessionPropertiesKey = bsonutil.MustHaveTag(PrestoConfig{}, "SessionProperties")
)

func (*PrestoConfig) SectionId() string { return "presto" }

func (c *PrestoConfig) Get(env Environment) error {
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	res := coll.FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = PrestoConfig{}
			return nil
		}
		return errors.Wrapf(err, "retrieving section '%s'", c.SectionId())
	}

	if err := res.Decode(c); err != nil {
		return errors.Wrapf(err, "decoding section '%s'", c.SectionId())
	}

	if err := c.setupDB(ctx); err != nil {
		grip.Alert(message.WrapError(err, message.Fields{
			"message": "setting up Presto DB client",
		}))
	}

	return nil
}

func (c *PrestoConfig) Set() error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	_, err := coll.UpdateOne(ctx, byId(c.SectionId()), bson.M{"$set": c}, options.Update().SetUpsert(true))
	return errors.Wrapf(err, "updating section '%s'", c.SectionId())
}

func (*PrestoConfig) ValidateAndDefault() error { return nil }

func (c *PrestoConfig) DB() *sql.DB { return c.db }

func (c *PrestoConfig) setupDB(ctx context.Context) error {
	dsnConfig := trino.Config{
		ServerURI:         c.formatURI(),
		Source:            c.Source,
		Catalog:           c.Catalog,
		Schema:            c.Schema,
		SessionProperties: c.SessionProperties,
	}
	dsn, err := dsnConfig.FormatDSN()
	if err != nil {
		return errors.Wrap(err, "formatting Presto DSN")
	}

	c.db, err = sql.Open("trino", dsn)
	if err != nil {
		return errors.Wrap(err, "opening Presto connection")
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return c.db.PingContext(ctx)
}

func (c *PrestoConfig) formatURI() string {
	var scheme string
	if c.TLS {
		scheme = "https"
	} else {
		scheme = "http"
	}

	// URI has format `http[s]://username[:password]@host[:port]`.
	return fmt.Sprintf(
		"%s://%s:%s@%s:%d",
		scheme,
		url.PathEscape(c.Username),
		url.PathEscape(c.Password),
		c.BaseURI,
		c.Port,
	)
}
