package data

import (
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
)

type CLIUpdateConnector struct{}

func (c *CLIUpdateConnector) GetCLIUpdate() (*model.APICLIUpdate, error) {
	update := &model.APICLIUpdate{}
	env := evergreen.GetEnvironment()
	config := env.ClientConfig()
	if config == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "no clients configured",
		}
	}

	if err := update.BuildFromService(*config); err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		}
	}

	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		}
	}
	if flags != nil {
		update.IgnoreUpdate = flags.CLIUpdatesDisabled
	}
	return update, nil
}

type MockCLIUpdateConnector struct {
	degradedModeOn bool
}

func (c *MockCLIUpdateConnector) GetCLIUpdate() (*model.APICLIUpdate, error) {
	update := &model.APICLIUpdate{
		ClientConfig: model.APIClientConfig{
			ClientBinaries: []model.APIClientBinary{
				model.APIClientBinary{
					Arch: model.ToStringPtr("amd64"),
					OS:   model.ToStringPtr("darwin"),
					URL:  model.ToStringPtr("localhost/clients/darwin_amd64/evergreen"),
				},
			},
			LatestRevision: model.ToStringPtr("2017-12-29"),
		},
		IgnoreUpdate: c.degradedModeOn,
	}

	return update, nil
}
