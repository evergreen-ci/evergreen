package fakeparameter

import (
	"context"
	"fmt"
<<<<<<< HEAD
=======
	"sync"
>>>>>>> d3028f45e (DEVPROD-9403: create parameter manager and cache)
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud/parameterstore"
	"github.com/evergreen-ci/evergreen/db"
<<<<<<< HEAD
=======
	"github.com/evergreen-ci/utility"
>>>>>>> d3028f45e (DEVPROD-9403: create parameter manager and cache)
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
<<<<<<< HEAD
		assert.Equal(t, p.Name, dbParam.ID)
=======
		assert.Equal(t, p.Name, dbParam.Name)
>>>>>>> d3028f45e (DEVPROD-9403: create parameter manager and cache)
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

	for managerTestName, makeParameterManager := range map[string]func() *parameterstore.ParameterManager{
		"CachingDisabled": func() *parameterstore.ParameterManager {
			return parameterstore.NewParameterManager("prefix", false, NewFakeSSMClient(), evergreen.GetEnvironment().DB())
		},
		// TODO (DEVPROD-9403): test same conditions pass with caching enabled.
	} {
		t.Run(managerTestName, func(t *testing.T) {
			for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, pm *parameterstore.ParameterManager){
				"PutAddsNewParameters": func(ctx context.Context, t *testing.T, pm *parameterstore.ParameterManager) {
					for i := 0; i < 5; i++ {
						basename := fmt.Sprintf("name-%d", i)
						value := fmt.Sprintf("value-%d", i)
						p, err := pm.Put(ctx, basename, value)
						require.NoError(t, err)
						checkParam(ctx, t, p, fmt.Sprintf("/prefix/%s", basename), basename, value)
					}
				},
				"PutIncludesPrefixForPartialName": func(ctx context.Context, t *testing.T, pm *parameterstore.ParameterManager) {
					const partialName = "/path/to/basename"
					const value = "value"
					p, err := pm.Put(ctx, partialName, value)
					require.NoError(t, err)
					checkParam(ctx, t, p, "/prefix/path/to/basename", "basename", value)
				},
				"PutOverwritesExistingParameter": func(ctx context.Context, t *testing.T, pm *parameterstore.ParameterManager) {
					const basename = "basename"
					const value = "old_value"
					oldParam, err := pm.Put(ctx, basename, value)
					require.NoError(t, err)
					checkParam(ctx, t, oldParam, "/prefix/basename", basename, value)

					const newValue = "new_value"
					newParam, err := pm.Put(ctx, basename, newValue)
					require.NoError(t, err)
					checkParam(ctx, t, newParam, "/prefix/basename", basename, newValue)
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
				},
				"DeleteNoopsForNonexistentParameter": func(ctx context.Context, t *testing.T, pm *parameterstore.ParameterManager) {
					assert.NoError(t, pm.Delete(ctx, "nonexistent"))
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
					const basename = "basename"
					const value = "value"
					p, err := pm.Put(ctx, basename, value)
					require.NoError(t, err)
					checkParam(ctx, t, p, "/prefix/basename", basename, value)

					foundParams, err := pm.Get(ctx, "/prefix/basename", "nonexistent", "/prefix/also-nonexistent")
					require.NoError(t, err)
					require.Len(t, foundParams, 1)
					checkParam(ctx, t, &foundParams[0], "/prefix/basename", basename, value)
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

					pm := makeParameterManager()

					tCase(ctx, t, pm)
				})
			}
		})
	}
<<<<<<< HEAD
=======

	t.Run("ConcurrentReadsAndWritesAreSafeAndReachEventualConsistency", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pm := parameterstore.NewParameterManager("prefix", true, NewFakeSSMClient(), evergreen.GetEnvironment().DB())
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
		assert.Equal(t, params[0].Value, dbParam.Value, "value returned from Get should agree with persisted value even after several concurrent reads/writes")
	})
>>>>>>> d3028f45e (DEVPROD-9403: create parameter manager and cache)
}
