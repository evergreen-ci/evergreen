package aviation

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetClientTLSConfig(t *testing.T) {
	for _, test := range []struct {
		name   string
		cas    []string
		usrCrt string
		usrKey string
		hasErr bool
	}{
		{
			name:   "CACertDNE",
			cas:    []string{"DNE"},
			usrCrt: filepath.Join("testdata", "usr.crt"),
			usrKey: filepath.Join("testdata", "usr.key"),
			hasErr: true,
		},
		{
			name:   "UserCrtDNE",
			cas:    []string{filepath.Join("testdata", "ca.crt")},
			usrCrt: "DNE",
			usrKey: "DNE",
			hasErr: true,
		},
		{
			name:   "ClientConfig",
			cas:    []string{filepath.Join("testdata", "ca.crt")},
			usrCrt: filepath.Join("testdata", "user.crt"),
			usrKey: filepath.Join("testdata", "user.key"),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			conf, err := GetClientTLSConfigFromFiles(test.cas, test.usrCrt, test.usrKey)
			if test.hasErr {
				assert.Error(t, err)
				assert.Nil(t, conf)
			} else {
				assert.NoError(t, err)

				expectedCrt, err := tls.LoadX509KeyPair(test.usrCrt, test.usrKey)
				require.NoError(t, err)
				require.Len(t, conf.Certificates, 1)
				assert.Equal(t, expectedCrt, conf.Certificates[0])

				cp := x509.NewCertPool()
				if runtime.GOOS != "windows" {
					cp, err = x509.SystemCertPool()
					require.NoError(t, err)
				}
				for _, ca := range test.cas {
					ca, err := ioutil.ReadFile(ca)
					require.NoError(t, err)
					require.True(t, cp.AppendCertsFromPEM(ca))
				}
				assert.Equal(t, cp, conf.RootCAs)
			}
		})
	}
}
