package aviation

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"

	"github.com/pkg/errors"
)

// GetClientTLSConfig creates a creates a client-side TLS configuration based
// on the given ca, cert, and key files.
func GetClientTLSConfig(caFile, crtFile, keyFile string) (*tls.Config, error) {
	ca, err := ioutil.ReadFile(caFile)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	cp := x509.NewCertPool()
	if !cp.AppendCertsFromPEM(ca) {
		return nil, errors.New("credentials: failed to append certificates")
	}

	crt, err := tls.LoadX509KeyPair(crtFile, keyFile)
	if err != nil {
		return nil, errors.Wrap(err, "problem reading client cert")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{crt},
		RootCAs:      cp,
	}, nil
}
