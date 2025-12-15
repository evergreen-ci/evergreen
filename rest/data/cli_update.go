package data

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
)

// GetCLIUpdate fetches the current cli version and the urls to download
func GetCLIUpdate(ctx context.Context, env evergreen.Environment) (*model.APICLIUpdate, error) {
	update := &model.APICLIUpdate{}
	config := env.ClientConfig()
	if config == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "no clients configured",
		}
	}

	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		}
	}

	update.BuildFromService(*config)
	update.IgnoreUpdate = flags.CLIUpdatesDisabled

	settings := env.Settings()

	if settings != nil && settings.AuthConfig.OAuth != nil {
		update.ClientConfig.OAuthIssuer = utility.ToStringPtr(settings.AuthConfig.OAuth.Issuer)
		update.ClientConfig.OAuthClientID = utility.ToStringPtr(settings.AuthConfig.OAuth.ClientID)
		update.ClientConfig.OAuthConnectorID = utility.ToStringPtr(settings.AuthConfig.OAuth.ConnectorID)
	}

	return update, nil
}
