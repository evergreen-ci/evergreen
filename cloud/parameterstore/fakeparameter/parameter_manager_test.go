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

// Tests for ParameterManager are in this package to avoid a circular
// dependency between the two packages.
func TestParameterManager(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()

	checkParamInDB := func(ctx context.Context, t *testing.T, p *parameterstore.Parameter) {
		dbParam, err := FindOneID(ctx, p.Name)
		require.NoError(t, err)
		require.NotZero(t, dbParam)
		assert.Equal(t, p.Name, dbParam.ID, "full name should be used as fake parameter ID")
		assert.Equal(t, p.Value, dbParam.Value)
	}
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
		// TODO (DEVPROD-9403): test same conditions work with caching enabled.
	} {
		t.Run(managerTestName, func(t *testing.T) {
			for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, pm *parameterstore.ParameterManager){
				"PutAddsNewParameters": func(ctx context.Context, t *testing.T, pm *parameterstore.ParameterManager) {
					for i := 0; i < 5; i++ {
						name := fmt.Sprintf("name-%d", i)
						value := fmt.Sprintf("value-%d", i)
						p, err := pm.Put(ctx, name, value)
						require.NoError(t, err)
						checkParam(ctx, t, p, fmt.Sprintf("/prefix/%s", name), name, value)
					}
				},
				"PutIncludesPrefixForPartialName": func(ctx context.Context, t *testing.T, pm *parameterstore.ParameterManager) {
					name := fmt.Sprintf("/path/to/name")
					value := "value"
					p, err := pm.Put(ctx, name, value)
					require.NoError(t, err)
					checkParam(ctx, t, p, "/prefix/path/to/name", "name", value)
				},
				"PutOverwritesExistingParameter": func(ctx context.Context, t *testing.T, pm *parameterstore.ParameterManager) {
					name := "name"
					value := "old_value"
					oldParam, err := pm.Put(ctx, name, value)
					require.NoError(t, err)
					checkParam(ctx, t, oldParam, "/prefix/name", name, value)

					newValue := "new_value"
					newParam, err := pm.Put(ctx, name, newValue)
					require.NoError(t, err)
					checkParam(ctx, t, newParam, "/prefix/name", name, newValue)
				},
				// kim: TODO: add remaining tests.
				"DeleteRemovesExistingParameter": func(ctx context.Context, t *testing.T, pm *parameterstore.ParameterManager) {

				},
				"DeleteNoopsForNonexistentParameter": func(ctx context.Context, t *testing.T, pm *parameterstore.ParameterManager) {

				},
				"GetReturnsAllExistingParameters": func(ctx context.Context, t *testing.T, pm *parameterstore.ParameterManager) {

				},
				"GetReturnsNoResultForNoParameters": func(ctx context.Context, t *testing.T, pm *parameterstore.ParameterManager) {

				},
				"GetReturnsExistingSubsetForMixOfExistingAndNonexistentParameters": func(ctx context.Context, t *testing.T, pm *parameterstore.ParameterManager) {

				},
				"GetReturnsNoResultForNonexistentParameter": func(ctx context.Context, t *testing.T, pm *parameterstore.ParameterManager) {

				},
				"GetStrictReturnsAllExistingParameters": func(ctx context.Context, t *testing.T, pm *parameterstore.ParameterManager) {

				},
				"GetStrictErrorsForMixOfExistingAndNonexistentParameters": func(ctx context.Context, t *testing.T, pm *parameterstore.ParameterManager) {

				},
				"GetStrictErrorsForNonexistentParameter": func(ctx context.Context, t *testing.T, pm *parameterstore.ParameterManager) {

				},
				"PutGetDeleteLifecycleWorks": func(ctx context.Context, t *testing.T, pm *parameterstore.ParameterManager) {
					name := "name"
					value := "value"
					p, err := pm.Put(ctx, name, value)
					require.NoError(t, err)
					assert.Equal(t, "/prefix/name", p.Name, "full name should include prefix")
					assert.Equal(t, name, p.Basename)
					assert.Equal(t, value, p.Value)
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
