package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// GET /rest/v2/task/{task_id}/installation_token/{owner}/{repo}

type createInstallationToken struct {
	owner string
	repo  string

	env evergreen.Environment
}

func makeCreateInstallationToken(env evergreen.Environment) gimlet.RouteHandler {
	return &createInstallationToken{
		env: env,
	}
}

func (g *createInstallationToken) Factory() gimlet.RouteHandler {
	return &createInstallationToken{
		env: g.env,
	}
}

func (g *createInstallationToken) Parse(ctx context.Context, r *http.Request) error {
	if g.owner = gimlet.GetVars(r)["owner"]; g.owner == "" {
		return errors.New("missing owner")
	}

	if g.repo = gimlet.GetVars(r)["repo"]; g.repo == "" {
		return errors.New("missing repo")
	}

	return nil
}

func (g *createInstallationToken) Run(ctx context.Context) gimlet.Responder {
	token, err := g.env.Settings().CreateInstallationToken(ctx, g.owner, g.repo, nil)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "creating installation token for '%s/%s'", g.owner, g.repo))
	}
	if token == "" {
		return gimlet.MakeJSONErrorResponder(errors.Errorf("no installation token returned for '%s/%s'", g.owner, g.repo))
	}

	return gimlet.NewJSONResponse(&apimodels.InstallationToken{
		Token: token,
	})
}
