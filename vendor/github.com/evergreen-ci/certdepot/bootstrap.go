package certdepot

import (
	"context"

	"github.com/pkg/errors"
	"github.com/square/certstrap/depot"
	"go.mongodb.org/mongo-driver/mongo"
)

// BootstrapDepotConfig contains options for BootstrapDepot. Must provide
// either the name of the FileDepot or the MongoDepot options, not both or
// neither.
type BootstrapDepotConfig struct {
	// Name of FileDepot (directory). If a MongoDepot is desired, leave
	// empty.
	FileDepot string `bson:"file_depot,omitempty" json:"file_depot,omitempty" yaml:"file_depot,omitempty"`
	// Options for setting up a MongoDepot. If a FileDepot is desired,
	// leave pointer nil or the struct empty.
	MongoDepot *MongoDBOptions `bson:"mongo_depot,omitempty" json:"mongo_depot,omitempty" yaml:"mongo_depot,omitempty"`
	// CA certificate, this is optional unless CAKey is not empty, in
	// which case a CA certificate must also be provided.
	CACert string `bson:"ca_cert" json:"ca_cert" yaml:"ca_cert"`
	// CA key, this is optional unless CACert is not empty, in which case
	// a CA key must also be provided.
	CAKey string `bson:"ca_key" json:"ca_key" yaml:"ca_key"`
	// Common name of the CA (required).
	CAName string `bson:"ca_name" json:"ca_name" yaml:"ca_name"`
	// Common name of the service (required).
	ServiceName string `bson:"service_name" json:"service_name" yaml:"service_name"`
	// Options to initialize a CA. This is optional and only used if there
	// is no existing `CAName` in the depot and `CACert` is empty.
	// `CAOpts.CommonName` must equal `CAName`.
	CAOpts *CertificateOptions `bson:"ca_opts,omitempty" json:"ca_opts,omitempty" yaml:"ca_opts,omitempty"`
	// Options to create a service certificate. This is optional and only
	// used if there is no existing `ServiceName` in the depot.
	// `ServiceOpts.CommonName` must equal `ServiceName`.
	// `ServiceOpts.CA` must equal `CAName`.
	ServiceOpts *CertificateOptions `bson:"service_opts,omitempty" json:"service_opts,omitempty" yaml:"service_opts,omitempty"`
}

// Validate ensures that the BootstrapDepotConfig is configured correctly.
func (c *BootstrapDepotConfig) Validate() error {
	if c.FileDepot != "" && c.MongoDepot != nil && !c.MongoDepot.IsZero() {
		return errors.New("cannot specify more than one depot configuration")
	}

	if c.FileDepot == "" && (c.MongoDepot == nil || c.MongoDepot.IsZero()) {
		return errors.New("must specify one depot configuration")
	}

	if c.CAName == "" || c.ServiceName == "" {
		return errors.New("must specify the name of the CA and service")
	}

	if (c.CACert != "" && c.CAKey == "") || (c.CACert == "" && c.CAKey != "") {
		return errors.New("must provide both cert and key file if want to bootstrap with existing CA")
	}

	if c.CAOpts != nil && c.CAOpts.CommonName != c.CAName {
		return errors.New("CAName and CAOpts.CommonName must be the same")
	}

	if c.ServiceOpts != nil && c.ServiceOpts.CommonName != c.ServiceName {
		return errors.New("ServiceName and ServiceOpts.CommonName must be the same")
	}

	if c.ServiceOpts != nil && c.ServiceOpts.CA != c.CAName {
		return errors.New("CAName and ServiceOpts.CA must be the same")
	}

	return nil
}

// BootstrapDepot creates a certificate depot with a CA and service
// certificate.
func BootstrapDepot(ctx context.Context, conf BootstrapDepotConfig) (Depot, error) {
	return BootstrapDepotWithMongoClient(ctx, nil, conf)
}

// BootstrapDepotWithMongoClient creates a certificate depot with a CA and
// service certificate using the provided mongo driver client.
func BootstrapDepotWithMongoClient(ctx context.Context, client *mongo.Client, conf BootstrapDepotConfig) (Depot, error) {
	d, err := CreateDepot(ctx, client, conf)
	if err != nil {
		return nil, errors.Wrap(err, "problem creating depot")
	}

	if conf.CACert != "" {
		if err = addCert(d, conf); err != nil {
			return nil, errors.Wrap(err, "problem adding a ca cert")
		}
	}
	if !depot.CheckCertificate(d, conf.CAName) {
		if err = createCA(d, conf); err != nil {
			return nil, errors.Wrap(err, "problem during certificate creation")
		}
	} else if !depot.CheckCertificate(d, conf.ServiceName) {
		if err = createServerCert(d, conf); err != nil {
			return nil, errors.Wrap(err, "problem checking the service certificate")
		}
	}

	return d, nil

}

// CreateDepot creates a certificate depot with the given BootstrapDepotConfig.
// If a mongo client is passed in it will be used to create the mongo depot.
func CreateDepot(ctx context.Context, client *mongo.Client, conf BootstrapDepotConfig) (Depot, error) {
	var d Depot
	var err error

	if err = conf.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid configuration")
	}

	if conf.FileDepot != "" {
		d, err = NewFileDepot(conf.FileDepot)
		if err != nil {
			return nil, errors.Wrap(err, "problem initializing the file deopt")
		}
	} else if !conf.MongoDepot.IsZero() {
		if client != nil {
			d, err = NewMongoDBCertDepotWithClient(ctx, client, conf.MongoDepot)
		} else {
			d, err = NewMongoDBCertDepot(ctx, conf.MongoDepot)
		}
		if err != nil {
			return nil, errors.Wrap(err, "problem initializing the mongo depot")
		}
	}

	return d, nil
}

func addCert(d depot.Depot, conf BootstrapDepotConfig) error {
	if err := d.Put(depot.CrtTag(conf.CAName), []byte(conf.CACert)); err != nil {
		return errors.Wrap(err, "problem adding CA cert to depot")
	}

	if err := d.Put(depot.PrivKeyTag(conf.CAName), []byte(conf.CAKey)); err != nil {
		return errors.Wrap(err, "problem adding CA key to depot")
	}

	return nil
}

func createCA(d depot.Depot, conf BootstrapDepotConfig) error {
	if conf.CAOpts == nil {
		return errors.New("cannot create a new CA with nil CA options")
	}
	if err := conf.CAOpts.Init(d); err != nil {
		return errors.Wrap(err, "problem initializing the ca")
	}
	if err := createServerCert(d, conf); err != nil {
		return errors.Wrap(err, "problem creating the server cert")
	}

	return nil
}

func createServerCert(d depot.Depot, conf BootstrapDepotConfig) error {
	if conf.ServiceOpts == nil {
		return errors.New("cannot create a new server cert with nil service options")
	}
	if err := conf.ServiceOpts.CertRequest(d); err != nil {
		return errors.Wrap(err, "problem creating service cert request")
	}
	if err := conf.ServiceOpts.Sign(d); err != nil {
		return errors.Wrap(err, "problem signing service key")
	}

	return nil
}
