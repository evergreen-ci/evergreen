package data

import (
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
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

	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		}
	}
	if flags.S3BinaryDownloadsDisabled {
		config.S3ClientBinaries = nil
	}

	if err := update.BuildFromService(*config); err != nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		}
	}

	update.IgnoreUpdate = flags.CLIUpdatesDisabled

	return update, nil
}

type MockCLIUpdateConnector struct {
	degradedModeOn bool
}

func (c *MockCLIUpdateConnector) GetCLIUpdate() (*model.APICLIUpdate, error) {
	update := &model.APICLIUpdate{
		ClientConfig: model.APIClientConfig{
			ClientBinaries: []model.APIClientBinary{
				{
					Arch: utility.ToStringPtr("amd64"),
					OS:   utility.ToStringPtr("darwin"),
					URL:  utility.ToStringPtr("localhost/clients/darwin_amd64/evergreen"),
				},
			},
			LatestRevision: utility.ToStringPtr("2017-12-29"),
		},
		IgnoreUpdate: c.degradedModeOn,
	}

	return update, nil
}
