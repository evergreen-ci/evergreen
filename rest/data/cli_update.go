package data

import (
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/model"
)

type CLIUpdateConnector struct{}

func (c *CLIUpdateConnector) GetCLIUpdate() (*model.APICLIUpdate, error) {
	update := &model.APICLIUpdate{}
	config := evergreen.GetEnvironment().ClientConfig()
	if err := update.BuildFromService(*config); err != nil {
		return nil, &rest.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		}
	}

	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		return nil, &rest.APIError{
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
					Arch: "amd64",
					OS:   "darwin",
					URL:  "localhost/clients/darwin_amd64/evergreen",
				},
			},
			LatestRevision: "2017-12-29",
		},
		IgnoreUpdate: c.degradedModeOn,
	}

	return update, nil
}
