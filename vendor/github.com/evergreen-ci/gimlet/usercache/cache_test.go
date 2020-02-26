package usercache

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/gimlet/util"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCache(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type implCases struct {
		name string
		test func(*testing.T, Cache)
	}

	opts, err := gimlet.NewBasicUserOptions("foo")
	require.NoError(t, err)

	for _, impl := range []struct {
		name    string
		factory func() (Cache, error)
		cases   []implCases
	}{
		{
			name: "InMemory",
			factory: func() (Cache, error) {
				return NewInMemory(ctx, time.Millisecond), nil
			},
			cases: []implCases{
				{
					name: "CleanMethodPrunes",
					test: func(t *testing.T, cache Cache) {
						c := cache.(*userCache)
						c.cache["foo"] = cacheValue{
							user:    gimlet.NewBasicUser(opts),
							created: time.Now().Add(-time.Hour),
						}
						assert.Len(t, c.cache, 1)
						c.clean()
						assert.Len(t, c.cache, 0)
					},
				},
				{
					name: "CleanMethodIsRunPeriodically",
					test: func(t *testing.T, cache Cache) {
						c := cache.(*userCache)
						c.cache["foo"] = cacheValue{
							user:    gimlet.NewBasicUser(opts),
							created: time.Now().Add(-time.Hour),
						}
						assert.Len(t, c.cache, 1)
						time.Sleep(2 * time.Millisecond)
						assert.Len(t, c.cache, 0)
					},
				},
				{
					name: "CleanLeavesNoTimeout",
					test: func(t *testing.T, cache Cache) {
						c := cache.(*userCache)
						c.cache["foo"] = cacheValue{
							user:    gimlet.NewBasicUser(opts),
							created: time.Now().Add(time.Hour),
						}
						assert.Len(t, c.cache, 1)
						c.clean()
						assert.Len(t, c.cache, 1)
					},
				},
				{
					name: "FindWithBrokenCache",
					test: func(t *testing.T, cache Cache) {
						c := cache.(*userCache)
						c.userToToken["foo"] = "0"
						c.cache["foo"] = cacheValue{
							user:    gimlet.NewBasicUser(opts),
							created: time.Now().Add(time.Hour),
						}
						_, _, err := cache.Find("foo")
						assert.Error(t, err)
					},
				},
				{
					name: "GetRespectsTTL",
					test: func(t *testing.T, cache Cache) {
						c := cache.(*userCache)
						c.cache["foo"] = cacheValue{
							user:    gimlet.NewBasicUser(opts),
							created: time.Now().Add(-time.Hour),
						}
						u, exists, err := cache.Get("foo")
						assert.NoError(t, err)
						assert.False(t, exists)
						assert.NotNil(t, u)
					},
				},
			},
		},
		{
			name: "ExternalMock",
			factory: func() (Cache, error) {
				users := make(map[string]gimlet.User)
				cache := make(map[string]gimlet.User)
				return NewExternal(ExternalOptions{
					PutUserGetToken: func(u gimlet.User) (string, error) {
						token, err := util.RandomString()
						if err != nil {
							return "", errors.WithStack(err)
						}
						cache[token] = u
						return token, nil
					},
					GetUserByToken: func(token string) (gimlet.User, bool, error) {
						u, ok := cache[token]
						return u, ok, nil
					},
					GetUserByID: func(id string) (gimlet.User, bool, error) {
						u, ok := users[id]
						if !ok {
							return nil, false, errors.New("not found")
						}
						return u, true, nil
					},
					GetOrCreateUser: func(u gimlet.User) (gimlet.User, error) {
						users[u.Username()] = u
						return u, nil
					},
					ClearUserToken: func(u gimlet.User, all bool) error {
						if all {
							users = make(map[string]gimlet.User)
							cache = make(map[string]gimlet.User)
							return nil
						}

						if _, ok := users[u.Username()]; !ok {
							return errors.New("not found")
						}
						delete(users, u.Username())
						for token, user := range cache {
							if user.Username() == u.Username() {
								delete(cache, token)
								break
							}
						}
						return nil
					},
				})
			},
		},
		{
			name: "InMemory",
			factory: func() (Cache, error) {
				return NewInMemory(ctx, time.Millisecond), nil
			},
		},
	} {
		t.Run(impl.name, func(t *testing.T) {
			t.Run("Impl", func(t *testing.T) {
				for _, test := range impl.cases {
					t.Run(test.name, func(t *testing.T) {
						cache, err := impl.factory()
						require.NoError(t, err)
						test.test(t, cache)
					})
				}
			})

			t.Run("AddUser", func(t *testing.T) {
				cache, err := impl.factory()
				require.NoError(t, err)
				const id = "username"
				opts, err := gimlet.NewBasicUserOptions(id)
				require.NoError(t, err)
				u := gimlet.NewBasicUser(opts)
				assert.NoError(t, cache.Add(u))
				cu, _, err := cache.Find(id)
				assert.NoError(t, err)
				assert.Equal(t, u, cu)
			})
			t.Run("PutGetRoundTrip", func(t *testing.T) {
				cache, err := impl.factory()
				require.NoError(t, err)
				const id = "username"
				opts, err := gimlet.NewBasicUserOptions(id)
				require.NoError(t, err)
				u := gimlet.NewBasicUser(opts)
				token, err := cache.Put(u)
				assert.NoError(t, err)
				assert.NotZero(t, token)

				cu, exists, err := cache.Get(token)
				assert.NoError(t, err)
				assert.True(t, exists)
				assert.Equal(t, u, cu)
			})
			t.Run("FindErrorsForNotFound", func(t *testing.T) {
				cache, err := impl.factory()
				require.NoError(t, err)
				cu, _, err := cache.Find("foo")
				assert.Error(t, err)
				assert.Nil(t, cu)
			})
			t.Run("GetCacheMiss", func(t *testing.T) {
				cache, err := impl.factory()
				require.NoError(t, err)
				cu, exists, err := cache.Get("nope")
				assert.NoError(t, err)
				assert.False(t, exists)
				assert.Nil(t, cu)
			})
			t.Run("GetOrCreateNewUser", func(t *testing.T) {
				cache, err := impl.factory()
				require.NoError(t, err)
				_, _, err = cache.Find("usr")
				assert.Error(t, err)

				opts, err := gimlet.NewBasicUserOptions("usr")
				require.NoError(t, err)
				u := gimlet.NewBasicUser(opts)

				cu, err := cache.GetOrCreate(u)
				require.NoError(t, err)
				assert.Equal(t, u, cu)

				_, _, err = cache.Find("usr")
				assert.NoError(t, err)
			})
			t.Run("GetOrCreateNewUser", func(t *testing.T) {
				cache, err := impl.factory()
				require.NoError(t, err)
				_, _, err = cache.Find("usr")
				assert.Error(t, err)

				opts, err := gimlet.NewBasicUserOptions("usr")
				require.NoError(t, err)
				u := gimlet.NewBasicUser(opts)

				_, err = cache.Put(u)
				require.NoError(t, err)

				cu, err := cache.GetOrCreate(u)
				require.NoError(t, err)
				assert.Equal(t, u, cu)
			})
			t.Run("ClearUser", func(t *testing.T) {
				cache, err := impl.factory()
				require.NoError(t, err)
				opts, err := gimlet.NewBasicUserOptions("usr")
				require.NoError(t, err)
				u := gimlet.NewBasicUser(opts)
				u, err = cache.GetOrCreate(u)
				require.NoError(t, err)
				require.NotNil(t, u)
				token, err := cache.Put(u)
				require.NoError(t, err)

				// Clear just this user
				err = cache.Clear(u, false)
				assert.NoError(t, err)

				noUser, isValidToken, err := cache.Get(token)
				assert.Nil(t, noUser)
				assert.False(t, isValidToken)
				assert.NoError(t, err)

				token, err = cache.Put(u)
				require.NoError(t, err)

				// Clear all users
				err = cache.Clear(nil, true)
				assert.NoError(t, err)

				u, isValidToken, err = cache.Get(token)
				assert.Nil(t, u)
				assert.False(t, isValidToken)
				assert.NoError(t, err)
			})
		})
	}
}
