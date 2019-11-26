package aviation

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"

	"github.com/pkg/errors"
)

// GetClientTLSConfig creates a creates a client-side TLS configuration based
// on the given ca, cert, and key.
func GetClientTLSConfig(ca, crt, key []byte) (*tls.Config, error) {
	cp := x509.NewCertPool()
	if !cp.AppendCertsFromPEM(ca) {
		return nil, errors.New("credentials: failed to append certificates")
	}

	keyPair, err := tls.X509KeyPair(crt, key)
	if err != nil {
		return nil, errors.Wrap(err, "problem reading client cert")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{keyPair},
		RootCAs:      cp,
	}, nil

}

// GetClientTLSConfigFromFiles creates a creates a client-side TLS
// configuration based on the given ca, cert, and key files.
func GetClientTLSConfigFromFiles(caFile, crtFile, keyFile string) (*tls.Config, error) {
	ca, err := ioutil.ReadFile(caFile)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	crt, err := ioutil.ReadFile(crtFile)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	key, err := ioutil.ReadFile(keyFile)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return GetClientTLSConfig(ca, crt, key)

}
