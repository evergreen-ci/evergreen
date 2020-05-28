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
		filepath.Join("testdata", "ca.crt"),
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
				CAFile:  filepath.Join("testdata", "ca.crt"),
			},
			hasErr: true,
		},
		{
			name: "CertFiles",
			opts: DialOptions{
				Address: "rpcAddress",
				CAFile:  filepath.Join("testdata", "ca.crt"),
				CrtFile: filepath.Join("testdata", "user.crt"),
				KeyFile: filepath.Join("testdata", "user.key"),
			},
			expectedOpts: 1,
		},
		{
			name: "TLSConf",
			opts: DialOptions{
				Address: "rpcAddress",
				TLSConf: tlsConf,
			},
			expectedOpts: 1,
		},
		{
			name: "Retries",
			opts: DialOptions{
				Address: "rpcAddress",
				TLSConf: tlsConf,
				Retries: 15,
			},
			expectedOpts: 3,
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
