package units

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

// mockReauthUserManager is a UserManager for testing reauthorization which
// always refreshes the user's login session when ReauthorizeUser is called.
type mockReauthUserManager struct {
	failReauth      bool
	attemptedReauth bool
}

func (*mockReauthUserManager) GetUserByToken(context.Context, string) (gimlet.User, error) {
	return nil, errors.New("GetUserByToken not implemented")
}
func (*mockReauthUserManager) CreateUserToken(string, string) (string, error) {
	return "", errors.New("CreateUserToken not implemented")
}
func (*mockReauthUserManager) GetLoginHandler(url string) http.HandlerFunc {
	return nil
}
func (*mockReauthUserManager) GetLoginCallbackHandler() http.HandlerFunc {
	return nil
}
func (*mockReauthUserManager) IsRedirect() bool {
	return false
}
func (um *mockReauthUserManager) ReauthorizeUser(u gimlet.User) error {
	um.attemptedReauth = true
	if um.failReauth {
		return errors.New("fail reauth")
	}
	_, err := user.PutLoginCache(u)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}
func (*mockReauthUserManager) GetUserByID(string) (gimlet.User, error) {
	return nil, errors.New("GetUserByID not implemented")
}
func (*mockReauthUserManager) GetOrCreateUser(gimlet.User) (gimlet.User, error) {
	return nil, errors.New("GetOrCreateUser not implemented")
}
func (*mockReauthUserManager) ClearUser(user gimlet.User, all bool) error {
	return errors.New("not implemented")
}
func (*mockReauthUserManager) GetGroupsForUser(string) ([]string, error) {
	return nil, errors.New("not implemented")
}

func TestReauthorizationJob(t *testing.T) {
	needsReauth := func(env *mock.Environment, u *user.DBUser) (bool, error) {
		dbUser, err := user.FindOneById(u.Username())
		if err != nil {
			return false, errors.WithStack(err)
		}
		if dbUser.LoginCache.Token == "" {
			return false, nil
		}
		return time.Since(dbUser.LoginCache.TTL) > time.Duration(env.Settings().AuthConfig.BackgroundReauthMinutes)*time.Minute, nil
	}
	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment, um *mockReauthUserManager, u *user.DBUser){
		"Succeeds": func(ctx context.Context, t *testing.T, env *mock.Environment, um *mockReauthUserManager, u *user.DBUser) {
			j := NewReauthorizationJob(env, u, "test")
			j.Run(ctx)
			assert.NoError(t, j.Error())
			needsReauth, err := needsReauth(env, u)
			assert.NoError(t, err)
			assert.False(t, needsReauth)
			assert.True(t, um.attemptedReauth)
		},
		"NoopsIfSessionHasNotExceededReauthPeriod": func(ctx context.Context, t *testing.T, env *mock.Environment, um *mockReauthUserManager, u *user.DBUser) {
			_, err := user.PutLoginCache(u)
			require.NoError(t, err)
			u, err = user.FindOneById(u.Id)
			require.NoError(t, err)
			j := NewReauthorizationJob(env, u, "test")
			j.Run(ctx)
			assert.NoError(t, j.Error())
			needsReauth, err := needsReauth(env, u)
			assert.NoError(t, err)
			assert.False(t, needsReauth)
			assert.False(t, um.attemptedReauth)
		},
		"NoopsIfUserLoggedOut": func(ctx context.Context, t *testing.T, env *mock.Environment, um *mockReauthUserManager, u *user.DBUser) {
			require.NoError(t, user.ClearLoginCache(u, false))
			var err error
			u, err = user.FindOneById(u.Id)
			require.NoError(t, err)
			j := NewReauthorizationJob(env, u, "test")
			j.Run(ctx)
			assert.NoError(t, j.Error())
			needsReauth, err := needsReauth(env, u)
			assert.NoError(t, err)
			assert.False(t, needsReauth)
			assert.False(t, um.attemptedReauth)
		},
		"NoopsIfReauthNotSupported": func(ctx context.Context, t *testing.T, env *mock.Environment, um *mockReauthUserManager, u *user.DBUser) {
			env.SetUserManagerInfo(evergreen.UserManagerInfo{})
			j := NewReauthorizationJob(env, u, "test")
			j.Run(ctx)
			assert.NoError(t, j.Error())
			needsReauth, err := needsReauth(env, u)
			assert.NoError(t, err)
			assert.True(t, needsReauth)
			assert.False(t, um.attemptedReauth)
		},
		"NoopsIfDegraded": func(ctx context.Context, t *testing.T, env *mock.Environment, um *mockReauthUserManager, u *user.DBUser) {
			require.NoError(t, evergreen.SetServiceFlags(evergreen.ServiceFlags{BackgroundReauthDisabled: true}))
			defer func() {
				assert.NoError(t, evergreen.SetServiceFlags(evergreen.ServiceFlags{BackgroundReauthDisabled: false}))
			}()
			j := NewReauthorizationJob(env, u, "test")
			j.Run(ctx)
			assert.NoError(t, j.Error())
			needsReauth, err := needsReauth(env, u)
			assert.NoError(t, err)
			assert.True(t, needsReauth)
			assert.False(t, um.attemptedReauth)
		},
		"ErrorsIfReauthFails": func(ctx context.Context, t *testing.T, env *mock.Environment, um *mockReauthUserManager, u *user.DBUser) {
			um.failReauth = true
			j := NewReauthorizationJob(env, u, "test")
			j.Run(ctx)
			assert.Error(t, j.Error())
			needsReauth, err := needsReauth(env, u)
			assert.NoError(t, err)
			assert.True(t, needsReauth)
			assert.True(t, um.attemptedReauth)
			assert.Equal(t, 1, u.LoginCache.ReauthAttempts)
		},
		"DoesNotRetryIfReauthExceedsMaxAttempts": func(ctx context.Context, t *testing.T, env *mock.Environment, um *mockReauthUserManager, u *user.DBUser) {
			um.failReauth = true
			require.NoError(t, user.UpdateOne(
				bson.M{user.IdKey: u.Id},
				bson.M{"$set": bson.M{bsonutil.GetDottedKeyName(user.LoginCacheKey, user.LoginCacheReauthAttemptsKey): maxReauthAttempts - 1}}),
			)
			u.LoginCache.ReauthAttempts = maxReauthAttempts - 1

			j := NewReauthorizationJob(env, u, "test")
			j.Run(ctx)
			assert.Error(t, j.Error())
			stillNeedsReauth, err := needsReauth(env, u)
			assert.NoError(t, err)
			assert.True(t, stillNeedsReauth)
			assert.True(t, um.attemptedReauth)

			dbUser, err := user.FindOneById(u.Id)
			assert.NoError(t, err)
			assert.NotEmpty(t, u.LoginCache.Token)
			assert.Equal(t, u.LoginCache.Token, dbUser.LoginCache.Token)

			// Once we've exceeded the max allowed attempts, this should no-op.
			um.attemptedReauth = false
			j = NewReauthorizationJob(env, dbUser, "test")
			j.Run(ctx)
			assert.NoError(t, j.Error())
			stillNeedsReauth, err = needsReauth(env, u)
			assert.NoError(t, err)
			assert.True(t, stillNeedsReauth)
			assert.False(t, um.attemptedReauth)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.Clear(user.Collection))
			defer func() {
				assert.NoError(t, db.Clear(user.Collection))
			}()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))
			env.EvergreenSettings.AuthConfig.BackgroundReauthMinutes = 1

			u := &user.DBUser{
				LoginCache: user.LoginCache{
					Token: "token",
					TTL:   time.Now().Add(-time.Hour),
				},
			}
			require.NoError(t, u.Insert())

			um := &mockReauthUserManager{}
			env.SetUserManager(um)
			env.SetUserManagerInfo(evergreen.UserManagerInfo{CanReauthorize: true})

			testCase(ctx, t, env, um, u)
		})
	}
}
