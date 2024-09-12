package fakeparameter

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud/parameterstore"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests for the ParameterManager functionality are in this package to avoid a
// circular dependency between the two packages.
func TestParameterManager(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()

	// checkParamInDB checks that the input parameter matches the document in
	// the DB.
	checkParamInDB := func(ctx context.Context, t *testing.T, p *parameterstore.Parameter) {
		dbParam, err := FindOneID(ctx, p.Name)
		require.NoError(t, err)
		require.NotZero(t, dbParam)
		assert.Equal(t, p.Name, dbParam.Name)
		assert.Equal(t, p.Value, dbParam.Value)
	}
	// checkParam checks that the input parameter matches the expected values
	// and that it matches a corresponding document in the DB.
	checkParam := func(ctx context.Context, t *testing.T, p *parameterstore.Parameter, name, basename, value string) {
		require.NotZero(t, p)
		assert.Equal(t, name, p.Name)
		assert.Equal(t, basename, p.Basename)
		assert.Equal(t, value, p.Value)
		checkParamInDB(ctx, t, p)
	}

	for managerTestName, makeParameterManager := range map[string]func(context.Context) (*parameterstore.ParameterManager, error){
		"CachingDisabled": func(ctx context.Context) (*parameterstore.ParameterManager, error) {
			return parameterstore.NewParameterManager(ctx, parameterstore.ParameterManagerOptions{
				PathPrefix:     "prefix",
				CachingEnabled: false,
				SSMClient:      NewFakeSSMClient(),
				DB:             evergreen.GetEnvironment().DB(),
			})
		},
		"CachingEnabled": func(ctx context.Context) (*parameterstore.ParameterManager, error) {
			return parameterstore.NewParameterManager(ctx, parameterstore.ParameterManagerOptions{
				PathPrefix:     "prefix",
				CachingEnabled: true,
				SSMClient:      NewFakeSSMClient(),
				DB:             evergreen.GetEnvironment().DB(),
			})
		},
	} {
		t.Run(managerTestName, func(t *testing.T) {
			for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, pm *parameterstore.ParameterManager){
				"PutAddsNewParameters": func(ctx context.Context, t *testing.T, pm *parameterstore.ParameterManager) {
					const numParams = 5
					basenames := make([]string, 0, numParams)
					expectedParams := make([]parameterstore.Parameter, 0, numParams)
					for i := 0; i < numParams; i++ {
						expectedParam := parameterstore.Parameter{
							Name:     fmt.Sprintf("/prefix/basename-%d", i),
							Basename: fmt.Sprintf("basename-%d", i),
							Value:    fmt.Sprintf("value-%d", i),
						}
						p, err := pm.Put(ctx, expectedParam.Basename, expectedParam.Value)
						require.NoError(t, err)
						checkParam(ctx, t, p, expectedParam.Name, expectedParam.Basename, expectedParam.Value)

						basenames = append(basenames, expectedParam.Basename)
						expectedParams = append(expectedParams, expectedParam)
					}

					foundParams, err := pm.Get(ctx, basenames...)
					require.NoError(t, err)
					require.Len(t, foundParams, len(basenames))
					for _, p := range foundParams {
						var found bool
						for i := 0; i < len(expectedParams); i++ {
							if p.Basename == expectedParams[i].Basename {
								checkParam(ctx, t, &p, expectedParams[i].Name, expectedParams[i].Basename, expectedParams[i].Value)
								found = true
								break
							}
						}
						if !found {
							assert.FailNow(t, "unexpected parameter", "found unexpected parameter with basename '%s'", p.Basename)
						}
					}
				},
				"PutIncludesPrefixForPartialName": func(ctx context.Context, t *testing.T, pm *parameterstore.ParameterManager) {
					const partialName = "/path/to/basename"
					const value = "value"
					p, err := pm.Put(ctx, partialName, value)
					require.NoError(t, err)
					checkParam(ctx, t, p, "/prefix/path/to/basename", "basename", value)

					foundParams, err := pm.Get(ctx, partialName)
					require.NoError(t, err)
					require.Len(t, foundParams, 1)
					checkParam(ctx, t, &foundParams[0], "/prefix/path/to/basename", "basename", value)
				},
				"PutOverwritesExistingParameter": func(ctx context.Context, t *testing.T, pm *parameterstore.ParameterManager) {
					const basename = "basename"
					const oldValue = "old_value"
					oldParam, err := pm.Put(ctx, basename, oldValue)
					require.NoError(t, err)
					checkParam(ctx, t, oldParam, "/prefix/basename", basename, oldValue)

					foundParams, err := pm.Get(ctx, basename, oldValue)
					require.NoError(t, err)
					require.Len(t, foundParams, 1)
					assert.Equal(t, basename, foundParams[0].Basename)
					assert.Equal(t, oldValue, foundParams[0].Value)
					assert.Equal(t, "/prefix/basename", foundParams[0].Name)

					const newValue = "new_value"
					newParam, err := pm.Put(ctx, basename, newValue)
					require.NoError(t, err)
					checkParam(ctx, t, newParam, "/prefix/basename", basename, newValue)

					foundParams, err = pm.Get(ctx, basename, oldValue)
					require.NoError(t, err)
					require.Len(t, foundParams, 1)
					assert.Equal(t, basename, foundParams[0].Basename)
					assert.Equal(t, newValue, foundParams[0].Value)
					assert.Equal(t, "/prefix/basename", foundParams[0].Name)
				},
				"DeleteRemovesExistingParameter": func(ctx context.Context, t *testing.T, pm *parameterstore.ParameterManager) {
					const basename = "basename"
					const value = "value"
					p, err := pm.Put(ctx, basename, value)
					require.NoError(t, err)
					checkParam(ctx, t, p, "/prefix/basename", basename, value)

					require.NoError(t, pm.Delete(ctx, basename))

					dbParam, err := FindOneID(ctx, basename)
					assert.NoError(t, err)
					assert.Zero(t, dbParam)

					foundParams, err := pm.Get(ctx, basename)
					assert.NoError(t, err)
					assert.Empty(t, foundParams)
				},
				"DeleteNoopsForNonexistentParameter": func(ctx context.Context, t *testing.T, pm *parameterstore.ParameterManager) {
					assert.NoError(t, pm.Delete(ctx, "nonexistent"))
					foundParams, err := pm.Get(ctx, "nonexistent")
					assert.NoError(t, err)
					assert.Empty(t, foundParams)
				},
				"GetReturnsAllExistingParameters": func(ctx context.Context, t *testing.T, pm *parameterstore.ParameterManager) {
					precreatedParams := map[string]parameterstore.Parameter{}
					var names []string
					const numParams = 5
					for i := 0; i < numParams; i++ {
						name := fmt.Sprintf("name-%d", i)
						value := fmt.Sprintf("value-%d", i)
						p, err := pm.Put(ctx, name, value)
						require.NoError(t, err)
						checkParam(ctx, t, p, fmt.Sprintf("/prefix/%s", name), name, value)
						precreatedParams[p.Name] = *p
						names = append(names, p.Name)
					}

					foundParams, err := pm.Get(ctx, names...)
					require.NoError(t, err)
					require.Len(t, foundParams, numParams)
					for _, foundParam := range foundParams {
						expectedParam, ok := precreatedParams[foundParam.Name]
						require.True(t, ok, "found unexpected param '%s'", foundParam.Name)
						checkParam(ctx, t, &foundParam, expectedParam.Name, expectedParam.Basename, expectedParam.Value)
					}
				},
				"GetReturnsMatchingFullNameParameterFromJustBasename": func(ctx context.Context, t *testing.T, pm *parameterstore.ParameterManager) {
					const basename = "basename"
					const value = "value"
					p, err := pm.Put(ctx, basename, value)
					require.NoError(t, err)
					checkParam(ctx, t, p, "/prefix/basename", basename, value)

					foundParams, err := pm.Get(ctx, basename)
					require.NoError(t, err)
					checkParam(ctx, t, &foundParams[0], "/prefix/basename", basename, value)
				},
				"GetReturnsNoResultForNoRequestedParameters": func(ctx context.Context, t *testing.T, pm *parameterstore.ParameterManager) {
					foundParams, err := pm.Get(ctx)
					assert.NoError(t, err)
					assert.Empty(t, foundParams)
				},
				"GetReturnsNoResultForNonexistentParameter": func(ctx context.Context, t *testing.T, pm *parameterstore.ParameterManager) {
					foundParams, err := pm.Get(ctx, "nonexistent")
					assert.NoError(t, err)
					assert.Empty(t, foundParams)
				},
				"GetReturnsExistingSubsetForMixOfExistingAndNonexistentParameters": func(ctx context.Context, t *testing.T, pm *parameterstore.ParameterManager) {
					p0 := parameterstore.Parameter{
						Name:     "/prefix/basename-0",
						Basename: "basename-0",
						Value:    "value-0",
					}
					p, err := pm.Put(ctx, p0.Basename, p0.Value)
					require.NoError(t, err)
					checkParam(ctx, t, p, p0.Name, p0.Basename, p0.Value)

					p1 := parameterstore.Parameter{
						Name:     "/prefix/basename-1",
						Basename: "basename-1",
						Value:    "value-1",
					}
					p, err = pm.Put(ctx, p1.Basename, p1.Value)
					require.NoError(t, err)
					checkParam(ctx, t, p, p1.Name, p1.Basename, p1.Value)

					p2 := parameterstore.Parameter{
						Name:     "/prefix/basename-2",
						Basename: "basename-2",
						Value:    "value-2",
					}
					p, err = pm.Put(ctx, p2.Basename, p2.Value)
					require.NoError(t, err)
					checkParam(ctx, t, p, p2.Name, p2.Basename, p2.Value)

					// If caching is enabled, get a subset of the created
					// parameters to pre-populate the cache.
					// If caching is disabled, this should still return
					// correct results.
					foundParams, err := pm.Get(ctx, "nonexistent", p0.Basename, p1.Basename)
					require.NoError(t, err)
					require.Len(t, foundParams, 2)
					for _, p := range foundParams {
						switch p.Basename {
						case p0.Basename:
							checkParam(ctx, t, &p, p0.Name, p0.Basename, p0.Value)
						case p1.Basename:
							checkParam(ctx, t, &p, p1.Name, p1.Basename, p1.Value)
						default:
							assert.FailNow(t, "unexpected parameter", "found unexpected parameter with basename '%s'", p.Basename)
						}
					}

					// Check the returned parameters more than once to
					// ensure caching returns consistent results. If caching is
					// disabled, this is also expected to return the same
					// results.
					foundParams, err = pm.Get(ctx, "nonexistent", p0.Basename, p1.Basename, p2.Basename)
					require.NoError(t, err)
					require.Len(t, foundParams, 3)
					for _, p := range foundParams {
						switch p.Basename {
						case p0.Basename:
							checkParam(ctx, t, &p, p0.Name, p0.Basename, p0.Value)
						case p1.Basename:
							checkParam(ctx, t, &p, p1.Name, p1.Basename, p1.Value)
						case p2.Basename:
							checkParam(ctx, t, &p, p2.Name, p2.Basename, p2.Value)
						default:
							assert.FailNow(t, "unexpected parameter", "found unexpected parameter with basename '%s'", p.Basename)
						}
					}
				},
				"GetStrictReturnsAllExistingParameters": func(ctx context.Context, t *testing.T, pm *parameterstore.ParameterManager) {
					precreatedParams := map[string]parameterstore.Parameter{}
					var names []string
					const numParams = 5
					for i := 0; i < numParams; i++ {
						name := fmt.Sprintf("name-%d", i)
						value := fmt.Sprintf("value-%d", i)
						p, err := pm.Put(ctx, name, value)
						require.NoError(t, err)
						checkParam(ctx, t, p, fmt.Sprintf("/prefix/%s", name), name, value)
						precreatedParams[p.Name] = *p
						names = append(names, p.Name)
					}

					foundParams, err := pm.GetStrict(ctx, names...)
					require.NoError(t, err)
					require.Len(t, foundParams, numParams)
					for _, foundParam := range foundParams {
						expectedParam, ok := precreatedParams[foundParam.Name]
						require.True(t, ok, "found unexpected param '%s'", foundParam.Name)
						checkParam(ctx, t, &foundParam, expectedParam.Name, expectedParam.Basename, expectedParam.Value)
					}
				},
				"GetStrictReturnsMatchingParameterFromJustBasename": func(ctx context.Context, t *testing.T, pm *parameterstore.ParameterManager) {
					const basename = "basename"
					const value = "value"
					p, err := pm.Put(ctx, basename, value)
					require.NoError(t, err)
					checkParam(ctx, t, p, "/prefix/basename", basename, value)

					foundParams, err := pm.Get(ctx, basename)
					require.NoError(t, err)
					checkParam(ctx, t, &foundParams[0], "/prefix/basename", basename, value)
				},
				"GetStrictReturnsNoResultForNoRequestedParameters": func(ctx context.Context, t *testing.T, pm *parameterstore.ParameterManager) {
					foundParams, err := pm.Get(ctx)
					assert.NoError(t, err)
					assert.Empty(t, foundParams)
				},
				"GetStrictErrorsForNonexistentParameter": func(ctx context.Context, t *testing.T, pm *parameterstore.ParameterManager) {
					foundParams, err := pm.GetStrict(ctx, "nonexistent")
					assert.Error(t, err)
					assert.Empty(t, foundParams)
				},
				"GetStrictErrorsForMixOfExistingAndNonexistentParameters": func(ctx context.Context, t *testing.T, pm *parameterstore.ParameterManager) {
					const basename = "basename"
					const value = "value"
					p, err := pm.Put(ctx, basename, value)
					require.NoError(t, err)
					checkParam(ctx, t, p, "/prefix/basename", basename, value)

					foundParams, err := pm.GetStrict(ctx, "/prefix/basename", "nonexistent", "/prefix/also-nonexistent")
					assert.Error(t, err)
					assert.Empty(t, foundParams)
				},
				"PutGetDeleteLifecycleWorks": func(ctx context.Context, t *testing.T, pm *parameterstore.ParameterManager) {
					name := "basename"
					value := "value"
					p, err := pm.Put(ctx, name, value)
					require.NoError(t, err)
					checkParam(ctx, t, p, "/prefix/basename", name, value)

					foundParams, err := pm.Get(ctx, name)
					require.NoError(t, err)
					require.Len(t, foundParams, 1)
					checkParam(ctx, t, &foundParams[0], "/prefix/basename", name, value)

					require.NoError(t, pm.Delete(ctx, name))

					foundParams, err = pm.Get(ctx, name)
					assert.NoError(t, err)
					assert.Empty(t, foundParams)
				},
			} {
				t.Run(tName, func(t *testing.T) {
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()

					require.NoError(t, db.ClearCollections(Collection))

					pm, err := makeParameterManager(ctx)
					require.NoError(t, err)

					tCase(ctx, t, pm)
				})
			}
		})
	}

	t.Run("ConcurrentReadsAndWritesAreSafeAndLastWriteWins", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pm, err := parameterstore.NewParameterManager(ctx, parameterstore.ParameterManagerOptions{
			PathPrefix:     "prefix",
			CachingEnabled: true,
			SSMClient:      NewFakeSSMClient(),
			DB:             evergreen.GetEnvironment().DB(),
		})
		require.NoError(t, err)
		const basename = "basename"
		const fullName = "/prefix/basename"

		var wg sync.WaitGroup
		startOps := make(chan struct{})

		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-startOps
				// Each writer puts a new random string in the DB. One of these
				// will eventually be the last write, and that's the persistent
				// value in a steady state.
				_, err := pm.Put(ctx, basename, utility.RandomString())
				assert.NoError(t, err)
			}()

			wg.Add(1)
			go func() {
				defer wg.Done()
				<-startOps
				for j := 0; j < 10; j++ {
					// Each reader reads the random strings from the DB and they
					// get cached. While this is happening, the value will be
					// fluctuating because there are concurrent writes, so the
					// cached value for the parameter will be fluctuating as
					// well.
					_, err := pm.Get(ctx, basename)
					assert.NoError(t, err)
				}
			}()
		}

		close(startOps)

		wg.Wait()

		dbParam, err := FindOneID(ctx, fullName)
		require.NoError(t, err)
		require.NotZero(t, dbParam)

		params, err := pm.Get(ctx, basename)
		require.NoError(t, err)
		require.Len(t, params, 1)
		assert.Equal(t, params[0].Name, dbParam.Name)
		assert.Equal(t, params[0].Basename, basename)
		// After the readers and writers are all finished and there's no more
		// modifications being made to the parameter, the parameter returned
		// from the ParameterManager must match the final value that was last
		// written to the DB. This is necessary to prove that, even with a
		// cache, it eventually reflects the most up-to-date value from the DB.
		// If this is ever flaky, that means the caching logic is faulty because
		// it returned a stale value.
		assert.Equal(t, params[0].Value, dbParam.Value, "value returned from Get should agree with the latest written value, even after several concurrent reads/writes")
	})
}
