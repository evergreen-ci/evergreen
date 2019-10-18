package service

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/gimlet"
)

func (as *APIServer) Buildlogger(w http.ResponseWriter, r *http.Request) {
	gimlet.WriteJSON(w, &apimodels.BuildloggerInfo{
		BaseURL:  as.Settings.LoggerConfig.BuildloggerBaseURL,
		RPCPort:  as.Settings.LoggerConfig.BuildloggerRPCPort,
		Username: as.Settings.LoggerConfig.BuildloggerUser,
		Password: as.Settings.LoggerConfig.BuildloggerPassword,
	})
}
