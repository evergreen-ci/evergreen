package aviation

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"runtime"

	"github.com/pkg/errors"
)

// GetClientTLSConfig creates a creates a client-side TLS configuration based
// on the given cas, cert, and key. If possible, the system cert pool is
// combined with the provided cas.
func GetClientTLSConfig(cas [][]byte, crt, key []byte) (*tls.Config, error) {
	cp, err := GetCACertPool(cas...)
	if err != nil {
		return nil, err
	}

	keyPair, err := tls.X509KeyPair(crt, key)
	if err != nil {
		return nil, errors.Wrap(err, "reading the client cert")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{keyPair},
		RootCAs:      cp,
	}, nil
}

// GetCACertPool is a convenience function that creates a cert pool with the
// given cas and, when possible, the system cert pool. On windows machines, the
// cert pool will be empty if no cas are passed in.
func GetCACertPool(cas ...[]byte) (*x509.CertPool, error) {
	cp := x509.NewCertPool()
	if runtime.GOOS != "windows" {
		// Windows, as always, has to be different and golang cannot
		// access the system cert pool.
		var err error
		cp, err = x509.SystemCertPool()
		if err != nil {
			return nil, errors.Wrap(err, "getting system cert pool")
		}
	}

	for _, ca := range cas {
		if !cp.AppendCertsFromPEM(ca) {
			return nil, errors.New("appending CA certificate to the cert pool")
		}
	}

	return cp, nil
}

// GetClientTLSConfigFromFiles creates a creates a client-side TLS
// configuration based on the given ca, cert, and key files.
func GetClientTLSConfigFromFiles(caFiles []string, crtFile, keyFile string) (*tls.Config, error) {
	cas := make([][]byte, len(caFiles))
	for i, caFile := range caFiles {
		ca, err := ioutil.ReadFile(caFile)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		cas[i] = ca
	}
	crt, err := ioutil.ReadFile(crtFile)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	key, err := ioutil.ReadFile(keyFile)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return GetClientTLSConfig(cas, crt, key)

}
