package route

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateInstallationToken(t *testing.T) {
	validOwner := "owner"
	validRepo := "repo"

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, gh *createInstallationToken, env *mock.Environment){
		"ParseErrorsOnEmptyOwnerAndRepo": func(ctx context.Context, t *testing.T, handler *createInstallationToken, env *mock.Environment) {
			url := fmt.Sprintf("/task/{task_id}/installation_token/%s/%s", "", "")
			request, err := http.NewRequest(http.MethodGet, url, bytes.NewReader(nil))
			assert.NoError(t, err)

			options := map[string]string{"owner": "", "repo": ""}
			request = gimlet.SetURLVars(request, options)

			assert.Error(t, handler.Parse(ctx, request))
		},
		"ParseErrorsOnEmptyOwner": func(ctx context.Context, t *testing.T, handler *createInstallationToken, env *mock.Environment) {
			url := fmt.Sprintf("/task/{task_id}/installation_token/%s/%s", "", validRepo)
			request, err := http.NewRequest(http.MethodGet, url, bytes.NewReader(nil))
			assert.NoError(t, err)

			options := map[string]string{"owner": "", "repo": validRepo}
			request = gimlet.SetURLVars(request, options)

			assert.Error(t, handler.Parse(ctx, request))
		},
		"ParseErrorsOnEmptyRepo": func(ctx context.Context, t *testing.T, handler *createInstallationToken, env *mock.Environment) {
			url := fmt.Sprintf("/task/{task_id}/installation_token/%s/%s", validOwner, "")
			request, err := http.NewRequest(http.MethodGet, url, bytes.NewReader(nil))
			assert.NoError(t, err)

			options := map[string]string{"owner": validOwner, "repo": ""}
			request = gimlet.SetURLVars(request, options)

			assert.Error(t, handler.Parse(ctx, request))
		},
		"ParseSucceeds": func(ctx context.Context, t *testing.T, handler *createInstallationToken, env *mock.Environment) {
			url := fmt.Sprintf("/task/{task_id}/installation_token/%s/%s", validOwner, validRepo)
			request, err := http.NewRequest(http.MethodGet, url, bytes.NewReader(nil))
			assert.NoError(t, err)

			options := map[string]string{"owner": validOwner, "repo": validRepo}
			request = gimlet.SetURLVars(request, options)

			assert.NoError(t, handler.Parse(ctx, request))
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))

			r, ok := makeCreateInstallationToken(env).(*createInstallationToken)
			require.True(t, ok)

			tCase(ctx, t, r, env)
		})
	}
}
