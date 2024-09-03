package fakeparameter

import (
	"context"
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud/parameterstore"
	"github.com/evergreen-ci/evergreen/db"
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
}
