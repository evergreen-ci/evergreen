package credentials

import (
	"context"
	"math"
	"time"

	"github.com/evergreen-ci/certdepot"
	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/grip"
	"github.com/mongodb/jasper/rpc"
	"github.com/pkg/errors"
	"github.com/square/certstrap/depot"
)

const (
	Collection = "credentials"
	CAName     = "evergreen"

	defaultExpiration = 30 * 24 * time.Hour // 1 month
)

var (
	serviceName        string
	errNotBootstrapped = errors.Errorf("%s collection has not been bootstrapped", Collection)
)

// Bootstrap performs one-time initialization of the credentials collection with
// the certificate authority and service certificate. In order to perform
// operations on this collection, collection, this must succeed.
func Bootstrap(env evergreen.Environment) error {
	settings := env.Settings()
	if settings.DomainName == "" {
		return errors.Errorf("bootstrapping %s collection requires domain name to be set in admin settings", settings.DomainName)
	}

	maxExpiration := time.Duration(math.MaxInt64)
	serviceConfig := certdepot.CertificateOptions{
		CA:         CAName,
		CommonName: settings.DomainName,
		Host:       settings.DomainName,
		Expires:    maxExpiration,
	}

	caConfig := certdepot.CertificateOptions{
		CA:         CAName,
		CommonName: CAName,
		Expires:    maxExpiration,
	}

	bootstrapConfig := certdepot.BootstrapDepotConfig{
		MongoDepot:  mongoConfig(settings),
		CAName:      CAName,
		CAOpts:      &caConfig,
		ServiceName: settings.DomainName,
		ServiceOpts: &serviceConfig,
	}

	ctx, cancel := env.Context()
	defer cancel()

	if _, err := certdepot.BootstrapDepotWithMongoClient(ctx, env.Client(), bootstrapConfig); err != nil {
		return errors.Wrapf(err, "could not bootstrap %s collection", Collection)
	}

	serviceName = settings.DomainName

	return nil
}

func mongoConfig(settings *evergreen.Settings) *certdepot.MongoDBOptions {
	return &certdepot.MongoDBOptions{
		MongoDBURI:     settings.Database.Url,
		DatabaseName:   settings.Database.DB,
		CollectionName: Collection,
	}
}

// getDepot returns the certificate depot connected to the database.
func getDepot(ctx context.Context, env evergreen.Environment) (certdepot.Depot, error) {
	return certdepot.NewMongoDBCertDepotWithClient(ctx, env.Client(), mongoConfig(env.Settings()))
}

func deleteIfExists(dpt certdepot.Depot, tags ...*depot.Tag) error {
	catcher := grip.NewBasicCatcher()
	for _, tag := range tags {
		if dpt.Check(tag) {
			catcher.Add(dpt.Delete(tag))
		}
	}
	return catcher.Resolve()
}

// SaveCredentials saves the credentials from the options for the given user
// name. If the credentials already exist, this will overwrite the existing
// credentials.
func SaveCredentials(ctx context.Context, env evergreen.Environment, name string, creds *rpc.Credentials) error {
	dpt, err := getDepot(ctx, env)
	if err != nil {
		return errors.Wrap(err, "could not get depot")
	}

	if err := deleteIfExists(dpt, certdepot.PrivKeyTag(name), certdepot.CrtTag(name)); err != nil {
		return errors.Wrap(err, "problem deleting existing key")
	}

	if err := dpt.Put(certdepot.PrivKeyTag(name), creds.Key); err != nil {
		return errors.Wrap(err, "problem saving key")
	}
	if err := dpt.Put(certdepot.CrtTag(name), creds.Cert); err != nil {
		return errors.Wrap(err, "problem saving certificate")
	}
	return nil
}

// GenerateInMemory generates the credentials without storing them in the
// database. This is not idempotent, so separate calls with the same inputs will
// return different credentials.
func GenerateInMemory(ctx context.Context, env evergreen.Environment, name string) (*rpc.Credentials, error) {
	opts := certdepot.CertificateOptions{
		CA:         CAName,
		CommonName: name,
		Host:       name,
		Expires:    defaultExpiration,
	}

	dpt, err := getDepot(ctx, env)
	if err != nil {
		return nil, errors.Wrap(err, "could not get depot")
	}

	pemCACrt, err := dpt.Get(certdepot.CrtTag(CAName))
	if err != nil {
		return nil, errors.Wrap(err, "problem getting CA certificate")
	}

	_, key, err := opts.CertRequestInMemory()
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

// FindByID gets the credentials for the given name.
func FindByID(ctx context.Context, env evergreen.Environment, name string) (*rpc.Credentials, error) {
	dpt, err := getDepot(ctx, env)
	if err != nil {
		return nil, errors.Wrap(err, "could not get depot")
	}

	caCrt, err := dpt.Get(certdepot.CrtTag(CAName))
	if err != nil {
		return nil, errors.Wrap(err, "problem getting CA certificate")
	}

	crt, err := dpt.Get(certdepot.CrtTag(name))
	if err != nil {
		return nil, errors.Wrap(err, "problem getting certificate")
	}

	key, err := dpt.Get(certdepot.PrivKeyTag(name))
	if err != nil {
		return nil, errors.Wrap(err, "problem getting key")
	}

	return rpc.NewCredentials(caCrt, crt, key)
}

// DeleteCredentials removes the credentials from the database.
func DeleteCredentials(ctx context.Context, env evergreen.Environment, name string) error {
	dpt, err := getDepot(ctx, env)
	if err != nil {
		return errors.Wrap(err, "could not get depot")
	}

	return deleteIfExists(dpt, certdepot.PrivKeyTag(name), certdepot.CrtTag(name))
}
