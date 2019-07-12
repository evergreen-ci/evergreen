package credentials

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/mongodb/jasper/rpc"
	"github.com/pkg/errors"
	"github.com/square/certstrap/pkix"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupEnv(ctx context.Context) (*mock.Environment, error) {
	env := &mock.Environment{}

	if err := env.Configure(ctx, "", nil); err != nil {
		return nil, errors.WithStack(err)
	}
	return env, nil
}

func TestDBOperations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	name := "name"

	withCancelledContext := func(ctx context.Context, fn func(context.Context)) {
		ctx, cancel := context.WithCancel(ctx)
		cancel()
		fn(ctx)
	}

	withSetupAndTeardown := func(t *testing.T, env evergreen.Environment, fn func()) {
		require.NoError(t, db.ClearCollections(Collection))
		defer func() {
			assert.NoError(t, db.ClearCollections(Collection))
		}()

		require.NoError(t, Bootstrap(env))

		fn()
	}

	for opName, opTests := range map[string]func(ctx context.Context, t *testing.T, env evergreen.Environment){
		"SaveByID": func(ctx context.Context, t *testing.T, env evergreen.Environment) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, creds *rpc.Credentials){
				"FailsForCancelledContext": func(ctx context.Context, t *testing.T, creds *rpc.Credentials) {
					withCancelledContext(ctx, func(ctx context.Context) {
						err := SaveByID(ctx, env, name, creds)
						assert.Error(t, err)
						assert.Contains(t, err.Error(), context.Canceled.Error())
					})
				},
				"Succeeds": func(ctx context.Context, t *testing.T, creds *rpc.Credentials) {
					assert.NoError(t, SaveByID(ctx, env, name, creds))
					dbCreds, err := FindByID(ctx, env, name)
					require.NoError(t, err)
					assert.Equal(t, creds.Cert, dbCreds.Cert)
					assert.Equal(t, creds.Key, dbCreds.Key)
					assert.Equal(t, creds.CACert, dbCreds.CACert)
				},
				"OverwritesExistingCredentials": func(ctx context.Context, t *testing.T, creds *rpc.Credentials) {
					require.NoError(t, SaveByID(ctx, env, name, creds))
					dbCreds, err := FindByID(ctx, env, name)
					require.NoError(t, err)
					assert.Equal(t, creds.Cert, dbCreds.Cert)
					assert.Equal(t, creds.Key, dbCreds.Key)
					assert.Equal(t, creds.CACert, dbCreds.CACert)

					time.Sleep(time.Second)
					newCreds, err := GenerateInMemory(ctx, env, "new"+name)
					require.NoError(t, err)
					require.NoError(t, SaveByID(ctx, env, name, newCreds))

					dbCreds, err = FindByID(ctx, env, name)
					require.NoError(t, err)
					assert.Equal(t, newCreds.Cert, dbCreds.Cert)
					assert.Equal(t, newCreds.Key, dbCreds.Key)
					assert.Equal(t, newCreds.CACert, dbCreds.CACert)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					withSetupAndTeardown(t, env, func() {
						creds, err := GenerateInMemory(ctx, env, name)
						require.NoError(t, err)

						testCase(ctx, t, creds)
					})
				})
			}
		},
		"GenerateInMemory": func(ctx context.Context, t *testing.T, env evergreen.Environment) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T){
				"FailsForCancelledContext": func(ctx context.Context, t *testing.T) {
					withCancelledContext(ctx, func(ctx context.Context) {
						_, err := GenerateInMemory(ctx, env, name)
						assert.Error(t, err)
						assert.Contains(t, err.Error(), context.Canceled.Error())
					})
				},
				"Succeeds": func(ctx context.Context, t *testing.T) {
					_, err := GenerateInMemory(ctx, env, name)
					require.NoError(t, err)
					_, err = FindByID(ctx, env, name)
					assert.Error(t, err)
				},
				"NotIdempotent": func(ctx context.Context, t *testing.T) {
					creds, err := GenerateInMemory(ctx, env, name)
					require.NoError(t, err)
					newCreds, err := GenerateInMemory(ctx, env, name)
					require.NoError(t, err)
					assert.Equal(t, creds.CACert, newCreds.CACert)
					assert.NotEqual(t, creds.Cert, newCreds.Cert)
					assert.NotEqual(t, creds.Key, newCreds.Key)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					withSetupAndTeardown(t, env, func() {
						testCase(ctx, t)
					})
				})
			}
		},
		"FindByID": func(ctx context.Context, t *testing.T, env evergreen.Environment) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T){
				"FailsForCancelledContext": func(ctx context.Context, t *testing.T) {
					withCancelledContext(ctx, func(ctx context.Context) {
						_, err := FindByID(ctx, env, name)
						assert.Error(t, err)
						assert.Contains(t, err.Error(), context.Canceled.Error())
					})
				},
				"FailsForNonexistent": func(ctx context.Context, t *testing.T) {
					_, err := FindByID(ctx, env, name)
					assert.Error(t, err)
				},
				"Succeeds": func(ctx context.Context, t *testing.T) {
					creds, err := GenerateInMemory(ctx, env, name)
					require.NoError(t, err)
					require.NoError(t, SaveByID(ctx, env, name, creds))

					dbCreds, err := FindByID(ctx, env, name)
					require.NoError(t, err)
					assert.Equal(t, creds.Key, dbCreds.Key)
					assert.Equal(t, creds.Cert, dbCreds.Cert)
					assert.Equal(t, creds.CACert, dbCreds.CACert)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					withSetupAndTeardown(t, env, func() {
						testCase(ctx, t)
					})
				})
			}
		},
		"DeleteByID": func(ctx context.Context, t *testing.T, env evergreen.Environment) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T){
				"NoopsWithNonexistent": func(ctx context.Context, t *testing.T) {
					assert.NoError(t, DeleteByID(ctx, env, name))
				},
				"DeletesWithExistingID": func(ctx context.Context, t *testing.T) {
					creds, err := GenerateInMemory(ctx, env, name)
					require.NoError(t, err)
					require.NoError(t, SaveByID(ctx, env, name, creds))
					require.NoError(t, DeleteByID(ctx, env, name))

					_, err = FindByID(ctx, env, name)
					assert.Error(t, err)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					withSetupAndTeardown(t, env, func() {
						testCase(ctx, t)
					})
				})
			}
		},
		"FindExpirationByID": func(ctx context.Context, t *testing.T, env evergreen.Environment) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T){
				"FailsForCancelledContext": func(ctx context.Context, t *testing.T) {
					withCancelledContext(ctx, func(ctx context.Context) {
						_, err := FindExpirationByID(ctx, env, name)
						require.Error(t, err)
						assert.Contains(t, err.Error(), context.Canceled.Error())
					})
				},
				"FailsForNonexistent": func(ctx context.Context, t *testing.T) {
					_, err := FindExpirationByID(ctx, env, name)
					assert.Error(t, err)
				},
				"Succeeds": func(ctx context.Context, t *testing.T) {
					creds, err := GenerateInMemory(ctx, env, name)
					require.NoError(t, err)
					require.NoError(t, SaveByID(ctx, env, name, creds))
					expiration, err := FindExpirationByID(ctx, env, name)
					require.NoError(t, err)
					crt, err := pkix.NewCertificateFromPEM(creds.Cert)
					require.NoError(t, err)
					rawCrt, err := crt.GetRawCertificate()
					require.NoError(t, err)
					assert.WithinDuration(t, rawCrt.NotAfter, expiration, time.Second)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					withSetupAndTeardown(t, env, func() {
						testCase(ctx, t)
					})
				})
			}
		},
	} {
		t.Run(opName, func(t *testing.T) {
			env, err := setupEnv(ctx)
			require.NoError(t, err)

			tctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			env.Settings().DomainName = "test"
			opTests(tctx, t, env)
		})
	}
}
