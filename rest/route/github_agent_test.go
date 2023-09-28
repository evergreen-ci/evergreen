package route

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateInstallationToken(t *testing.T) {
	validOwner := "owner"
	validRepo := "repo"

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, gh *createInstallationToken, env *mock.Environment){
		"parse should error on empty owner and repo": func(ctx context.Context, t *testing.T, handler *createInstallationToken, env *mock.Environment) {
			url := fmt.Sprintf("/github/installation_token/%s/%s", "", "")
			request, err := http.NewRequest(http.MethodGet, url, bytes.NewReader(nil))
			assert.NoError(t, err)

			options := map[string]string{"owner": "", "repo": ""}
			request = gimlet.SetURLVars(request, options)

			assert.Error(t, handler.Parse(context.Background(), request))
		},
		"parse should error on empty owner": func(ctx context.Context, t *testing.T, handler *createInstallationToken, env *mock.Environment) {
			url := fmt.Sprintf("/github/installation_token/%s/%s", "", validRepo)
			request, err := http.NewRequest(http.MethodGet, url, bytes.NewReader(nil))
			assert.NoError(t, err)

			options := map[string]string{"owner": "", "repo": validRepo}
			request = gimlet.SetURLVars(request, options)

			assert.Error(t, handler.Parse(context.Background(), request))
		},
		"parse should error on empty repo": func(ctx context.Context, t *testing.T, handler *createInstallationToken, env *mock.Environment) {
			url := fmt.Sprintf("/github/installation_token/%s/%s", validOwner, "")
			request, err := http.NewRequest(http.MethodGet, url, bytes.NewReader(nil))
			assert.NoError(t, err)

			options := map[string]string{"owner": validOwner, "repo": ""}
			request = gimlet.SetURLVars(request, options)

			assert.Error(t, handler.Parse(context.Background(), request))
		},
		"parse should not error on valid owner and repo": func(ctx context.Context, t *testing.T, handler *createInstallationToken, env *mock.Environment) {
			url := fmt.Sprintf("/github/installation_token/%s/%s", validOwner, validRepo)
			request, err := http.NewRequest(http.MethodGet, url, bytes.NewReader(nil))
			assert.NoError(t, err)

			options := map[string]string{"owner": validOwner, "repo": validRepo}
			request = gimlet.SetURLVars(request, options)

			assert.NoError(t, handler.Parse(context.Background(), request))
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))

			r, ok := makeCreateInstallationToken(evergreen.GetEnvironment()).(*createInstallationToken)
			r.env = env
			require.True(t, ok)

			tCase(ctx, t, r, env)
		})
	}
}
