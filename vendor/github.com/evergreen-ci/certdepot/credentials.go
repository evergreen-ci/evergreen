package certdepot

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io/ioutil"
	"os"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// Credentials represent a bundle of assets for doing TLS
// authentication.
type Credentials struct {
	// CACert is the PEM-encoded client CA certificate. If the credentials are
	// used by a client, this should be the certificate of the root CA to verify
	// the server certificate. If the credentials are used by a server, this
	// should be the certificate of the root CA to verify the client
	// certificate.
	CACert []byte `bson:"ca_cert" json:"ca_cert" yaml:"ca_cert"`
	// Cert is the PEM-encoded certificate.
	Cert []byte `bson:"cert" json:"cert" yaml:"cert"`
	// Key is the PEM-encoded private key.
	Key []byte `bson:"key" json:"key" yaml:"key"`

	// ServerName is the name of the service being contacted.
	ServerName string `bson:"server_name" json:"server_name" yaml:"server_name"`
}

// NewCredentials initializes a new Credential struct.
func NewCredentials(caCert, cert, key []byte) (*Credentials, error) {
	creds := &Credentials{
		CACert: caCert,
		Cert:   cert,
		Key:    key,
	}

	if err := creds.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid credentials")
	}

	return creds, nil
}

// NewCredentialsFromFile parses the PEM-encoded credentials in JSON format in
// the file at path into a Credentials struct.
func NewCredentialsFromFile(path string) (*Credentials, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrap(err, "error opening credentials file")
	}
	defer file.Close()

	contents, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, errors.Wrap(err, "error reading credentials file")
	}

	creds := Credentials{}
	if err := json.Unmarshal(contents, &creds); err != nil {
		return nil, errors.Wrap(err, "error unmarshalling contents of credentials file")
	}

	if err := creds.Validate(); err != nil {
		return nil, errors.Wrap(err, "read invalid credentials from file")
	}

	return &creds, nil
}

// Validate checks that the Credentials are all set to non-empty values.
func (c *Credentials) Validate() error {
	catcher := grip.NewBasicCatcher()

	catcher.NewWhen(len(c.CACert) == 0, "CA certificate should not be empty")
	catcher.NewWhen(len(c.Cert) == 0, "certificate should not be empty")
	catcher.NewWhen(len(c.Key) == 0, "key should not be empty")

	return catcher.Resolve()
}

// Resolve converts the Credentials struct into a tls.Config.
func (c *Credentials) Resolve() (*tls.Config, error) {
	if err := c.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid credentials")
	}

	caCerts := x509.NewCertPool()
	if !caCerts.AppendCertsFromPEM(c.CACert) {
		return nil, errors.New("failed to append client CA certificate")
	}

	cert, err := tls.X509KeyPair(c.Cert, c.Key)
	if err != nil {
		return nil, errors.Wrap(err, "problem loading key pair")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},

		// Server-specific options
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  caCerts,

		// Client-specific options
		RootCAs:    caCerts,
		ServerName: c.ServerName,
	}, nil
}

// Export exports the Credentials struct into JSON-encoded bytes.
func (c *Credentials) Export() ([]byte, error) {
	if err := c.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid credentials")
	}

	b, err := json.Marshal(c)
	if err != nil {
		return nil, errors.Wrap(err, "error exporting credentials")
	}

	return b, nil
}
