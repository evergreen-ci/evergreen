package aviation

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDial(t *testing.T) {
	ctx := context.TODO()
	tlsConf, err := GetClientTLSConfigFromFiles(
		[]string{filepath.Join("testdata", "ca.crt")},
		filepath.Join("testdata", "user.crt"),
		filepath.Join("testdata", "user.key"),
	)
	require.NoError(t, err)

	for _, test := range []struct {
		name         string
		opts         DialOptions
		expectedOpts int
		hasErr       bool
	}{
		{
			name:   "NoAddress",
			opts:   DialOptions{},
			hasErr: true,
		},
		{
			name: "MissingCertFiles",
			opts: DialOptions{
				Address: "rpcAddress",
				Retries: 10,
				CAFiles: []string{filepath.Join("testdata", "ca.crt")},
			},
			hasErr: true,
		},
		{
			name: "MissingCA",
			opts: DialOptions{
				Address: "rpcAddress",
				Retries: 10,
				CrtFile: filepath.Join("testdata", "user.crt"),
				KeyFile: filepath.Join("testdata", "user.key"),
			},
			hasErr: true,
		},
		{
			name: "UsernameAndAPIKeyNoAPIUserHeader",
			opts: DialOptions{
				Address:      "rpcAddress",
				Username:     "username",
				APIKey:       "apikey",
				APIKeyHeader: "api-key",
			},
			hasErr: true,
		},
		{
			name: "UsernameAndAPIKeyNoAPIKeyHeader",
			opts: DialOptions{
				Address:       "rpcAddress",
				Username:      "username",
				APIKey:        "apikey",
				APIUserHeader: "api-user",
			},
			hasErr: true,
		},
		{
			name: "UsernameAndAPIKeyNoHeaders",
			opts: DialOptions{
				Address:  "rpcAddress",
				Username: "username",
				APIKey:   "apikey",
			},
			hasErr: true,
		},
		{
			name: "CertFiles",
			opts: DialOptions{
				Address: "rpcAddress",
				CAFiles: []string{filepath.Join("testdata", "ca.crt")},
				CrtFile: filepath.Join("testdata", "user.crt"),
				KeyFile: filepath.Join("testdata", "user.key"),
			},
			expectedOpts: 2,
		},
		{
			name: "TLSConf",
			opts: DialOptions{
				Address: "rpcAddress",
				TLSConf: tlsConf,
			},
			expectedOpts: 2,
		},
		{
			name: "UsernameAndAPIKeyWithTLS",
			opts: DialOptions{
				Address:       "rpcAddress",
				TLSConf:       tlsConf,
				Username:      "username",
				APIKey:        "apikey",
				APIUserHeader: "api-user",
				APIKeyHeader:  "api-key",
			},
			expectedOpts: 3,
		},
		{
			name: "UsernameAndAPIKeyNoTLS",
			opts: DialOptions{
				Address:       "rpcAddress",
				Username:      "username",
				APIKey:        "apikey",
				APIUserHeader: "api-user",
				APIKeyHeader:  "api-key",
			},
			expectedOpts: 3,
		},
		{
			name: "NoAuth",
			opts: DialOptions{
				Address: "rpcAddress",
			},
			expectedOpts: 2,
		},
		{
			name: "Retries",
			opts: DialOptions{
				Address: "rpcAddress",
				TLSConf: tlsConf,
				Retries: 15,
			},
			expectedOpts: 4,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.hasErr {
				conn, err := Dial(ctx, test.opts)
				assert.Error(t, err)
				assert.Nil(t, conn)
			} else {
				opts, err := test.opts.getOpts()
				require.NoError(t, err)
				require.Len(t, opts, test.expectedOpts)

				conn, err := Dial(ctx, test.opts)
				assert.NoError(t, err)
				assert.NotNil(t, conn)
			}
		})
	}
}
