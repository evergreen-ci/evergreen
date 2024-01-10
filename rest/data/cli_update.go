package data

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
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
	if flags.S3BinaryDownloadsDisabled {
		config.S3ClientBinaries = nil
	}

	update.BuildFromService(*config)
	update.IgnoreUpdate = flags.CLIUpdatesDisabled

	return update, nil
}
