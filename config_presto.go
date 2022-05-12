package evergreen

import (
	"database/sql"
	"fmt"
	"net/url"
	"sync"

	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"github.com/prestodb/presto-go-client/presto"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type PrestoConfig struct {
	BaseURI           string            `bson:"base_uri" json:"base_uri" yaml:"base_uri"`
	TLS               bool              `bson:"tls" json:"tls" yaml:"tls"`
	Port              int               `bson:"port" json:"port" yaml:"port"`
	Username          string            `bson:"username" json:"username" yaml:"username"`
	Password          string            `bson:"password" json:"password" yaml:"password"`
	Source            string            `bson:"source" json:"source" yaml:"source"`
	Catalog           string            `bson:"catalog" json:"catalog" yaml:"catalog"`
	Schema            string            `bson:"schema" json:"schema" yaml:"schema"`
	SessionProperties map[string]string `bson:"session_properties" json:"session_properties" yaml:"session_properties"`

	mu sync.Mutex `bson:"mu" json:"mu" yaml:"mu"`
	db *sql.DB    `bson:"db" json:"db" yaml:"db"`
}

var (
	PrestoConfigBaseURIKey           = bsonutil.MustHaveTag(PrestoConfig{}, "BaseURI")
	PrestoConfigPortKey              = bsonutil.MustHaveTag(PrestoConfig{}, "Port")
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
			return errors.New("must set Presto config")
		}
		return errors.Wrapf(err, "retrieving section '%s'", c.SectionId())
	}

	if err := res.Decode(c); err != nil {
		return errors.Wrapf(err, "decoding section '%s'", c.SectionId())
	}

	if err := c.setupDB(); err != nil {
		grip.Alert(message.WrapError(err, message.Fields{
			"message": "setting up Presto DB client",
		}))
	}
	return c.setupDB()
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

func (c *PrestoConfig) DB() *sql.DB {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.db
}

func (c *PrestoConfig) setupDB() error {
	dsnConfig := presto.Config{
		PrestoURI:         c.formatURI(),
		Source:            c.Source,
		Catalog:           c.Catalog,
		Schema:            c.Schema,
		SessionProperties: c.SessionProperties,
	}
	dsn, err := dsnConfig.FormatDSN()
	if err != nil {
		return errors.Wrap(err, "formatting Presto DSN")
	}

	c.db, err = sql.Open("presto", dsn)
	return errors.Wrap(err, "opening Presto connection")
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
