package credentials

import (
	"context"
	"time"

	"github.com/evergreen-ci/certdepot"
	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/grip"
	"github.com/mongodb/jasper/rpc"
	"github.com/pkg/errors"
	"github.com/square/certstrap/depot"
)

const (
	Collection  = "credentials"
	CAName      = "evergreen"
	ServiceName = "evergreen.mongodb.com"

	defaultConnectionTimeout   = 10 * time.Second
	defaultCertificateDuration = 7 * 24 * time.Hour // 1 week
)

// init performs one-time initialization of the cert collection with the
// certificate authority and evergreen service certificate.
func init() {
	env := evergreen.GetEnvironment()
	settings := env.Settings()
	maxExpiration := time.Duration(1<<63 - 1)

	caConfig := certdepot.CertificateOptions{
		CA:         CAName,
		CommonName: CAName,
		Expires:    maxExpiration,
	}

	serviceConfig := certdepot.CertificateOptions{
		CA:         CAName,
		CommonName: ServiceName,
		Host:       ServiceName,
		Expires:    maxExpiration,
	}

	bootstrapConfig := certdepot.BootstrapDepotConfig{
		MongoDepot:  mongoConfig(settings),
		CAName:      CAName,
		CAOpts:      &caConfig,
		ServiceName: ServiceName,
		ServiceOpts: &serviceConfig,
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultConnectionTimeout)
	defer cancel()

	if _, err := certdepot.BootstrapDepotWithMongoClient(ctx, env.Client(), bootstrapConfig); err != nil {
		grip.EmergencyFatal(errors.Wrap(err, "could not bootstrap cert collection"))
	}
}

func mongoConfig(settings *evergreen.Settings) *certdepot.MongoDBOptions {
	return &certdepot.MongoDBOptions{
		MongoDBURI:     settings.Database.Url,
		DatabaseName:   settings.Database.DB,
		CollectionName: Collection,
	}
}

// getDepot returns the certificate depot connected to the database.
func getDepot(ctx context.Context, env evergreen.Environment) (depot.Depot, error) {
	return certdepot.NewMongoDBCertDepotWithClient(ctx, env.Client(), mongoConfig(env.Settings()))
}

func deleteIfExists(dpt depot.Depot, tag *depot.Tag) error {
	if dpt.Check(tag) {
		return errors.Wrap(dpt.Delete(tag), "problem deleting existing tag")
	}
	return nil
}

// SaveCredentials saves the credentials from the options for the given user
// name. If the credentials already exist, this will overwrite the existing
// credentials.
func SaveCredentials(name string, creds *rpc.Credentials) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultConnectionTimeout)
	defer cancel()

	dpt, err := getDepot(ctx, evergreen.GetEnvironment())
	if err != nil {
		return errors.Wrap(err, "could not get depot")
	}

	if err := deleteIfExists(dpt, depot.PrivKeyTag(name)); err != nil {
		return errors.Wrap(err, "problem deleting existing key")
	}
	if err := deleteIfExists(dpt, depot.CrtTag(name)); err != nil {
		return errors.Wrap(err, "problem deleting existing certificate")
	}

	if err := dpt.Put(depot.PrivKeyTag(name), creds.Key); err != nil {
		return errors.Wrap(err, "problem saving key")
	}
	if err := dpt.Put(depot.CrtTag(name), creds.Cert); err != nil {
		return errors.Wrap(err, "problem saving certificate")
	}
	return nil
}

// GenerateInMemory generates the credentials without storing them in the
// database.
func GenerateInMemory(name string) (*rpc.Credentials, error) {
	opts := certdepot.CertificateOptions{
		CA:         CAName,
		CommonName: name,
		Host:       name,
		Expires:    defaultCertificateDuration,
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultConnectionTimeout)
	defer cancel()

	dpt, err := getDepot(ctx, evergreen.GetEnvironment())
	if err != nil {
		return nil, errors.Wrap(err, "could not get depot")
	}

	pemCACrt, err := dpt.Get(depot.CrtTag(CAName))
	if err != nil {
		return nil, errors.Wrap(err, "problem getting CA certificate")
	}

	_, key, err := opts.CertRequestInMemory(dpt)
	if err != nil {
		return nil, errors.Wrap(err, "problem making certificate request and key")
	}

	pemKey, err := key.ExportPrivate()
	if err != nil {
		return nil, errors.Wrap(err, "problem exporting key")
	}

	crt, err := opts.SignInMemory(dpt)
	if err != nil {
		return nil, errors.Wrap(err, "problem signing certificate request")
	}

	pemCrt, err := crt.Export()
	if err != nil {
		return nil, errors.Wrap(err, "problem exporting certificate")
	}

	return rpc.NewCredentials(pemCACrt, pemCrt, pemKey)
}

// FindById gets the credentials for the given name.
func FindById(name string) (*rpc.Credentials, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultConnectionTimeout)
	defer cancel()

	dpt, err := getDepot(ctx, evergreen.GetEnvironment())
	if err != nil {
		return nil, errors.Wrap(err, "could not get depot")
	}

	caCrt, err := dpt.Get(depot.CrtTag(CAName))
	if err != nil {
		return nil, errors.Wrap(err, "problem getting CA certificate")
	}

	crt, err := dpt.Get(depot.CrtTag(name))
	if err != nil {
		return nil, errors.Wrap(err, "problem getting certificate")
	}

	key, err := dpt.Get(depot.PrivKeyTag(name))
	if err != nil {
		return nil, errors.Wrap(err, "problem getting key")
	}

	return rpc.NewCredentials(caCrt, crt, key)
}

// DeleteCredentials removes the credentials from the database.
func DeleteCredentials(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultConnectionTimeout)
	defer cancel()

	dpt, err := getDepot(ctx, evergreen.GetEnvironment())
	if err != nil {
		return errors.Wrap(err, "could not get depot")
	}

	catcher := grip.NewBasicCatcher()

	catcher.Wrap(deleteIfExists(dpt, depot.PrivKeyTag(name)), "problem deleting key")
	catcher.Wrap(deleteIfExists(dpt, depot.CrtTag(name)), "problem deleting certificate")

	return catcher.Resolve()
}
