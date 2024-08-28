package fakeparameter

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/cloud/parameterstore"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests for the parameterstore.ParameterRecord functionality are in this
// package to avoid a circular dependency between the top-level evergreen
// package and the parameterstore package.

func TestBumpParameterRecord(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(parameterstore.Collection))
	}()

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment){
		"CreatesNewParameterRecord": func(ctx context.Context, t *testing.T, env *mock.Environment) {
			const name = "name"
			lastUpdated := utility.BSONTime(time.Now())
			require.NoError(t, parameterstore.BumpParameterRecord(ctx, env.DB(), name, lastUpdated))

			rec, err := parameterstore.FindOneID(ctx, env.DB(), name)
			require.NoError(t, err)
			assert.Equal(t, name, rec.Name)
			assert.Equal(t, lastUpdated, rec.LastUpdated)
		},
		"UpdatesExistingParameterRecord": func(ctx context.Context, t *testing.T, env *mock.Environment) {
			const name = "name"
			lastUpdated := utility.BSONTime(time.Now())
			originalRec := parameterstore.ParameterRecord{
				Name:        name,
				LastUpdated: lastUpdated.Add(-time.Hour),
			}
			require.NoError(t, originalRec.Insert(ctx, env.DB()))

			require.NoError(t, parameterstore.BumpParameterRecord(ctx, env.DB(), name, lastUpdated))

			rec, err := parameterstore.FindOneID(ctx, env.DB(), name)
			require.NoError(t, err)
			assert.Equal(t, name, rec.Name)
			assert.Equal(t, lastUpdated, rec.LastUpdated, "last updated time should have been updated")
		},
		"DoesNotBumpNewerParameterRecord": func(ctx context.Context, t *testing.T, env *mock.Environment) {
			const name = "name"
			lastUpdated := utility.BSONTime(time.Now())
			originalRec := parameterstore.ParameterRecord{
				Name:        name,
				LastUpdated: lastUpdated,
			}
			require.NoError(t, originalRec.Insert(ctx, env.DB()))

			assert.Error(t, parameterstore.BumpParameterRecord(ctx, env.DB(), name, lastUpdated.Add(-time.Hour)))

			rec, err := parameterstore.FindOneID(ctx, env.DB(), name)
			require.NoError(t, err)
			assert.Equal(t, name, rec.Name)
			assert.Equal(t, lastUpdated, rec.LastUpdated, "last updated time should not have changed")
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			require.NoError(t, db.ClearCollections(parameterstore.Collection))

			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))

			tCase(ctx, t, env)
		})
	}
}

func TestParameterRecordFindOneID(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(parameterstore.Collection))
	}()

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment){
		"ReturnsNilWhenNotFound": func(ctx context.Context, t *testing.T, env *mock.Environment) {
			rec, err := parameterstore.FindOneID(ctx, env.DB(), "nonexistent")
			assert.NoError(t, err)
			assert.Zero(t, rec)
		},
		"ReturnsFoundRecord": func(ctx context.Context, t *testing.T, env *mock.Environment) {
			const name = "name"
			expectedRec := parameterstore.ParameterRecord{
				Name:        name,
				LastUpdated: utility.BSONTime(time.Now()),
			}
			require.NoError(t, expectedRec.Insert(ctx, env.DB()))

			rec, err := parameterstore.FindOneID(ctx, env.DB(), name)
			assert.NoError(t, err)
			require.NotZero(t, rec)
			assert.Equal(t, expectedRec, *rec)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))

			tCase(ctx, t, env)
		})
	}
}

func TestParameterRecordFindByIDs(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(parameterstore.Collection))
	}()

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment){
		"ReturnsNoResultsWhenNotFound": func(ctx context.Context, t *testing.T, env *mock.Environment) {
			foundParams, err := parameterstore.FindByIDs(ctx, env.DB(), "nonexistent0", "nonexistent1")
			assert.NoError(t, err)
			assert.Empty(t, foundParams)
		},
		"ReturnsPartiallyFoundRecords": func(ctx context.Context, t *testing.T, env *mock.Environment) {
			const name = "name"
			expectedRec := parameterstore.ParameterRecord{
				Name:        name,
				LastUpdated: utility.BSONTime(time.Now()),
			}
			require.NoError(t, expectedRec.Insert(ctx, env.DB()))

			recs, err := parameterstore.FindByIDs(ctx, env.DB(), name, "nonexistent")
			assert.NoError(t, err)
			require.Len(t, recs, 1)
			assert.Equal(t, expectedRec, recs[0])
		},
		"ReturnsNoResultsForNoNames": func(ctx context.Context, t *testing.T, env *mock.Environment) {
			recs, err := parameterstore.FindByIDs(ctx, env.DB())
			assert.NoError(t, err)
			assert.Empty(t, recs)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))

			tCase(ctx, t, env)
		})
	}
}
