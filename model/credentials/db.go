package credentials

import (
	"context"
	"math"
	"time"

	"github.com/evergreen-ci/certdepot"
	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/jasper/rpc"
	"github.com/pkg/errors"
	"github.com/square/certstrap/depot"
	"github.com/square/certstrap/pkix"
)

const (
	Collection = "credentials"
	CAName     = "evergreen"

	defaultExpiration = 365 * 24 * time.Hour // 1 year (approx.)
)

var serviceName string

// Constants for bson struct tags.
var (
	IDKey            = bsonutil.MustHaveTag(certdepot.User{}, "ID")
	CertKey          = bsonutil.MustHaveTag(certdepot.User{}, "Cert")
	PrivateKeyKey    = bsonutil.MustHaveTag(certdepot.User{}, "PrivateKey")
	CertReqKey       = bsonutil.MustHaveTag(certdepot.User{}, "CertReq")
	CertRevocListKey = bsonutil.MustHaveTag(certdepot.User{}, "CertRevocList")
	TTLKey           = bsonutil.MustHaveTag(certdepot.User{}, "TTL")
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

// SaveByID saves the credentials from the options for the given user
// name. If the credentials already exist, this will overwrite the existing
// credentials.
func SaveByID(ctx context.Context, env evergreen.Environment, name string, creds *rpc.Credentials) error {
	dpt, err := getDepot(ctx, env)
	if err != nil {
		return errors.Wrap(err, "could not get depot")
	}

	if err = deleteIfExists(dpt, certdepot.CsrTag(name), certdepot.PrivKeyTag(name), certdepot.CrtTag(name)); err != nil {
		return errors.Wrap(err, "problem deleting existing credentials")
	}

	if err = dpt.Put(certdepot.PrivKeyTag(name), creds.Key); err != nil {
		return errors.Wrap(err, "problem saving key")
	}

	if err = dpt.Put(certdepot.CrtTag(name), creds.Cert); err != nil {
		return errors.Wrap(err, "problem saving certificate")
	}

	crt, err := pkix.NewCertificateFromPEM(creds.Cert)
	if err != nil {
		return errors.Wrap(err, "could not get certificate from PEM bytes")
	}
	rawCrt, err := crt.GetRawCertificate()
	if err != nil {
		return errors.Wrap(err, "could not get x509 certificate")
	}
	if err := certdepot.PutTTL(dpt, name, rawCrt.NotAfter); err != nil {
		return errors.Wrap(err, "could not put expiration on credentials")
	}

	return nil
}

// GenerateInMemory generates the credentials without storing them in the
// database. This is not idempotent, so separate calls with the same inputs will
// return different credentials. This will fail if credentials for the given
// name already exist in the database.
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

	creds, err := rpc.NewCredentials(pemCACrt, pemCrt, pemKey)
	if err != nil {
		return nil, errors.Wrap(err, "could not create RPC credentials")
	}

	return creds, nil
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

// FindExpirationByID returns the time at which the credentials for the given
// name will expire.
func FindExpirationByID(ctx context.Context, env evergreen.Environment, name string) (time.Time, error) {
	dpt, err := getDepot(ctx, env)
	if err != nil {
		return time.Time{}, errors.Wrap(err, "could not get depot")
	}

	return certdepot.GetTTL(dpt, name)
}

// DeleteByID removes the credentials from the database if they exist.
func DeleteByID(ctx context.Context, env evergreen.Environment, name string) error {
	dpt, err := getDepot(ctx, env)
	if err != nil {
		return errors.Wrap(err, "could not get depot")
	}

	return deleteIfExists(dpt, certdepot.CsrTag(name), certdepot.PrivKeyTag(name), certdepot.CrtTag(name))
}
