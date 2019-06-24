package credentials

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const defaultTestTimeout = 5 * time.Second

func TestForJasperClient(t *testing.T) {
	name := "test"

	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, env evergreen.Environment){
		"SucceedsAfterBootstrapping": func(ctx context.Context, t *testing.T, env evergreen.Environment) {
			expectedCreds, err := FindByID(ctx, env, name)
			require.NoError(t, err)

			creds, err := ForJasperClient(ctx, env)
			require.NoError(t, err)

			assert.Equal(t, expectedCreds.Cert, creds.Cert)
			assert.Equal(t, expectedCreds.Key, creds.Key)
			assert.Equal(t, expectedCreds.CACert, creds.CACert)
		},
		"FailsForNonexistentServiceName": func(ctx context.Context, t *testing.T, env evergreen.Environment) {
			require.NoError(t, DeleteCredentials(ctx, env, serviceName))
			_, err := ForJasperClient(ctx, env)
			assert.Error(t, err)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), defaultTestTimeout)
			defer cancel()

			env, err := setupEnv(ctx)
			require.NoError(t, err)

			env.Settings().DomainName = name

			withSetupAndTeardown(t, env, func() {
				testCase(ctx, t, env)
			})
		})
	}
}
