package credentials

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/jasper/rpc"
	"github.com/square/certstrap/pkix"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withSetupAndTeardown(ctx context.Context, t *testing.T, fn func()) {
	require.NoError(t, db.ClearCollections(Collection))
	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()

	env := testutil.NewEnvironment(ctx, t)
	env.Settings().DomainName = "test"
	require.NoError(t, Bootstrap(env))

	fn()
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

	for opName, opTests := range map[string]func(ctx context.Context, t *testing.T){
		"SaveByID": func(ctx context.Context, t *testing.T) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, creds *rpc.Credentials){
				"FailsForCancelledContext": func(ctx context.Context, t *testing.T, creds *rpc.Credentials) {
					withCancelledContext(ctx, func(ctx context.Context) {
						err := SaveByID(ctx, name, creds)
						assert.Error(t, err)
						assert.Contains(t, err.Error(), context.Canceled.Error())
					})
				},
				"Succeeds": func(ctx context.Context, t *testing.T, creds *rpc.Credentials) {
					assert.NoError(t, SaveByID(ctx, name, creds))
					dbCreds, err := FindByID(ctx, name)
					require.NoError(t, err)
					assert.Equal(t, creds.Cert, dbCreds.Cert)
					assert.Equal(t, creds.Key, dbCreds.Key)
					assert.Equal(t, creds.CACert, dbCreds.CACert)
					assert.Equal(t, name, dbCreds.ServerName)
				},
				"OverwritesExistingCredentials": func(ctx context.Context, t *testing.T, creds *rpc.Credentials) {
					require.NoError(t, SaveByID(ctx, name, creds))
					dbCreds, err := FindByID(ctx, name)
					require.NoError(t, err)
					assert.Equal(t, creds.Cert, dbCreds.Cert)
					assert.Equal(t, creds.Key, dbCreds.Key)
					assert.Equal(t, creds.CACert, dbCreds.CACert)
					assert.Equal(t, name, dbCreds.ServerName)

					ttl, err := FindExpirationByID(ctx, name)
					require.NoError(t, err)

					// We have to sleep in order to verify that the TTL is
					// updated properly.
					time.Sleep(time.Second)
					newName := "new" + name
					newCreds, err := GenerateInMemory(ctx, newName)
					require.NoError(t, err)
					require.NoError(t, SaveByID(ctx, name, newCreds))

					dbCreds, err = FindByID(ctx, name)
					require.NoError(t, err)
					assert.Equal(t, newCreds.Cert, dbCreds.Cert)
					assert.Equal(t, newCreds.Key, dbCreds.Key)
					assert.Equal(t, newCreds.CACert, dbCreds.CACert)
					assert.Equal(t, name, dbCreds.ServerName)

					newTTL, err := FindExpirationByID(ctx, name)
					require.NoError(t, err)
					assert.NotEqual(t, ttl, newTTL)
					assert.WithinDuration(t, ttl, newTTL, 10*time.Second)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					withSetupAndTeardown(ctx, t, func() {
						creds, err := GenerateInMemory(ctx, name)
						require.NoError(t, err)

						testCase(ctx, t, creds)
					})
				})
			}
		},
		"GenerateInMemory": func(ctx context.Context, t *testing.T) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T){
				"FailsForCancelledContext": func(ctx context.Context, t *testing.T) {
					withCancelledContext(ctx, func(ctx context.Context) {
						_, err := GenerateInMemory(ctx, name)
						assert.Error(t, err)
						assert.Contains(t, err.Error(), context.Canceled.Error())
					})
				},
				"Succeeds": func(ctx context.Context, t *testing.T) {
					creds, err := GenerateInMemory(ctx, name)
					require.NoError(t, err)
					assert.NotEmpty(t, creds.Cert)
					assert.NotEmpty(t, creds.Key)
					assert.NotEmpty(t, creds.CACert)
					assert.NotEmpty(t, creds.ServerName)
					assert.Equal(t, name, creds.ServerName)

					_, err = FindByID(ctx, name)
					assert.Error(t, err)
				},
				"NotIdempotent": func(ctx context.Context, t *testing.T) {
					creds, err := GenerateInMemory(ctx, name)
					require.NoError(t, err)
					newCreds, err := GenerateInMemory(ctx, name)
					require.NoError(t, err)
					assert.Equal(t, creds.CACert, newCreds.CACert)
					assert.NotEqual(t, creds.Cert, newCreds.Cert)
					assert.NotEqual(t, creds.Key, newCreds.Key)
					assert.Equal(t, creds.ServerName, newCreds.ServerName)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					withSetupAndTeardown(ctx, t, func() {
						testCase(ctx, t)
					})
				})
			}
		},
		"FindByID": func(ctx context.Context, t *testing.T) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T){
				"FailsForCancelledContext": func(ctx context.Context, t *testing.T) {
					withCancelledContext(ctx, func(ctx context.Context) {
						_, err := FindByID(ctx, name)
						assert.Error(t, err)
						assert.Contains(t, err.Error(), context.Canceled.Error())
					})
				},
				"FailsForNonexistent": func(ctx context.Context, t *testing.T) {
					_, err := FindByID(ctx, name)
					assert.Error(t, err)
				},
				"Succeeds": func(ctx context.Context, t *testing.T) {
					creds, err := GenerateInMemory(ctx, name)
					require.NoError(t, err)
					require.NoError(t, SaveByID(ctx, name, creds))

					dbCreds, err := FindByID(ctx, name)
					require.NoError(t, err)
					assert.Equal(t, creds.Key, dbCreds.Key)
					assert.Equal(t, creds.Cert, dbCreds.Cert)
					assert.Equal(t, creds.CACert, dbCreds.CACert)
					assert.Equal(t, name, creds.ServerName)
					assert.Equal(t, creds.ServerName, dbCreds.ServerName)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					withSetupAndTeardown(ctx, t, func() {
						testCase(ctx, t)
					})
				})
			}
		},
		"DeleteByID": func(ctx context.Context, t *testing.T) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T){
				"NoopsWithNonexistent": func(ctx context.Context, t *testing.T) {
					assert.NoError(t, DeleteByID(ctx, name))
				},
				"DeletesWithExistingID": func(ctx context.Context, t *testing.T) {
					creds, err := GenerateInMemory(ctx, name)
					require.NoError(t, err)
					require.NoError(t, SaveByID(ctx, name, creds))
					require.NoError(t, DeleteByID(ctx, name))

					_, err = FindByID(ctx, name)
					assert.Error(t, err)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					withSetupAndTeardown(ctx, t, func() {
						testCase(ctx, t)
					})
				})
			}
		},
		"FindExpirationByID": func(ctx context.Context, t *testing.T) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T){
				"FailsForCancelledContext": func(ctx context.Context, t *testing.T) {
					withCancelledContext(ctx, func(ctx context.Context) {
						_, err := FindExpirationByID(ctx, name)
						require.Error(t, err)
						assert.Contains(t, err.Error(), context.Canceled.Error())
					})
				},
				"FailsForNonexistent": func(ctx context.Context, t *testing.T) {
					_, err := FindExpirationByID(ctx, name)
					assert.Error(t, err)
				},
				"Succeeds": func(ctx context.Context, t *testing.T) {
					creds, err := GenerateInMemory(ctx, name)
					require.NoError(t, err)
					require.NoError(t, SaveByID(ctx, name, creds))
					expiration, err := FindExpirationByID(ctx, name)
					require.NoError(t, err)
					crt, err := pkix.NewCertificateFromPEM(creds.Cert)
					require.NoError(t, err)
					rawCrt, err := crt.GetRawCertificate()
					require.NoError(t, err)
					assert.WithinDuration(t, rawCrt.NotAfter, expiration, time.Second)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					withSetupAndTeardown(ctx, t, func() {
						testCase(ctx, t)
					})
				})
			}
		},
	} {
		t.Run(opName, func(t *testing.T) {
			tctx, cancel := context.WithTimeout(ctx, defaultTestTimeout)
			defer cancel()

			opTests(tctx, t)
		})
	}
}
