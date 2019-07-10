package certdepot

import (
	"time"

	"github.com/mongodb/anser/bsonutil"
)

// User stores information for a user in the mongo certificate depot.
type User struct {
	ID            string    `bson:"_id"`
	Cert          string    `bson:"cert"`
	PrivateKey    string    `bson:"private_key"`
	CertReq       string    `bson:"cert_req"`
	CertRevocList string    `bson:"cert_revoc_list"`
	TTL           time.Time `bson:"ttl,omitempty"`
}

var (
	userIDKey            = bsonutil.MustHaveTag(User{}, "ID")
	userCertKey          = bsonutil.MustHaveTag(User{}, "Cert")
	userPrivateKeyKey    = bsonutil.MustHaveTag(User{}, "PrivateKey")
	userCertReqKey       = bsonutil.MustHaveTag(User{}, "CertReq")
	userCertRevocListKey = bsonutil.MustHaveTag(User{}, "CertRevocList")
	userTTLKey           = bsonutil.MustHaveTag(User{}, "TTL")
)

// MongoDBOptions conatins options for NewMongoDBCertDepot,
// NewMongoDBCertDepotWithClient, NewMgoCertDepot, and
// NewMgoCertDepotWithSession.
type MongoDBOptions struct {
	MongoDBURI           string        `bson:"mongodb_uri" json:"mongodb_uri" yaml:"mongodb_uri"`
	DatabaseName         string        `bson:"db_name" json:"db_name" yaml:"db_name"`
	CollectionName       string        `bson:"coll_name" json:"coll_name" yaml:"coll_name"`
	MongoDBDialTimeout   time.Duration `bson:"dial_timeout,omitempty" json:"dial_timeout,omitempty" yaml:"dial_timeout,omitempty"`
	MongoDBSocketTimeout time.Duration `bson:"socket_timeout,omitempty" json:"socket_timeout,omitempty" yaml:"socket_timeout,omitempty"`
}

// IsZero returns whether the given MongoDBOptions struct holds the "zero"
// value of the struct.
func (opts *MongoDBOptions) IsZero() bool {
	if opts.DatabaseName == "" && opts.CollectionName == "" {
		return true
	}

	return false
}

func (opts *MongoDBOptions) validate() error {
	if opts.MongoDBURI == "" {
		opts.MongoDBURI = "mongodb://localhost:27017"
	}
	if opts.MongoDBDialTimeout <= 0 {
		opts.MongoDBDialTimeout = 2 * time.Second
	}
	if opts.MongoDBSocketTimeout <= 0 {
		opts.MongoDBSocketTimeout = time.Minute
	}
	if opts.DatabaseName == "" {
		opts.DatabaseName = "certDepot"
	}
	if opts.CollectionName == "" {
		opts.CollectionName = "certs"
	}

	return nil
}
