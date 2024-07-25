package parameterstore

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetParameter(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db := initDB(ctx, t)
	require.NoError(t, db.Drop(ctx))

	t.Run("LocalBackend", func(t *testing.T) {
		t.Run("NewParameter", func(t *testing.T) {
			defer func() { require.NoError(t, db.Drop(ctx)) }()

			opts := ParameterStoreOptions{
				Database:   db,
				Prefix:     "/config",
				SSMBackend: false,
			}
			p := parameterStore{opts: opts}
			name := "name"
			val := "val"
			assert.NoError(t, p.SetParameter(ctx, name, val))

			dbParams, err := p.find(ctx, []string{p.prefixedName(name)})
			assert.NoError(t, err)
			require.Len(t, dbParams, 1)
			assert.Equal(t, val, dbParams[0].Value)
		})

		t.Run("ExistingParameter", func(t *testing.T) {
			defer func() { require.NoError(t, db.Drop(ctx)) }()

			opts := ParameterStoreOptions{
				Database:   db,
				Prefix:     "/config",
				SSMBackend: false,
			}
			p := parameterStore{opts: opts}
			name := "name"
			val := "val"
			_, err := db.Collection(collection).InsertOne(ctx, parameter{ID: name, Value: val})
			require.NoError(t, err)

			newVal := "newVal"
			assert.NoError(t, p.SetParameter(ctx, name, newVal))

			dbParams, err := p.find(ctx, []string{p.prefixedName(name)})
			assert.NoError(t, err)
			require.Len(t, dbParams, 1)
			assert.Equal(t, newVal, dbParams[0].Value)
		})
	})

	t.Run("SSMBackend", func(t *testing.T) {
		t.Run("NewParameter", func(t *testing.T) {
			defer func() { require.NoError(t, db.Drop(ctx)) }()
			parameterCache = make(map[string]cachedParameter)

			opts := ParameterStoreOptions{
				Database:   db,
				Prefix:     "/config",
				SSMBackend: true,
			}
			p := parameterStore{opts: opts, ssm: &ssmClientMock{}}
			name := "name"
			val := "val"
			now := time.Now()
			assert.NoError(t, p.SetParameter(ctx, name, val))

			dbParams, err := p.find(ctx, []string{p.prefixedName(name)})
			assert.NoError(t, err)
			require.Len(t, dbParams, 1)
			assert.Empty(t, dbParams[0].Value)

			assert.Len(t, parameterCache, 1)
			param, ok := parameterCache[p.prefixedName(name)]
			assert.True(t, ok)
			assert.Equal(t, val, param.value)
			assert.False(t, dbParams[0].LastUpdate.Before(now.Truncate(time.Millisecond)))
			assert.False(t, param.lastRefresh.Before(now))
			assert.True(t, dbParams[0].LastUpdate.Equal(param.lastRefresh.Truncate(time.Millisecond)))
		})

		t.Run("ExistingParameterUncached", func(t *testing.T) {
			defer func() { require.NoError(t, db.Drop(ctx)) }()
			parameterCache = make(map[string]cachedParameter)

			name := "name"
			val := "val"
			lastUpdate := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
			_, err := db.Collection(collection).InsertOne(ctx, parameter{ID: name, LastUpdate: lastUpdate})
			require.NoError(t, err)

			opts := ParameterStoreOptions{
				Database:   db,
				Prefix:     "/config",
				SSMBackend: true,
			}
			p := parameterStore{opts: opts, ssm: &ssmClientMock{
				parameters: map[string]parameter{
					name: {ID: name, Value: val, LastUpdate: lastUpdate},
				},
			}}

			newVal := "newVal"
			now := time.Now()
			assert.NoError(t, p.SetParameter(ctx, name, newVal))

			dbParams, err := p.find(ctx, []string{p.prefixedName(name)})
			assert.NoError(t, err)
			require.Len(t, dbParams, 1)
			assert.Empty(t, dbParams[0].Value)

			assert.Len(t, parameterCache, 1)
			param, ok := parameterCache[p.prefixedName(name)]
			assert.True(t, ok)
			assert.Equal(t, newVal, param.value)
			assert.False(t, dbParams[0].LastUpdate.Before(now.Truncate(time.Millisecond)))
			assert.False(t, param.lastRefresh.Before(now))
			assert.True(t, dbParams[0].LastUpdate.Equal(param.lastRefresh.Truncate(time.Millisecond)))
		})

		t.Run("ExistingParameterCached", func(t *testing.T) {
			defer func() { require.NoError(t, db.Drop(ctx)) }()

			name := "name"
			val := "val"
			lastUpdate := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)

			opts := ParameterStoreOptions{
				Database:   db,
				Prefix:     "/config",
				SSMBackend: true,
			}
			p := parameterStore{opts: opts, ssm: &ssmClientMock{
				parameters: map[string]parameter{
					name: {ID: name, Value: val, LastUpdate: lastUpdate},
				},
			}}

			parameterCache = map[string]cachedParameter{
				p.prefixedName(name): {value: val, lastRefresh: lastUpdate},
			}

			_, err := db.Collection(collection).InsertOne(ctx, parameter{ID: name, LastUpdate: lastUpdate})
			require.NoError(t, err)

			newVal := "newVal"
			now := time.Now()
			assert.NoError(t, p.SetParameter(ctx, name, newVal))

			dbParams, err := p.find(ctx, []string{p.prefixedName(name)})
			assert.NoError(t, err)
			require.Len(t, dbParams, 1)
			assert.Empty(t, dbParams[0].Value)

			assert.Len(t, parameterCache, 1)
			param, ok := parameterCache[p.prefixedName(name)]
			assert.True(t, ok)
			assert.Equal(t, newVal, param.value)
			assert.False(t, dbParams[0].LastUpdate.Before(now.Truncate(time.Millisecond)))
			assert.False(t, param.lastRefresh.Before(now))
			assert.True(t, dbParams[0].LastUpdate.Equal(param.lastRefresh.Truncate(time.Millisecond)))
		})

		t.Run("SSMError", func(t *testing.T) {
			defer func() { require.NoError(t, db.Drop(ctx)) }()
			parameterCache = make(map[string]cachedParameter)

			opts := ParameterStoreOptions{
				Database:   db,
				Prefix:     "/config",
				SSMBackend: true,
			}
			p := parameterStore{opts: opts, ssm: &ssmClientMock{putError: true}}
			name := "name"
			val := "val"
			assert.Error(t, p.SetParameter(ctx, name, val))

			dbParams, err := p.find(ctx, []string{p.prefixedName(name)})
			assert.NoError(t, err)
			assert.Len(t, dbParams, 0)
			assert.Len(t, parameterCache, 0)
		})
	})
}

func TestGetParameters(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db := initDB(ctx, t)
	require.NoError(t, db.Drop(ctx))

	t.Run("LocalBackend", func(t *testing.T) {
		t.Run("ValueSet", func(t *testing.T) {
			defer func() { require.NoError(t, db.Drop(ctx)) }()
			parameterCache = make(map[string]cachedParameter)

			opts := ParameterStoreOptions{
				Database:   db,
				Prefix:     "/config",
				SSMBackend: false,
			}
			p := parameterStore{opts: opts}
			param := parameter{ID: "/config/a", Value: "value"}
			_, err := db.Collection(collection).InsertOne(ctx, param)
			require.NoError(t, err)

			paramMap, err := p.GetParameters(ctx, []string{p.basename(param.ID)})
			assert.NoError(t, err)
			assert.Len(t, paramMap, 1)
			paramVal, ok := paramMap[p.basename(param.ID)]
			assert.True(t, ok)
			assert.Equal(t, param.Value, paramVal)
		})
		t.Run("ValueUnset", func(t *testing.T) {
			defer func() { require.NoError(t, db.Drop(ctx)) }()
			parameterCache = make(map[string]cachedParameter)

			opts := ParameterStoreOptions{
				Database:   db,
				Prefix:     "/config",
				SSMBackend: false,
			}
			p := parameterStore{opts: opts}
			param := parameter{ID: "/config/a"}
			_, err := db.Collection(collection).InsertOne(ctx, param)
			require.NoError(t, err)

			paramMap, err := p.GetParameters(ctx, []string{p.basename(param.ID)})
			assert.NoError(t, err)
			assert.Len(t, paramMap, 0)
		})

		t.Run("NonexistentParameter", func(t *testing.T) {
			parameterCache = make(map[string]cachedParameter)

			opts := ParameterStoreOptions{
				Database:   db,
				Prefix:     "/config",
				SSMBackend: false,
			}
			p := parameterStore{opts: opts}
			paramMap, err := p.GetParameters(ctx, []string{"not_there"})
			assert.NoError(t, err)
			assert.Len(t, paramMap, 0)
		})
	})

	t.Run("SSMBackend", func(t *testing.T) {
		t.Run("ValueSetInDatabase", func(t *testing.T) {
			defer func() { require.NoError(t, db.Drop(ctx)) }()
			parameterCache = make(map[string]cachedParameter)

			opts := ParameterStoreOptions{
				Database:   db,
				Prefix:     "/config",
				SSMBackend: true,
			}
			p := parameterStore{opts: opts}
			param := parameter{ID: "/config/a", Value: "value"}
			_, err := db.Collection(collection).InsertOne(ctx, param)
			require.NoError(t, err)

			res, err := p.GetParameters(ctx, []string{p.basename(param.ID)})
			assert.NoError(t, err)
			require.Len(t, res, 1)

			fetchedParam, ok := res[p.basename(param.ID)]
			assert.True(t, ok)
			assert.Equal(t, param.Value, fetchedParam)
		})

		t.Run("ValueInCache", func(t *testing.T) {
			defer func() { require.NoError(t, db.Drop(ctx)) }()

			opts := ParameterStoreOptions{
				Database:   db,
				Prefix:     "/config",
				SSMBackend: true,
			}
			p := parameterStore{opts: opts}

			val := "val"
			lastUpdate := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
			param := parameter{ID: "/config/a", LastUpdate: lastUpdate}
			parameterCache = map[string]cachedParameter{
				param.ID: {value: val, lastRefresh: lastUpdate.Add(time.Second)},
			}
			_, err := db.Collection(collection).InsertOne(ctx, param)
			require.NoError(t, err)

			res, err := p.GetParameters(ctx, []string{p.basename(param.ID)})
			assert.NoError(t, err)
			require.Len(t, res, 1)

			fetchedParam, ok := res[p.basename(param.ID)]
			assert.True(t, ok)
			assert.Equal(t, val, fetchedParam)
		})

		t.Run("ValueFromSSM", func(t *testing.T) {
			defer func() { require.NoError(t, db.Drop(ctx)) }()
			parameterCache = make(map[string]cachedParameter)

			lastUpdate := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
			param := parameter{ID: "/config/a", LastUpdate: lastUpdate}
			_, err := db.Collection(collection).InsertOne(ctx, param)
			require.NoError(t, err)

			val := "val"
			param.Value = val
			opts := ParameterStoreOptions{
				Database:   db,
				Prefix:     "/config",
				SSMBackend: true,
			}
			p := parameterStore{opts: opts, ssm: &ssmClientMock{parameters: map[string]parameter{param.ID: param}}}

			res, err := p.GetParameters(ctx, []string{p.basename(param.ID)})
			assert.NoError(t, err)
			require.Len(t, res, 1)

			fetchedParam, ok := res[p.basename(param.ID)]
			assert.True(t, ok)
			assert.Equal(t, val, fetchedParam)
		})

		t.Run("ParameterMissingInDatabase", func(t *testing.T) {
			defer func() { require.NoError(t, db.Drop(ctx)) }()
			parameterCache = make(map[string]cachedParameter)

			lastUpdate := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
			param := parameter{ID: "/config/a", Value: "val", LastUpdate: lastUpdate}

			opts := ParameterStoreOptions{
				Database:   db,
				Prefix:     "/config",
				SSMBackend: true,
			}
			p := parameterStore{opts: opts, ssm: &ssmClientMock{parameters: map[string]parameter{param.ID: param}}}

			res, err := p.GetParameters(ctx, []string{p.basename(param.ID)})
			assert.NoError(t, err)
			require.Len(t, res, 1)

			fetchedParam, ok := res[p.basename(param.ID)]
			assert.True(t, ok)
			assert.Equal(t, param.Value, fetchedParam)

			dbParams, err := p.find(ctx, []string{param.ID})
			require.NoError(t, err)
			require.Len(t, dbParams, 1)
			assert.Equal(t, param.ID, dbParams[0].ID)
			assert.True(t, param.LastUpdate.Truncate(time.Millisecond).Equal(dbParams[0].LastUpdate))
		})

		t.Run("SSMError", func(t *testing.T) {
			defer func() { require.NoError(t, db.Drop(ctx)) }()
			parameterCache = make(map[string]cachedParameter)

			lastUpdate := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
			param := parameter{ID: "/config/a", LastUpdate: lastUpdate}
			_, err := db.Collection(collection).InsertOne(ctx, param)
			require.NoError(t, err)

			val := "val"
			param.Value = val
			opts := ParameterStoreOptions{
				Database:   db,
				Prefix:     "/config",
				SSMBackend: true,
			}
			p := parameterStore{opts: opts, ssm: &ssmClientMock{getError: true}}

			_, err = p.GetParameters(ctx, []string{p.basename(param.ID)})
			assert.Error(t, err)
		})
	})
}
