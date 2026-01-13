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
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
func (*mockReauthUserManager) CreateUserToken(context.Context, string, string) (string, error) {
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
func (um *mockReauthUserManager) ReauthorizeUser(ctx context.Context, u gimlet.User) error {
	um.attemptedReauth = true
	if um.failReauth {
		return errors.New("fail reauth")
	}
	_, err := user.PutLoginCache(ctx, u)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}
func (*mockReauthUserManager) GetUserByID(context.Context, string) (gimlet.User, error) {
	return nil, errors.New("GetUserByID not implemented")
}
func (*mockReauthUserManager) GetOrCreateUser(context.Context, gimlet.User) (gimlet.User, error) {
	return nil, errors.New("GetOrCreateUser not implemented")
}
func (*mockReauthUserManager) ClearUser(_ context.Context, user gimlet.User, all bool) error {
	return errors.New("not implemented")
}
func (*mockReauthUserManager) GetGroupsForUser(string) ([]string, error) {
	return nil, errors.New("not implemented")
}

func TestReauthorizeUserJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = testutil.TestSpan(ctx, t)

	needsReauth := func(env *mock.Environment, u *user.DBUser) (bool, error) {
		dbUser, err := user.FindOneByIdContext(t.Context(), u.Username())
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
			j := NewReauthorizeUserJob(env, u, "test")
			j.Run(ctx)
			assert.NoError(t, j.Error())
			needsReauth, err := needsReauth(env, u)
			assert.NoError(t, err)
			assert.False(t, needsReauth)
			assert.True(t, um.attemptedReauth)
		},
		"NoopsIfSessionHasNotExceededReauthPeriod": func(ctx context.Context, t *testing.T, env *mock.Environment, um *mockReauthUserManager, u *user.DBUser) {
			_, err := user.PutLoginCache(ctx, u)
			require.NoError(t, err)
			u, err = user.FindOneByIdContext(t.Context(), u.Id)
			require.NoError(t, err)
			j := NewReauthorizeUserJob(env, u, "test")
			j.Run(ctx)
			assert.NoError(t, j.Error())
			needsReauth, err := needsReauth(env, u)
			assert.NoError(t, err)
			assert.False(t, needsReauth)
			assert.False(t, um.attemptedReauth)
		},
		"NoopsIfUserLoggedOut": func(ctx context.Context, t *testing.T, env *mock.Environment, um *mockReauthUserManager, u *user.DBUser) {
			require.NoError(t, user.ClearLoginCache(u))
			var err error
			u, err = user.FindOneByIdContext(t.Context(), u.Id)
			require.NoError(t, err)
			j := NewReauthorizeUserJob(env, u, "test")
			j.Run(ctx)
			assert.NoError(t, j.Error())
			needsReauth, err := needsReauth(env, u)
			assert.NoError(t, err)
			assert.False(t, needsReauth)
			assert.False(t, um.attemptedReauth)
		},
		"NoopsIfReauthNotSupported": func(ctx context.Context, t *testing.T, env *mock.Environment, um *mockReauthUserManager, u *user.DBUser) {
			env.SetUserManagerInfo(evergreen.UserManagerInfo{})
			j := NewReauthorizeUserJob(env, u, "test")
			j.Run(ctx)
			assert.Error(t, j.Error())
			needsReauth, err := needsReauth(env, u)
			assert.NoError(t, err)
			assert.True(t, needsReauth)
			assert.False(t, um.attemptedReauth)
		},
		"NoopsIfDegraded": func(ctx context.Context, t *testing.T, env *mock.Environment, um *mockReauthUserManager, u *user.DBUser) {
			require.NoError(t, evergreen.SetServiceFlags(ctx, evergreen.ServiceFlags{BackgroundReauthDisabled: true}))
			defer func() {
				assert.NoError(t, evergreen.SetServiceFlags(ctx, evergreen.ServiceFlags{BackgroundReauthDisabled: false}))
			}()
			j := NewReauthorizeUserJob(env, u, "test")
			j.Run(ctx)
			assert.NoError(t, j.Error())
			needsReauth, err := needsReauth(env, u)
			assert.NoError(t, err)
			assert.True(t, needsReauth)
			assert.False(t, um.attemptedReauth)
		},
		"ErrorsIfReauthFails": func(ctx context.Context, t *testing.T, env *mock.Environment, um *mockReauthUserManager, u *user.DBUser) {
			um.failReauth = true
			j := NewReauthorizeUserJob(env, u, "test")
			j.Run(ctx)
			assert.Error(t, j.Error())
			needsReauth, err := needsReauth(env, u)
			assert.NoError(t, err)
			assert.True(t, needsReauth)
			assert.True(t, um.attemptedReauth)
			assert.True(t, j.RetryInfo().NeedsRetry)
		},
		"LogsOutUserIfUserHitsMaxReauthAttempts": func(ctx context.Context, t *testing.T, env *mock.Environment, um *mockReauthUserManager, u *user.DBUser) {
			um.failReauth = true
			j := NewReauthorizeUserJob(env, u, "test")
			j.UpdateRetryInfo(amboy.JobRetryOptions{
				CurrentAttempt: utility.ToIntPtr(maxReauthAttempts - 1),
			})
			j.Run(ctx)
			assert.Error(t, j.Error())
			stillNeedsReauth, err := needsReauth(env, u)
			assert.NoError(t, err)
			assert.False(t, stillNeedsReauth)
			assert.True(t, um.attemptedReauth)

			dbUser, err := user.FindOneByIdContext(t.Context(), u.Id)
			assert.NoError(t, err)
			assert.Zero(t, dbUser.LoginCache)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			tctx, cancel := context.WithCancel(ctx)
			defer cancel()
			tctx = testutil.TestSpan(tctx, t)

			require.NoError(t, db.Clear(user.Collection))
			defer func() {
				assert.NoError(t, db.Clear(user.Collection))
			}()

			env := &mock.Environment{}
			require.NoError(t, env.Configure(tctx))
			env.EvergreenSettings.AuthConfig.BackgroundReauthMinutes = 1

			u := &user.DBUser{
				Id: "me",
				LoginCache: user.LoginCache{
					Token: "token",
					TTL:   time.Now().Add(-time.Hour),
				},
			}
			require.NoError(t, u.Insert(ctx))

			um := &mockReauthUserManager{}
			env.SetUserManager(um)
			env.SetUserManagerInfo(evergreen.UserManagerInfo{CanReauthorize: true})

			testCase(tctx, t, env, um, u)
		})
	}
}
