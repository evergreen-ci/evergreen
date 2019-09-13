package credentials

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const defaultTestTimeout = 10 * time.Second

func TestForJasperClient(t *testing.T) {
	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T){
		"SucceedsAfterBootstrapping": func(ctx context.Context, t *testing.T) {
			expectedCreds, err := FindByID(ctx, serviceName)
			require.NoError(t, err)

			creds, err := ForJasperClient(ctx)
			require.NoError(t, err)

			assert.Equal(t, expectedCreds.Cert, creds.Cert)
			assert.Equal(t, expectedCreds.Key, creds.Key)
			assert.Equal(t, expectedCreds.CACert, creds.CACert)
		},
		"FailsForNonexistentServiceName": func(ctx context.Context, t *testing.T) {
			require.NoError(t, DeleteByID(ctx, serviceName))
			_, err := ForJasperClient(ctx)
			assert.Error(t, err)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), defaultTestTimeout)
			defer cancel()

			withSetupAndTeardown(ctx, t, func() {
				testCase(ctx, t)
			})
		})
	}
}
